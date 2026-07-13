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

// Every test here is named for the CLAIM it defends, and each one guards a specific
// way a continuity subsystem rots: a second steward appears, a returning zombie
// interleaves its writes, a crash erases the history, a seizure of authority leaves no
// trace — or, the quietest and worst, an agent's "done ✅" gets laundered into a fact.

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

// mustClaim claims or fails the test, returning the epoch the holder must now present
// on every write. Returning the EPOCH rather than the View is deliberate: it is the
// token, and a test that forgets to carry it is a test that would not have caught a
// fencing bug.
func mustClaim(t *testing.T, s *Store, who principal.Ref, when time.Time) uint64 {
	t.Helper()
	v, err := s.Claim(who, "", when)
	if err != nil {
		t.Fatalf("Claim(%s): %v", who.Name, err)
	}
	return v.Authority.Epoch
}

// mustRecord appends an entry or fails the test.
func mustRecord(t *testing.T, s *Store, e Entry, epoch uint64, when time.Time) Entry {
	t.Helper()
	out, err := s.Record(e, epoch, when)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	return out
}

// evidenced is an entry whose success claim points at SOMETHING. Note what it is not:
// verified. Nobody has checked that command ran.
func evidenced(who principal.Ref, ws, summary string) Entry {
	return Entry{
		Actor: who, Kind: KindEffect, Workstream: ws, Summary: summary,
		Outcome:  OutcomeSuccess,
		Evidence: []Evidence{{Kind: "command", Ref: "go test ./..."}},
	}
}

// mustGrant mints an interactive operator-assertion capability for `who`.
func mustGrant(t *testing.T, s *Store, who principal.Ref, when time.Time) Grant {
	t.Helper()
	g, err := s.Authorize(GrantRequest{
		Grantee: who, Actor: "qiangli", Reason: "test", Confirmed: true,
	}, when)
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	return g
}

// mustTakeover seizes the seat with a fresh grant, returning the new epoch.
func mustTakeover(t *testing.T, s *Store, who principal.Ref, when time.Time) uint64 {
	t.Helper()
	g := mustGrant(t, s, who, when)
	v, err := s.Takeover(who, TakeoverRequest{GrantID: g.ID, Interactive: true}, when)
	if err != nil {
		t.Fatalf("Takeover(%s): %v", who.Name, err)
	}
	return v.Authority.Epoch
}

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
// guarantee in this package is decoration — so this is the first thing that must hold,
// and it must hold under a real race, not a polite sequential one.
func TestSeatIsSingletonUnderConcurrentClaims(t *testing.T) {
	s := newStore(t)

	const n = 16
	var wg sync.WaitGroup
	won := make([]bool, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Claim(agent(string(rune('a'+i))), "", at(0)); err == nil {
				won[i] = true
			}
		}()
	}
	wg.Wait()

	winners := 0
	for _, w := range won {
		if w {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("exactly one agent must win the seat, got %d winners", winners)
	}

	// And the journal must agree: one claim event, not sixteen.
	rep, err := s.Replay()
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	claims := 0
	for _, e := range rep.Entries {
		if e.Kind == KindSeatClaimed {
			claims++
		}
	}
	if claims != 1 {
		t.Fatalf("the journal records %d claims; a singleton has exactly 1", claims)
	}
}

func TestReclaimByLiveHolderIsJustAHeartbeat(t *testing.T) {
	s := newStore(t)
	e1 := mustClaim(t, s, agent("a"), at(0))

	v, err := s.Claim(agent("a"), "still here", at(time.Minute))
	if err != nil {
		t.Fatalf("re-claim by the live holder must succeed: %v", err)
	}
	if v.Authority.Epoch != e1 {
		t.Fatalf("re-claiming your own live seat must NOT bump the epoch: %d → %d", e1, v.Authority.Epoch)
	}
	rep, _ := s.Replay()
	if n := len(rep.Entries); n != 1 {
		t.Fatalf("a re-claim is a heartbeat, not history: expected 1 journal entry, got %d", n)
	}
}

func TestClaimRefusesALiveSeat(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("a"), at(0))

	_, err := s.Claim(agent("b"), "", at(time.Minute))
	var held *ErrHeld
	if !errors.As(err, &held) {
		t.Fatalf("claiming a live seat must fail with ErrHeld, got %v", err)
	}
}

// A lapse is the ONE ordinary way a seat changes hands without authorization — and it
// requires a heartbeat record we actually trust. See the adversarial suite for every
// way an untrustworthy one is refused.
func TestLapsedSeatIsClaimableAndFencesTheIncumbent(t *testing.T) {
	s := newStore(t)
	e1 := mustClaim(t, s, agent("a"), at(0))

	v, err := s.Status(at(TTL + time.Minute))
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if v.Liveness != LivenessLapsed {
		t.Fatalf("a heartbeat older than the TTL is LAPSED, got %q", v.Liveness)
	}
	if !v.Claimable {
		t.Fatal("a lapsed seat with a trustworthy heartbeat is the ordinary recovery path — it must be claimable")
	}

	e2 := mustClaim(t, s, agent("b"), at(TTL+2*time.Minute))
	if e2 <= e1 {
		t.Fatalf("a change of holder must bump the fencing epoch: %d → %d", e1, e2)
	}

	// The incumbent was never asked, and may come back at any moment. It is FENCED.
	_, err = s.Record(evidenced(agent("a"), "ws", "i'm back"), e1, at(TTL+3*time.Minute))
	var fenced *ErrFenced
	if !errors.As(err, &fenced) {
		t.Fatalf("the lapsed incumbent returning at its old epoch must be FENCED, got %v", err)
	}
}

func TestLiveHeartbeatKeepsTheSeat(t *testing.T) {
	s := newStore(t)
	ep := mustClaim(t, s, agent("a"), at(0))

	if err := s.Heartbeat(agent("a"), ep, at(TTL-time.Minute)); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	// The heartbeat moved the clock forward, so what would have lapsed has not.
	v, _ := s.Status(at(TTL + time.Minute))
	if v.Liveness != LivenessLive {
		t.Fatalf("a heartbeat inside the TTL keeps the seat live, got %q", v.Liveness)
	}
	if _, err := s.Claim(agent("b"), "", at(TTL+time.Minute)); err == nil {
		t.Fatal("a live seat must not be claimable")
	}
}

func TestRecordingRefreshesLiveness(t *testing.T) {
	s := newStore(t)
	ep := mustClaim(t, s, agent("a"), at(0))
	mustRecord(t, s, evidenced(agent("a"), "ws", "did a thing"), ep, at(TTL-time.Minute))

	v, _ := s.Status(at(TTL + time.Minute))
	if v.Liveness != LivenessLive {
		t.Fatalf("writing to the journal is self-evidently being alive; liveness = %q", v.Liveness)
	}
}

