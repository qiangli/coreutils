// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package steward

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/principal"
)

// Every test here is named for the CLAIM it defends, and each one guards a
// specific way a continuity subsystem rots: a second steward appears, a returning
// zombie interleaves its writes, a crash erases the history, or — the quietest and
// worst — an agent's unevidenced "done ✅" gets laundered into a fact.

// ─── helpers ──────────────────────────────────────────────────────────────────

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s
}

// agent builds a stable, distinct identity. Name+Host, never an Episode: SameHolder
// matches on episode FIRST, so two test agents sharing one would be treated as the
// same logical steward and every singleton assertion here would pass vacuously.
func agent(name string) principal.Ref {
	return principal.Ref{Kind: principal.KindAgent, Name: name, Host: "test-host"}
}

var t0 = time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)

func at(d time.Duration) time.Time { return t0.Add(d) }

// mustClaim claims or fails the test.
func mustClaim(t *testing.T, s *Store, who principal.Ref, when time.Time) View {
	t.Helper()
	v, err := s.Claim(who, "", when)
	if err != nil {
		t.Fatalf("Claim(%s): %v", who.Name, err)
	}
	return v
}

// mustRecord appends an entry or fails the test.
func mustRecord(t *testing.T, s *Store, e Entry, when time.Time) Entry {
	t.Helper()
	out, err := s.Record(e, 0, when)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	return out
}

// evidenced is an entry whose success claim is actually backed by something.
func evidenced(who principal.Ref, ws, summary string) Entry {
	return Entry{
		Actor: who, Kind: KindEffect, Workstream: ws, Summary: summary,
		Outcome:  OutcomeSuccess,
		Evidence: []Evidence{{Kind: "command", Ref: "go test ./..."}},
	}
}

// journalBytes reads the raw journal.
func journalBytes(t *testing.T, s *Store) []byte {
	t.Helper()
	b, err := os.ReadFile(s.journalPath())
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	return b
}

// ─── singleton acquisition ────────────────────────────────────────────────────

// The seat is a SINGLETON. If two agents can both believe they hold it, every other
// guarantee in this package is decoration — so this is the first thing that must
// hold, and it must hold under a real race, not a polite sequential one.
func TestSeatIsSingletonUnderConcurrentClaims(t *testing.T) {
	s := newStore(t)

	const n = 12
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		winners []string
		held    int
	)
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			v, err := s.Claim(agent(string(rune('a'+i))), "", at(0))
			mu.Lock()
			defer mu.Unlock()
			var e *ErrHeld
			switch {
			case err == nil:
				winners = append(winners, v.Authority.Holder.Name)
			case errors.As(err, &e):
				held++
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}(i)
	}
	wg.Wait()

	if len(winners) != 1 {
		t.Fatalf("the seat is a singleton: %d agents believe they hold it (%v) — a lost acquisition race means two stewards writing the host's authoritative record", len(winners), winners)
	}
	if held != n-1 {
		t.Fatalf("expected %d losers to be told the seat is held, got %d", n-1, held)
	}

	// And the journal agrees: exactly ONE seat.claimed entry.
	rep, err := s.Replay()
	if err != nil {
		t.Fatal(err)
	}
	claims := 0
	for _, e := range rep.Entries {
		if e.Kind == KindSeatClaimed {
			claims++
		}
	}
	if claims != 1 {
		t.Fatalf("journal records %d claims for a singleton seat; want 1", claims)
	}
}

// Re-claiming a seat you already hold and are live in is a HEARTBEAT, not a new
// tenure. If it minted an epoch, a steward's own routine keep-alive would fence
// itself — and it would climb the epoch ladder forever for no reason.
func TestReclaimByLiveHolderIsJustAHeartbeat(t *testing.T) {
	s := newStore(t)
	first := mustClaim(t, s, agent("alice"), at(0))

	again, err := s.Claim(agent("alice"), "", at(time.Minute))
	if err != nil {
		t.Fatalf("a live holder re-claiming its own seat must succeed: %v", err)
	}
	if again.Authority.Epoch != first.Authority.Epoch {
		t.Fatalf("re-claim bumped the epoch %d → %d: a steward would fence itself with its own keep-alive",
			first.Authority.Epoch, again.Authority.Epoch)
	}

	rep, _ := s.Replay()
	if len(rep.Entries) != 1 {
		t.Fatalf("re-claim wrote a journal entry (%d total): a heartbeat is a pulse, not history", len(rep.Entries))
	}
}

// A LIVE seat is not claimable. Taking one is `takeover`, and takeover is a human's
// decision — the whole point of separating the two verbs.
func TestClaimRefusesALiveSeat(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))

	_, err := s.Claim(agent("bob"), "", at(time.Minute))
	var held *ErrHeld
	if !errors.As(err, &held) {
		t.Fatalf("claiming a live seat must be refused with ErrHeld, got %v", err)
	}
	// The refusal must TEACH: it names the remedy, or the agent will invent one.
	if !strings.Contains(err.Error(), "takeover") {
		t.Fatalf("ErrHeld must point at the remedy (`steward takeover`); got: %v", err)
	}
}

// ─── heartbeat / staleness ────────────────────────────────────────────────────

// A stale heartbeat proves A LIVENESS LAPSE AND NOTHING MORE.
//
// This is the single most misread signal in every lease system: "the heartbeat is
// old" is treated as "the holder is dead", and then a returning incumbent — which
// was merely throttled or mid-thought — silently corrupts the record. Here it must
// degrade to `lapsed` while AUTHORITY stays exactly where it was.
func TestStaleHeartbeatProvesOnlyALivenessLapse(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))

	v, err := s.Status(at(TTL + time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if v.Liveness != LivenessLapsed {
		t.Fatalf("liveness = %q, want %q", v.Liveness, LivenessLapsed)
	}
	// AUTHORITY is untouched. A lapse says nothing about who holds the seat.
	if v.Authority.Vacant || v.Authority.Holder.Name != "alice" {
		t.Fatalf("a lapsed heartbeat vacated the seat: authority=%+v — a lapse is not a death, "+
			"and inferring one would let a throttled steward be quietly erased", v.Authority)
	}
	if !v.Claimable {
		t.Fatal("a lapsed seat must be claimable — otherwise a crashed steward blocks the host until a human intervenes")
	}
}

func TestLiveHeartbeatKeepsTheSeat(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))

	// Alice keeps breathing right up to the edge of the TTL.
	for d := time.Duration(0); d < 3*TTL; d += TTL / 2 {
		if err := s.Heartbeat(agent("alice"), at(d)); err != nil {
			t.Fatalf("Heartbeat at %v: %v", d, err)
		}
		v, err := s.Status(at(d))
		if err != nil {
			t.Fatal(err)
		}
		if v.Liveness != LivenessLive {
			t.Fatalf("at %v: liveness = %q, want live — a steward that keeps heartbeating must keep the seat", d, v.Liveness)
		}
	}
	// A heartbeat is a pulse, not history: none of that reached the journal.
	rep, _ := s.Replay()
	if len(rep.Entries) != 1 {
		t.Fatalf("heartbeats wrote %d journal entries; a journal that recorded every pulse would bury the events that matter", len(rep.Entries)-1)
	}
}

