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
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
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

// ScopeID identifies the host AND user this seat belongs to.
//
// Both halves are load-bearing. The seat is "one steward per host/user", and the
// store lives under $HOME — but $HOME is not reliably one machine's. A shared or
// network home directory (NFS, a synced home, a container image with a baked-in
// home) is mounted on several hosts at once, and a store keyed only by $HOME would
// silently MERGE the seats of every machine sharing it: two hosts, two live
// stewards, one journal, one epoch ladder, and an endless mutual fencing war
// between agents that never had anything to do with each other.
//
// So the default store is keyed by (hostname, user). The short hash suffix keeps
// two hosts that happen to share a hostname ("localhost") apart from each other
// only insofar as their usernames differ — it is a disambiguator, not a secret.
func ScopeID() string { return scopeFor(hostname(), username()) }

// scopeFor is ScopeID's pure core, so the host/user isolation can be tested without
// an env hook in production code.
func scopeFor(host, who string) string {
	sum := sha256.Sum256([]byte(host + "\x00" + who))
	return slug(host) + "-" + slug(who) + "-" + hex.EncodeToString(sum[:4])
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil || strings.TrimSpace(h) == "" {
		return "unknown-host"
	}
	return h
}

// username resolves the account, preferring the ambient environment (which works
// with CGO_ENABLED=0 on every platform) and falling back to os/user and then the
// numeric uid. An unnamed account still gets a stable, distinguishable scope.
func username() string {
	for _, env := range []string{"USER", "USERNAME", "LOGNAME"} {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			return v
		}
	}
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return "uid" + strconv.Itoa(os.Getuid())
}

// slug reduces a name to a filesystem-safe token. Lossy on purpose — the hash
// suffix in ScopeID carries the precision, this half carries the readability.
func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteByte('-')
		}
		if b.Len() >= 32 {
			break
		}
	}
	if b.Len() == 0 {
		return "x"
	}
	return strings.Trim(b.String(), "-")
}

// DefaultDir is the host/user-scoped seat: ~/.bashy/steward/<host>-<user>-<hash>,
// or $BASHY_STEWARD_DIR verbatim when set.
//
// HOST-WIDE and cwd-INDEPENDENT, deliberately. A steward is not a property of a
// checkout — it is the human's continuous point of contact across every project on
// the machine. Keying it to a repository would produce one steward per clone, which
// is precisely what the singleton exists to prevent.
//
// MIGRATION: an earlier revision of this package stored the seat directly at
// ~/.bashy/steward. That path is no longer read. A store written there predates the
// host/user scoping and can be adopted by pointing $BASHY_STEWARD_DIR at it, or
// moved into place: `mv ~/.bashy/steward ~/.bashy/steward.old && mkdir -p
// ~/.bashy/steward/$(bashy steward scope) && mv ~/.bashy/steward.old/*
// ~/.bashy/steward/$(bashy steward scope)/`. Only do that on the host the old
// journal actually belongs to — that judgement is the entire reason the old layout
// was wrong.
func DefaultDir() string {
	if v := os.Getenv("BASHY_STEWARD_DIR"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "bashy-steward", ScopeID())
	}
	return filepath.Join(home, ".bashy", "steward", ScopeID())
}

// Store is the on-disk steward state: one journal (the authority), one seat file
// (liveness only), and directories of derived or optional artifacts.
type Store struct {
	dir string
	// scope is the host/user this store speaks for. It is stamped into grants so a
	// capability minted for one seat cannot be replayed against another.
	scope string
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

// WithScope overrides the scope id stamped into grants. Tests use it to prove that
// a grant minted for one host/user is refused by another.
func WithScope(scope string) Option {
	return func(s *Store) {
		if scope != "" {
			s.scope = scope
		}
	}
}

// Open prepares the store directory. The journal records what an agent did across
// every project on the host, so it is owner-only (0700/0600) like the audit log.
func Open(dir string, opts ...Option) (*Store, error) {
	if dir == "" {
		dir = DefaultDir()
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("steward: store dir: %w", err)
	}
	s := &Store{dir: dir, scope: ScopeID(), maxTranscript: MaxTranscriptBytes}
	for _, o := range opts {
		o(s)
	}
	return s, nil
}

func (s *Store) Dir() string   { return s.dir }
func (s *Store) Scope() string { return s.scope }

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