func TestEpochIsMonotonicAcrossRelease(t *testing.T) {
	s := newStore(t)
	e1 := mustClaim(t, s, agent("a"), at(0))
	if err := s.Release(agent("a"), e1, "done", at(time.Minute)); err != nil {
		t.Fatalf("Release: %v", err)
	}
	e2 := mustClaim(t, s, agent("b"), at(2*time.Minute))
	if e2 <= e1 {
		t.Fatalf("the epoch ladder never resets — a release must not lower it: %d → %d", e1, e2)
	}
}

func TestReleaseIsOptionalForCorrectness(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("a"), at(0))
	// 'a' vanishes. No release, no goodbye, no cooperation.
	e2 := mustClaim(t, s, agent("b"), at(TTL+time.Minute))
	if e2 != 2 {
		t.Fatalf("a successor must not need the predecessor's cooperation; epoch = %d", e2)
	}
}

func TestReleasingAVacantSeatIsANoOp(t *testing.T) {
	s := newStore(t)
	if err := s.Release(agent("a"), 1, "", at(0)); err != nil {
		t.Fatalf("releasing a seat nobody holds is a no-op, not an error: %v", err)
	}
}

// ─── fencing ──────────────────────────────────────────────────────────────────

// The returning zombie is the case the epoch exists for: a steward that lapsed, was
// replaced, and comes back mid-sentence believing it still holds the seat.
func TestReturningZombieIsFenced(t *testing.T) {
	s := newStore(t)
	zombieEpoch := mustClaim(t, s, agent("zombie"), at(0))
	mustRecord(t, s, evidenced(agent("zombie"), "ws", "before the lapse"), zombieEpoch, at(time.Minute))

	successorEpoch := mustClaim(t, s, agent("successor"), at(TTL+time.Minute))
	mustRecord(t, s, evidenced(agent("successor"), "ws", "took over"), successorEpoch, at(TTL+2*time.Minute))

	_, err := s.Record(evidenced(agent("zombie"), "ws", "as I was saying…"), zombieEpoch, at(TTL+3*time.Minute))
	var fenced *ErrFenced
	if !errors.As(err, &fenced) {
		t.Fatalf("a zombie presenting its old epoch must be FENCED, got %v", err)
	}
	if fenced.Presented != zombieEpoch || fenced.Current != successorEpoch {
		t.Fatalf("the fence must name both epochs: presented %d, current %d", fenced.Presented, fenced.Current)
	}
	// The error explains the ZOMBIE'S situation, not a stranger's. An agent that reads
	// "you are not the holder" may decide to re-claim — and overwrite its successor.
	if !strings.Contains(fenced.Error(), "tenure ended") {
		t.Fatalf("ErrFenced must explain the zombie to itself, got: %v", fenced)
	}
}

// Fencing is a property of the TOKEN, not of the identity. The same logical principal,
// returning with a stale token, is fenced exactly like a stranger — because "being
// yourself" is not a credential, and a tenure that ended is over no matter whose hand
// the old token is in.
func TestStaleTokenIsFencedEvenForTheSameLogicalPrincipal(t *testing.T) {
	s := newStore(t)
	a := agent("a")
	old := mustClaim(t, s, a, at(0))

	// 'a' lapses, and 'a' — the same logical agent, a new process — re-claims. New tenure,
	// new epoch. The OLD process is still out there holding `old`.
	fresh := mustClaim(t, s, a, at(TTL+time.Minute))
	if fresh == old {
		t.Fatalf("a re-claim after a lapse is a NEW tenure and must bump the epoch (%d)", old)
	}

	_, err := s.Record(evidenced(a, "ws", "from the old process"), old, at(TTL+2*time.Minute))
	var fenced *ErrFenced
	if !errors.As(err, &fenced) {
		t.Fatalf("the same principal presenting a stale token must be FENCED (not waved through), got %v", err)
	}
}

// Zero is not "whatever I hold". It is an absence, and it is refused — because the
// agent that would use it is precisely the agent that does not know its tenure ended.
func TestEpochZeroIsRefusedOnEveryMutation(t *testing.T) {
	s := newStore(t)
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))
	seq := mustRecord(t, s, evidenced(a, "ws", "a thing"), ep, at(time.Minute)).Seq

	var noEpoch *ErrNoEpoch
	check := func(name string, err error) {
		t.Helper()
		if !errors.As(err, &noEpoch) {
			t.Fatalf("%s with epoch 0 must be refused with ErrNoEpoch, got %v", name, err)
		}
	}

	_, err := s.Record(evidenced(a, "ws", "x"), 0, at(2*time.Minute))
	check("Record", err)

	_, err = s.Decide(a, 0, "ws", "x", "why", nil, at(2*time.Minute))
	check("Decide", err)

	_, err = s.Attest(a, 0, Verification{TargetSeq: seq, Result: OutcomeSuccess, Method: "looked"}, nil, at(2*time.Minute))
	check("Attest", err)

	_, err = s.Transcript(a, 0, "ws", "x", strings.NewReader("hi"), at(2*time.Minute))
	check("Transcript", err)

	_, err = s.OpenWorkstream(a, 0, "ws2", "x", at(2*time.Minute))
	check("OpenWorkstream", err)

	_, err = s.UpdateWorkstream(a, 0, "ws", WorkstreamUpdate{Priority: P0}, at(2*time.Minute))
	check("UpdateWorkstream", err)

	_, err = s.CloseWorkstream(a, 0, "ws", "x", OutcomeSuccess, nil, at(2*time.Minute))
	check("CloseWorkstream", err)

	_, err = s.Checkpoint(a, 0, "x", at(2*time.Minute))
	check("Checkpoint", err)

	r, _ := s.Reconcile(context.Background(), at(2*time.Minute))
	_, err = s.RecordReconciliation(a, 0, r, at(2*time.Minute))
	check("RecordReconciliation", err)

	check("Heartbeat", s.Heartbeat(a, 0, at(2*time.Minute)))
	check("Release", s.Release(a, 0, "", at(2*time.Minute)))
}