// Recording IS a heartbeat. A steward actively writing to the journal is
// self-evidently alive; making it prove that separately is busywork that only ever
// fails at the worst moment.
func TestRecordingRefreshesLiveness(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))

	// Well past the TTL — but she records, which is itself proof of life.
	mustRecord(t, s, evidenced(agent("alice"), "api", "migrated the schema"), at(TTL+time.Minute))

	v, err := s.Status(at(TTL + 2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if v.Liveness != LivenessLive {
		t.Fatalf("liveness = %q after a fresh journal write; a steward that is writing is alive", v.Liveness)
	}
}

func TestHeartbeatFromNonHolderIsRejected(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))

	err := s.Heartbeat(agent("bob"), at(time.Minute))
	var nh *ErrNotHolder
	if !errors.As(err, &nh) {
		t.Fatalf("a non-holder must not be able to keep someone else's seat alive; got %v", err)
	}
}

// ─── takeover (human-authorized) ──────────────────────────────────────────────

// Takeover WITHOUT a named human is refused. An agent that could decide on its own
// to seize the seat would eventually decide to do it to a healthy steward.
func TestTakeoverRequiresHumanAuthorization(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))

	_, err := s.Takeover(agent("bob"), Authorization{Reason: "she looks stuck"}, at(time.Minute))
	var ua *ErrUnauthorized
	if !errors.As(err, &ua) {
		t.Fatalf("takeover with no --authorized-by must be refused; got %v", err)
	}

	// And it must not have touched a thing.
	v, _ := s.Status(at(time.Minute))
	if v.Authority.Holder.Name != "alice" {
		t.Fatalf("a refused takeover still changed the holder to %q", v.Authority.Holder.Name)
	}
}

// A human CAN seize a live seat, and the record says who, from whom, and why.
// An unexplained seizure of authority is indistinguishable from a hijack.
func TestTakeoverSeizesALiveSeatAndRecordsItsAuthority(t *testing.T) {
	s := newStore(t)
	alice := mustClaim(t, s, agent("alice"), at(0))

	v, err := s.Takeover(agent("bob"), Authorization{By: "qiangli", Reason: "alice wedged on a rate limit"}, at(time.Minute))
	if err != nil {
		t.Fatalf("Takeover: %v", err)
	}
	if v.Authority.Holder.Name != "bob" {
		t.Fatalf("holder = %q, want bob", v.Authority.Holder.Name)
	}
	if v.Authority.Epoch <= alice.Authority.Epoch {
		t.Fatalf("takeover must BUMP the epoch (%d → %d); without a higher epoch the prior holder is not fenced",
			alice.Authority.Epoch, v.Authority.Epoch)
	}
	if v.Authority.AuthorizedBy != "qiangli" {
		t.Fatalf("authorized_by = %q, want qiangli", v.Authority.AuthorizedBy)
	}
	if v.Authority.TakenOverFrom == nil || v.Authority.TakenOverFrom.Name != "alice" {
		t.Fatalf("the record must name who was fenced; taken_over_from = %+v", v.Authority.TakenOverFrom)
	}

	// The authorization survives REPLAY — it is in the hash-chained journal, not
	// only in a status file that a crash could take with it.
	changes, _, _, err := s.History()
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, c := range changes {
		if c.Kind == KindSeatTakeover && c.AuthorizedBy == "qiangli" {
			found = true
		}
	}
	if !found {
		t.Fatal("the takeover's human authorization did not survive replay; history: it must live in the journal")
	}
}

// ─── fencing ──────────────────────────────────────────────────────────────────

// THE PAYOFF. A steward that lapsed, was taken over, and comes back believing it
// still holds the seat is REJECTED — loudly — rather than silently interleaving its
// writes with the real steward's.
//
// This is exactly the scenario a stale heartbeat cannot rule out, which is why the
// epoch exists at all.
func TestReturningZombieIsFenced(t *testing.T) {
	s := newStore(t)
	alice := mustClaim(t, s, agent("alice"), at(0))
	aliceEpoch := alice.Authority.Epoch

	// Alice lapses; bob is authorized to take the seat.
	if _, err := s.Takeover(agent("bob"), Authorization{By: "qiangli", Reason: "recovery"}, at(TTL+time.Minute)); err != nil {
		t.Fatal(err)
	}

	// Alice comes back, mid-sentence, still holding her old fencing token.
	_, err := s.Record(Entry{
		Actor: agent("alice"), Kind: KindEffect, Workstream: "api",
		Summary: "…and then I deployed it", Outcome: OutcomeSuccess,
		Evidence: []Evidence{{Kind: "command", Ref: "kubectl apply"}},
	}, aliceEpoch, at(TTL+2*time.Minute))

	var fenced *ErrFenced
	if !errors.As(err, &fenced) {
		t.Fatalf("a returning zombie steward MUST be fenced, got %v — otherwise its writes interleave with the real steward's and the record is corrupt", err)
	}
	if fenced.Presented != aliceEpoch || fenced.Current <= aliceEpoch {
		t.Fatalf("fencing error is wrong: presented=%d current=%d (alice held %d)", fenced.Presented, fenced.Current, aliceEpoch)
	}

	// Nothing of hers reached the journal.
	entries, _, _ := s.Log(Filter{Workstream: "api"})
	for _, e := range entries {
		if e.Actor.Name == "alice" {
			t.Fatalf("a fenced write landed in the journal: seq %d %q", e.Seq, e.Summary)
		}
	}
}

// Old-epoch mutation is rejected even when the actor IS still the holder — e.g. a
// long-running process that captured epoch 1, lapsed, and re-claimed as epoch 2,
// while an old goroutine still carries the stale token.
func TestOldEpochMutationIsRejectedEvenForTheCurrentHolder(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))

	// Alice lapses and re-claims: same holder, NEW epoch.
	v2, err := s.Claim(agent("alice"), "", at(TTL+time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if v2.Authority.Epoch != 2 {
		t.Fatalf("re-claim after a lapse must mint a new epoch; got %d", v2.Authority.Epoch)
	}

	_, err = s.Record(evidenced(agent("alice"), "api", "stale-token write"), 1, at(TTL+2*time.Minute))
	var fenced *ErrFenced
	if !errors.As(err, &fenced) {
		t.Fatalf("a write bearing epoch 1 while the seat is at epoch 2 must be fenced, got %v", err)
	}
}

// The epoch ladder is MONOTONIC. If a release lowered it, a fenced holder could
// become un-fenced simply by waiting — which would make the fence worthless.
func TestEpochIsMonotonicAcrossRelease(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))
	if err := s.Release(agent("alice"), 0, "done", at(time.Minute)); err != nil {
		t.Fatal(err)
	}
	v, err := s.Claim(agent("bob"), "", at(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if v.Authority.Epoch != 2 {
		t.Fatalf("epoch after release+claim = %d, want 2: the ladder must never descend, "+
			"or a fenced holder un-fences itself by waiting", v.Authority.Epoch)
	}
}

// A bystander cannot write the host's authoritative record.
func TestNonHolderCannotWrite(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))

	_, err := s.Record(evidenced(agent("mallory"), "api", "I was here"), 0, at(time.Minute))
	var nh *ErrNotHolder
	if !errors.As(err, &nh) {
		t.Fatalf("a non-holder must not write to the journal; got %v", err)
	}
}

func TestWriteToVacantSeatIsRejected(t *testing.T) {
	s := newStore(t)
	_, err := s.Record(evidenced(agent("alice"), "api", "no seat"), 0, at(0))
	var nh *ErrNotHolder
	if !errors.As(err, &nh) || !nh.Vacant {
		t.Fatalf("writing with no seat held must be refused as vacant; got %v", err)
	}
}

