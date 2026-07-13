// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package steward

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// These tests are written from the ATTACKER'S side of the table.
//
// Every one of them is a way to end up with two stewards, a forged seizure, a
// laundered claim, or a silently shortened journal — and every one of them is a thing
// the previous revision of this package actually allowed. The suite above proves the
// design works when everyone is honest and the machine does not lie. This one proves
// it holds when they are not and it does.

// ─── 1. a shared home must not merge seats across hosts ───────────────────────

// $HOME is not reliably one machine's. A network home, a synced home, or a container
// image with a baked-in home is mounted on several hosts at once — and a store keyed
// only by $HOME would silently merge the seats of every machine sharing it: two live
// stewards, one journal, one epoch ladder, and an endless mutual fencing war between
// agents that never had anything to do with each other.
func TestSharedHomeDoesNotMergeSeatsAcrossHosts(t *testing.T) {
	// Same home, same user, DIFFERENT machine — the network-home case.
	a := scopeFor("host-a", "qiangli")
	b := scopeFor("host-b", "qiangli")
	if a == b {
		t.Fatalf("two hosts sharing one home must NOT share one seat (both scoped to %q). Otherwise both "+
			"machines' agents claim the same journal, and every one of them fences the others forever.", a)
	}
	// Same machine, DIFFERENT account — also two stewards.
	if scopeFor("host-a", "someone-else") == a {
		t.Fatal("two users on one host must not share a seat")
	}
	// And the scoping is stable: the same host/user always resolves to the same seat,
	// or a steward would lose its own journal on restart.
	if scopeFor("host-a", "qiangli") != a {
		t.Fatal("the scope must be deterministic — an agent has to find its own seat again")
	}

	// The default store is that scope, under the home directory.
	home := t.TempDir()
	t.Setenv("BASHY_STEWARD_DIR", "")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // windows
	dir := DefaultDir()
	if want := filepath.Join(home, ".bashy", "steward", ScopeID()); dir != want {
		t.Fatalf("DefaultDir = %q, want %q", dir, want)
	}
	// The pre-scoping layout (~/.bashy/steward itself) is no longer the store — that is
	// the whole point, and the migration note in DefaultDir's doc says how to adopt one.
	if dir == filepath.Join(home, ".bashy", "steward") {
		t.Fatal("the unscoped path is exactly the bug")
	}
}

// The override is preserved — a host that wants to point the seat somewhere explicit
// (a migration, a test, a mounted volume) still can.
func TestStewardDirOverrideIsHonored(t *testing.T) {
	t.Setenv("BASHY_STEWARD_DIR", "/tmp/explicit-steward")
	if got := DefaultDir(); got != "/tmp/explicit-steward" {
		t.Fatalf("$BASHY_STEWARD_DIR must win verbatim, got %q", got)
	}
}