// A stored entry never carries epoch 0, so replay refuses one that does: it was not
// written by this package, and an unfenced entry in the journal is exactly what a
// forger needs.
func TestReplayRefusesAnEntryWithEpochZero(t *testing.T) {
	s := newStore(t)
	ep := mustClaim(t, s, agent("a"), at(0))
	mustRecord(t, s, evidenced(agent("a"), "ws", "real"), ep, at(time.Minute))

	forged := `{"schema":"` + SchemaVersion + `","seq":3,"prev_hash":"x","id":"f","time":"2026-07-13T12:05:00Z",` +
		`"actor":{"name":"forger"},"epoch":0,"kind":"effect","summary":"unfenced","hash":"x"}` + "\n"
	f, err := os.OpenFile(s.journalPath(), os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(forged)
	f.Close()

	rep, _ := s.Replay()
	if !rep.Corrupt {
		t.Fatal("an entry with epoch 0 must not replay as valid")
	}
	if len(rep.Entries) != 2 {
		t.Fatalf("the valid prefix before the forgery must survive: got %d entries", len(rep.Entries))
	}
}

func TestNonHolderCannotWrite(t *testing.T) {
	s := newStore(t)
	ep := mustClaim(t, s, agent("holder"), at(0))

	_, err := s.Record(evidenced(agent("stranger"), "ws", "sneaking in"), ep, at(time.Minute))
	var notHolder *ErrNotHolder
	if !errors.As(err, &notHolder) {
		t.Fatalf("a bystander must not write the host's authoritative record, got %v", err)
	}
}

func TestWriteToVacantSeatIsRejected(t *testing.T) {
	s := newStore(t)
	_, err := s.Record(evidenced(agent("a"), "ws", "no seat"), 1, at(0))
	var notHolder *ErrNotHolder
	if !errors.As(err, &notHolder) || !notHolder.Vacant {
		t.Fatalf("writing on a vacant seat must be rejected, got %v", err)
	}
}

// The epoch ladder must not be climbable by anyone already on it: a generic write may
// not forge a seat event and mint itself an epoch.
func TestRecordCannotForgeASeatEvent(t *testing.T) {
	s := newStore(t)
	ep := mustClaim(t, s, agent("a"), at(0))

	for _, k := range []Kind{KindSeatClaimed, KindSeatTakeover, KindSeatReleased} {
		_, err := s.Record(Entry{Actor: agent("a"), Kind: k, Summary: "forged"}, ep, at(time.Minute))
		if err == nil {
			t.Fatalf("Record must refuse to forge a %s seat event", k)
		}
	}
}

func TestAppendRefusesAnUnknownKind(t *testing.T) {
	s := newStore(t)
	ep := mustClaim(t, s, agent("a"), at(0))
	_, err := s.Record(Entry{Actor: agent("a"), Kind: Kind("whatever"), Summary: "x"}, ep, at(time.Minute))
	if err == nil || !strings.Contains(err.Error(), "unknown kind") {
		t.Fatalf("an entry of a kind no projection understands must be refused, got %v", err)
	}
}

// ─── takeover & authorization ─────────────────────────────────────────────────

func TestTakeoverRequiresACapability(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("incumbent"), at(0))

	_, err := s.Takeover(agent("usurper"), TakeoverRequest{Interactive: true}, at(time.Minute))
	var unauth *ErrUnauthorized
	if !errors.As(err, &unauth) {
		t.Fatalf("seizing a live seat with no capability must be refused, got %v", err)
	}
}

func TestTakeoverSeizesALiveSeatAndRecordsItsAuthority(t *testing.T) {
	s := newStore(t)
	oldEpoch := mustClaim(t, s, agent("incumbent"), at(0))

	newEpoch := mustTakeover(t, s, agent("successor"), at(time.Minute))
	if newEpoch <= oldEpoch {
		t.Fatalf("a takeover must bump the epoch: %d → %d", oldEpoch, newEpoch)
	}

	v, _ := s.Status(at(2 * time.Minute))
	if !SameHolder(v.Authority.Holder, agent("successor")) {
		t.Fatalf("the seat must have changed hands, holder = %s", holderName(v.Authority.Holder))
	}
	if v.Authority.TakenOverFrom == nil || v.Authority.TakenOverFrom.Name != "incumbent" {
		t.Fatal("the record must say WHO was fenced — an unexplained seizure is indistinguishable from a hijack")
	}
	a := v.Authority.Authz
	if a == nil || a.Actor != "qiangli" || a.Provenance != ProvenanceOperatorAssertion {
		t.Fatalf("the record must carry the capability the seizure was performed under, got %+v", a)
	}

	// And the incumbent, who was never asked, is fenced from that instant.
	_, err := s.Record(evidenced(agent("incumbent"), "ws", "still working"), oldEpoch, at(3*time.Minute))
	var fenced *ErrFenced
	if !errors.As(err, &fenced) {
		t.Fatalf("the seized incumbent must be fenced, got %v", err)
	}
}

// A grant is SINGLE-USE, and the journal — not the grant file — is what makes it so.
func TestGrantCannotBeReplayed(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("a"), at(0))

	g := mustGrant(t, s, agent("b"), at(time.Minute))
	if _, err := s.Takeover(agent("b"), TakeoverRequest{GrantID: g.ID, Interactive: true}, at(2*time.Minute)); err != nil {
		t.Fatalf("first use of a grant must work: %v", err)
	}

	// Restore the grant file, exactly as a backup or a `cp` would. The journal still
	// remembers the takeover that consumed the nonce.
	if err := writeJSONAtomic(filepath.Join(s.grantDir(), g.ID+".json"), g); err != nil {
		t.Fatal(err)
	}
	_, err := s.Takeover(agent("b"), TakeoverRequest{GrantID: g.ID, Interactive: true}, at(3*time.Minute))
	var unauth *ErrUnauthorized
	if !errors.As(err, &unauth) || !strings.Contains(err.Error(), "already been used") {
		t.Fatalf("a replayed grant must be refused even when its file is restored, got %v", err)
	}
}

func TestNoninteractiveTakeoverRequiresAnExternalReceipt(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("a"), at(0))
	g := mustGrant(t, s, agent("b"), at(time.Minute)) // operator-assertion

	_, err := s.Takeover(agent("b"), TakeoverRequest{GrantID: g.ID, Interactive: false}, at(2*time.Minute))
	var unauth *ErrUnauthorized
	if !errors.As(err, &unauth) {
		t.Fatalf("an UNATTENDED takeover on an operator ASSERTION must be refused: with nobody present, "+
			"'a human approved' is a sentence with no author. got %v", err)
	}

	// With a receipt — an artifact somebody can go and audit — it is allowed.
	approval := filepath.Join(t.TempDir(), "approval.txt")
	if err := os.WriteFile(approval, []byte("approved by oncall, ticket OPS-42"), 0o600); err != nil {
		t.Fatal(err)
	}
	g2, err := s.Authorize(GrantRequest{
		Grantee: agent("b"), Actor: "oncall", Reason: "wedged",
		ReceiptPath: approval, ReceiptIssuer: "ops:OPS-42",
	}, at(2*time.Minute))
	if err != nil {
		t.Fatalf("Authorize with a receipt: %v", err)
	}
	if g2.Provenance != ProvenanceExternalReceipt {
		t.Fatalf("a receipt-backed grant must be labelled as such, got %q", g2.Provenance)
	}
	if _, err := s.Takeover(agent("b"), TakeoverRequest{GrantID: g2.ID, Interactive: false}, at(3*time.Minute)); err != nil {
		t.Fatalf("an unattended takeover backed by a receipt must be allowed: %v", err)
	}
}

