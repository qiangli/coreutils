// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package steward

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/principal"
)

// SchemaVersion is the on-disk contract for every artifact this package writes:
// journal entries, the seat file, and checkpoints. Bump on a breaking change.
const SchemaVersion = "bashy-steward-v1"

// genesis is the chain's public root: the PrevHash of the first entry in a fresh
// journal. Fixed and known, so a verifier needs no side channel to check the head.
const genesis = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

// Kind classifies a journal entry — and, crucially, its AUTHORITY.
//
// The three classes are not decoration. An effect is a claim about the world and
// must carry evidence. A decision is a claim about intent and must carry a
// rationale. A transcript is a claim about nothing at all: it is an artifact,
// nothing derives from it, and deleting it changes no view.
type Kind string

const (
	// KindEffect — AUTHORITATIVE. Something changed in the world (a command ran,
	// a PR merged, a service restarted). Carries evidence, or it projects unknown.
	KindEffect Kind = "effect"
	// KindObservation — AUTHORITATIVE. Something was observed to be true, without
	// this steward having caused it. Also evidence-bearing.
	KindObservation Kind = "observation"
	// KindDecision — AUTHORITATIVE. An explicit, durable decision record: what was
	// decided and WHY. It asserts intent, not effect, so it needs a rationale
	// rather than evidence.
	KindDecision Kind = "decision"
	// KindTranscript — NON-AUTHORITATIVE. An optional hash-linked conversation
	// artifact. No projection reads it. It exists so a human can go back and see
	// how a decision was reached; the decision record is what actually binds.
	KindTranscript Kind = "transcript"

	// Seat lifecycle. These are the AUTHORITY events: the epoch ladder is derived
	// from them and from nowhere else, which is what makes the seat recoverable by
	// replay when seat.json is gone.
	KindSeatClaimed  Kind = "seat.claimed"
	KindSeatTakeover Kind = "seat.takeover"
	KindSeatReleased Kind = "seat.released"

	// Workstream lifecycle.
	KindWorkstreamOpen  Kind = "workstream.open"
	KindWorkstreamClose Kind = "workstream.close"

	// KindReconcile records an explicit reconciliation: someone compared the
	// journal against reality and wrote down what they found, including what could
	// NOT be established.
	KindReconcile Kind = "reconcile"
	// KindCheckpoint marks that a checkpoint was materialized at this watermark.
	KindCheckpoint Kind = "checkpoint"
)

// Authoritative reports whether anything may be DERIVED from an entry of this
// kind. Transcripts are the sole exception, and the exception is the point.
func (k Kind) Authoritative() bool { return k != KindTranscript }

// SeatEvent reports whether this kind establishes seat authority (and therefore
// mints its own epoch, rather than being fenced against the current one).
func (k Kind) SeatEvent() bool {
	switch k {
	case KindSeatClaimed, KindSeatTakeover, KindSeatReleased:
		return true
	}
	return false
}

// Outcome is what an entry claims happened.
type Outcome string

const (
	OutcomeSuccess  Outcome = "success"
	OutcomeFailed   Outcome = "failed"
	OutcomeUnknown  Outcome = "unknown"
	OutcomeDegraded Outcome = "degraded"
)

// Evidence is a pointer to something a skeptic could go and check: a command
// that ran, a file, a commit, a test result, a URL. The Digest, when present,
// binds the reference to exact bytes.
//
// Evidence is what separates a fact from a story. An LLM writes fluent,
// confident prose about work it did not do; the only defense that scales is to
// refuse to promote an unevidenced claim into a fact, which is exactly what
// Entry.EffectiveOutcome does.
type Evidence struct {
	Kind   string `json:"kind"` // command | file | commit | test | url | digest
	Ref    string `json:"ref"`
	Digest string `json:"digest,omitempty"`
	Note   string `json:"note,omitempty"`
}

