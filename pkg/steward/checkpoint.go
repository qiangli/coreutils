// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package steward

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/principal"
)

// Checkpoint is a materialized projection of the journal at a watermark.
//
// It is a CACHE with a receipt, never a competing truth. Everything in it is
// re-derivable from the journal, and the watermark plus the journal digest are the
// receipt that says exactly which history it came from. Delete every checkpoint on
// the host and you have lost nothing but the time it takes to recompute them.
//
// This is the discipline that keeps a checkpoint honest. The tempting design — let
// a checkpoint be edited, let it accumulate state the journal never saw — produces
// an artifact that is faster to read and impossible to trust, and the first time it
// disagrees with the journal nobody can say which one is wrong.
type Checkpoint struct {
	SchemaVersion string    `json:"schema_version"`
	ID            string    `json:"id"`
	CreatedAt     time.Time `json:"created_at"`

	// Watermark is the journal seq this checkpoint projects. JournalDigest is the
	// chain head hash AT that watermark — together they pin the exact history, so a
	// checkpoint can be VERIFIED rather than trusted.
	Watermark     uint64 `json:"watermark"`
	JournalDigest string `json:"journal_digest"`

	Epoch  uint64        `json:"epoch"`
	Holder principal.Ref `json:"holder,omitzero"`

	Board Board `json:"board"`

	// Degraded carries every unresolved claim forward, by name. A checkpoint that
	// quietly dropped its unknowns would be worse than no checkpoint: it would look
	// like a clean bill of health.
	Degraded []string `json:"degraded,omitempty"`

	// Note is the operator's reason for taking this checkpoint.
	Note string `json:"note,omitempty"`
}

// entriesUpTo returns the prefix of entries at or below a watermark. Watermark 0
// means "everything".
func entriesUpTo(entries []Entry, watermark uint64) []Entry {
	if watermark == 0 {
		return entries
	}
	var out []Entry
	for _, e := range entries {
		if e.Seq > watermark {
			break
		}
		out = append(out, e)
	}
	return out
}

// headAt returns the chain hash at a watermark — the digest that pins the history.
func headAt(entries []Entry) string {
	if n := len(entries); n > 0 {
		return entries[n-1].Hash
	}
	return genesis
}

// ProjectCheckpoint derives a checkpoint from entries, PURELY. Same entries and
// same watermark always yield the same board and the same digests — no clock, no
// randomness, no ambient state leaks in. CreatedAt/ID/Note are stamped by the
// caller, and are deliberately NOT part of the board digest, so a re-derivation an
// hour later still proves the same history.
func ProjectCheckpoint(entries []Entry, watermark uint64) Checkpoint {
	prefix := entriesUpTo(entries, watermark)
	board := ProjectBoard(prefix)
	auth := deriveAuthority(&Replay{Entries: prefix, MaxEpoch: maxEpochOf(prefix)})

	ck := Checkpoint{
		SchemaVersion: SchemaVersion,
		Watermark:     board.Watermark,
		JournalDigest: headAt(prefix),
		Epoch:         auth.Epoch,
		Holder:        auth.Holder,
		Board:         board,
	}
	for _, ws := range board.Workstreams {
		if ws.Outcome == OutcomeUnknown || ws.Outcome == OutcomeDegraded {
			ck.Degraded = append(ck.Degraded, fmt.Sprintf("%s: %s (%s)", ws.Name, ws.Outcome, ws.Confidence))
		}
	}
	return ck
}

func maxEpochOf(entries []Entry) uint64 {
	var m uint64
	for _, e := range entries {
		if e.Epoch > m {
			m = e.Epoch
		}
	}
	return m
}

