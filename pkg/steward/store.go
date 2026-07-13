// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package steward

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// clockSkew is how far a timestamp may run ahead of our clock before we stop
// believing it. Two minutes: enough for NTP drift between processes on one host,
// far too little to hide a heartbeat forged into the future.
const clockSkew = 2 * time.Minute

// MaxTranscriptBytes bounds a transcript artifact. A transcript is a courtesy
// artifact that nothing derives from (see Store.Transcript), so it gets a hard
// ceiling: an unbounded read from an agent-supplied stream is a way to fill the
// human's disk with something no projection will ever look at.
const MaxTranscriptBytes int64 = 8 << 20

// MaxReceiptBytes bounds an external authorization receipt artifact.
const MaxReceiptBytes int64 = 1 << 20

// Store is the on-disk steward state: one journal (the authority), one seat file
// (liveness only), and directories of derived or optional artifacts.
type Store struct {
	dir string
	// scope is the machine/account this store speaks for. It is stamped into grants so
	// a capability minted for one seat cannot be replayed against another, and it is
	// BOUND into the store (scope.json) so a store that travels to another machine is
	// refused rather than adopted. See scope.go.
	scope Scope
	// scopeProvider resolves that identity. Injectable so the isolation can be tested.
	scopeProvider ScopeProvider
	// verifier is the injected root of trust. Nil means every authority transition
	// fails closed — see verifier.go, which is the argument for why that is right.
	verifier Verifier
	// maxTranscript bounds transcript artifacts; overridable for tests.
	maxTranscript int64
}

// Option configures a Store.
type Option func(*Store)

// WithMaxTranscriptBytes overrides the transcript ceiling.
func WithMaxTranscriptBytes(n int64) Option {
	return func(s *Store) {
		if n > 0 {
			s.maxTranscript = n
		}
	}
}

// WithVerifier injects the root of trust for authority transitions (claim, takeover).
//
// This is the integration hook. A host with a channel the agent cannot write into —
// bashy meet, a desktop confirmation, an approval service, a signature it can check —
// implements Verifier and passes it here. WITHOUT IT, NO AUTHORITY TRANSITION IS
// POSSIBLE: the store fails closed, because a capability that rests only on store state
// is one the agent can mint by writing a file. See verifier.go.
func WithVerifier(v Verifier) Option {
	return func(s *Store) { s.verifier = v }
}

// WithScopeProvider overrides how the seat's identity is resolved.
//
// The default reads the OS: a stable machine id and the process's real OS account. This
// hook exists so those isolation properties can be TESTED — two machines sharing a
// hostname, one machine under two accounts — none of which is reachable by setting an
// environment variable any more, which was the entire point of removing them.
func WithScopeProvider(p ScopeProvider) Option {
	return func(s *Store) {
		if p != nil {
			s.scopeProvider = p
		}
	}
}

// WithScope pins the seat identity to a fixed id. Tests use it to prove that a grant
// minted for one seat is refused by another.
func WithScope(id string) Option {
	return WithScopeProvider(StaticScope(Scope{
		ID: id, Machine: "static:" + id, Account: "static:" + id, Host: id, Source: "static",
	}))
}