// Seat lifecycle events mint their own epoch, so a generic Record must not be able
// to forge one — otherwise the fencing ladder is climbable by anyone.
func TestRecordCannotForgeASeatEvent(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))

	_, err := s.Record(Entry{Actor: agent("alice"), Kind: KindSeatTakeover, Summary: "I hereby seize the seat"}, 0, at(time.Minute))
	if err == nil {
		t.Fatal("Record accepted a seat lifecycle event: the epoch ladder must not be climbable by a generic write")
	}
	if !strings.Contains(err.Error(), "claim/takeover/release") {
		t.Fatalf("the refusal must name the right verbs; got: %v", err)
	}
}

// ─── crash recovery, with no handoff note ─────────────────────────────────────

// THE HARD REQUIREMENT: continuity must survive an incumbent who never says goodbye.
//
// Simulate a total crash — the process is gone, seat.json is gone with it, and there
// is no handoff note anywhere. A successor must still reconstruct WHO held the seat,
// at WHAT epoch, and WHAT they had established, purely by replaying the journal.
func TestCrashRecoveryWithoutAHandoffNote(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))
	mustRecord(t, s, evidenced(agent("alice"), "api", "migrated the schema"), at(time.Minute))
	mustRecord(t, s, Entry{
		Actor: agent("alice"), Kind: KindDecision, Workstream: "api",
		Summary: "drop the v1 endpoint", Rationale: "no callers in 90 days",
	}, at(2*time.Minute))

	// CRASH. The liveness record dies with the process. No goodbye, no handoff note,
	// no cooperation of any kind from the incumbent.
	if err := os.Remove(s.seatPath()); err != nil {
		t.Fatal(err)
	}

	// A brand-new process opens the same store, knowing nothing.
	succ, err := Open(s.Dir())
	if err != nil {
		t.Fatal(err)
	}

	v, err := succ.Status(at(3 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	// AUTHORITY survived: the journal remembers who held the seat and at which epoch.
	if v.Authority.Vacant || v.Authority.Holder.Name != "alice" || v.Authority.Epoch != 1 {
		t.Fatalf("authority did not survive the crash: %+v — it must be replayable from the journal alone", v.Authority)
	}
	// LIVENESS did not, and says so honestly rather than inventing a death.
	if v.Liveness != LivenessUnknown {
		t.Fatalf("liveness = %q, want %q: 'I have no idea' and 'I checked and it is late' are different facts", v.Liveness, LivenessUnknown)
	}
	if !v.Claimable {
		t.Fatal("a seat with no liveness record must be claimable, or a crash deadlocks the host forever")
	}

	// The successor learns what happened AND what alice was steering toward — by
	// replay, not by being told.
	board, _, err := succ.Board()
	if err != nil {
		t.Fatal(err)
	}
	if len(board.Workstreams) != 1 || board.Workstreams[0].Name != "api" {
		t.Fatalf("the successor could not reconstruct the board: %+v", board.Workstreams)
	}
	if board.Workstreams[0].Decisions != 1 {
		t.Fatal("the successor lost alice's DECISION: effects tell you what was done, only a decision record tells you what it was for")
	}

	// And it can take the seat with no cooperation from alice at all.
	v2, err := succ.Claim(agent("bob"), "picking up after a crash", at(4*time.Minute))
	if err != nil {
		t.Fatalf("a successor must be able to claim after a crash with no handoff note: %v", err)
	}
	if v2.Authority.Epoch != 2 {
		t.Fatalf("epoch after recovery = %d, want 2 (alice is now fenced)", v2.Authority.Epoch)
	}
}

// Claiming the seat is an act of AUTHORITY, not a checkout. It must not touch a
// repository — no diff captured, no working tree restored, nothing.
//
// This is the boundary against pkg/handoff, and it is the whole reason both exist:
// WORK is a diff, a SEAT is a mandate, and only one of them should ever mutate a repo.
func TestSeatLifecycleTouchesNoRepository(t *testing.T) {
	s := newStore(t)

	// A "repository" with work in flight.
	repo := t.TempDir()
	dirty := filepath.Join(repo, "in-flight.go")
	if err := os.WriteFile(dirty, []byte("package main // uncommitted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(dirty)
	if err != nil {
		t.Fatal(err)
	}

	mustClaim(t, s, agent("alice"), at(0))
	mustRecord(t, s, evidenced(agent("alice"), "api", "did a thing"), at(time.Minute))
	if err := s.Release(agent("alice"), 0, "standing down", at(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	mustClaim(t, s, agent("bob"), at(3*time.Minute))

	// The working tree is byte-identical, and nothing new appeared beside it.
	after, err := os.ReadFile(dirty)
	if err != nil {
		t.Fatalf("the seat lifecycle disturbed the working tree: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("claiming/releasing the seat MUTATED a working file — a seat is a mandate, not a checkout")
	}
	ents, err := os.ReadDir(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 {
		t.Fatalf("the seat lifecycle created %d files in the repo; it must create none", len(ents)-1)
	}

	// And no journal entry smuggled a diff in as an artifact.
	rep, _ := s.Replay()
	for _, e := range rep.Entries {
		if e.Artifact != nil {
			t.Fatalf("seq %d captured an artifact during a pure seat lifecycle: %+v", e.Seq, e.Artifact)
		}
	}
}

func TestReleaseIsOptionalForCorrectness(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))
	// Alice never releases. She simply vanishes. The seat still becomes claimable.
	v, err := s.Claim(agent("bob"), "", at(TTL+time.Minute))
	if err != nil {
		t.Fatalf("an unreleased seat must still expire: %v", err)
	}
	if v.Authority.Holder.Name != "bob" {
		t.Fatalf("holder = %q, want bob", v.Authority.Holder.Name)
	}
}

func TestReleasingAVacantSeatIsANoOp(t *testing.T) {
	s := newStore(t)
	if err := s.Release(agent("alice"), 0, "", at(0)); err != nil {
		t.Fatalf("releasing a vacant seat must be a no-op, not an error: %v", err)
	}
}

// ─── append-only replay + hash chain ──────────────────────────────────────────

// The journal is APPEND-ONLY and hash-chained: alter or remove any entry and every
// later entry stops verifying. A record you can quietly rewrite is not a record.
func TestReplayDetectsAnAlteredEntry(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))
	mustRecord(t, s, evidenced(agent("alice"), "api", "the truth"), at(time.Minute))
	mustRecord(t, s, evidenced(agent("alice"), "api", "later work"), at(2*time.Minute))

	// Someone edits history: rewrite the middle entry's summary in place.
	raw := string(journalBytes(t, s))
	tampered := strings.Replace(raw, `"the truth"`, `"a lie!!!!"`, 1)
	if tampered == raw {
		t.Fatal("test bug: nothing was tampered")
	}
	if err := os.WriteFile(s.journalPath(), []byte(tampered), 0o600); err != nil {
		t.Fatal(err)
	}

	rep, err := s.Replay()
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Corrupt {
		t.Fatal("replay accepted an altered entry: the hash chain must make tampering detectable")
	}
	// It stopped AT the tampered entry, keeping the valid prefix before it.
	if len(rep.Entries) != 1 {
		t.Fatalf("replay kept %d entries; the valid prefix before the tamper is 1 (the seat claim)", len(rep.Entries))
	}
}

func TestReplayIsDeterministicAndChainsFromGenesis(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))
	mustRecord(t, s, evidenced(agent("alice"), "api", "one"), at(time.Minute))

	rep, err := s.Replay()
	if err != nil {
		t.Fatal(err)
	}
	if rep.Corrupt {
		t.Fatalf("fresh journal replayed as corrupt: %s", rep.CorruptReason)
	}
	if rep.Entries[0].PrevHash != genesis {
		t.Fatalf("the first entry must chain from the public genesis root, got %q", rep.Entries[0].PrevHash)
	}
	for i, e := range rep.Entries {
		if e.Seq != uint64(i+1) {
			t.Fatalf("entry %d has seq %d: sequence must be dense and monotonic", i, e.Seq)
		}
		if e.Schema != SchemaVersion {
			t.Fatalf("entry %d carries schema %q, want %q — every artifact is schema-versioned", i, e.Schema, SchemaVersion)
		}
	}
	// Verify() over the raw bytes agrees — a third party can check the chain with no
	// access to the Store.
	v := Verify(strings.NewReader(string(journalBytes(t, s))))
	if v.Corrupt || v.Head != rep.Head {
		t.Fatalf("independent Verify disagreed with Replay: corrupt=%v head=%q want %q", v.Corrupt, v.Head, rep.Head)
	}
}

// ─── corrupt-tail recovery ────────────────────────────────────────────────────

// A crash mid-append leaves a torn final line. That must NOT erase the history
// before it: refusing to read a whole journal because its last 40 bytes are garbage
// would turn a survivable crash into total amnesia — the exact failure this
// subsystem exists to prevent.
func TestCorruptTailDoesNotHidePriorHistory(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))
	mustRecord(t, s, evidenced(agent("alice"), "api", "real work"), at(time.Minute))
	mustRecord(t, s, evidenced(agent("alice"), "db", "more real work"), at(2*time.Minute))

	good := journalBytes(t, s)

	// The power goes out mid-write: a half-serialized final line.
	f, err := os.OpenFile(s.journalPath(), os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"schema":"bashy-steward-v1","seq":4,"prev_ha`); err != nil {
		t.Fatal(err)
	}
	f.Close()

	rep, err := s.Replay()
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Corrupt {
		t.Fatal("a torn tail must be REPORTED, not silently ignored")
	}
	if len(rep.Entries) != 3 {
		t.Fatalf("a torn tail hid valid history: %d entries survived, want 3 — the history before the tear is exactly what a successor needs", len(rep.Entries))
	}
	// The board still projects from the valid prefix.
	board := ProjectBoard(rep.Entries)
	if len(board.Workstreams) != 2 {
		t.Fatalf("board lost workstreams to a torn tail: %+v", board.Workstreams)
	}

	// WRITES refuse, rather than forking the chain around the damage.
	_, err = s.Record(evidenced(agent("alice"), "api", "carrying on regardless"), 0, at(3*time.Minute))
	var ct *ErrCorruptTail
	if !errors.As(err, &ct) {
		t.Fatalf("appending onto a corrupt tail must be refused; got %v", err)
	}
	if ct.ValidEntries != 3 {
		t.Fatalf("the error must tell the operator how much good history survives (got %d, want 3) — "+
			"otherwise a repair looks like it might cost them everything", ct.ValidEntries)
	}

	// REPAIR truncates only the unreadable bytes, and never a valid entry.
	discarded, err := s.Repair(agent("alice"), at(4*time.Minute))
	if err != nil {
		t.Fatalf("Repair: %v", err)
	}
	if discarded == 0 {
		t.Fatal("Repair discarded nothing but the tail was corrupt")
	}

	rep2, err := s.Replay()
	if err != nil {
		t.Fatal(err)
	}
	if rep2.Corrupt {
		t.Fatalf("journal still corrupt after repair: %s", rep2.CorruptReason)
	}
	// The three valid entries are intact and BYTE-IDENTICAL to what they were — plus
	// the repair note, which is history too.
	if len(rep2.Entries) < 3 {
		t.Fatalf("repair destroyed valid entries: %d remain, want >= 3", len(rep2.Entries))
	}
	if !strings.HasPrefix(string(journalBytes(t, s)), string(good)) {
		t.Fatal("repair rewrote bytes it should never have touched: the valid prefix must be preserved exactly")
	}
	// A repair is never a clean success — data WAS lost, and the record says so.
	last := rep2.Entries[len(rep2.Entries)-1]
	if last.Kind != KindReconcile || last.Outcome != OutcomeDegraded {
		t.Fatalf("the repair must be recorded as a DEGRADED reconcile (got kind=%q outcome=%q): "+
			"a log that silently healed itself would be worthless, since 'it repaired itself' and "+
			"'someone tampered with it' would look identical", last.Kind, last.Outcome)
	}
}