// Checkpoint materializes and stores a verified checkpoint at the journal's
// current head, and records THAT IT DID SO in the journal.
//
// The recording is not ceremony: it is what makes `steward history` able to say a
// checkpoint was taken even after the checkpoint file is gone. The file is the
// cache; the journal entry is the memory.
func (s *Store) Checkpoint(actor principal.Ref, note string, now time.Time) (Checkpoint, error) {
	now = mustUTC(now)
	var out Checkpoint
	err := s.withLock(func() error {
		rep, err := s.Replay()
		if err != nil {
			return err
		}
		if rep.Corrupt {
			return &ErrCorruptTail{Line: rep.CorruptLine, Reason: rep.CorruptReason, ValidEntries: len(rep.Entries)}
		}

		ck := ProjectCheckpoint(rep.Entries, 0)
		ck.ID = fmt.Sprintf("ck-%s-%04d", now.UTC().Format("20060102T150405Z"), ck.Watermark)
		ck.CreatedAt = now
		ck.Note = note

		if err := os.MkdirAll(s.checkpointDir(), 0o700); err != nil {
			return err
		}
		if err := writeJSONAtomic(filepath.Join(s.checkpointDir(), ck.ID+".json"), ck); err != nil {
			return err
		}

		// Record the checkpoint in the journal — evidence-bearing, so it never
		// projects as an unevidenced success.
		e := Entry{
			Actor:     actor,
			Epoch:     deriveAuthority(rep).Epoch,
			Kind:      KindCheckpoint,
			Ref:       ck.ID,
			Summary:   fmt.Sprintf("checkpoint %s at watermark %d (%d workstreams, %d degraded)", ck.ID, ck.Watermark, len(ck.Board.Workstreams), len(ck.Degraded)),
			Rationale: note,
			Outcome:   OutcomeSuccess,
			Evidence: []Evidence{
				{Kind: "digest", Ref: ck.ID, Digest: ck.Board.Digest, Note: "board-digest"},
				{Kind: "digest", Ref: fmt.Sprintf("watermark:%d", ck.Watermark), Digest: ck.JournalDigest, Note: "journal-head"},
			},
		}
		if _, err := appendEntry(s.journalPath(), rep, e, now); err != nil {
			return err
		}
		out = ck
		return nil
	})
	return out, err
}

// VerifyCheckpoint re-derives a stored checkpoint from the journal and reports
// whether it still holds.
//
// Because a checkpoint is a pure projection, "still holds" is decidable rather
// than a matter of trust: re-project at the same watermark and compare digests. A
// mismatch means the journal beneath it changed — which, given the hash chain,
// means someone rewrote history. That is worth finding out about.
type CheckpointVerdict struct {
	ID            string `json:"id"`
	Reproducible  bool   `json:"reproducible"`
	Watermark     uint64 `json:"watermark"`
	StoredDigest  string `json:"stored_digest"`
	DerivedDigest string `json:"derived_digest"`
	StoredHead    string `json:"stored_journal_digest"`
	DerivedHead   string `json:"derived_journal_digest"`
	Reason        string `json:"reason,omitempty"`
}

// VerifyCheckpoint checks a stored checkpoint against the live journal.
func (s *Store) VerifyCheckpoint(ck Checkpoint) (CheckpointVerdict, error) {
	rep, err := s.Replay()
	if err != nil {
		return CheckpointVerdict{}, err
	}
	derived := ProjectCheckpoint(rep.Entries, ck.Watermark)
	v := CheckpointVerdict{
		ID:            ck.ID,
		Watermark:     ck.Watermark,
		StoredDigest:  ck.Board.Digest,
		DerivedDigest: derived.Board.Digest,
		StoredHead:    ck.JournalDigest,
		DerivedHead:   derived.JournalDigest,
	}
	switch {
	case ck.JournalDigest != derived.JournalDigest:
		v.Reason = "journal head at this watermark differs — the underlying history was rewritten"
	case ck.Board.Digest != derived.Board.Digest:
		v.Reason = "board no longer re-derives from the same entries — projection or data changed"
	default:
		v.Reproducible = true
	}
	return v, nil
}

// LoadCheckpoint reads a stored checkpoint by id.
func (s *Store) LoadCheckpoint(id string) (Checkpoint, error) {
	var ck Checkpoint
	found, err := readJSON(filepath.Join(s.checkpointDir(), id+".json"), &ck)
	if err != nil {
		return Checkpoint{}, err
	}
	if !found {
		return Checkpoint{}, fmt.Errorf("steward: no such checkpoint %q", id)
	}
	return ck, nil
}

// ListCheckpoints returns the stored checkpoint files, newest first. Missing files
// are not an error: the journal remembers that a checkpoint was taken even when
// the cache of it is gone (see History).
func (s *Store) ListCheckpoints() ([]Checkpoint, error) {
	entries, err := os.ReadDir(s.checkpointDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Checkpoint
	for _, de := range entries {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".json") {
			continue
		}
		var ck Checkpoint
		if found, err := readJSON(filepath.Join(s.checkpointDir(), de.Name()), &ck); err != nil || !found {
			continue // a corrupt checkpoint must not hide the healthy ones
		}
		out = append(out, ck)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