func TestDefaultDirIsIndependentOfWorkingDirectory(t *testing.T) {
	t.Setenv("BASHY_STEWARD_DIR", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	first := DefaultDir()
	sub := filepath.Join(t.TempDir(), "some", "repo")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(sub); err != nil {
		t.Fatal(err)
	}
	if DefaultDir() != first {
		t.Fatal("the seat is a property of the HOST, not of the checkout you happen to be standing in")
	}
}

// ─── 2. authority recovery must fail closed ───────────────────────────────────

// Each of these is a way to make the liveness record untrustworthy, and each one used
// to be a way to TAKE THE SEAT with no authorization at all. `rm seat.json` and the
// host is yours. That is the bug this table closes.
//
// Every case must land on LivenessUnknown — NOT claimable — and Claim must refuse.
func TestUntrustworthyHeartbeatIsUnknownAndNotClaimable(t *testing.T) {
	holder := agent("incumbent")

	cases := []struct {
		name    string
		corrupt func(t *testing.T, s *Store, auth Authority)
		wantIn  string
	}{
		{
			name:    "missing",
			corrupt: func(t *testing.T, s *Store, _ Authority) { os.Remove(s.seatPath()) },
			wantIn:  "no heartbeat record",
		},
		{
			name: "unparsable",
			corrupt: func(t *testing.T, s *Store, _ Authority) {
				os.WriteFile(s.seatPath(), []byte("{{{not json"), 0o600)
			},
			wantIn: "unreadable",
		},
		{
			name: "schema mismatch",
			corrupt: func(t *testing.T, s *Store, a Authority) {
				writeJSONAtomic(s.seatPath(), Seat{
					SchemaVersion: "bashy-steward-v99", Holder: a.Holder, Epoch: a.Epoch,
					AcquiredAt: a.AcquiredAt, Heartbeat: at(time.Minute),
				})
			},
			wantIn: "schema",
		},
		{
			name: "wrong holder",
			corrupt: func(t *testing.T, s *Store, a Authority) {
				writeJSONAtomic(s.seatPath(), Seat{
					SchemaVersion: SchemaVersion, Holder: agent("impostor"), Epoch: a.Epoch,
					AcquiredAt: a.AcquiredAt, Heartbeat: at(time.Minute),
				})
			},
			wantIn: "the journal says",
		},
		{
			name: "wrong epoch",
			corrupt: func(t *testing.T, s *Store, a Authority) {
				writeJSONAtomic(s.seatPath(), Seat{
					SchemaVersion: SchemaVersion, Holder: a.Holder, Epoch: a.Epoch + 7,
					AcquiredAt: a.AcquiredAt, Heartbeat: at(time.Minute),
				})
			},
			wantIn: "epoch",
		},
		{
			name: "heartbeat in the future",
			corrupt: func(t *testing.T, s *Store, a Authority) {
				writeJSONAtomic(s.seatPath(), Seat{
					SchemaVersion: SchemaVersion, Holder: a.Holder, Epoch: a.Epoch,
					AcquiredAt: a.AcquiredAt, Heartbeat: at(48 * time.Hour),
				})
			},
			wantIn: "future",
		},
		{
			name: "heartbeat predating its own tenure",
			corrupt: func(t *testing.T, s *Store, a Authority) {
				writeJSONAtomic(s.seatPath(), Seat{
					SchemaVersion: SchemaVersion, Holder: a.Holder, Epoch: a.Epoch,
					AcquiredAt: a.AcquiredAt, Heartbeat: at(-24 * time.Hour),
				})
			},
			wantIn: "predates",
		},
		{
			name: "zero heartbeat",
			corrupt: func(t *testing.T, s *Store, a Authority) {
				writeJSONAtomic(s.seatPath(), Seat{
					SchemaVersion: SchemaVersion, Holder: a.Holder, Epoch: a.Epoch, AcquiredAt: a.AcquiredAt,
				})
			},
			wantIn: "no heartbeat",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newStore(t)
			ep := mustClaim(t, s, holder, at(0))
			auth := deriveAuthority(mustReplay(t, s))
			c.corrupt(t, s, auth)

			now := at(2 * time.Minute)
			v, err := s.Status(now)
			if err != nil {
				t.Fatalf("Status: %v", err)
			}
			if v.Liveness != LivenessUnknown {
				t.Fatalf("liveness must be UNKNOWN, got %q", v.Liveness)
			}
			if v.Claimable {
				t.Fatal("UNKNOWN IS NOT CLAIMABLE. 'I cannot trust the record' is a fact about the record, " +
					"not about the holder — and every way of producing it is also a way of producing it deliberately")
			}
			if !strings.Contains(v.LivenessReason, c.wantIn) {
				t.Fatalf("the reason must say what is wrong: got %q, want something mentioning %q", v.LivenessReason, c.wantIn)
			}
			// Authority is untouched: the journal still knows who holds the seat.
			if v.Authority.Vacant || !SameHolder(v.Authority.Holder, holder) || v.Authority.Epoch != ep {
				t.Fatalf("authority must survive a bad cache, got %+v", v.Authority)
			}

			// And an ordinary Claim is REFUSED.
			_, err = s.Claim(agent("usurper"), "", now)
			var unknown *ErrLivenessUnknown
			if !errors.As(err, &unknown) {
				t.Fatalf("Claim on an untrustworthy seat must fail with ErrLivenessUnknown, got %v", err)
			}

			// The only ways forward: the holder proves liveness…
			if err := s.Heartbeat(holder, ep, now); err != nil {
				t.Fatalf("the holder must be able to restore the record: %v", err)
			}
			if v, _ := s.Status(now); v.Liveness != LivenessLive {
				t.Fatalf("after the holder heartbeats, liveness is live, got %q", v.Liveness)
			}
		})
	}
}

// …or a successor takes it over, on the record, with a capability.
func TestTakeoverIsTheRecoveryPathFromAnUnknownSeat(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("incumbent"), at(0))
	if err := os.Remove(s.seatPath()); err != nil {
		t.Fatal(err)
	}

	newEpoch := mustTakeover(t, s, agent("successor"), at(time.Minute))
	v, _ := s.Status(at(2 * time.Minute))
	if !SameHolder(v.Authority.Holder, agent("successor")) || v.Authority.Epoch != newEpoch {
		t.Fatal("takeover must recover a seat whose liveness cannot be established")
	}
	if v.Authority.Authz == nil {
		t.Fatal("and it must leave a receipt: an unexplained seizure is indistinguishable from a hijack")
	}
}