func TestRepairOnAnIntactJournalIsANoOp(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))
	before := journalBytes(t, s)

	discarded, err := s.Repair(agent("alice"), at(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if discarded != 0 {
		t.Fatalf("repair discarded %d bytes from an intact journal", discarded)
	}
	if string(journalBytes(t, s)) != string(before) {
		t.Fatal("repair modified an intact journal")
	}
}

// ─── evidence: unknown/degraded preservation ──────────────────────────────────

// THE LOAD-BEARING RULE. A claim of success with no evidence is not success — it is
// UNKNOWN. An agent writes fluent, confident prose about work it did not do; the
// only defense that scales is to refuse to promote an unevidenced claim into a fact.
func TestUnevidencedSuccessProjectsAsUnknown(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))

	e := mustRecord(t, s, Entry{
		Actor: agent("alice"), Kind: KindEffect, Workstream: "api",
		Summary: "shipped it, all tests pass ✅", Outcome: OutcomeSuccess, // …and nothing to point at.
	}, at(time.Minute))

	// The journal records the CLAIM faithfully — it is an honest record of what was
	// asserted…
	if e.Outcome != OutcomeSuccess {
		t.Fatalf("the journal must record the claim as made, got %q", e.Outcome)
	}
	// …but nothing will ever project it as a fact.
	if e.EffectiveOutcome() != OutcomeUnknown {
		t.Fatalf("effective outcome = %q, want unknown: an unevidenced 'done ✅' must never become a fact", e.EffectiveOutcome())
	}

	board, _, err := s.Board()
	if err != nil {
		t.Fatal(err)
	}
	ws := board.Workstreams[0]
	if ws.Outcome != OutcomeUnknown || ws.Confidence != ConfidenceUnknown {
		t.Fatalf("board shows outcome=%q confidence=%q; an unevidenced success must show as unknown", ws.Outcome, ws.Confidence)
	}
	if !board.Degraded {
		t.Fatal("the board must flag itself degraded at the TOP LEVEL, or a status check misses it by reading only the happy rows")
	}
}

func TestEvidencedSuccessIsVerified(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))
	mustRecord(t, s, evidenced(agent("alice"), "api", "shipped it"), at(time.Minute))

	board, _, err := s.Board()
	if err != nil {
		t.Fatal(err)
	}
	ws := board.Workstreams[0]
	if ws.Outcome != OutcomeSuccess || ws.Confidence != ConfidenceVerified {
		t.Fatalf("an evidenced success must verify: outcome=%q confidence=%q", ws.Outcome, ws.Confidence)
	}
	if board.Degraded {
		t.Fatal("a fully-evidenced board must not report itself degraded")
	}
}

