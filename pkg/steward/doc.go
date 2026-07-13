// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Package steward is the host/user-scoped seat of authority and continuity.
//
// There is exactly ONE steward per host/user. Not one per repository, not one
// per checkout, not one per terminal — one per machine-and-account, held by
// whoever currently holds the seat, and answerable across every project on that
// host. The seat lives in ~/.bashy/steward (or $BASHY_STEWARD_DIR) and is
// completely independent of the working directory: you can claim it from
// anywhere, and it means the same thing everywhere.
//
// # What this is NOT
//
// It is not pkg/handoff. Handoff is TASK- and ARTIFACT-scoped: it captures an
// in-flight working tree (a diff, untracked files) so a successor inherits real
// work. Steward captures no working tree, restores no working tree, and touches
// no repository. Claiming the seat is an act of AUTHORITY, not a checkout.
//
// The two compose — a steward may well hand a task off with `bashy handoff` —
// but conflating them is what made "hand off your work" ambiguous in the first
// place: WORK is a diff, a SEAT is a mandate, and only one of them should ever
// mutate your repo.
//
// # Continuity without a handoff note
//
// The hard requirement is that continuity must survive an incumbent who never
// says goodbye. A steward that crashed, was rate-limited, or simply vanished
// leaves no note — and a successor must still be able to pick up the seat, read
// what happened, and know what is true. So:
//
//   - Acquisition NEVER requires talking to the incumbent.
//   - There is NO handoff note on the critical path. The journal IS the note.
//   - A successor reconstructs everything by REPLAY, not by being told.
//
// # One journal, many views
//
// A single append-only, hash-chained journal (journal.jsonl) is the ONLY
// authority. Board, status, log, conversation, history, and checkpoints are all
// read-only PROJECTIONS derived by replaying it. None of them is a second place
// where truth lives, so none of them can disagree with the journal — the most
// common way a state machine rots is that a cached view becomes a competing
// writable truth, and this package structurally cannot do that.
//
// # Three authority classes
//
// Not every entry carries the same weight, and pretending otherwise is how a
// model's prose gets mistaken for a fact:
//
//	effect / observation   AUTHORITATIVE. Something happened in the world, and
//	                       the entry carries EVIDENCE for it.
//	decision               AUTHORITATIVE. An explicit, durable decision record
//	                       with a rationale. It does not claim an effect.
//	transcript             NON-AUTHORITATIVE. An optional hash-linked artifact
//	                       (a conversation dump). Nothing derives from it.
//
// Transcripts are OPTIONAL by contract: delete every transcript artifact on the
// host and the board, the status, the history, and every checkpoint must be
// bit-identical. TestTranscriptDeletionDoesNotAffectProjections pins this.
//
// # Missing evidence yields unknown, never success
//
// An entry claiming success WITHOUT evidence does not project as success — it
// projects as unknown. See Entry.EffectiveOutcome. The journal still records the
// claim faithfully (it is an honest record of what was asserted), but no view
// will ever promote an unevidenced assertion into a fact. Degradation only ever
// travels one way: a failure without evidence stays a failure, because the safe
// direction is never to upgrade.
//
// # The seat: heartbeat lease + monotonic fencing epoch
//
// Authority is split from liveness, and the split is the whole trick:
//
//	AUTHORITY (who holds the seat, at which epoch)  — derived from the JOURNAL
//	LIVENESS  (is the holder still breathing)       — from seat.json's heartbeat
//
// Authority is therefore recoverable by replay alone. Delete seat.json entirely
// and the holder and epoch survive; only liveness is lost, and unknown liveness
// honestly degrades to "lapsed" rather than inventing a death.
//
// A STALE HEARTBEAT PROVES ONLY A LIVENESS LAPSE. It does not prove the
// incumbent is dead, and this package never claims it does. The incumbent may be
// mid-thought, rate-limited, or on a slow network, and may come back. That is
// precisely why the epoch exists: a successor claiming an expired seat bumps the
// monotonically increasing epoch, and the returning incumbent — still holding the
// OLD epoch — is FENCED: its mutations are rejected (ErrFenced), loudly, instead
// of silently interleaving with the new steward's.
//
// # Takeover is a human act
//
// Claim takes a VACANT or EXPIRED seat and is the ordinary path. Takeover seizes
// a LIVE one and is the emergency path — so it requires explicit human
// authorization (--authorized-by) and records who authorized it and why. It never
// asks the incumbent's permission, because an incumbent that could be asked would
// not need to be taken over.
//
// # Durability
//
// Every write is atomic (temp + fsync + rename) and every read/decide/write cycle
// is serialized under an exclusive file lock — a real flock on Unix and a real
// LockFileEx on Windows (see lock_*.go), so unlike the older claim registry this
// package has no honest-but-racy Windows gap to apologize for. On any OTHER
// platform there is no lock (lock_other.go), and that is stated rather than
// pretended: what still holds there is the fencing epoch, which turns a lost
// acquisition race into a DETECTED ErrFenced rather than two silent stewards.
//
// A crash mid-append can still leave a torn final line. That is tolerated on
// READ — a corrupt tail never hides the valid history before it — and refused on
// WRITE until an explicit `steward reconcile --repair-tail`, which truncates only
// the trailing bytes AFTER the last valid entry and can never remove a valid one.
package steward