// ─── 3. forged, replayed, expired, wrong-epoch, wrong-grantee capabilities ────

func TestTakeoverRefusesEveryBadCapability(t *testing.T) {
	grantee := agent("successor")

	cases := []struct {
		name   string
		mutate func(t *testing.T, s *Store, g *Grant, req *TakeoverRequest)
		wantIn string
	}{
		{
			name:   "no capability at all",
			mutate: func(_ *testing.T, _ *Store, _ *Grant, req *TakeoverRequest) { req.GrantID = "" },
			wantIn: "no authorization was presented",
		},
		{
			name:   "a capability that does not exist",
			mutate: func(_ *testing.T, _ *Store, _ *Grant, req *TakeoverRequest) { req.GrantID = "g-deadbeef" },
			wantIn: "no such authorization",
		},
		{
			name: "expired",
			mutate: func(t *testing.T, s *Store, g *Grant, _ *TakeoverRequest) {
				g.ExpiresAt = at(-time.Hour)
				rewriteGrant(t, s, *g)
			},
			wantIn: "expired",
		},
		{
			name: "dated into the future",
			mutate: func(t *testing.T, s *Store, g *Grant, _ *TakeoverRequest) {
				g.IssuedAt = at(24 * time.Hour)
				g.ExpiresAt = at(25 * time.Hour)
				rewriteGrant(t, s, *g)
			},
			wantIn: "not valid yet",
		},
		{
			name: "minted for a different agent",
			mutate: func(t *testing.T, s *Store, g *Grant, _ *TakeoverRequest) {
				g.Grantee = agent("somebody-else")
				rewriteGrant(t, s, *g)
			},
			wantIn: "was minted for",
		},
		{
			name: "minted against a different epoch",
			mutate: func(t *testing.T, s *Store, g *Grant, _ *TakeoverRequest) {
				g.FromEpoch = 99
				rewriteGrant(t, s, *g)
			},
			wantIn: "authorizes seizing epoch 99",
		},
		{
			name: "minted for a different seat",
			mutate: func(t *testing.T, s *Store, g *Grant, _ *TakeoverRequest) {
				g.Scope = "some-other-host-user-abc123"
				rewriteGrant(t, s, *g)
			},
			wantIn: "does not travel between hosts",
		},
		{
			name: "wrong action",
			mutate: func(t *testing.T, s *Store, g *Grant, _ *TakeoverRequest) {
				g.Action = "read"
				rewriteGrant(t, s, *g)
			},
			wantIn: "is for \"read\"",
		},
		{
			name: "wrong schema",
			mutate: func(t *testing.T, s *Store, g *Grant, _ *TakeoverRequest) {
				g.SchemaVersion = "bashy-steward-v99"
				rewriteGrant(t, s, *g)
			},
			wantIn: "schema",
		},
		{
			name: "no nonce, so its single use cannot be tracked",
			mutate: func(t *testing.T, s *Store, g *Grant, req *TakeoverRequest) {
				g.ID = ""
				// A grant with no id cannot live under an id, so present it as a file.
				p := filepath.Join(t.TempDir(), "g.json")
				if err := writeJSONAtomic(p, *g); err != nil {
					t.Fatal(err)
				}
				req.GrantID, req.GrantPath = "", p
			},
			wantIn: "carries no nonce",
		},
		{
			name: "unknown provenance",
			mutate: func(t *testing.T, s *Store, g *Grant, _ *TakeoverRequest) {
				g.Provenance = "vibes"
				rewriteGrant(t, s, *g)
			},
			wantIn: "unknown provenance",
		},
		{
			name: "claims a receipt it does not carry",
			mutate: func(t *testing.T, s *Store, g *Grant, _ *TakeoverRequest) {
				g.Provenance = ProvenanceExternalReceipt
				g.Receipt = nil
				rewriteGrant(t, s, *g)
			},
			wantIn: "claims an external receipt but carries none",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newStore(t)
			incumbentEpoch := mustClaim(t, s, agent("incumbent"), at(0))

			g := mustGrant(t, s, grantee, at(time.Minute))
			req := TakeoverRequest{GrantID: g.ID, Interactive: true}
			c.mutate(t, s, &g, &req)

			_, err := s.Takeover(grantee, req, at(2*time.Minute))
			var unauth *ErrUnauthorized
			if !errors.As(err, &unauth) {
				t.Fatalf("the takeover must be REFUSED, got %v", err)
			}
			if !strings.Contains(err.Error(), c.wantIn) {
				t.Fatalf("error must explain what was wrong (%q), got: %v", c.wantIn, err)
			}
			// The incumbent still holds the seat, at the same epoch. Nothing moved.
			v, _ := s.Status(at(3 * time.Minute))
			if !SameHolder(v.Authority.Holder, agent("incumbent")) || v.Authority.Epoch != incumbentEpoch {
				t.Fatalf("a refused takeover must change NOTHING, got %+v", v.Authority)
			}
		})
	}
}