// Degradation travels ONE WAY. A failure without evidence stays a failure: the cost
// of a false "success" is unbounded, the cost of a false "failed" is a second look.
func TestFailureWithoutEvidenceStaysFailure(t *testing.T) {
	e := Entry{Kind: KindEffect, Outcome: OutcomeFailed, Summary: "it broke"}
	if got := e.EffectiveOutcome(); got != OutcomeFailed {
		t.Fatalf("effective outcome = %q, want failed: we never upgrade toward the happy path", got)
	}
	if !e.Degraded() {
		// A failure is a settled, evidenced-enough-to-act-on fact, not an unknown.
		if e.EffectiveOutcome() != OutcomeFailed {
			t.Fatal("a failure must remain a failure")
		}
	}
}

// "Closed" and "verified done" are DIFFERENT FACTS. Collapsing them is how a status
// board starts reporting wishes as facts.
func TestClosedWithoutEvidenceIsClosedAndUnknown(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))
	if _, err := s.OpenWorkstream(agent("alice"), "api", "the API migration", at(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CloseWorkstream(agent("alice"), "api", "all done!", OutcomeSuccess, nil, at(2*time.Minute)); err != nil {
		t.Fatal(err)
	}

	board, _, err := s.Board()
	if err != nil {
		t.Fatal(err)
	}
	ws := board.Workstreams[0]
	if ws.State != WorkstreamClosed {
		t.Fatalf("state = %q, want closed", ws.State)
	}
	if ws.Outcome != OutcomeUnknown {
		t.Fatalf("outcome = %q, want unknown: closing does not conjure evidence", ws.Outcome)
	}
	if len(ws.Degraded) == 0 || !strings.Contains(ws.Degraded[0], "no evidence") {
		t.Fatalf("the board must say WHICH claim is unproven, in words: %v", ws.Degraded)
	}
}

// A later, evidenced entry SETTLES an earlier unknown. An unknown that a subsequent
// fact resolved is history, not a live problem — otherwise the board never goes green
// again and everyone learns to ignore it.
func TestAnEvidencedEntrySettlesAnEarlierUnknown(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))
	mustRecord(t, s, Entry{
		Actor: agent("alice"), Kind: KindEffect, Workstream: "api",
		Summary: "probably shipped", Outcome: OutcomeSuccess,
	}, at(time.Minute))
	mustRecord(t, s, evidenced(agent("alice"), "api", "confirmed shipped"), at(2*time.Minute))

	board, _, err := s.Board()
	if err != nil {
		t.Fatal(err)
	}
	if board.Degraded {
		t.Fatal("an unknown that a later evidenced entry settled must not keep the board red forever")
	}
	if board.Workstreams[0].Confidence != ConfidenceVerified {
		t.Fatalf("confidence = %q, want verified", board.Workstreams[0].Confidence)
	}
}

// ─── reconcile: unknown/degraded honesty ──────────────────────────────────────

func TestReconcileReportsDegradedForUnprovenClaims(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))
	mustRecord(t, s, Entry{
		Actor: agent("alice"), Kind: KindEffect, Workstream: "api",
		Summary: "migrated everything", Outcome: OutcomeSuccess,
	}, at(time.Minute))

	r, err := s.Reconcile(at(2 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if r.Health != HealthDegraded {
		t.Fatalf("health = %q, want degraded: an unevidenced claim is exactly what reconcile exists to surface", r.Health)
	}
	if len(r.Unproven) != 1 {
		t.Fatalf("unproven = %d, want 1", len(r.Unproven))
	}
	if !strings.Contains(r.Unproven[0].Why, "no evidence") {
		t.Fatalf("reconcile must say WHY a claim is unproven: %q", r.Unproven[0].Why)
	}
	if r.SchemaVersion != SchemaVersion {
		t.Fatalf("reconciliation is unversioned: %q", r.SchemaVersion)
	}
}

func TestReconcileReportsUnknownForADamagedJournal(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))
	mustRecord(t, s, evidenced(agent("alice"), "api", "real work"), at(time.Minute))

	f, _ := os.OpenFile(s.journalPath(), os.O_WRONLY|os.O_APPEND, 0o600)
	f.WriteString("{ this is not json\n")
	f.Close()

	r, err := s.Reconcile(at(2 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if r.Health != HealthUnknown {
		t.Fatalf("health = %q, want unknown: damage to the RECORD outranks unproven claims within it", r.Health)
	}
	if r.JournalIntact {
		t.Fatal("reconcile reported an intact journal over a torn one")
	}
	// The valid prefix is still counted and still usable.
	if r.JournalEntries != 2 {
		t.Fatalf("reconcile counted %d valid entries, want 2 — what survives is still valid", r.JournalEntries)
	}
	if !strings.Contains(r.CorruptTail, "valid") {
		t.Fatalf("the corrupt-tail note must reassure that prior history survives: %q", r.CorruptTail)
	}
}

// A reconciliation that found damage must never be replayable later as a success.
func TestRecordedReconciliationMirrorsItsVerdict(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))
	mustRecord(t, s, Entry{
		Actor: agent("alice"), Kind: KindEffect, Workstream: "api",
		Summary: "trust me", Outcome: OutcomeSuccess,
	}, at(time.Minute))

	r, err := s.Reconcile(at(2 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	e, err := s.RecordReconciliation(agent("alice"), r, at(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if e.Outcome != OutcomeDegraded {
		t.Fatalf("the recorded reconciliation claims outcome %q for a DEGRADED verdict — "+
			"a check that found problems must not replay as a success", e.Outcome)
	}
}

func TestReconcileOnAHealthyStoreIsOK(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))
	mustRecord(t, s, evidenced(agent("alice"), "api", "did it, here is the proof"), at(time.Minute))

	r, err := s.Reconcile(at(2 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if r.Health != HealthOK {
		t.Fatalf("health = %q, want ok. unproven=%+v degraded=%v", r.Health, r.Unproven, r.Board.Degraded)
	}
}

// ─── checkpoints: reproducible projections, never a competing truth ───────────

// A checkpoint is a PURE projection: same entries, same watermark → same board and
// the same digests. No clock, no randomness, no ambient state may leak in, or
// "reproducible" means nothing.
func TestCheckpointProjectionIsPure(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))
	mustRecord(t, s, evidenced(agent("alice"), "api", "one"), at(time.Minute))
	mustRecord(t, s, evidenced(agent("alice"), "db", "two"), at(2*time.Minute))

	rep, err := s.Replay()
	if err != nil {
		t.Fatal(err)
	}
	a := ProjectCheckpoint(rep.Entries, 0)
	b := ProjectCheckpoint(rep.Entries, 0)
	if a.Board.Digest != b.Board.Digest || a.JournalDigest != b.JournalDigest {
		t.Fatal("ProjectCheckpoint is not pure: two derivations of the same entries disagree")
	}

	// Re-derived in a DIFFERENT store, from the same journal bytes: still identical.
	// A projection that depended on where it ran would not be a projection.
	other, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other.journalPath(), journalBytes(t, s), 0o600); err != nil {
		t.Fatal(err)
	}
	rep2, err := other.Replay()
	if err != nil {
		t.Fatal(err)
	}
	c := ProjectCheckpoint(rep2.Entries, 0)
	if c.Board.Digest != a.Board.Digest {
		t.Fatalf("the same journal projected a different board on another host: %s vs %s", c.Board.Digest, a.Board.Digest)
	}
}

