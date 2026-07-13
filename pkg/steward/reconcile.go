// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package steward

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/qiangli/coreutils/pkg/principal"
)

// Health is the honest verdict on the subsystem as a whole.
type Health string

const (
	// HealthOK — the journal is intact and every claim in it is established.
	HealthOK Health = "ok"
	// HealthDegraded — the record is readable but something in it could not be
	// established (an unevidenced success, a self-declared degraded outcome).
	HealthDegraded Health = "degraded"
	// HealthUnknown — the record itself is damaged (a corrupt tail). What survives
	// is still valid; what came after it cannot be spoken for.
	//
	// There is no "failed" here on purpose. The subsystem never returns success in
	// the face of missing evidence, but neither does it invent a failure it cannot
	// prove: unknown means unknown.
	HealthUnknown Health = "unknown"
)

// Reconciliation is the explicit "what do we actually know?" report.
//
// It is the verb a successor runs FIRST, before touching anything: it says who
// holds the seat, whether the journal is intact, which claims are unproven, and
// which artifacts have gone missing — the difference between inheriting a system
// and inheriting a story about a system.
type Reconciliation struct {
	SchemaVersion string    `json:"schema_version"`
	At            time.Time `json:"at"`
	Health        Health    `json:"health"`

	Seat View `json:"seat"`

	JournalIntact  bool   `json:"journal_intact"`
	JournalEntries int    `json:"journal_entries"`
	JournalHead    string `json:"journal_head"`
	// CorruptTail describes an unreadable tail, if any. The entries BEFORE it are
	// still fully valid and are still counted above — that is the whole point.
	CorruptTail string `json:"corrupt_tail,omitempty"`

	Board Board `json:"board"`

	// Unproven lists entries that claimed an outcome they did not evidence. These
	// are the claims a successor must not take on faith.
	Unproven []UnprovenClaim `json:"unproven,omitempty"`

	// MissingArtifacts lists transcript artifacts whose bytes are gone. This is
	// NOT an error and never degrades health: transcripts are optional by contract,
	// and every projection is derived without them.
	MissingArtifacts []string `json:"missing_artifacts,omitempty"`

	// TamperedArtifacts lists artifacts whose bytes no longer match their recorded
	// digest. THIS is worth alarm — a present-but-altered artifact is a lie, where
	// an absent one is merely a gap.
	TamperedArtifacts []string `json:"tampered_artifacts,omitempty"`

	// CheckpointsVerified reports each stored checkpoint's reproducibility.
	CheckpointsVerified []CheckpointVerdict `json:"checkpoints_verified,omitempty"`
}

// UnprovenClaim is one entry whose assertion could not be established.
type UnprovenClaim struct {
	Seq        uint64  `json:"seq"`
	Workstream string  `json:"workstream,omitempty"`
	Claimed    Outcome `json:"claimed"`
	Effective  Outcome `json:"effective"`
	Summary    string  `json:"summary"`
	Why        string  `json:"why"`
}

// Reconcile compares the journal against reality and reports what it can and
// cannot establish. It is READ-ONLY unless record is set.
//
// The result is allowed — required, even — to say "I don't know". A reconciliation
// that always produced a clean verdict would be worthless; the only useful thing it
// can do is tell you precisely where the record runs out.
func (s *Store) Reconcile(now time.Time) (Reconciliation, error) {
	now = mustUTC(now)
	rep, err := s.Replay()
	if err != nil {
		return Reconciliation{}, err
	}
	view, err := s.viewFrom(rep, now)
	if err != nil {
		return Reconciliation{}, err
	}

	r := Reconciliation{
		SchemaVersion:  SchemaVersion,
		At:             now,
		Seat:           view,
		JournalIntact:  rep.Intact(),
		JournalEntries: len(rep.Entries),
		JournalHead:    rep.Head,
		Board:          ProjectBoard(rep.Entries),
	}
	if rep.Corrupt {
		r.CorruptTail = fmt.Sprintf("line %d: %s (the %d entries before it are valid and are counted above)",
			rep.CorruptLine, rep.CorruptReason, len(rep.Entries))
	}

	for _, e := range rep.Entries {
		if !e.Kind.Authoritative() {
			continue
		}
		if e.Outcome != "" && e.Degraded() {
			why := "outcome was recorded as " + string(e.Outcome)
			if e.Outcome == OutcomeSuccess {
				why = "claimed success with no evidence — a claim nobody can check is not a fact"
			}
			r.Unproven = append(r.Unproven, UnprovenClaim{
				Seq: e.Seq, Workstream: e.Workstream,
				Claimed: e.Outcome, Effective: e.EffectiveOutcome(),
				Summary: e.Summary, Why: why,
			})
		}
	}

	// Artifacts: absent is fine, altered is not.
	for _, e := range rep.Entries {
		if e.Artifact == nil || e.Artifact.Path == "" {
			continue
		}
		path := filepath.Join(s.dir, e.Artifact.Path)
		b, err := os.ReadFile(path)
		if err != nil {
			r.MissingArtifacts = append(r.MissingArtifacts,
				fmt.Sprintf("seq %d: %s (optional — no projection depends on it)", e.Seq, e.Artifact.Path))
			continue
		}
		sum := sha256.Sum256(b)
		if got := "sha256:" + hex.EncodeToString(sum[:]); got != e.Artifact.Digest {
			r.TamperedArtifacts = append(r.TamperedArtifacts,
				fmt.Sprintf("seq %d: %s (digest mismatch — the bytes were altered)", e.Seq, e.Artifact.Path))
		}
	}

	if cks, err := s.ListCheckpoints(); err == nil {
		for _, ck := range cks {
			if v, err := s.VerifyCheckpoint(ck); err == nil {
				r.CheckpointsVerified = append(r.CheckpointsVerified, v)
			}
		}
	}

	r.Health = r.deriveHealth()
	return r, nil
}