// A receipt whose bytes were edited after the fact is worse than no receipt.
func TestTamperedReceiptIsRefused(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("a"), at(0))

	approval := filepath.Join(t.TempDir(), "approval.txt")
	if err := os.WriteFile(approval, []byte("approved: emergency only"), 0o600); err != nil {
		t.Fatal(err)
	}
	g, err := s.Authorize(GrantRequest{
		Grantee: agent("b"), Actor: "oncall", ReceiptPath: approval, ReceiptIssuer: "ops",
	}, at(time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	// Rewrite the stored artifact — the thing a later auditor would go and read.
	stored := filepath.Join(s.dir, filepath.FromSlash(g.Receipt.Path))
	if err := os.WriteFile(stored, []byte("approved: anything, forever"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = s.Takeover(agent("b"), TakeoverRequest{GrantID: g.ID}, at(2*time.Minute))
	if err == nil || !strings.Contains(err.Error(), "altered") {
		t.Fatalf("a receipt whose bytes no longer match its digest must be refused, got %v", err)
	}

	// And so must a receipt whose bytes are simply GONE: the artifact that justified a
	// seizure has to be there to be audited.
	if err := os.Remove(stored); err != nil {
		t.Fatal(err)
	}
	_, err = s.Takeover(agent("b"), TakeoverRequest{GrantID: g.ID}, at(3*time.Minute))
	if err == nil || !strings.Contains(err.Error(), "bytes are gone") {
		t.Fatalf("a receipt with no bytes must be refused, got %v", err)
	}
}

func TestAuthorizeRefusesAnUnboundOrUnattributedCapability(t *testing.T) {
	s := newStore(t)
	mustClaim(t, s, agent("a"), at(0))

	if _, err := s.Authorize(GrantRequest{Actor: "qiangli", Confirmed: true}, at(time.Minute)); err == nil {
		t.Fatal("a capability with no grantee is a skeleton key — it must be refused")
	}
	if _, err := s.Authorize(GrantRequest{Grantee: agent("b"), Confirmed: true}, at(time.Minute)); err == nil {
		t.Fatal("a capability naming no operator must be refused")
	}
	if _, err := s.Authorize(GrantRequest{Grantee: agent("b"), Actor: "x"}, at(time.Minute)); err == nil {
		t.Fatal("with no interactive confirmation and no receipt, there is nobody asserting anything — refuse")
	}
	if _, err := s.Authorize(GrantRequest{
		Grantee: agent("b"), Actor: "x", Confirmed: true, TTL: 30 * 24 * time.Hour,
	}, at(time.Minute)); err == nil {
		t.Fatal("a capability that outlives the situation that justified it is a backdoor with a nice name")
	}
}

func rewriteGrant(t *testing.T, s *Store, g Grant) {
	t.Helper()
	if err := writeJSONAtomic(filepath.Join(s.grantDir(), g.ID+".json"), g); err != nil {
		t.Fatal(err)
	}
}

// ─── 5. every journal append goes through the authority gate ──────────────────

// Checkpoint used to accept ANY actor — reasoning that a checkpoint is only a cache.
// But it appends to the journal (so a bystander could grow the host's authoritative
// record at will) and it drops a file a later reader trusts to summarize what happened.
func TestNonHolderCannotCheckpoint(t *testing.T) {
	s := newStore(t)
	ep := mustClaim(t, s, agent("holder"), at(0))

	_, err := s.Checkpoint(agent("stranger"), ep, "sneaking in", at(time.Minute))
	var notHolder *ErrNotHolder
	if !errors.As(err, &notHolder) {
		t.Fatalf("a bystander must not checkpoint, got %v", err)
	}

	// Not even a file was left behind.
	if des, err := os.ReadDir(s.checkpointDir()); err == nil && len(des) > 0 {
		t.Fatal("a refused checkpoint must not leave a file a human might find and believe")
	}

	// And a fenced holder is refused too.
	_, err = s.Checkpoint(agent("holder"), ep+1, "", at(2*time.Minute))
	var fenced *ErrFenced
	if !errors.As(err, &fenced) {
		t.Fatalf("checkpointing at a stale epoch must be fenced, got %v", err)
	}
}

func TestNonHolderCannotRepair(t *testing.T) {
	s := newStore(t)
	ep := mustClaim(t, s, agent("holder"), at(0))
	mustRecord(t, s, evidenced(agent("holder"), "api", "real work"), ep, at(time.Minute))
	appendRaw(t, s, `{"schema":"bashy-steward-v1","seq":3,"prev`) // a torn final append

	before := journalBytes(t, s)

	_, err := s.Repair(agent("stranger"), ep, at(2*time.Minute))
	var notHolder *ErrNotHolder
	if !errors.As(err, &notHolder) {
		t.Fatalf("a damaged journal is not a licence for a stranger to truncate the host's record, got %v", err)
	}

	_, err = s.Repair(agent("holder"), ep+1, at(2*time.Minute))
	var fenced *ErrFenced
	if !errors.As(err, &fenced) {
		t.Fatalf("a FENCED holder must not repair either, got %v", err)
	}

	if string(journalBytes(t, s)) != string(before) {
		t.Fatal("a refused repair must not have touched a single byte")
	}
}

// ─── 7. repair: only a genuinely torn final append ────────────────────────────

func TestRepairTruncatesOnlyATornFinalAppendAndQuarantinesIt(t *testing.T) {
	s := newStore(t)
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))
	mustRecord(t, s, evidenced(a, "api", "work that must survive"), ep, at(time.Minute))

	// The crash: a partial line, no trailing newline.
	torn := `{"schema":"bashy-steward-v1","seq":3,"prev_hash":"sha256:ab`
	appendRaw(t, s, torn)

	// Reads still work, and the valid prefix is intact — a torn tail never hides the
	// history before it.
	rep := mustReplay(t, s)
	if !rep.Corrupt || len(rep.Entries) != 2 {
		t.Fatalf("the valid prefix must survive the tear: corrupt=%v entries=%d", rep.Corrupt, len(rep.Entries))
	}

	plan, err := s.PlanRepair()
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Repairable || plan.Kind != CorruptTornAppend {
		t.Fatalf("a torn final append IS repairable: %+v", plan)
	}

	res, err := s.Repair(a, ep, at(2*time.Minute))
	if err != nil {
		t.Fatalf("Repair: %v", err)
	}
	if res.Discarded != int64(len(torn)) {
		t.Fatalf("exactly the torn bytes are discarded: %d, want %d", res.Discarded, len(torn))
	}

	// QUARANTINED — the exact bytes, by digest. "The tool ate it" is not an answer to
	// "what was in those bytes?".
	qbytes, err := os.ReadFile(filepath.Join(s.dir, filepath.FromSlash(res.QuarantinePath)))
	if err != nil {
		t.Fatalf("the discarded bytes must be quarantined: %v", err)
	}
	if string(qbytes) != torn {
		t.Fatalf("the quarantine must hold EXACTLY what was discarded, got %q", qbytes)
	}
	if digestOf(qbytes) != res.SuffixDigest {
		t.Fatal("the quarantined bytes must match the digest in the receipt")
	}

	// RECEIPTED — degraded, under the holder's epoch, naming the quarantine.
	if res.Receipt.Kind != KindRepair || res.Receipt.Outcome != OutcomeDegraded {
		t.Fatalf("a repair is never a clean success: %s/%s", res.Receipt.Kind, res.Receipt.Outcome)
	}
	if res.Receipt.Epoch != ep {
		t.Fatalf("the receipt is written under the holder's epoch, got %d", res.Receipt.Epoch)
	}
	found := false
	for _, e := range res.Receipt.Evidence {
		if e.Kind == "quarantine" && e.Digest == res.SuffixDigest {
			found = true
		}
	}
	if !found {
		t.Fatal("the receipt must point at the quarantined bytes, by digest")
	}

	// And the journal is whole again, with nothing lost but the torn fragment.
	rep = mustReplay(t, s)
	if rep.Corrupt {
		t.Fatal("the journal must replay cleanly after the repair")
	}
	if len(rep.Entries) != 3 { // claim + effect + repair receipt
		t.Fatalf("expected 3 entries after the repair, got %d", len(rep.Entries))
	}
	if rep.Entries[1].Summary != "work that must survive" {
		t.Fatal("a valid entry can never be removed by a repair")
	}
}

// The two refusals that keep `repair` from being a data-loss tool with a nice name.
func TestRepairFailsClosedOnAnythingButATornAppend(t *testing.T) {
	t.Run("mid-log damage with valid records after it", func(t *testing.T) {
		s := newStore(t)
		a := agent("a")
		ep := mustClaim(t, s, a, at(0))
		mustRecord(t, s, evidenced(a, "api", "one"), ep, at(time.Minute))
		mustRecord(t, s, evidenced(a, "api", "two"), ep, at(2*time.Minute))

		// Corrupt a line in the MIDDLE. Everything after it was completely written.
		lines := strings.SplitAfter(string(journalBytes(t, s)), "\n")
		lines[1] = "{ garbage in the middle\n"
		if err := os.WriteFile(s.journalPath(), []byte(strings.Join(lines, "")), 0o600); err != nil {
			t.Fatal(err)
		}

		plan, err := s.PlanRepair()
		if err != nil {
			t.Fatal(err)
		}
		if plan.Repairable {
			t.Fatal("truncating from mid-log damage would discard records that were COMPLETELY WRITTEN")
		}
		if !strings.Contains(plan.Reason, "MIDDLE") {
			t.Fatalf("the refusal must say why: %q", plan.Reason)
		}

		before := journalBytes(t, s)
		_, err = s.Repair(a, ep, at(3*time.Minute))
		var notRepairable *ErrNotRepairable
		if !errors.As(err, &notRepairable) {
			t.Fatalf("Repair must FAIL CLOSED, got %v", err)
		}
		if string(journalBytes(t, s)) != string(before) {
			t.Fatal("a refused repair must not truncate a single byte")
		}
	})

	t.Run("a complete record that does not chain", func(t *testing.T) {
		s := newStore(t)
		a := agent("a")
		ep := mustClaim(t, s, a, at(0))
		e := mustRecord(t, s, evidenced(a, "api", "one"), ep, at(time.Minute))

		// A fully-formed entry whose hash does not verify. That is not a torn write — it
		// is TAMPERING, and a tool that silently truncated it would delete the evidence
		// and call it a repair.
		forged := e
		forged.Seq = 3
		forged.PrevHash = e.Hash
		forged.Summary = "planted"
		forged.Hash = "sha256:" + strings.Repeat("0", 64)
		appendEntryRaw(t, s, forged)

		plan, err := s.PlanRepair()
		if err != nil {
			t.Fatal(err)
		}
		if plan.Repairable {
			t.Fatal("a COMPLETE record that does not chain is evidence of tampering, and must never be auto-truncated")
		}

		before := journalBytes(t, s)
		_, err = s.Repair(a, ep, at(2*time.Minute))
		var notRepairable *ErrNotRepairable
		if !errors.As(err, &notRepairable) {
			t.Fatalf("Repair must fail closed on tampering, got %v", err)
		}
		if string(journalBytes(t, s)) != string(before) {
			t.Fatal("the evidence must still be there")
		}
	})

	t.Run("a corrupt suffix that still contains a later valid record", func(t *testing.T) {
		s := newStore(t)
		a := agent("a")
		ep := mustClaim(t, s, a, at(0))
		e := mustRecord(t, s, evidenced(a, "api", "one"), ep, at(time.Minute))

		// Garbage, THEN a well-formed line. The garbage alone looks like a torn write; the
		// complete line after it proves the process kept going, so those bytes were never
		// torn — and truncating from the garbage would take the valid record with them.
		appendRaw(t, s, "{ garbage\n")
		appendEntryRaw(t, s, e)

		plan, err := s.PlanRepair()
		if err != nil {
			t.Fatal(err)
		}
		if plan.Repairable {
			t.Fatal("a suffix containing later complete lines is not a torn final append")
		}
		if !strings.Contains(plan.Reason, "complete line") {
			t.Fatalf("the refusal must name the reason, got %q", plan.Reason)
		}
	})
}

func TestRepairOnAnIntactJournalIsANoOp(t *testing.T) {
	s := newStore(t)
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))
	mustRecord(t, s, evidenced(a, "api", "work"), ep, at(time.Minute))
	before := journalBytes(t, s)

	res, err := s.Repair(a, ep, at(2*time.Minute))
	if err != nil {
		t.Fatalf("repairing an intact journal is a no-op, not an error: %v", err)
	}
	if res.Discarded != 0 {
		t.Fatalf("nothing to discard, got %d", res.Discarded)
	}
	if string(journalBytes(t, s)) != string(before) {
		t.Fatal("a no-op repair must not write anything — not even a receipt")
	}
}