// The provenance is recorded HONESTLY. An operator assertion is not a proof of
// humanity, and the journal says "assertion", not "authorized by a human".
func TestOperatorAssertionIsLabelledAsAnAssertion(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("a"), at(0))
	mustTakeover(t, s, agent("b"), at(time.Minute))

	changes, _, _, err := s.History()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range changes {
		if c.Kind != KindSeatTakeover {
			continue
		}
		if c.Authz == nil {
			t.Fatal("a takeover must carry its authorization in the history")
		}
		if c.Authz.Provenance != ProvenanceOperatorAssertion {
			t.Fatalf("provenance must be recorded verbatim, got %q", c.Authz.Provenance)
		}
		if c.Authz.Receipt != nil {
			t.Fatal("an operator assertion has no receipt, and must not claim one")
		}
		return
	}
	t.Fatal("no takeover found in the history")
}

// ─── evidence: reference vs verification ──────────────────────────────────────

// The single most load-bearing rule. An agent writes fluent, confident prose about
// work it did not do; the only defense that scales is to refuse to promote an
// unevidenced claim into a fact.
func TestUnevidencedSuccessProjectsAsUnknown(t *testing.T) {
	s := newStore(t)
	ep := mustClaim(t, s, agent("a"), at(0))
	e := mustRecord(t, s, Entry{
		Actor: agent("a"), Kind: KindEffect, Workstream: "api",
		Summary: "shipped it 🎉", Outcome: OutcomeSuccess, // …and nothing to point at
	}, ep, at(time.Minute))

	if e.Outcome != OutcomeSuccess {
		t.Fatal("the CLAIM must be recorded faithfully — this is an honest record of what was asserted")
	}
	if e.EffectiveOutcome() != OutcomeUnknown {
		t.Fatalf("but no view may BELIEVE it: effective outcome = %q, want unknown", e.EffectiveOutcome())
	}

	board, _, _ := s.Board()
	ws := board.Workstreams[0]
	if ws.Outcome != OutcomeUnknown || ws.Confidence != ConfidenceUnknown {
		t.Fatalf("the board must show unknown/unknown, got %s/%s", ws.Outcome, ws.Confidence)
	}
	if !board.Degraded {
		t.Fatal("the board's headline must not hide an unestablished outcome")
	}
}

// The rule the earlier revision was missing. A reference is a POINTER, not a check —
// and it is exactly as easy for a model to fabricate as the prose it accompanies.
func TestEvidencedSuccessIsAssertedNotVerified(t *testing.T) {
	s := newStore(t)
	ep := mustClaim(t, s, agent("a"), at(0))
	mustRecord(t, s, evidenced(agent("a"), "api", "migrated the schema"), ep, at(time.Minute))

	board, _, _ := s.Board()
	ws := board.Workstreams[0]
	if ws.Outcome != OutcomeSuccess {
		t.Fatalf("an evidenced success keeps its outcome, got %q", ws.Outcome)
	}
	if ws.Confidence != ConfidenceAsserted {
		t.Fatalf("a claim NOBODY CHECKED is asserted, never verified: got %q. "+
			"'command:go test ./...' records that an agent SAYS it ran the tests", ws.Confidence)
	}
	if board.Asserted != 1 {
		t.Fatalf("the board headline must count unverified claims, got %d", board.Asserted)
	}
}

// …and this is the only thing that closes the gap.
func TestVerificationPromotesAClaimToVerified(t *testing.T) {
	s := newStore(t)
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))
	target := mustRecord(t, s, evidenced(a, "api", "migrated the schema"), ep, at(time.Minute))

	if _, err := s.Attest(a, ep, Verification{
		TargetSeq: target.Seq, TargetHash: target.Hash,
		Result: OutcomeSuccess, Method: "re-ran the suite on a clean checkout",
	}, []Evidence{{Kind: "command", Ref: "go test ./...", Digest: "sha256:" + strings.Repeat("a", 64)}}, at(2*time.Minute)); err != nil {
		t.Fatalf("Attest: %v", err)
	}

	board, _, _ := s.Board()
	ws := board.Workstreams[0]
	if ws.Confidence != ConfidenceVerified {
		t.Fatalf("an attested claim is verified, got %q", ws.Confidence)
	}
	if board.Asserted != 0 {
		t.Fatalf("a verified claim is no longer merely asserted, got %d asserted", board.Asserted)
	}
}

// A verification can move a claim BACKWARDS. Degradation travels one way.
func TestVerificationCanRefuteAClaim(t *testing.T) {
	s := newStore(t)
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))
	target := mustRecord(t, s, evidenced(a, "api", "shipped"), ep, at(time.Minute))

	if _, err := s.Attest(a, ep, Verification{
		TargetSeq: target.Seq, TargetHash: target.Hash,
		Result: OutcomeFailed, Method: "the endpoint 502s",
	}, nil, at(2*time.Minute)); err != nil {
		t.Fatalf("Attest: %v", err)
	}

	board, _, _ := s.Board()
	ws := board.Workstreams[0]
	if ws.Confidence != ConfidenceRefuted || ws.Outcome != OutcomeFailed {
		t.Fatalf("a refuted success must degrade to failed/refuted, got %s/%s", ws.Outcome, ws.Confidence)
	}
}

// An attestation names the exact BYTES it vouched for, or it is an attestation of
// whatever ends up at that sequence number.
func TestVerificationBindsToTheTargetHash(t *testing.T) {
	s := newStore(t)
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))
	target := mustRecord(t, s, evidenced(a, "api", "shipped"), ep, at(time.Minute))

	_, err := s.Attest(a, ep, Verification{
		TargetSeq: target.Seq, TargetHash: "sha256:" + strings.Repeat("f", 64),
		Result: OutcomeSuccess, Method: "trust me",
	}, nil, at(2*time.Minute))
	if err == nil || !strings.Contains(err.Error(), "exact bytes") {
		t.Fatalf("attesting to a hash that is not what is at that seq must be refused, got %v", err)
	}
}

func TestVerificationRequiresAMethod(t *testing.T) {
	s := newStore(t)
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))
	target := mustRecord(t, s, evidenced(a, "api", "shipped"), ep, at(time.Minute))

	_, err := s.Attest(a, ep, Verification{TargetSeq: target.Seq, Result: OutcomeSuccess}, nil, at(2*time.Minute))
	if err == nil {
		t.Fatal("an unexplained 'I verified it' is the same trust-me claim it is supposed to replace")
	}
}

func TestFailureWithoutEvidenceStaysFailure(t *testing.T) {
	s := newStore(t)
	ep := mustClaim(t, s, agent("a"), at(0))
	e := mustRecord(t, s, Entry{
		Actor: agent("a"), Kind: KindEffect, Workstream: "api",
		Summary: "the migration blew up", Outcome: OutcomeFailed,
	}, ep, at(time.Minute))

	if e.EffectiveOutcome() != OutcomeFailed {
		t.Fatalf("degradation travels ONE WAY — an unevidenced failure is still a failure, got %q", e.EffectiveOutcome())
	}
}