// deriveHealth grades the whole record. Damage to the record itself outranks
// unproven claims within it: if the journal is torn, everything past the tear is
// unknown, and calling that merely "degraded" would understate it.
func (r *Reconciliation) deriveHealth() Health {
	if !r.JournalIntact {
		return HealthUnknown
	}
	for _, v := range r.CheckpointsVerified {
		if !v.Reproducible {
			return HealthUnknown // a checkpoint that no longer re-derives means history moved
		}
	}
	if len(r.TamperedArtifacts) > 0 {
		return HealthUnknown
	}
	if len(r.Unproven) > 0 || r.Board.Degraded {
		return HealthDegraded
	}
	// A seat whose liveness we cannot establish is a degradation, not a failure —
	// a lapsed heartbeat proves only a lapse.
	if r.Seat.Liveness == LivenessUnknown {
		return HealthDegraded
	}
	return HealthOK
}

// RecordReconciliation appends the reconciliation to the journal as an explicit
// event, so "we checked, and here is what we could not establish" becomes part of
// the permanent record rather than a thing printed once and lost.
//
// The entry's outcome mirrors the health verdict — including unknown — so a
// reconciliation that found damage can never be replayed later as a success.
func (s *Store) RecordReconciliation(actor principal.Ref, r Reconciliation, now time.Time) (Entry, error) {
	outcome := OutcomeSuccess
	switch r.Health {
	case HealthDegraded:
		outcome = OutcomeDegraded
	case HealthUnknown:
		outcome = OutcomeUnknown
	}

	ev := []Evidence{
		{Kind: "digest", Ref: fmt.Sprintf("journal-head:%d", r.JournalEntries), Digest: r.JournalHead, Note: "journal-head"},
		{Kind: "digest", Ref: fmt.Sprintf("board:%d", r.Board.Watermark), Digest: r.Board.Digest, Note: "board-digest"},
	}
	summary := fmt.Sprintf("reconciled: %s — %d entries, %d workstreams, %d unproven claim(s)",
		r.Health, r.JournalEntries, len(r.Board.Workstreams), len(r.Unproven))
	if r.CorruptTail != "" {
		summary += "; journal tail unreadable"
	}

	return s.Record(Entry{
		Actor:    actor,
		Kind:     KindReconcile,
		Summary:  summary,
		Outcome:  outcome,
		Evidence: ev,
	}, 0, now)
}

// Repair truncates an unreadable journal tail and records that it did so.
//
// It removes ONLY the bytes after the last entry that verified — never a valid
// entry — and the reconcile entry it writes afterwards means the truncation itself
// is part of the permanent history. A log that could quietly heal itself would be a
// log nobody could rely on, since "it repaired itself" and "someone edited it"
// would be indistinguishable.
func (s *Store) Repair(actor principal.Ref, now time.Time) (discarded int64, err error) {
	now = mustUTC(now)
	err = s.withLock(func() error {
		n, err := RepairTail(s.journalPath())
		discarded = n
		return err
	})
	if err != nil || discarded == 0 {
		return discarded, err
	}
	// Best-effort: the truncation already succeeded and is the thing that mattered.
	// If the seat is vacant (nobody to attribute the repair to), the note is skipped
	// rather than failing a completed repair.
	_, _ = s.Record(Entry{
		Actor:   actor,
		Kind:    KindReconcile,
		Summary: fmt.Sprintf("repaired journal: discarded %d unreadable trailing byte(s) after the last valid entry", discarded),
		Outcome: OutcomeDegraded, // a repair is never a clean success — data was lost
		Evidence: []Evidence{
			{Kind: "note", Ref: fmt.Sprintf("discarded-bytes:%d", discarded), Note: "tail-repair"},
		},
	}, 0, now)
	return discarded, nil
}