// ParseEvidence reads the CLI form "kind:ref[#digest]" — e.g.
// "command:go test ./...", "commit:de6485c", "file:/tmp/out.log#sha256:abc…".
// A bare string with no recognized kind prefix is recorded as a note-kind
// reference rather than being rejected: a weak piece of evidence is still
// better than a silently dropped one.
func ParseEvidence(s string) (Evidence, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Evidence{}, fmt.Errorf("steward: empty evidence")
	}
	ev := Evidence{Kind: "note", Ref: s}
	if k, rest, ok := strings.Cut(s, ":"); ok && rest != "" {
		switch k {
		case "command", "file", "commit", "test", "url", "digest", "note":
			ev.Kind, ev.Ref = k, rest
		}
	}
	if ref, dig, ok := strings.Cut(ev.Ref, "#"); ok && strings.HasPrefix(dig, "sha256:") {
		ev.Ref, ev.Digest = ref, dig
	}
	return ev, nil
}

// Artifact is an optional, hash-linked blob referenced by an entry — typically a
// transcript. The Digest binds the entry to exact bytes; the Path is a HINT
// about where those bytes were last seen.
//
// The bytes are allowed to be gone. A missing artifact is a degradation of the
// record's richness, never of its truth: every projection is derived from the
// entries, so a host that deletes every artifact still reports the same board.
type Artifact struct {
	Path   string `json:"path,omitempty"` // relative to the store dir
	Digest string `json:"digest"`         // sha256:…
	Bytes  int64  `json:"bytes,omitempty"`
	Media  string `json:"media,omitempty"`
}

// Entry is one journal record. Field order is the canonical serialization order
// for the hash — DO NOT reorder without bumping SchemaVersion.
type Entry struct {
	Schema   string `json:"schema"`
	Seq      uint64 `json:"seq"`
	PrevHash string `json:"prev_hash"`
	ID       string `json:"id"`
	Time     string `json:"time"` // RFC3339Nano, UTC

	// Actor is who wrote this entry, and Epoch is the seat epoch they held when
	// they wrote it. Together they are the fencing token: an entry bearing a
	// superseded epoch is rejected at append (see ErrFenced) and is recognizable
	// forever after in the log.
	Actor principal.Ref `json:"actor"`
	Epoch uint64        `json:"epoch"`

	Kind Kind `json:"kind"`

	// Workstream is the strand of work this belongs to; Ref is a free pointer
	// (issue id, PR, host, service) that the board shows but does not interpret.
	Workstream string `json:"workstream,omitempty"`
	Ref        string `json:"ref,omitempty"`

	Summary   string     `json:"summary"`
	Rationale string     `json:"rationale,omitempty"`
	Outcome   Outcome    `json:"outcome,omitempty"`
	Evidence  []Evidence `json:"evidence,omitempty"`
	Artifact  *Artifact  `json:"artifact,omitempty"`

	// Hash is H(prev_hash ‖ canonical(entry with Hash="")). Last field, so the
	// canonical form is simply this entry with this one field zeroed.
	Hash string `json:"hash"`
}

// HasEvidence reports whether the entry carries at least one checkable reference.
func (e Entry) HasEvidence() bool { return len(e.Evidence) > 0 }

// EffectiveOutcome is the outcome a PROJECTION is allowed to believe, which is
// not always the outcome that was claimed.
//
// The rule: a claim of success with no evidence is not success — it is unknown.
// This is the single most load-bearing line in the package. An agent that says
// "done ✅" and cannot point at anything has told you a story, and a system that
// records stories as facts is worse than one that records nothing, because it
// launders confidence.
//
// Degradation travels one way only. A FAILURE without evidence stays a failure;
// we never upgrade toward the happy path, because the cost of a false "success"
// is unbounded and the cost of a false "failed" is a second look.
func (e Entry) EffectiveOutcome() Outcome {
	if e.Outcome == OutcomeSuccess && !e.HasEvidence() {
		return OutcomeUnknown
	}
	return e.Outcome
}

