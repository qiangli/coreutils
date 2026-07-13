// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package steward

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/qiangli/coreutils/pkg/principal"
)

// Record appends an authoritative entry to the journal, FENCED against the seat.
//
// Three gates, in order, all under one lock:
//
//  1. The journal must be readable. A corrupt tail refuses the write (ErrCorruptTail)
//     rather than forking the chain around the damage.
//  2. The actor must hold the seat (ErrNotHolder). Authority is singular; a
//     bystander does not get to write the host's authoritative record.
//  3. The presented epoch must be the CURRENT epoch (ErrFenced). This is what stops
//     a returning zombie steward — one that lapsed and was taken over — from
//     interleaving its writes with the real one's.
//
// Pass epoch 0 to mean "whatever epoch I currently hold", which is what an
// interactive caller wants; a long-running process that captured its epoch at
// claim time should pass it explicitly, because that is the whole point of holding
// a fencing token.
func (s *Store) Record(e Entry, epoch uint64, now time.Time) (Entry, error) {
	now = mustUTC(now)
	var out Entry
	err := s.withLock(func() error {
		rep, err := s.Replay()
		if err != nil {
			return err
		}
		if rep.Corrupt {
			return &ErrCorruptTail{Line: rep.CorruptLine, Reason: rep.CorruptReason, ValidEntries: len(rep.Entries)}
		}
		auth := deriveAuthority(rep)
		if auth.Vacant {
			return &ErrNotHolder{Actor: e.Actor, Vacant: true}
		}
		// FENCING IS CHECKED ON THE TOKEN, BEFORE IDENTITY — and the order matters.
		//
		// The case this exists for is a steward that lapsed, was taken over, and came
		// back still holding its old epoch. By then it is no longer the holder, so
		// checking identity first would reject it as a mere bystander (ErrNotHolder)
		// and never tell it the one thing it needs to know: your tenure ENDED, the
		// world moved on, re-read the journal before you do anything else. Both errors
		// refuse the write, so safety is identical — but only one of them explains a
		// zombie to itself, and an agent that misreads "you are not the holder" as
		// "I should just claim the seat again" will happily overwrite the steward that
		// replaced it.
		if epoch != 0 && epoch != auth.Epoch {
			return &ErrFenced{Presented: epoch, Current: auth.Epoch, Holder: auth.Holder}
		}
		if !SameHolder(auth.Holder, e.Actor) {
			return &ErrNotHolder{Actor: e.Actor, Holder: auth.Holder}
		}
		// Epoch 0 means "whatever I currently hold" — and by here, we ARE the holder.
		e.Epoch = auth.Epoch

		if e.Kind.SeatEvent() {
			// Seat lifecycle mints its own epoch and must go through Claim / Takeover /
			// Release, which know how to bump it. Letting a generic write forge one
			// would make the fencing ladder climbable by anyone.
			return fmt.Errorf("steward: %s is a seat lifecycle event — use claim/takeover/release, not record", e.Kind)
		}

		stored, err := appendEntry(s.journalPath(), rep, e, now)
		if err != nil {
			return err
		}
		out = stored
		// Writing to the journal IS a heartbeat: a steward actively recording is
		// self-evidently alive, and making it prove that separately is busywork that
		// only ever fails at the worst moment.
		_ = s.writeSeat(auth, "", now)
		return nil
	})
	return out, err
}

// Decide is the ergonomic front door for an explicit decision record: what was
// decided, and WHY.
//
// A decision asserts INTENT, not effect, so it needs a rationale rather than
// evidence — and it is authoritative on its own terms. This is the entry a
// successor reads to understand not just what happened on this host, but what the
// previous steward had concluded and was steering toward, which no amount of
// effect-replay would recover.
func (s *Store) Decide(actor principal.Ref, workstream, summary, rationale string, evidence []Evidence, now time.Time) (Entry, error) {
	return s.Record(Entry{
		Actor:      actor,
		Kind:       KindDecision,
		Workstream: workstream,
		Summary:    summary,
		Rationale:  rationale,
		Evidence:   evidence,
	}, 0, now)
}

// Transcript stores an OPTIONAL, non-authoritative conversation artifact and
// records a hash-linked pointer to it.
//
// The bytes go in transcripts/<digest>.txt and the entry carries the digest, so
// the record is tamper-evident. Nothing in any projection reads the artifact —
// delete the whole transcripts directory and every board, status, history, and
// checkpoint on the host is bit-identical. That is a contract, not an accident,
// and TestTranscriptDeletionDoesNotAffectProjections holds it to it.
//
// Why bother storing it at all: a decision record says what was decided; a
// transcript lets a human go back and see how the room got there. Useful, and
// never load-bearing.
func (s *Store) Transcript(actor principal.Ref, workstream, summary string, content io.Reader, now time.Time) (Entry, error) {
	now = mustUTC(now)
	b, err := io.ReadAll(content)
	if err != nil {
		return Entry{}, err
	}
	sum := sha256.Sum256(b)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	name := hex.EncodeToString(sum[:]) + ".txt"

	if err := os.MkdirAll(s.transcriptDir(), 0o700); err != nil {
		return Entry{}, err
	}
	if err := writeBytesAtomic(filepath.Join(s.transcriptDir(), name), b); err != nil {
		return Entry{}, err
	}

	return s.Record(Entry{
		Actor:      actor,
		Kind:       KindTranscript,
		Workstream: workstream,
		Summary:    summary,
		Artifact: &Artifact{
			Path:   filepath.Join("transcripts", name),
			Digest: digest,
			Bytes:  int64(len(b)),
			Media:  "text/plain",
		},
	}, 0, now)
}

// OpenWorkstream records the start of a strand of work.
func (s *Store) OpenWorkstream(actor principal.Ref, name, title string, now time.Time) (Entry, error) {
	return s.Record(Entry{
		Actor:      actor,
		Kind:       KindWorkstreamOpen,
		Workstream: name,
		Summary:    title,
	}, 0, now)
}

// CloseWorkstream records the end of a strand of work, with its outcome.
//
// Note what this does NOT do: it does not force the outcome to success. If the
// closing entry claims success with no evidence, the board still projects unknown
// (Entry.EffectiveOutcome), so "closed" and "verified done" remain different facts
// — which is the difference between a status board and a wish list.
func (s *Store) CloseWorkstream(actor principal.Ref, name, summary string, outcome Outcome, evidence []Evidence, now time.Time) (Entry, error) {
	return s.Record(Entry{
		Actor:      actor,
		Kind:       KindWorkstreamClose,
		Workstream: name,
		Summary:    summary,
		Outcome:    outcome,
		Evidence:   evidence,
	}, 0, now)
}

// writeBytesAtomic is writeJSONAtomic for raw content.
func writeBytesAtomic(path string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