func TestClosedWithoutEvidenceIsClosedAndUnknown(t *testing.T) {
	s := newStore(t)
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))
	if _, err := s.OpenWorkstream(a, ep, "api", "the api migration", at(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CloseWorkstream(a, ep, "api", "all done!", OutcomeSuccess, nil, at(2*time.Minute)); err != nil {
		t.Fatal(err)
	}

	board, _, _ := s.Board()
	ws := board.Workstreams[0]
	if ws.State != WorkstreamClosed {
		t.Fatalf("state must be closed, got %q", ws.State)
	}
	if ws.Outcome != OutcomeUnknown {
		t.Fatalf("'closed' and 'verified done' are different facts: outcome = %q", ws.Outcome)
	}
	if ws.Lane != LaneDone {
		t.Fatalf("a closed strand sits in the done lane, got %q", ws.Lane)
	}
}

// ─── the Kanban board ─────────────────────────────────────────────────────────

func TestBoardIsAKanbanDerivedPurelyFromTheJournal(t *testing.T) {
	s := newStore(t)
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))

	if _, err := s.OpenWorkstream(a, ep, "api", "the api migration", at(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateWorkstream(a, ep, "api", WorkstreamUpdate{
		Lane: LaneInProgress, Priority: P0, Owner: "qiangli",
		Agents: []string{"claude-opus"}, NextAction: "run the race gate",
		NextAt: at(time.Hour).Format(time.RFC3339),
		Links:  []Link{{Kind: "issue", Ref: "88"}, {Kind: "weave", Ref: "run-88"}},
	}, at(2*time.Minute)); err != nil {
		t.Fatal(err)
	}

	board, _, _ := s.Board()
	ws := board.Workstreams[0]
	if ws.Lane != LaneInProgress || ws.Priority != P0 || ws.Owner != "qiangli" {
		t.Fatalf("the Kanban fields must fold from the journal, got lane=%s pri=%s owner=%s", ws.Lane, ws.Priority, ws.Owner)
	}
	if ws.NextAction != "run the race gate" || ws.NextAt.IsZero() {
		t.Fatalf("next action/checkpoint must fold, got %q / %v", ws.NextAction, ws.NextAt)
	}
	if len(ws.Links) != 2 || len(ws.Agents) != 1 {
		t.Fatalf("links and agents must fold, got %v / %v", ws.Links, ws.Agents)
	}

	// A blocked strand shows as BLOCKED regardless of the lane anyone typed — a board
	// that lets you park a blocked item in "in-progress" hides the only thing worth
	// looking at.
	if _, err := s.UpdateWorkstream(a, ep, "api", WorkstreamUpdate{Blockers: []string{"waiting on review of #412"}}, at(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	board, _, _ = s.Board()
	if board.Workstreams[0].Lane != LaneBlocked || board.Blocked != 1 {
		t.Fatalf("live blockers must force the blocked lane, got %q (blocked=%d)", board.Workstreams[0].Lane, board.Blocked)
	}

	// And unblocking is as much of an event as blocking.
	if _, err := s.UpdateWorkstream(a, ep, "api", WorkstreamUpdate{Clear: []string{"blockers"}}, at(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	board, _, _ = s.Board()
	if board.Workstreams[0].Lane != LaneInProgress || board.Blocked != 0 {
		t.Fatalf("clearing the blockers must return the strand to its lane, got %q", board.Workstreams[0].Lane)
	}
}

func TestBoardRejectsAnInvalidLaneOrPriority(t *testing.T) {
	s := newStore(t)
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))
	if _, err := s.UpdateWorkstream(a, ep, "api", WorkstreamUpdate{Lane: "wherever"}, at(time.Minute)); err == nil {
		t.Fatal("an invented lane must be refused, not silently stored")
	}
	if _, err := s.UpdateWorkstream(a, ep, "api", WorkstreamUpdate{Priority: "urgent"}, at(time.Minute)); err == nil {
		t.Fatal("an invented priority must be refused")
	}
}

func TestAnEvidencedEntrySettlesAnEarlierUnknown(t *testing.T) {
	s := newStore(t)
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))
	mustRecord(t, s, Entry{Actor: a, Kind: KindEffect, Workstream: "api",
		Summary: "probably fine", Outcome: OutcomeSuccess}, ep, at(time.Minute))
	mustRecord(t, s, evidenced(a, "api", "actually verified the migration"), ep, at(2*time.Minute))

	board, _, _ := s.Board()
	ws := board.Workstreams[0]
	if ws.Outcome != OutcomeSuccess || ws.Confidence != ConfidenceAsserted {
		t.Fatalf("a later evidenced entry settles the earlier unknown, got %s/%s", ws.Outcome, ws.Confidence)
	}
	if board.Degraded {
		t.Fatal("an early unknown that a later entry settled is history, not a live problem")
	}
}

// ─── the journal as the only authority ────────────────────────────────────────

// The whole recovery story: seat.json is a cache, and the journal survives without it.
func TestAuthoritySurvivesLosingTheLivenessRecord(t *testing.T) {
	s := newStore(t)
	ep := mustClaim(t, s, agent("crashed"), at(0))
	mustRecord(t, s, evidenced(agent("crashed"), "api", "did real work"), ep, at(time.Minute))

	// The heartbeat file is gone — a crash, a cleanup script, a hostile `rm`.
	if err := os.Remove(s.seatPath()); err != nil {
		t.Fatal(err)
	}

	v, err := s.Status(at(2 * time.Minute))
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	// AUTHORITY survives — replayed from the journal.
	if v.Authority.Vacant || !SameHolder(v.Authority.Holder, agent("crashed")) || v.Authority.Epoch != ep {
		t.Fatalf("authority must replay from the journal without seat.json, got %+v", v.Authority)
	}
	// LIVENESS does not, and it is honest about it.
	if v.Liveness != LivenessUnknown {
		t.Fatalf("with no heartbeat record, liveness is UNKNOWN, got %q", v.Liveness)
	}
	// And the holder can restore it — the journal still says the seat is theirs.
	if err := s.Heartbeat(agent("crashed"), ep, at(3*time.Minute)); err != nil {
		t.Fatalf("the holder must be able to rebuild the liveness record from the journal: %v", err)
	}
	v, _ = s.Status(at(4 * time.Minute))
	if v.Liveness != LivenessLive {
		t.Fatalf("after heartbeating, liveness is live again, got %q", v.Liveness)
	}
}

func TestSeatLifecycleTouchesNoRepository(t *testing.T) {
	// A steward moves a MANDATE, not a checkout. If claiming the seat ever starts
	// touching a repository, `steward` and `handoff` have quietly become the same verb —
	// and the distinction is the whole reason both exist.
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	head := filepath.Join(repo, ".git", "HEAD")
	if err := os.WriteFile(head, []byte("ref: refs/heads/main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tracked := filepath.Join(repo, "file.txt")
	if err := os.WriteFile(tracked, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	before := snapshot(t, repo)

	s := newStore(t)
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))
	mustRecord(t, s, evidenced(a, "api", "worked in that repo"), ep, at(time.Minute))
	if err := s.Release(a, ep, "done", at(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	ep2 := mustClaim(t, s, agent("b"), at(3*time.Minute))
	if _, err := s.Checkpoint(agent("b"), ep2, "", at(4*time.Minute)); err != nil {
		t.Fatal(err)
	}

	if after := snapshot(t, repo); after != before {
		t.Fatalf("the seat lifecycle must not touch a repository.\nbefore: %s\nafter:  %s", before, after)
	}
}

func snapshot(t *testing.T, dir string) string {
	t.Helper()
	var sb strings.Builder
	err := filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		sb.WriteString(rel + "|")
		if !fi.IsDir() {
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			sb.WriteString(digestOf(b))
		}
		sb.WriteString("\n")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return sb.String()
}

func TestReplayDetectsAnAlteredEntry(t *testing.T) {
	s := newStore(t)
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))
	mustRecord(t, s, evidenced(a, "api", "the truth"), ep, at(time.Minute))
	mustRecord(t, s, evidenced(a, "api", "more truth"), ep, at(2*time.Minute))

	raw := journalBytes(t, s)
	tampered := strings.Replace(string(raw), "the truth", "a lie!!!!", 1)
	if err := os.WriteFile(s.journalPath(), []byte(tampered), 0o600); err != nil {
		t.Fatal(err)
	}

	rep, _ := s.Replay()
	if !rep.Corrupt {
		t.Fatal("an altered entry must break the chain — that is what the chain is FOR")
	}
	if rep.CorruptKind != CorruptInvalid {
		t.Fatalf("tampering is not a torn write: kind = %q", rep.CorruptKind)
	}
	if len(rep.Entries) != 1 {
		t.Fatalf("the valid prefix before the alteration survives: got %d entries", len(rep.Entries))
	}
}

func TestReplayChainsFromGenesis(t *testing.T) {
	s := newStore(t)
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))
	mustRecord(t, s, evidenced(a, "api", "one"), ep, at(time.Minute))

	rep, _ := s.Replay()
	if rep.Entries[0].PrevHash != genesis {
		t.Fatalf("the first entry must chain from the public genesis root, got %q", rep.Entries[0].PrevHash)
	}
	prev := genesis
	for i, e := range rep.Entries {
		if e.PrevHash != prev {
			t.Fatalf("entry %d does not link to its predecessor", i)
		}
		if e.computeHash(prev) != e.Hash {
			t.Fatalf("entry %d's hash does not verify", i)
		}
		prev = e.Hash
	}
}