// appendEntryRaw writes a pre-built entry to the journal verbatim, bypassing every
// gate. This is the forger's tool, and it exists so the tests can be the forger.
func appendEntryRaw(t *testing.T, s *Store, e Entry) {
	t.Helper()
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	appendRaw(t, s, string(b)+"\n")
}

// ─── 8. locking must be real, or fail closed ──────────────────────────────────

// There is no no-op lock. On a platform that cannot serialize, every MUTATION fails
// closed — because a lock that silently does nothing is worse than none: the caller
// believes it is protected while two agents interleave and both take the seat.
func TestUnsupportedLockFailsEveryMutationClosed(t *testing.T) {
	s := newStore(t)
	ep := mustClaim(t, s, agent("a"), at(0)) // claimed while locking still works

	// Simulate a platform with no file locking.
	orig := lockAcquire
	lockAcquire = func(*os.File) (func(), error) { return nil, ErrLockUnsupported }
	t.Cleanup(func() { lockAcquire = orig })

	a := agent("a")
	mutations := map[string]error{
		"Claim":      errFrom(func() error { _, err := s.Claim(a, "", at(time.Minute)); return err }),
		"Takeover":   errFrom(func() error { _, err := s.Takeover(a, TakeoverRequest{GrantID: "g-x"}, at(time.Minute)); return err }),
		"Record":     errFrom(func() error { _, err := s.Record(evidenced(a, "ws", "x"), ep, at(time.Minute)); return err }),
		"Heartbeat":  s.Heartbeat(a, ep, at(time.Minute)),
		"Release":    s.Release(a, ep, "", at(time.Minute)),
		"Checkpoint": errFrom(func() error { _, err := s.Checkpoint(a, ep, "", at(time.Minute)); return err }),
		"Authorize": errFrom(func() error {
			_, err := s.Authorize(GrantRequest{Grantee: a, Actor: "x", Confirmed: true}, at(time.Minute))
			return err
		}),
		"Repair": errFrom(func() error { _, err := s.Repair(a, ep, at(time.Minute)); return err }),
	}
	for name, err := range mutations {
		if !errors.Is(err, ErrLockUnsupported) {
			t.Fatalf("%s must fail closed with ErrLockUnsupported on an unlockable platform, got %v", name, err)
		}
	}

	// Reads keep working: they never take the lock, and a host that cannot mutate can
	// still be inspected.
	if _, err := s.Status(at(time.Minute)); err != nil {
		t.Fatalf("Status must still work without a lock: %v", err)
	}
	if _, _, err := s.Board(); err != nil {
		t.Fatalf("Board must still work without a lock: %v", err)
	}
	if _, err := s.Reconcile(context.Background(), at(time.Minute)); err != nil {
		t.Fatalf("Reconcile must still work without a lock: %v", err)
	}
}

