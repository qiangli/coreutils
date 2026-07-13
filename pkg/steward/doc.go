// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Package steward is the host/user-scoped seat of authority and continuity: exactly
// one steward per machine-and-account, holding an append-only, hash-chained,
// evidence-carrying journal that outlives whoever holds the seat.
//
// # The problem
//
// An agentic host accumulates two kinds of loss, and they are different things.
//
// LOST WORK is a session dying with an uncommitted diff in the tree. That is
// pkg/handoff's problem: capture the working tree, restore it into a successor's
// checkout.
//
// LOST AUTHORITY AND LOST TRUTH is the agent that has been answering for this machine
// vanishing, and with it the only account of what was actually done, what was decided,
// and what was merely CLAIMED. Nobody can say who is in charge, and nobody can
// distinguish "we verified that" from "an agent said so". That is this package, and it
// is the harder one, because the incumbent NEVER GETS TO SAY GOODBYE. A steward that
// crashed, hit a rate limit, or was killed leaves no handoff note. Continuity has to
// work anyway.
//
// # The design, and why each part is load-bearing
//
// ONE JOURNAL, MANY VIEWS. journal.jsonl is the only authority. Board, status, log,
// conversation, history and checkpoints are read-only PROJECTIONS derived by replay. A
// view has no state of its own, so it structurally cannot drift into a competing
// writable truth.
//
// A REFERENCE IS NOT A VERIFICATION. This is the spine. An entry claiming success with
// NOTHING to point at projects as UNKNOWN (Entry.EffectiveOutcome). An entry claiming
// success WITH references projects as ASSERTED — still not verified. Only a
// KindVerification entry, where somebody went and looked, reaches ConfidenceVerified.
// The reason is simple and unpleasant: an agent can attach a plausible command string
// or commit hash to work it never did exactly as easily as to work it did, so a
// reference buys AUDITABILITY and nothing else. Degradation travels one way — a failure
// without evidence stays a failure, and a refutation is believed where a self-serving
// upgrade is not.
//
// AUTHORITY vs LIVENESS. Authority (holder, epoch) replays from the journal; liveness
// comes from seat.json, which is a CACHE that is validated against the journal before
// it is believed at all — wrong schema, wrong holder, wrong epoch, or a timestamp from
// the future and it is discarded, not merely discounted.
//
// A STALE HEARTBEAT PROVES ONLY A LAPSE. It never proves death. So Claim takes a seat
// only when it is VACANT or when a TRUSTWORTHY heartbeat says the holder is LAPSED, and
// the claim bumps a monotonic fencing epoch so the returning incumbent is rejected
// (ErrFenced) rather than silently interleaving its writes.
//
// AN UNREADABLE LIVENESS RECORD PROVES LESS THAN A LAPSE, SO IT IS NOT CLAIMABLE. "I
// looked and the incumbent is late" is a fact about the incumbent; "I cannot find or
// trust the record" is a fact about the RECORD, and every way of producing it is also a
// way of producing it deliberately. Deleting one file must not be enough to take a
// healthy steward's seat. Recovering from unknown is a takeover.
//
// EVERY AUTHORITATIVE WRITE PRESENTS A FENCING EPOCH, AND ZERO IS NOT A VALUE. There is
// no "use whatever epoch is current" shortcut — that convenience was a hole, because an
// agent that does not know its tenure ended is precisely the agent that would use it.
// The epoch is checked BEFORE identity, so the same logical principal returning with a
// stale token is fenced exactly like a stranger.
//
// TAKEOVER CONSUMES A DURABLE CAPABILITY. Grant is single-use (its nonce is recorded in
// the journal by the takeover that spends it), expiring, and bound to one grantee, one
// seat scope, and one epoch. Read Provenance for exactly what it does and does not
// prove: this package runs as the user, so it cannot produce cryptographic evidence
// that a human was present, and it says so rather than pretending. An operator
// assertion is labelled an ASSERTION; an unattended takeover requires an external
// receipt somebody can go and audit.
//
// TRANSCRIPTS ARE OPTIONAL BY CONTRACT. Delete every artifact and every projection is
// bit-identical (TestTranscriptDeletionDoesNotAffectProjections).
//
// TORN TAILS ARE REPAIRABLE; TAMPERING IS NOT. Replay always returns the valid PREFIX,
// so a crash that tore the last append never hides the history before it. Repair
// truncates ONLY a torn final append — mid-log damage, or a complete record that does
// not chain, fails closed, because a tool that silently truncated those would delete
// the evidence of tampering and call it a repair. Every repair quarantines the exact
// bytes it discards, by digest, and writes a degraded receipt under the holder's epoch;
// a receipt that cannot be written is an error, never a shrug.
//
// RECONCILE DOES NOT CLAIM TO HAVE CHECKED REALITY. Comparing a claim against the world
// needs an adapter that knows how (Observer). With none supplied, the report says
// plainly that nothing was compared — a re-read of the journal is a spellcheck, not a
// reality check.
//
// # Durability and concurrency
//
// Atomic temp+fsync+rename for every file; journal appends are fsynced; the whole
// read/decide/write cycle is serialized under a real file lock (flock on unix,
// LockFileEx on Windows). There is no no-op lock fallback: a platform with no locking
// fails every mutation closed (ErrLockUnsupported), because a lock that silently does
// nothing is worse than none — the caller believes it is protected while two agents
// interleave and both claim the seat.
//
// # What this is NOT
//
// It is not pkg/handoff. Claiming the seat restores no working tree, captures no diff,
// and touches no repository (TestSeatLifecycleTouchesNoRepository). WORK is a diff; a
// SEAT is a mandate.
package steward