// Degraded reports whether this entry's claim could not be established — either
// it says so itself, or it claimed success and brought nothing to back it up.
func (e Entry) Degraded() bool {
	switch e.EffectiveOutcome() {
	case OutcomeUnknown, OutcomeDegraded:
		return true
	}
	return false
}

// computeHash returns the chain hash for e given the previous entry's hash. The
// entry's own Hash is cleared first, so each link binds to the full content of
// the one before it: alter or delete any entry and every later entry's hash
// stops verifying.
func (e Entry) computeHash(prevHash string) string {
	e.Hash = ""
	body, _ := json.Marshal(e)
	h := sha256.New()
	h.Write([]byte(prevHash))
	h.Write([]byte{0}) // separator, so prev‖body is unambiguous
	h.Write(body)
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// newID mints a stable, sortable entry id.
func newID(now time.Time, seq uint64) string {
	return fmt.Sprintf("%s-%04d", now.UTC().Format("20060102T150405Z"), seq)
}

// Replay is the result of walking the journal: the valid prefix, plus an honest
// account of anything it could not read.
//
// "Valid PREFIX" is the important word. A journal whose final line was torn by a
// crash still has a perfectly good history before the tear, and that history is
// what a successor needs. Refusing to read the whole file because its last 40
// bytes are garbage would turn a survivable crash into total amnesia — which is
// the exact failure this subsystem exists to prevent.
type Replay struct {
	Entries  []Entry
	Head     string // chain hash of the last valid entry (genesis if none)
	HeadSeq  uint64
	MaxEpoch uint64

	// Corrupt reports that bytes after the valid prefix could not be replayed.
	Corrupt       bool
	CorruptLine   int    // 1-based line number where replay stopped
	CorruptReason string // why it stopped
	// ValidBytes is the byte offset just past the last valid entry — the exact
	// truncation point RepairTail uses, so a repair can never remove a valid entry.
	ValidBytes int64
}

// Intact reports a fully-readable journal.
func (r *Replay) Intact() bool { return !r.Corrupt }

// ErrCorruptTail is returned by Append when the journal's tail is unreadable.
// Writes refuse rather than silently forking the chain around the damage; reads
// carry on. Clearing it is an explicit, human-invoked act:
// `steward reconcile --repair-tail`.
type ErrCorruptTail struct {
	Line   int
	Reason string
	// ValidEntries is how much good history survives — stated in the error so the
	// operator learns immediately that a repair costs them nothing but the torn tail.
	ValidEntries int
}

func (e *ErrCorruptTail) Error() string {
	return fmt.Sprintf("steward: journal tail is corrupt at line %d (%s); %d valid entries before it are intact — "+
		"run `steward reconcile --repair-tail` to truncate the unreadable tail and resume appending",
		e.Line, e.Reason, e.ValidEntries)
}

// replayReader walks a journal stream, verifying every link, and stops at the
// first byte it cannot trust — returning everything valid before it.
func replayReader(r io.Reader) *Replay {
	out := &Replay{Head: genesis}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<20), 8<<20)

	prevHash := genesis
	var prevSeq uint64
	var offset int64
	line := 0

	for sc.Scan() {
		line++
		raw := sc.Bytes()
		// +1 for the newline the scanner stripped. Tracked so RepairTail knows the
		// exact byte at which the good history ends.
		lineBytes := int64(len(raw)) + 1

		text := strings.TrimSpace(string(raw))
		if text == "" {
			offset += lineBytes
			continue
		}

		var e Entry
		if err := json.Unmarshal([]byte(text), &e); err != nil {
			out.stop(line, "unparsable entry: "+err.Error())
			return out
		}
		if e.PrevHash != prevHash {
			out.stop(line, "prev_hash mismatch (an earlier entry was altered or removed)")
			return out
		}
		if e.Seq != prevSeq+1 {
			out.stop(line, fmt.Sprintf("seq gap (expected %d, got %d)", prevSeq+1, e.Seq))
			return out
		}
		if want := e.computeHash(prevHash); want != e.Hash {
			out.stop(line, "hash mismatch (this entry was altered)")
			return out
		}

		out.Entries = append(out.Entries, e)
		prevHash, prevSeq = e.Hash, e.Seq
		if e.Epoch > out.MaxEpoch {
			out.MaxEpoch = e.Epoch
		}
		offset += lineBytes
		out.ValidBytes = offset
	}
	if err := sc.Err(); err != nil {
		out.stop(line+1, "read error: "+err.Error())
		return out
	}
	out.Head, out.HeadSeq = prevHash, prevSeq
	return out
}