func errFrom(fn func() error) error { return fn() }

// ─── 10. transcripts: authorized and bounded before any byte is written ───────

func TestTranscriptRefusesAnUnauthorizedWriterBeforeTouchingDisk(t *testing.T) {
	s := newStore(t)
	ep := mustClaim(t, s, agent("holder"), at(0))

	_, err := s.Transcript(agent("stranger"), ep, "ws", "x", strings.NewReader("a lot of bytes"), at(time.Minute))
	var notHolder *ErrNotHolder
	if !errors.As(err, &notHolder) {
		t.Fatalf("a bystander must not write into the steward's store, got %v", err)
	}
	if _, err := os.Stat(s.transcriptDir()); err == nil {
		t.Fatal("the authority check must happen BEFORE a single byte is written to disk")
	}

	// A fenced holder is refused too — and again, before the write.
	_, err = s.Transcript(agent("holder"), ep+1, "ws", "x", strings.NewReader("bytes"), at(time.Minute))
	var fenced *ErrFenced
	if !errors.As(err, &fenced) {
		t.Fatalf("a stale token must be fenced, got %v", err)
	}
}

func TestTranscriptIsBounded(t *testing.T) {
	s, err := Open(t.TempDir(), WithMaxTranscriptBytes(64))
	if err != nil {
		t.Fatal(err)
	}
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))

	_, err = s.Transcript(a, ep, "ws", "huge", strings.NewReader(strings.Repeat("x", 65)), at(time.Minute))
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("an unbounded read from an agent-supplied stream is a way to fill the disk with bytes no "+
			"projection will ever read; got %v", err)
	}
	// Nothing was written.
	if des, err := os.ReadDir(s.transcriptDir()); err == nil && len(des) > 0 {
		t.Fatal("an over-limit transcript must leave no artifact behind")
	}
	// Exactly at the limit is fine.
	if _, err := s.Transcript(a, ep, "ws", "ok", strings.NewReader(strings.Repeat("x", 64)), at(2*time.Minute)); err != nil {
		t.Fatalf("a transcript exactly at the limit must be accepted: %v", err)
	}
}