// Open prepares the store directory. The journal records what an agent did across
// every project on the host, so it is owner-only (0700/0600) like the audit log.
//
// It RESOLVES and BINDS the seat's identity (see scope.go): a store that was born on
// another machine or under another OS account is refused here, before anything can read
// or write it.
func Open(dir string, opts ...Option) (*Store, error) {
	s := &Store{maxTranscript: MaxTranscriptBytes, scopeProvider: OSScope{}}
	for _, o := range opts {
		o(s)
	}

	sc, err := s.scopeProvider.Scope()
	if err != nil {
		return nil, err
	}
	s.scope = sc

	if dir == "" {
		if v := os.Getenv("BASHY_STEWARD_DIR"); v != "" {
			dir = v
		} else if dir, err = defaultDirFor(sc); err != nil {
			return nil, err
		}
	}
	s.dir = dir

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("steward: store dir: %w", err)
	}
	if err := s.bindScope(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Dir() string       { return s.dir }
func (s *Store) Scope() string     { return s.scope.ID }
func (s *Store) ScopeInfo() Scope  { return s.scope }
func (s *Store) HasVerifier() bool { return s.verifier != nil }

// ─── committed-but-incomplete ─────────────────────────────────────────────────

// ErrCommitted reports an operation whose JOURNAL APPEND SUCCEEDED and whose follow-up
// housekeeping did not.
//
// This exists because the alternative is silent corruption of the caller's beliefs. The
// journal is the authority; everything else this package writes — the seat.json liveness
// cache, the spent-grant marker, the removal of a released seat file — is derived. So
// when the append lands and the cache write then fails, returning a bare error tells the
// caller "your claim failed", and a caller that believes that RETRIES. The retry replays
// against a journal that already contains the claim: at best it is refused confusingly,
// at worst it appends a second seat event and mints a second epoch, fencing the very
// tenure the first call successfully acquired.
//
// So the operation reports what actually happened: IT COMMITTED. The seq and epoch are
// carried so the caller can proceed as the holder it now is, and the message says which
// derived artifact is stale and how to rebuild it — which, for the seat cache, is a
// plain `steward heartbeat --epoch N`, an idempotent operation that reconstructs it from
// the journal. That is the whole recovery.
//
// Callers that only want to know "did this work" can keep using errors.Is/As; callers
// that must not retry check for this type. The journal is fine either way — which is the
// point of putting the authority in exactly one place.
type ErrCommitted struct {
	Op    string // "claim", "takeover", "record", "release", …
	Seq   uint64 // the journal seq that WAS committed (0 if the op appends nothing)
	Epoch uint64 // the epoch it was committed under
	Cause error  // what failed AFTER the commit
}

func (e *ErrCommitted) Error() string {
	return fmt.Sprintf("steward: %s WAS COMMITTED to the journal (seq %d, epoch %d) — but the derived state that "+
		"follows it could not be written: %v.\n"+
		"DO NOT RETRY IT. The journal is the authority and it already holds this operation; retrying would append it a "+
		"second time. What is stale is the liveness cache, which is derived and rebuildable: run "+
		"`steward heartbeat --epoch %d`, which reconstructs it from the journal and is safe to repeat.",
		e.Op, e.Seq, e.Epoch, e.Cause, e.Epoch)
}

func (e *ErrCommitted) Unwrap() error { return e.Cause }

// Committed reports the seq and epoch that reached the journal.
func (e *ErrCommitted) Committed() (seq, epoch uint64) { return e.Seq, e.Epoch }

// committed wraps a post-append failure, or returns nil if there was none.
func committed(op string, seq, epoch uint64, err error) error {
	if err == nil {
		return nil
	}
	return &ErrCommitted{Op: op, Seq: seq, Epoch: epoch, Cause: err}
}

// failpoint is a test hook for crash simulation, and it is a no-op in production.
//
// The durability arguments in this package — a repair is atomic, a commit is either
// visible or not — are claims about what happens when the process dies at the worst
// possible instant. Those instants are unreachable from an ordinary test, so the code
// names them, and the tests kill the process there. A named failpoint that nothing can
// trigger in production is a cheap price for a durability property that is actually
// exercised rather than merely asserted in a comment.
var failpoint = func(string) error { return nil }

func (s *Store) journalPath() string   { return filepath.Join(s.dir, "journal.jsonl") }
func (s *Store) seatPath() string      { return filepath.Join(s.dir, "seat.json") }
func (s *Store) lockPath() string      { return filepath.Join(s.dir, "steward.lock") }
func (s *Store) checkpointDir() string { return filepath.Join(s.dir, "checkpoints") }
func (s *Store) transcriptDir() string { return filepath.Join(s.dir, "transcripts") }
func (s *Store) grantDir() string      { return filepath.Join(s.dir, "grants") }
func (s *Store) receiptDir() string    { return filepath.Join(s.dir, "receipts") }
func (s *Store) quarantineDir() string { return filepath.Join(s.dir, "quarantine") }

// withLock serializes an entire read/decide/write cycle.
//
// The lock is essential exactly here: Claim must READ the journal, decide the seat
// is free, and WRITE its claim — and if two agents interleave those three steps,
// both conclude the seat is vacant and both take it. That is the race the singleton
// contract exists to forbid, reproduced inside the mechanism meant to enforce it.
//
// There is no no-op fallback. On a platform with no file locking, lockFile returns
// ErrLockUnsupported and every MUTATION fails closed with it; reads, which never
// take the lock, keep working. A lock that silently does nothing is worse than no
// lock, because the caller believes it is protected: it converts "this platform
// cannot host a steward" into "this platform hosts two stewards and neither knows".
// lockAcquire is the lock withLock takes. It is a var so a test can simulate a
// platform with no file locking and prove that every mutation there fails CLOSED
// rather than proceeding unserialized (TestUnsupportedLockFailsEveryMutationClosed).
var lockAcquire = lockFile

func (s *Store) withLock(fn func() error) error {
	f, err := os.OpenFile(s.lockPath(), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("steward: lock: %w", err)
	}
	defer f.Close()
	unlock, err := lockAcquire(f)
	if err != nil {
		return fmt.Errorf("steward: lock: %w", err)
	}
	defer unlock()
	return fn()
}

// Replay walks the journal and returns the valid prefix plus an honest account of
// any unreadable tail. This is the ONLY way state is ever derived: every view in
// this package is a pure function of what Replay returns.
func (s *Store) Replay() (*Replay, error) { return readJournal(s.journalPath()) }

// writeJSONAtomic writes v to path atomically and durably: a reader — possibly a
// successor mid-recovery — sees either the whole previous file or the whole new
// one, never a half-written blend, and a rename the OS acknowledged survives a
// crash.
func writeJSONAtomic(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeBytesAtomic(path, append(b, '\n'))
}

// writeBytesAtomic is writeJSONAtomic for raw content.
func writeBytesAtomic(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds

	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	// fsync the DATA before the rename: a rename that lands while the contents are
	// still in the page cache can leave a correctly-named, empty file after a crash.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	// fsync the DIRECTORY so the rename itself is durable, not just the bytes.
	// Best-effort: some filesystems refuse to open a directory for sync, and a
	// missing directory fsync is a weaker guarantee, not a wrong one.
	if d, err := os.Open(filepath.Dir(path)); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

func readJSON(path string, v any) (bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	return true, nil
}

// mustUTC normalizes a caller-supplied clock. Every timestamp this package writes
// is UTC: a journal that outlives the session that wrote it must not be readable
// only in the timezone it happened to be born in.
func mustUTC(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}

func digestOf(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// jsonUnmarshalStrict rejects trailing garbage as well as bad syntax, so "is this a
// complete record?" cannot be answered yes by a fragment that merely starts like one.
func jsonUnmarshalStrict(b []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return fmt.Errorf("trailing data after the JSON value")
	}
	return nil
}