func TestCheckpointIsReproducibleAndVerifiable(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))
	mustRecord(t, s, evidenced(agent("alice"), "api", "one"), at(time.Minute))

	ck, err := s.Checkpoint(agent("alice"), "before the risky bit", at(2*time.Minute))
	if err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	if ck.SchemaVersion != SchemaVersion {
		t.Fatalf("checkpoint is unversioned: %q", ck.SchemaVersion)
	}
	if ck.JournalDigest == "" || ck.Board.Digest == "" {
		t.Fatal("a checkpoint without its receipt (watermark + digests) is a cache nobody can verify")
	}

	v, err := s.VerifyCheckpoint(ck)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Reproducible {
		t.Fatalf("a fresh checkpoint must re-derive from the journal: %s", v.Reason)
	}

	// It keeps verifying as the journal GROWS — the watermark pins the history it
	// projected, so later entries cannot invalidate an earlier checkpoint.
	mustRecord(t, s, evidenced(agent("alice"), "db", "later work"), at(3*time.Minute))
	v2, err := s.VerifyCheckpoint(ck)
	if err != nil {
		t.Fatal(err)
	}
	if !v2.Reproducible {
		t.Fatalf("appending to the journal broke an old checkpoint: %s — the watermark exists precisely to prevent this", v2.Reason)
	}

	// Reload from disk and re-verify: the stored file is the same projection.
	loaded, err := s.LoadCheckpoint(ck.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Board.Digest != ck.Board.Digest {
		t.Fatal("the checkpoint on disk does not match the one that was returned")
	}
}

// If the journal beneath a checkpoint is rewritten, the checkpoint must STOP
// verifying. That is the alarm: given the hash chain, a mismatch means someone
// rewrote history.
func TestCheckpointStopsVerifyingIfHistoryIsRewritten(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))
	mustRecord(t, s, evidenced(agent("alice"), "api", "one"), at(time.Minute))
	ck, err := s.Checkpoint(agent("alice"), "", at(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	// Rewrite history: drop the middle entry entirely.
	lines := strings.Split(strings.TrimSpace(string(journalBytes(t, s))), "\n")
	rewritten := strings.Join([]string{lines[0], lines[2]}, "\n") + "\n"
	if err := os.WriteFile(s.journalPath(), []byte(rewritten), 0o600); err != nil {
		t.Fatal(err)
	}

	v, err := s.VerifyCheckpoint(ck)
	if err != nil {
		t.Fatal(err)
	}
	if v.Reproducible {
		t.Fatal("a checkpoint kept verifying after the journal beneath it was rewritten — it must be the alarm, not a rubber stamp")
	}
	if v.Reason == "" {
		t.Fatal("a non-reproducible checkpoint must say why")
	}
}

// The checkpoint FILE is a cache; the fact that a checkpoint was TAKEN is history.
// Delete every file and the journal still remembers.
func TestCheckpointFilesAreACacheTheJournalIsTheMemory(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))
	ck, err := s.Checkpoint(agent("alice"), "", at(time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	if err := os.RemoveAll(s.checkpointDir()); err != nil {
		t.Fatal(err)
	}
	cks, err := s.ListCheckpoints()
	if err != nil {
		t.Fatalf("a missing checkpoint dir must not be an error: %v", err)
	}
	if len(cks) != 0 {
		t.Fatalf("expected no checkpoint FILES, got %d", len(cks))
	}

	// …but history remembers it was taken, at which watermark.
	_, refs, _, err := s.History()
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].ID != ck.ID {
		t.Fatalf("the journal forgot a checkpoint whose file was deleted: %+v", refs)
	}
}