func TestEvidenceOrderDoesNotAffectTheHash(t *testing.T) {
	e1 := Entry{Actor: agent("a"), Epoch: 1, Kind: KindEffect, Summary: "x", Evidence: []Evidence{
		{Kind: "command", Ref: "go test"}, {Kind: "commit", Ref: "abc"},
	}}
	e2 := Entry{Actor: agent("a"), Epoch: 1, Kind: KindEffect, Summary: "x", Evidence: []Evidence{
		{Kind: "commit", Ref: "abc"}, {Kind: "command", Ref: "go test"},
	}}
	sortEvidence(e1.Evidence)
	sortEvidence(e2.Evidence)
	if e1.computeHash(genesis) != e2.computeHash(genesis) {
		t.Fatal("the same evidence in a different flag order must hash identically")
	}
}

// ─── transcripts are optional by contract ─────────────────────────────────────

func TestTranscriptDeletionDoesNotAffectProjections(t *testing.T) {
	s := newStore(t)
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))
	if _, err := s.OpenWorkstream(a, ep, "api", "the migration", at(time.Minute)); err != nil {
		t.Fatal(err)
	}
	mustRecord(t, s, evidenced(a, "api", "did the thing"), ep, at(2*time.Minute))
	if _, err := s.Decide(a, ep, "api", "drop v1", "no callers in 90d", nil, at(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Transcript(a, ep, "api", "how we got here", strings.NewReader("a long conversation"), at(4*time.Minute)); err != nil {
		t.Fatal(err)
	}

	boardBefore, _, _ := s.Board()
	ckBefore := ProjectCheckpoint(mustReplay(t, s).Entries, 0)

	// Delete every artifact byte on the host.
	if err := os.RemoveAll(s.transcriptDir()); err != nil {
		t.Fatal(err)
	}

	boardAfter, _, _ := s.Board()
	ckAfter := ProjectCheckpoint(mustReplay(t, s).Entries, 0)

	if boardBefore.Digest != boardAfter.Digest {
		t.Fatal("deleting a transcript changed the board — transcripts are NOT authoritative, and this is the test that says so")
	}
	if ckBefore.Board.Digest != ckAfter.Board.Digest || ckBefore.JournalDigest != ckAfter.JournalDigest {
		t.Fatal("deleting a transcript changed a checkpoint projection")
	}
}

func mustReplay(t *testing.T, s *Store) *Replay {
	t.Helper()
	rep, err := s.Replay()
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func TestTamperedArtifactIsFlagged(t *testing.T) {
	s := newStore(t)
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))
	e, err := s.Transcript(a, ep, "api", "the conversation", strings.NewReader("original"), at(time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(s.dir, filepath.FromSlash(e.Artifact.Path))
	if err := os.WriteFile(path, []byte("REWRITTEN"), 0o600); err != nil {
		t.Fatal(err)
	}

	r, err := s.Reconcile(context.Background(), at(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.TamperedArtifacts) != 1 {
		t.Fatal("an artifact whose bytes no longer match its digest is a LIE, and must be flagged")
	}
	if r.Health != HealthUnknown {
		t.Fatalf("tampering degrades health to unknown, got %q", r.Health)
	}
	// An ABSENT artifact, by contrast, is merely a gap.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	r, _ = s.Reconcile(context.Background(), at(3*time.Minute))
	if len(r.MissingArtifacts) != 1 || len(r.TamperedArtifacts) != 0 {
		t.Fatal("a missing artifact is a gap, not a lie")
	}
	if r.Health == HealthUnknown {
		t.Fatal("a missing OPTIONAL artifact must not degrade health to unknown")
	}
}

// ─── reconcile ────────────────────────────────────────────────────────────────

// Reconcile must not claim it compared reality when it compared nothing.
func TestReconcileWithNoAdapterSaysItCheckedNothing(t *testing.T) {
	s := newStore(t)
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))
	mustRecord(t, s, evidenced(a, "api", "shipped"), ep, at(time.Minute))

	r, err := s.Reconcile(context.Background(), at(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if r.RealityCompared {
		t.Fatal("with no adapter, NOTHING was compared against reality — saying otherwise is the most dangerous lie in the system")
	}
	if !strings.Contains(r.RealityNote, "NOTHING was compared") {
		t.Fatalf("the report must say so in prose, got %q", r.RealityNote)
	}
	if len(r.Asserted) != 1 {
		t.Fatalf("an unchecked reference must be listed as asserted, got %d", len(r.Asserted))
	}
	if r.Health != HealthDegraded {
		t.Fatalf("a claim nobody checked is not a clean bill of health, got %q", r.Health)
	}
}

// stubObserver is a host-supplied adapter that actually went and looked.
type stubObserver struct {
	name string
	obs  []Observation
	err  error
}

func (o stubObserver) Name() string { return o.name }
func (o stubObserver) Observe(context.Context, []Entry) ([]Observation, error) {
	return o.obs, o.err
}

func TestReconcileWithAnAdapterReportsWhatItFound(t *testing.T) {
	s := newStore(t)
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))
	e := mustRecord(t, s, evidenced(a, "api", "shipped"), ep, at(time.Minute))

	r, err := s.Reconcile(context.Background(), at(2*time.Minute), stubObserver{
		name: "git",
		obs:  []Observation{{Seq: e.Seq, TargetHash: e.Hash, Result: OutcomeSuccess, Detail: "commit is on main"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !r.RealityCompared || len(r.Observations) != 1 {
		t.Fatal("an adapter that returned observations means reality WAS compared")
	}
	if r.Observations[0].Observer != "git" {
		t.Fatalf("the observation must name its adapter, got %q", r.Observations[0].Observer)
	}
	// …and an observation is still not a verification until it is RECORDED as one.
	if len(r.Asserted) != 1 {
		t.Fatal("an observation the steward has not journaled does not promote the claim")
	}
}

func TestReconcileReportsAFailedAdapterHonestly(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("a"), at(0))
	r, err := s.Reconcile(context.Background(), at(time.Minute), stubObserver{name: "ci", err: errors.New("network is down")})
	if err != nil {
		t.Fatal(err)
	}
	if r.RealityCompared {
		t.Fatal("an adapter that FAILED compared nothing")
	}
	if len(r.ObserverErrors) != 1 || !strings.Contains(r.RealityNote, "FAILED") {
		t.Fatalf("the failure must be surfaced, got %+v / %q", r.ObserverErrors, r.RealityNote)
	}
}

func TestReconcileReportsUnknownForADamagedJournal(t *testing.T) {
	s := newStore(t)
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))
	mustRecord(t, s, evidenced(a, "api", "real work"), ep, at(time.Minute))
	appendRaw(t, s, "{ not json at all")

	r, err := s.Reconcile(context.Background(), at(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if r.Health != HealthUnknown {
		t.Fatalf("a damaged RECORD is unknown, not merely degraded: got %q", r.Health)
	}
	if r.JournalEntries != 2 {
		t.Fatalf("the valid entries before the damage are still counted: got %d", r.JournalEntries)
	}
	if r.CorruptTail == "" {
		t.Fatal("the reconciliation must say where the record runs out")
	}
}

func TestRecordedReconciliationMirrorsItsVerdict(t *testing.T) {
	s := newStore(t)
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))
	mustRecord(t, s, Entry{Actor: a, Kind: KindEffect, Workstream: "api",
		Summary: "done!", Outcome: OutcomeSuccess}, ep, at(time.Minute))

	r, _ := s.Reconcile(context.Background(), at(2*time.Minute))
	e, err := s.RecordReconciliation(a, ep, r, at(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if e.Outcome != OutcomeDegraded {
		t.Fatalf("a reconciliation that found unproven claims may never be replayed as a success, got %q", e.Outcome)
	}
	if !strings.Contains(e.Summary, "reality NOT compared") {
		t.Fatalf("the recorded entry must say whether reality was actually checked, got %q", e.Summary)
	}
}

func TestReconcileIsOKOnlyWhenEverythingHasBeenChecked(t *testing.T) {
	s := newStore(t)
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))
	target := mustRecord(t, s, evidenced(a, "api", "shipped"), ep, at(time.Minute))

	r, _ := s.Reconcile(context.Background(), at(2*time.Minute))
	if r.Health != HealthDegraded {
		t.Fatalf("an unchecked claim is degraded, got %q", r.Health)
	}

	if _, err := s.Attest(a, ep, Verification{
		TargetSeq: target.Seq, TargetHash: target.Hash, Result: OutcomeSuccess, Method: "looked",
	}, nil, at(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	r, _ = s.Reconcile(context.Background(), at(4*time.Minute))
	if r.Health != HealthOK {
		t.Fatalf("once every claim is attested, health is ok: got %q (%d unproven, %d asserted)",
			r.Health, len(r.Unproven), len(r.Asserted))
	}
}

// ─── checkpoints ──────────────────────────────────────────────────────────────

func TestCheckpointProjectionIsPure(t *testing.T) {
	s := newStore(t)
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))
	mustRecord(t, s, evidenced(a, "api", "one"), ep, at(time.Minute))
	mustRecord(t, s, evidenced(a, "web", "two"), ep, at(2*time.Minute))
	entries := mustReplay(t, s).Entries

	first := ProjectCheckpoint(entries, 0)
	for range 5 {
		if got := ProjectCheckpoint(entries, 0); got.Board.Digest != first.Board.Digest {
			t.Fatal("the projection is not pure — same entries must always yield the same board")
		}
	}
}

func TestCheckpointIsReproducibleAndVerifiable(t *testing.T) {
	s := newStore(t)
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))
	mustRecord(t, s, evidenced(a, "api", "work"), ep, at(time.Minute))

	ck, err := s.Checkpoint(a, ep, "before the risky bit", at(2*time.Minute))
	if err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	v, err := s.VerifyCheckpoint(ck)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Reproducible {
		t.Fatalf("a fresh checkpoint must re-derive: %s", v.Reason)
	}

	// And the JOURNAL remembers it was taken, even if the file is deleted.
	if err := os.RemoveAll(s.checkpointDir()); err != nil {
		t.Fatal(err)
	}
	_, cks, _, err := s.History()
	if err != nil {
		t.Fatal(err)
	}
	if len(cks) != 1 || cks[0].ID != ck.ID {
		t.Fatal("the file is a cache; the journal entry is the memory")
	}
}