// stop records where replay lost trust, keeping every entry read so far.
func (r *Replay) stop(line int, reason string) {
	r.Corrupt, r.CorruptLine, r.CorruptReason = true, line, reason
	if n := len(r.Entries); n > 0 {
		r.Head, r.HeadSeq = r.Entries[n-1].Hash, r.Entries[n-1].Seq
	}
}

// Verify walks a journal stream and reports whether the whole chain holds. It is
// the same walk Replay does; this is the caller-facing "is my history intact?".
func Verify(r io.Reader) *Replay { return replayReader(r) }

// readJournal replays the journal file. A missing journal is an empty one — a
// fresh host has no history, and that is not an error.
func readJournal(path string) (*Replay, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Replay{Head: genesis}, nil
		}
		return nil, err
	}
	defer f.Close()
	return replayReader(f), nil
}

// appendEntry links e to the chain head and durably appends it. The caller MUST
// hold the store lock and MUST have already replayed to obtain rep — append is
// never allowed to re-derive the head on its own, because a read-decide-write
// that re-reads outside the lock is the race this whole package exists to avoid.
func appendEntry(path string, rep *Replay, e Entry, now time.Time) (Entry, error) {
	if rep.Corrupt {
		return Entry{}, &ErrCorruptTail{Line: rep.CorruptLine, Reason: rep.CorruptReason, ValidEntries: len(rep.Entries)}
	}
	e.Schema = SchemaVersion
	e.Seq = rep.HeadSeq + 1
	e.PrevHash = rep.Head
	if e.Time == "" {
		e.Time = now.UTC().Format(time.RFC3339Nano)
	}
	if e.ID == "" {
		e.ID = newID(now, e.Seq)
	}
	sortEvidence(e.Evidence)
	e.Hash = e.computeHash(e.PrevHash)

	line, err := json.Marshal(e)
	if err != nil {
		return Entry{}, err
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return Entry{}, err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return Entry{}, err
	}
	// fsync: an entry that the journal reported as written must survive the power
	// going out a microsecond later. The journal is the only authority there is —
	// if it can lose a write, everything derived from it is a guess.
	if err := f.Sync(); err != nil {
		return Entry{}, err
	}
	return e, nil
}

// sortEvidence gives evidence a canonical order so the hash of an entry does not
// depend on the order flags happened to arrive in.
func sortEvidence(evs []Evidence) {
	sort.SliceStable(evs, func(i, j int) bool {
		if evs[i].Kind != evs[j].Kind {
			return evs[i].Kind < evs[j].Kind
		}
		return evs[i].Ref < evs[j].Ref
	})
}

// RepairTail truncates the unreadable tail of a journal, and NOTHING else.
//
// It cuts at Replay.ValidBytes — the byte just past the last entry that verified
// — so a valid entry can never be removed by a repair. The bytes it discards are
// bytes no reader was able to trust anyway. Deliberately explicit and
// human-invoked: silently self-healing a chain would make the chain worthless,
// since "the log repaired itself" and "someone tampered with the log" would look
// identical.
func RepairTail(path string) (discarded int64, err error) {
	rep, err := readJournal(path)
	if err != nil {
		return 0, err
	}
	if !rep.Corrupt {
		return 0, nil
	}
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	discarded = fi.Size() - rep.ValidBytes
	if discarded <= 0 {
		return 0, nil
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	if err := f.Truncate(rep.ValidBytes); err != nil {
		return 0, err
	}
	if err := f.Sync(); err != nil {
		return 0, err
	}
	return discarded, nil
}