// A checkpoint that quietly dropped its unknowns would be worse than no checkpoint:
// it would look like a clean bill of health.
func TestCheckpointCarriesDegradedClaimsForward(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))
	mustRecord(t, s, Entry{
		Actor: agent("alice"), Kind: KindEffect, Workstream: "api",
		Summary: "shipped, honest", Outcome: OutcomeSuccess,
	}, at(time.Minute))

	ck, err := s.Checkpoint(agent("alice"), "", at(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(ck.Degraded) == 0 {
		t.Fatal("the checkpoint dropped its unresolved claims — a checkpoint that hides its unknowns is a clean bill of health nobody earned")
	}
	if !strings.Contains(ck.Degraded[0], "api") {
		t.Fatalf("the degraded list must name WHICH workstream is unproven: %v", ck.Degraded)
	}
}

// ─── transcripts are OPTIONAL, by contract ────────────────────────────────────

// THE NAMED TEST (doc.go and cli.go both promise it by name).
//
// Delete every transcript artifact on the host and the board, the status, the
// history, and every checkpoint must be BIT-IDENTICAL. Nothing authoritative may
// ever depend on a non-authoritative artifact — otherwise a model's prose quietly
// becomes load-bearing.
func TestTranscriptDeletionDoesNotAffectProjections(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))
	mustRecord(t, s, evidenced(agent("alice"), "api", "migrated the schema"), at(time.Minute))
	if _, err := s.Decide(agent("alice"), "api", "drop v1", "no callers in 90d", nil, at(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Transcript(agent("alice"), "api",
		"the conversation where we agreed to drop v1",
		strings.NewReader("human: should we drop v1?\nagent: yes, nobody calls it.\n"), at(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	mustRecord(t, s, evidenced(agent("alice"), "db", "vacuumed"), at(4*time.Minute))

	// Take the checkpoint FIRST — it appends a journal entry of its own, and a
	// snapshot taken before it would differ afterwards for a reason that has nothing
	// to do with transcripts.
	beforeCk, err := s.Checkpoint(agent("alice"), "", at(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	// Snapshot every projection WITH the transcript present.
	beforeBoard, _, err := s.Board()
	if err != nil {
		t.Fatal(err)
	}
	beforeStatus, err := s.Status(at(5 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	beforeChanges, beforeCks, _, err := s.History()
	if err != nil {
		t.Fatal(err)
	}

	// The transcript artifact exists on disk…
	tdir := s.transcriptDir()
	files, err := os.ReadDir(tdir)
	if err != nil || len(files) != 1 {
		t.Fatalf("expected exactly one transcript artifact on disk, got %v (%v)", files, err)
	}

	// …now DELETE every one of them.
	if err := os.RemoveAll(tdir); err != nil {
		t.Fatal(err)
	}

	// Every projection must be bit-identical.
	afterBoard, _, err := s.Board()
	if err != nil {
		t.Fatalf("the board must derive without transcripts: %v", err)
	}
	if afterBoard.Digest != beforeBoard.Digest {
		t.Fatalf("deleting transcripts CHANGED the board digest (%s → %s): a projection depends on a "+
			"non-authoritative artifact, which means a model's prose is load-bearing",
			beforeBoard.Digest, afterBoard.Digest)
	}

	afterStatus, err := s.Status(at(5 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if afterStatus.Authority != beforeStatus.Authority || afterStatus.Liveness != beforeStatus.Liveness {
		t.Fatal("deleting transcripts changed the seat status")
	}

	afterChanges, afterCks, _, err := s.History()
	if err != nil {
		t.Fatal(err)
	}
	if len(afterChanges) != len(beforeChanges) || len(afterCks) != len(beforeCks) {
		t.Fatal("deleting transcripts changed the history")
	}

	// And a checkpoint re-derived now must equal the one taken before the deletion.
	v, err := s.VerifyCheckpoint(beforeCk)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Reproducible {
		t.Fatalf("a checkpoint stopped re-deriving once its transcripts were deleted: %s", v.Reason)
	}

	// Reconcile NOTICES the artifact is gone — and does NOT call that a failure.
	// An absent transcript is a gap in richness, never a gap in truth.
	r, err := s.Reconcile(at(6 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.MissingArtifacts) != 1 {
		t.Fatalf("reconcile did not notice the missing artifact: %+v", r.MissingArtifacts)
	}
	if r.Health == HealthUnknown {
		t.Fatal("a missing TRANSCRIPT must not damage health: it is optional by contract, and no projection depends on it")
	}
	// The decision it accompanied is untouched — the decision is what binds.
	conv, _, err := s.Conversation(Filter{Kinds: []Kind{KindDecision}})
	if err != nil {
		t.Fatal(err)
	}
	if len(conv) != 1 || conv[0].Rationale != "no callers in 90d" {
		t.Fatal("deleting a transcript damaged the DECISION record it accompanied")
	}
}

// A present-but-ALTERED artifact is a different matter: an absent one is a gap, an
// altered one is a lie.
func TestTamperedArtifactIsFlagged(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))
	e, err := s.Transcript(agent("alice"), "api", "the conversation",
		strings.NewReader("the original words\n"), at(time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(s.Dir(), e.Artifact.Path)
	if err := os.WriteFile(path, []byte("words nobody actually said\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	r, err := s.Reconcile(at(2 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.TamperedArtifacts) != 1 {
		t.Fatalf("a rewritten artifact must be detected by its digest: %+v", r.TamperedArtifacts)
	}
	if r.Health != HealthUnknown {
		t.Fatalf("health = %q; a tampered artifact must raise the alarm", r.Health)
	}
}

// ─── view derivation ──────────────────────────────────────────────────────────

// Every view is a pure function of the journal. Same entries → same views, always.
func TestViewsAreDerivedPurelyFromTheJournal(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))
	if _, err := s.OpenWorkstream(agent("alice"), "api", "the API migration", at(time.Minute)); err != nil {
		t.Fatal(err)
	}
	mustRecord(t, s, evidenced(agent("alice"), "api", "migrated"), at(2*time.Minute))
	if _, err := s.Decide(agent("alice"), "api", "drop v1", "no callers", nil, at(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CloseWorkstream(agent("alice"), "api", "done", OutcomeSuccess,
		[]Evidence{{Kind: "commit", Ref: "de6485c"}}, at(4*time.Minute)); err != nil {
		t.Fatal(err)
	}

	// A fresh Store over the SAME directory — nothing cached, nothing carried over.
	fresh, err := Open(s.Dir())
	if err != nil {
		t.Fatal(err)
	}
	board, _, err := fresh.Board()
	if err != nil {
		t.Fatal(err)
	}
	if len(board.Workstreams) != 1 {
		t.Fatalf("board = %+v", board.Workstreams)
	}
	ws := board.Workstreams[0]
	if ws.Title != "the API migration" {
		t.Fatalf("title = %q", ws.Title)
	}
	if ws.State != WorkstreamClosed || ws.Outcome != OutcomeSuccess || ws.Confidence != ConfidenceVerified {
		t.Fatalf("ws = %+v; an evidenced close must project as closed+success+verified", ws)
	}
	if ws.Decisions != 1 {
		t.Fatalf("decisions = %d, want 1", ws.Decisions)
	}
	if ws.Entries != 4 {
		t.Fatalf("entries = %d, want 4 (open, effect, decision, close)", ws.Entries)
	}

	// Log filters.
	dec, _, err := fresh.Log(Filter{Kinds: []Kind{KindDecision}})
	if err != nil {
		t.Fatal(err)
	}
	if len(dec) != 1 || dec[0].Rationale != "no callers" {
		t.Fatalf("decision filter: %+v", dec)
	}

	// Chronological order is preserved — entry order IS time order.
	all, _, err := fresh.Log(Filter{})
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(all); i++ {
		if all[i].Seq <= all[i-1].Seq {
			t.Fatal("the log is not chronological; a reordered log invents a history the chain does not attest to")
		}
	}

	// Limit keeps the LAST n, still in order.
	last2, _, err := fresh.Log(Filter{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(last2) != 2 || last2[1].Seq != all[len(all)-1].Seq {
		t.Fatalf("--limit must keep the recent tail: %+v", last2)
	}
}

// The "what do I NOT know?" query — the first thing a successor needs.
func TestDegradedOnlyFilterSurfacesTheUnproven(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))
	mustRecord(t, s, evidenced(agent("alice"), "api", "proven"), at(time.Minute))
	mustRecord(t, s, Entry{
		Actor: agent("alice"), Kind: KindEffect, Workstream: "db",
		Summary: "trust me bro", Outcome: OutcomeSuccess,
	}, at(2*time.Minute))

	got, _, err := s.Log(Filter{DegradedOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Workstream != "db" {
		t.Fatalf("--degraded must surface exactly the unproven claims, got %+v", got)
	}
}

func TestConversationShowsDecisionsAndTranscripts(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))
	mustRecord(t, s, evidenced(agent("alice"), "api", "an effect, not a decision"), at(time.Minute))
	if _, err := s.Decide(agent("alice"), "api", "drop v1", "no callers", nil, at(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Transcript(agent("alice"), "api", "how we got there",
		strings.NewReader("…"), at(3*time.Minute)); err != nil {
		t.Fatal(err)
	}

	conv, _, err := s.Conversation(Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(conv) != 2 {
		t.Fatalf("conversation = %d entries, want 2 (the decision and the transcript, not the effect)", len(conv))
	}
	if conv[0].Kind != KindDecision || conv[1].Kind != KindTranscript {
		t.Fatalf("conversation must stay in sequence: %q then %q", conv[0].Kind, conv[1].Kind)
	}
}

func TestHistoryReconstructsTheAuthorityLadder(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))
	if err := s.Release(agent("alice"), 0, "standing down", at(time.Minute)); err != nil {
		t.Fatal(err)
	}
	mustClaim(t, s, agent("bob"), at(2*time.Minute))
	if _, err := s.Takeover(agent("carol"), Authorization{By: "qiangli", Reason: "bob wedged"}, at(3*time.Minute)); err != nil {
		t.Fatal(err)
	}

	changes, _, _, err := s.History()
	if err != nil {
		t.Fatal(err)
	}
	want := []Kind{KindSeatClaimed, KindSeatReleased, KindSeatClaimed, KindSeatTakeover}
	if len(changes) != len(want) {
		t.Fatalf("history has %d changes, want %d: %+v", len(changes), len(want), changes)
	}
	for i, k := range want {
		if changes[i].Kind != k {
			t.Fatalf("change %d is %q, want %q", i, changes[i].Kind, k)
		}
	}
	// Epochs never descend.
	for i := 1; i < len(changes); i++ {
		if changes[i].Epoch < changes[i-1].Epoch {
			t.Fatalf("the epoch ladder descended: %d → %d", changes[i-1].Epoch, changes[i].Epoch)
		}
	}
	if changes[3].AuthorizedBy != "qiangli" {
		t.Fatalf("the takeover's authorizing human is missing from history: %+v", changes[3])
	}
}

// ─── follow ───────────────────────────────────────────────────────────────────

// Follow must deliver every entry appended after it started, and nothing before —
// and it must be testable without sleeping around a filesystem race, or it is a
// tailer nobody can trust to have delivered everything.
func TestFollowStreamsNewEntriesOnly(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))
	mustRecord(t, s, evidenced(agent("alice"), "api", "BEFORE the follow started"), at(time.Minute))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got := make(chan Entry, 8)
	done := make(chan error, 1)
	go func() {
		done <- s.Follow(ctx, Filter{Workstream: "api"}, 5*time.Millisecond, func(e Entry) error {
			got <- e
			return nil
		})
	}()

	// Give Follow a moment to establish its starting watermark, then append.
	time.Sleep(30 * time.Millisecond)
	mustRecord(t, s, evidenced(agent("alice"), "api", "AFTER the follow started"), at(2*time.Minute))
	mustRecord(t, s, evidenced(agent("alice"), "other", "filtered out"), at(3*time.Minute))
	mustRecord(t, s, evidenced(agent("alice"), "api", "also after"), at(4*time.Minute))

	var seen []string
	deadline := time.After(3 * time.Second)
	for len(seen) < 2 {
		select {
		case e := <-got:
			seen = append(seen, e.Summary)
		case <-deadline:
			t.Fatalf("follow delivered %d entries in time, want 2: %v", len(seen), seen)
		}
	}
	cancel()

	if err := <-done; err != nil {
		t.Fatalf("a cancelled follow must exit cleanly (Ctrl-C is how it ENDS, not how it fails): %v", err)
	}
	for _, s := range seen {
		if strings.Contains(s, "BEFORE") {
			t.Fatal("follow replayed the backlog; it must stream only what happens NEXT")
		}
		if strings.Contains(s, "filtered") {
			t.Fatal("follow ignored its filter")
		}
	}
}

// ─── schema versioning + evidence parsing ─────────────────────────────────────

func TestEveryArtifactIsSchemaVersioned(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("alice"), at(0))
	ck, err := s.Checkpoint(agent("alice"), "", at(time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	rep, _ := s.Replay()
	for _, e := range rep.Entries {
		if e.Schema != SchemaVersion {
			t.Fatalf("journal entry seq %d is unversioned (%q)", e.Seq, e.Schema)
		}
	}
	if ck.SchemaVersion != SchemaVersion {
		t.Fatalf("checkpoint is unversioned: %q", ck.SchemaVersion)
	}

	var seat Seat
	found, err := readJSON(s.seatPath(), &seat)
	if err != nil || !found {
		t.Fatalf("seat file: found=%v err=%v", found, err)
	}
	if seat.SchemaVersion != SchemaVersion {
		t.Fatalf("seat file is unversioned: %q", seat.SchemaVersion)
	}

	board, _, err := s.Board()
	if err != nil {
		t.Fatal(err)
	}
	if board.SchemaVersion != SchemaVersion {
		t.Fatalf("board is unversioned: %q", board.SchemaVersion)
	}
}

func TestParseEvidence(t *testing.T) {
	for _, tc := range []struct {
		in        string
		kind, ref string
		digest    string
	}{
		{"command:go test ./...", "command", "go test ./...", ""},
		{"commit:de6485c", "commit", "de6485c", ""},
		{"file:/tmp/out.log#sha256:abc", "file", "/tmp/out.log", "sha256:abc"},
		{"url:https://example.com/x", "url", "https://example.com/x", ""},
		// A bare string is recorded as a note rather than rejected: weak evidence
		// still beats silently dropped evidence.
		{"the human confirmed it", "note", "the human confirmed it", ""},
		// An unknown prefix is not a kind — keep the whole string.
		{"banana:split", "note", "banana:split", ""},
	} {
		got, err := ParseEvidence(tc.in)
		if err != nil {
			t.Fatalf("ParseEvidence(%q): %v", tc.in, err)
		}
		if got.Kind != tc.kind || got.Ref != tc.ref || got.Digest != tc.digest {
			t.Errorf("ParseEvidence(%q) = %+v, want kind=%q ref=%q digest=%q", tc.in, got, tc.kind, tc.ref, tc.digest)
		}
	}
	if _, err := ParseEvidence("  "); err == nil {
		t.Error("empty evidence must be rejected")
	}
}

// Evidence order must not change an entry's hash — otherwise the same facts,
// supplied in a different flag order, would produce a different chain.
func TestEvidenceOrderDoesNotAffectTheHash(t *testing.T) {
	a := Entry{
		Actor: agent("alice"), Kind: KindEffect, Summary: "x", Seq: 1, PrevHash: genesis,
		Evidence: []Evidence{{Kind: "command", Ref: "b"}, {Kind: "command", Ref: "a"}},
	}
	b := Entry{
		Actor: agent("alice"), Kind: KindEffect, Summary: "x", Seq: 1, PrevHash: genesis,
		Evidence: []Evidence{{Kind: "command", Ref: "a"}, {Kind: "command", Ref: "b"}},
	}
	sortEvidence(a.Evidence)
	sortEvidence(b.Evidence)
	if a.computeHash(genesis) != b.computeHash(genesis) {
		t.Fatal("the same evidence in a different order hashed differently; canonical ordering is what makes the chain reproducible")
	}
}

func TestDefaultDirHonorsTheEnvOverride(t *testing.T) {
	t.Setenv("BASHY_STEWARD_DIR", "/tmp/custom-steward")
	if got := DefaultDir(); got != "/tmp/custom-steward" {
		t.Fatalf("DefaultDir() = %q, want the BASHY_STEWARD_DIR override", got)
	}
}

// The seat is HOST-scoped, not repo-scoped: it must not vary with the working
// directory. Keying it to a repo would produce one steward per clone — precisely
// what the singleton exists to prevent.
func TestDefaultDirIsIndependentOfWorkingDirectory(t *testing.T) {
	t.Setenv("BASHY_STEWARD_DIR", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	want := filepath.Join(home, ".bashy", "steward")

	a := DefaultDir()
	t.Chdir(t.TempDir())
	b := DefaultDir()

	if a != b || a != want {
		t.Fatalf("DefaultDir moved with the cwd (%q → %q, want %q): the steward seat is one per host/user, not one per checkout", a, b, want)
	}
}

func TestSameHolderMatchesTheLogicalAgentNotThePID(t *testing.T) {
	// One logical agent, many processes (a shell, a subagent, a hook). None of them
	// should be told it is colliding with itself.
	a := principal.Ref{Name: "claude", Host: "h1", Episode: "ep-1"}
	b := principal.Ref{Name: "claude-subagent", Host: "h1", Episode: "ep-1"}
	if !SameHolder(a, b) {
		t.Fatal("two processes of the same episode must be the same logical steward")
	}
	c := principal.Ref{Name: "claude", Host: "h1"}
	d := principal.Ref{Name: "claude", Host: "h1"}
	if !SameHolder(c, d) {
		t.Fatal("same name+host must match when there is no episode")
	}
	e := principal.Ref{Name: "claude", Host: "h2"}
	if SameHolder(c, e) {
		t.Fatal("the same agent name on a DIFFERENT host is a different steward")
	}
}