func TestCheckpointStopsVerifyingIfHistoryIsRewritten(t *testing.T) {
	s := newStore(t)
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))
	mustRecord(t, s, evidenced(a, "api", "work"), ep, at(time.Minute))
	ck, err := s.Checkpoint(a, ep, "", at(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	// Rewrite history beneath it.
	raw := string(journalBytes(t, s))
	raw = strings.Replace(raw, `"work"`, `"different work"`, 1)
	if err := os.WriteFile(s.journalPath(), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	v, err := s.VerifyCheckpoint(ck)
	if err != nil {
		t.Fatal(err)
	}
	if v.Reproducible {
		t.Fatal("a checkpoint over rewritten history must STOP verifying — that is the whole point of the receipt")
	}
}

func TestCheckpointCarriesUnprovenClaimsForward(t *testing.T) {
	s := newStore(t)
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))
	mustRecord(t, s, Entry{Actor: a, Kind: KindEffect, Workstream: "api",
		Summary: "done!", Outcome: OutcomeSuccess}, ep, at(time.Minute)) // unevidenced
	mustRecord(t, s, evidenced(a, "web", "shipped"), ep, at(2*time.Minute)) // asserted

	ck, err := s.Checkpoint(a, ep, "", at(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(ck.Unresolved) != 1 {
		t.Fatalf("a checkpoint that dropped its unknowns would look like a clean bill of health; got %v", ck.Unresolved)
	}
	if len(ck.Asserted) != 1 {
		t.Fatalf("it must also carry forward what was merely ASSERTED; got %v", ck.Asserted)
	}
}

// ─── follow / log / history ───────────────────────────────────────────────────

func TestFollowStreamsNewEntriesOnly(t *testing.T) {
	s := newStore(t)
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))
	mustRecord(t, s, evidenced(a, "api", "before the follow"), ep, at(time.Minute))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got := make(chan Entry, 8)
	done := make(chan error, 1)
	go func() {
		done <- s.Follow(ctx, Filter{}, 5*time.Millisecond, func(e Entry) error {
			got <- e
			return nil
		})
	}()

	time.Sleep(20 * time.Millisecond)
	mustRecord(t, s, evidenced(a, "api", "after the follow"), ep, at(2*time.Minute))

	select {
	case e := <-got:
		if e.Summary != "after the follow" {
			t.Fatalf("follow must start from the head, not replay the backlog; got %q", e.Summary)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("follow delivered nothing")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("a cancelled follow is how follow ENDS, not how it fails: %v", err)
	}
}

func TestDegradedOnlyFilterSurfacesTheUnproven(t *testing.T) {
	s := newStore(t)
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))
	mustRecord(t, s, evidenced(a, "api", "evidenced"), ep, at(time.Minute))
	mustRecord(t, s, Entry{Actor: a, Kind: KindEffect, Workstream: "api",
		Summary: "unevidenced", Outcome: OutcomeSuccess}, ep, at(2*time.Minute))

	entries, _, err := s.Log(Filter{DegradedOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Summary != "unevidenced" {
		t.Fatalf("--degraded is the 'what do I NOT know' query; got %d entries", len(entries))
	}
}

func TestHistoryReconstructsTheAuthorityLadder(t *testing.T) {
	s := newStore(t)
	e1 := mustClaim(t, s, agent("first"), at(0))
	if err := s.Release(agent("first"), e1, "handing off", at(time.Minute)); err != nil {
		t.Fatal(err)
	}
	mustClaim(t, s, agent("second"), at(2*time.Minute))
	mustTakeover(t, s, agent("third"), at(3*time.Minute))

	changes, _, _, err := s.History()
	if err != nil {
		t.Fatal(err)
	}
	want := []Kind{KindSeatClaimed, KindSeatReleased, KindSeatClaimed, KindSeatTakeover}
	if len(changes) != len(want) {
		t.Fatalf("expected %d seat events, got %d", len(want), len(changes))
	}
	for i, k := range want {
		if changes[i].Kind != k {
			t.Fatalf("event %d: want %s, got %s", i, k, changes[i].Kind)
		}
	}
	if changes[3].Authz == nil || changes[3].Authz.Actor != "qiangli" {
		t.Fatal("a takeover must record the capability it was performed under")
	}
	if changes[3].Epoch <= changes[2].Epoch {
		t.Fatal("the epoch ladder must be monotonic across the whole history")
	}
}

// ─── misc invariants ──────────────────────────────────────────────────────────

func TestEveryArtifactIsSchemaVersioned(t *testing.T) {
	s := newStore(t)
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))
	mustRecord(t, s, evidenced(a, "api", "x"), ep, at(time.Minute))
	ck, err := s.Checkpoint(a, ep, "", at(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	g := mustGrant(t, s, agent("b"), at(3*time.Minute))

	if ck.SchemaVersion != SchemaVersion || g.SchemaVersion != SchemaVersion {
		t.Fatal("every stored artifact carries the schema it was written under")
	}
	var seat Seat
	if found, err := readJSON(s.seatPath(), &seat); err != nil || !found {
		t.Fatal("seat file must exist")
	}
	if seat.SchemaVersion != SchemaVersion {
		t.Fatal("the seat cache must be schema-versioned — an unversioned cache cannot be validated")
	}
	for _, e := range mustReplay(t, s).Entries {
		if e.Schema != SchemaVersion {
			t.Fatalf("entry seq %d has schema %q", e.Seq, e.Schema)
		}
	}
}

func TestParseEvidence(t *testing.T) {
	cases := []struct {
		in         string
		kind, ref  string
		digest     string
		wantDigest bool
	}{
		{in: "command:go test ./...", kind: "command", ref: "go test ./..."},
		{in: "commit:de6485c", kind: "commit", ref: "de6485c"},
		{in: "file:/tmp/o.log#sha256:abc", kind: "file", ref: "/tmp/o.log", digest: "sha256:abc", wantDigest: true},
		{in: "just some prose", kind: "note", ref: "just some prose"},
		// An unknown prefix is NOT silently promoted to a kind — it stays a note, whole.
		{in: "unknownkind:x", kind: "note", ref: "unknownkind:x"},
	}
	for _, c := range cases {
		ev, err := ParseEvidence(c.in)
		if err != nil {
			t.Fatalf("ParseEvidence(%q): %v", c.in, err)
		}
		if ev.Kind != c.kind || ev.Ref != c.ref {
			t.Fatalf("ParseEvidence(%q) = %s:%s, want %s:%s", c.in, ev.Kind, ev.Ref, c.kind, c.ref)
		}
		if c.wantDigest && ev.Digest != c.digest {
			t.Fatalf("ParseEvidence(%q) digest = %q, want %q", c.in, ev.Digest, c.digest)
		}
		if c.wantDigest && !ev.DigestBound() {
			t.Fatalf("ParseEvidence(%q) must be digest-bound", c.in)
		}
	}
	if _, err := ParseEvidence("   "); err == nil {
		t.Fatal("empty evidence must be rejected")
	}
}

func TestSameHolderMatchesTheLogicalAgentNotThePID(t *testing.T) {
	a := principal.Ref{Name: "agent", Host: "h", Episode: "ep-1"}
	b := principal.Ref{Name: "agent", Host: "h", Episode: "ep-1"}
	if !SameHolder(a, b) {
		t.Fatal("one logical agent runs many processes; they are the same holder")
	}
	c := principal.Ref{Name: "other", Host: "h", Episode: "ep-2"}
	if SameHolder(a, c) {
		t.Fatal("different agents are different holders")
	}
}

// appendRaw writes raw bytes to the end of the journal, simulating damage.
func appendRaw(t *testing.T, s *Store, raw string) {
	t.Helper()
	f, err := os.OpenFile(s.journalPath(), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(raw); err != nil {
		t.Fatal(err)
	}
}