// If the journal append fails, the artifact we just wrote is litter — and worse, it is
// litter that looks like evidence.
func TestTranscriptCleansUpItsArtifactIfTheJournalRefuses(t *testing.T) {
	s := newStore(t)
	a := agent("a")
	ep := mustClaim(t, s, a, at(0))

	// Authority passes the pre-check, then the seat moves under us before the append.
	// Simulated by handing Record a corrupt journal: the pre-check ran on the clean
	// replay, the append refuses.
	rep := mustReplay(t, s)
	if _, err := authorize(rep, a, ep); err != nil {
		t.Fatal(err)
	}
	appendRaw(t, s, "{ torn")

	_, err := s.Transcript(a, ep, "ws", "x", strings.NewReader("some content"), at(time.Minute))
	if err == nil {
		t.Fatal("the append must have failed")
	}
	des, rerr := os.ReadDir(s.transcriptDir())
	if rerr == nil && len(des) > 0 {
		t.Fatalf("an artifact with no entry pointing at it is litter that looks like evidence; found %d files", len(des))
	}
}

// ─── the seat cache is never an authority ─────────────────────────────────────

// A planted seat.json must not be able to CREATE authority out of nothing.
func TestAPlantedSeatFileGrantsNoAuthority(t *testing.T) {
	s := newStore(t)

	// The seat is vacant. Plant a heartbeat record claiming otherwise.
	if err := writeJSONAtomic(s.seatPath(), Seat{
		SchemaVersion: SchemaVersion, Holder: agent("impostor"), Epoch: 99,
		AcquiredAt: at(0), Heartbeat: at(0),
	}); err != nil {
		t.Fatal(err)
	}

	v, err := s.Status(at(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !v.Authority.Vacant {
		t.Fatal("authority comes from the JOURNAL. A planted cache file cannot conjure a holder out of nothing")
	}
	if v.Liveness != LivenessVacant || !v.Claimable {
		t.Fatalf("a vacant seat stays vacant and claimable, got %q", v.Liveness)
	}
	// And the impostor's epoch does not poison the ladder.
	ep := mustClaim(t, s, agent("honest"), at(2*time.Minute))
	if ep != 1 {
		t.Fatalf("the epoch ladder derives from the journal alone; got %d", ep)
	}
	// The stale cache is replaced, not merged: no field of it survives.
	var seat Seat
	if _, err := readJSON(s.seatPath(), &seat); err != nil {
		t.Fatal(err)
	}
	if seat.Epoch != 1 || !SameHolder(seat.Holder, agent("honest")) {
		t.Fatalf("the cache must be rewritten from the journal, got %+v", seat)
	}
	if seat.AcquiredAt != at(2*time.Minute) {
		t.Fatal("a field must never be carried forward out of a cache we would have refused to read")
	}
}

// ─── attendance is not a formality ────────────────────────────────────────────

// /dev/null IS A CHARACTER DEVICE, and that one fact quietly undoes the whole
// unattended-takeover rule.
//
// The usual isatty shortcut — os.ModeCharDevice on a Stat of stdin — answers "yes,
// attended" for a process whose stdin is /dev/null: a daemon, a cron job, a CI runner, a
// headless agent loop. Those are not merely non-terminals, they are THE unattended
// callers, the ones the external-receipt requirement exists for. Under the shortcut the
// most unattended process on the host is judged the most attended, and an operator
// ASSERTION — "a human approved this" — becomes spendable with no human within a mile of
// the machine.
//
// So the check asks the OS whether the descriptor is a terminal, and this test pins it
// with the file that breaks the shortcut.
func TestDevNullIsNotATerminal(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Skipf("cannot open %s: %v", os.DevNull, err)
	}
	defer f.Close()

	// The shortcut this must NOT be. If this ever stops holding, the test below is
	// proving nothing and should be re-read rather than deleted.
	fi, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeCharDevice == 0 {
		t.Skipf("%s is not a character device on this platform; the trap this guards does not exist here", os.DevNull)
	}

	if interactive(f) {
		t.Fatalf("%s was judged a terminal: an unattended process could then spend an operator ASSERTION "+
			"on a takeover, which is exactly what the external-receipt rule forbids", os.DevNull)
	}
}

// A non-file reader is never a terminal either — an in-process caller (a test, a hook,
// an embedding host piping a script) is unattended by construction.
func TestAPipeIsNotATerminal(t *testing.T) {
	if interactive(strings.NewReader("y\n")) {
		t.Fatal("a non-file reader has no terminal behind it")
	}
}
