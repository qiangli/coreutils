// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package steward

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The CLI is the surface a cold successor actually touches, so these tests assert
// the two things that make it trustworthy: the JSON envelopes are stable enough to
// parse, and the human output TELLS THE TRUTH — especially when the truth is
// "nobody established this".

// cli runs `steward <args…>` against an isolated store and returns stdout.
func cli(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	// A stable identity, so the seat is held by a known name rather than whatever
	// agent-detection happens to infer on the machine running the tests.
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/tester")
	t.Setenv("BASHY_EPISODE", "ep-test")
	// `claim` exports the fencing token with a raw os.Setenv, which would otherwise
	// survive into the NEXT test in this process and hand it a tenure it never took.
	// Re-registering the variable at its CURRENT value snapshots it for cleanup without
	// clearing it, so a claim earlier in this same test still counts — which is the
	// actual UX being tested (claim exports the epoch; later commands inherit it).
	t.Setenv(EpochEnv, os.Getenv(EpochEnv))

	cmd := NewStewardCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	// Stdin is pinned to a NON-TERMINAL, always. Left unset, cobra hands the command
	// os.Stdin, and whether that is a terminal depends on how the suite was launched —
	// so `go test` from a shell would take the attended path and the same test in CI
	// would take the unattended one. The unattended path is the one that matters here,
	// and it is the one an agent will actually be on.
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs(append([]string{"--dir", dir}, args...))
	err := cmd.Execute()
	return out.String(), err
}

// mustCLI runs and fails the test on error.
func mustCLI(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := cli(t, dir, args...)
	if err != nil {
		t.Fatalf("steward %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

func TestCLIStatusOnAVacantSeat(t *testing.T) {
	dir := t.TempDir()
	out := mustCLI(t, dir, "status")
	if !strings.Contains(out, "VACANT") {
		t.Fatalf("status on a fresh host must say the seat is vacant:\n%s", out)
	}
	if !strings.Contains(out, "steward claim") {
		t.Fatalf("status must tell the reader how to take a vacant seat:\n%s", out)
	}
}

func TestCLIClaimThenStatus(t *testing.T) {
	dir := t.TempDir()

	out := mustCLI(t, dir, "claim", "--intent", "on call")
	if !strings.Contains(out, "epoch 1") {
		t.Fatalf("the first claim must be epoch 1:\n%s", out)
	}

	out = mustCLI(t, dir, "status")
	if !strings.Contains(out, "live") || !strings.Contains(out, "tester") {
		t.Fatalf("status must show the live holder:\n%s", out)
	}
	if !strings.Contains(out, "on call") {
		t.Fatalf("status must surface the holder's intent:\n%s", out)
	}
}

// The --json envelope must be parseable and carry the schema version, or no other
// program can safely consume it.
func TestCLIStatusJSONIsStable(t *testing.T) {
	dir := t.TempDir()
	mustCLI(t, dir, "claim")
	mustCLI(t, dir, "record", "-m", "did a thing", "--workstream", "api",
		"--outcome", "success", "-e", "command:go test ./...")

	out := mustCLI(t, dir, "--json", "status")

	var env statusEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("status --json is not valid JSON: %v\n%s", err, out)
	}
	if env.SchemaVersion != SchemaVersion {
		t.Fatalf("schema_version = %q, want %q", env.SchemaVersion, SchemaVersion)
	}
	if env.Seat.Liveness != LivenessLive {
		t.Fatalf("seat.liveness = %q, want live", env.Seat.Liveness)
	}
	if env.Seat.Authority.Epoch != 1 {
		t.Fatalf("seat.authority.epoch = %d, want 1", env.Seat.Authority.Epoch)
	}
	if !env.Journal.Intact || env.Journal.Entries != 2 {
		t.Fatalf("journal = %+v, want 2 intact entries", env.Journal)
	}
	// An evidenced success is ASSERTED, never verified: `-e command:go test ./...`
	// records that somebody SAID they ran the tests. Only `steward verify` closes that
	// gap, and the JSON envelope must not blur it — a consumer that read this as
	// "verified" would be laundering a reference into a check.
	if len(env.Board.Workstreams) != 1 || env.Board.Workstreams[0].Confidence != ConfidenceAsserted {
		t.Fatalf("board = %+v; an evidenced success is asserted, not verified", env.Board.Workstreams)
	}
	if env.Board.Asserted != 1 {
		t.Fatalf("the board must COUNT the unchecked claims, got %d", env.Board.Asserted)
	}
}

// …and `verify` is what closes it. This is the one promotion path in the whole CLI.
func TestCLIVerifyIsTheOnlyThingThatReachesVerified(t *testing.T) {
	dir := t.TempDir()
	mustCLI(t, dir, "claim")
	mustCLI(t, dir, "record", "-m", "shipped the migration", "--workstream", "api",
		"--outcome", "success", "-e", "commit:de6485c")

	// seq 1 is the claim, seq 2 the effect.
	out := mustCLI(t, dir, "verify", "--seq", "2", "--result", "success",
		"--method", "re-ran the suite on a clean checkout", "-e", "command:go test ./...")
	if !strings.Contains(out, "verification recorded") {
		t.Fatalf("verify:\n%s", out)
	}

	var env statusEnvelope
	if err := json.Unmarshal([]byte(mustCLI(t, dir, "--json", "status")), &env); err != nil {
		t.Fatal(err)
	}
	if env.Board.Workstreams[0].Confidence != ConfidenceVerified {
		t.Fatalf("an attested claim must project as verified, got %q", env.Board.Workstreams[0].Confidence)
	}

	// A verification with no --method is the same trust-me claim it replaces.
	_, err := cli(t, dir, "verify", "--seq", "2", "--result", "success")
	if err == nil || !strings.Contains(err.Error(), "method") {
		t.Fatalf("verify must demand HOW it checked, got %v", err)
	}
}

// THE ONE THAT MATTERS. An agent records "done ✅" with nothing to point at, and the
// CLI must tell it — to its face — that the board will not believe it.
func TestCLIRecordWarnsWhenSuccessHasNoEvidence(t *testing.T) {
	dir := t.TempDir()
	mustCLI(t, dir, "claim")

	out := mustCLI(t, dir, "record", "-m", "shipped it, all green", "--workstream", "api", "--outcome", "success")
	if !strings.Contains(out, "NO EVIDENCE") {
		t.Fatalf("recording an unevidenced success must warn the agent that it will not project as one:\n%s", out)
	}
	if !strings.Contains(out, "unknown") {
		t.Fatalf("the warning must name what the board WILL show:\n%s", out)
	}

	// And the board agrees.
	board := mustCLI(t, dir, "board")
	if !strings.Contains(board, "unknown") {
		t.Fatalf("the board must show the unevidenced claim as unknown:\n%s", board)
	}
	if !strings.Contains(board, "NOBODY ESTABLISHED") {
		t.Fatalf("the board must lead with the honest headline:\n%s", board)
	}
}

func TestCLILogDegradedSurfacesOnlyTheUnproven(t *testing.T) {
	dir := t.TempDir()
	mustCLI(t, dir, "claim")
	mustCLI(t, dir, "record", "-m", "proven work", "--workstream", "api",
		"--outcome", "success", "-e", "commit:de6485c")
	mustCLI(t, dir, "record", "-m", "trust me bro", "--workstream", "db", "--outcome", "success")

	out := mustCLI(t, dir, "log", "--degraded")
	if strings.Contains(out, "proven work") {
		t.Fatalf("--degraded leaked an evidenced entry:\n%s", out)
	}
	if !strings.Contains(out, "trust me bro") {
		t.Fatalf("--degraded must surface the unproven claim:\n%s", out)
	}

	// --json must be a parseable array of entries.
	jsonOut := mustCLI(t, dir, "--json", "log", "--degraded")
	var entries []Entry
	if err := json.Unmarshal([]byte(jsonOut), &entries); err != nil {
		t.Fatalf("log --json is not valid JSON: %v\n%s", err, jsonOut)
	}
	if len(entries) != 1 || entries[0].Workstream != "db" {
		t.Fatalf("log --degraded --json = %+v, want exactly the db entry", entries)
	}
}

func TestCLIDecideRequiresARationale(t *testing.T) {
	dir := t.TempDir()
	mustCLI(t, dir, "claim")

	_, err := cli(t, dir, "decide", "-m", "drop v1")
	if err == nil {
		t.Fatal("a decision with no rationale must be refused: a successor can replay every effect and still never recover WHY")
	}
	if !strings.Contains(err.Error(), "rationale") {
		t.Fatalf("the refusal must name the missing flag: %v", err)
	}

	out := mustCLI(t, dir, "decide", "-m", "drop v1", "--rationale", "no callers in 90d", "--workstream", "api")
	if !strings.Contains(out, "decision recorded") {
		t.Fatalf("decide:\n%s", out)
	}

	conv := mustCLI(t, dir, "conversation")
	if !strings.Contains(conv, "DECISION") || !strings.Contains(conv, "no callers in 90d") {
		t.Fatalf("conversation must show the decision and its rationale:\n%s", conv)
	}
}

// mintReceiptGrant mints an EXTERNAL-RECEIPT capability and returns its id.
//
// A test has no terminal, so it is by definition an unattended caller — exactly the
// case where an operator ASSERTION is worth nothing and the package demands an artifact
// somebody can go and audit instead. That is not a testing workaround; it is the
// unattended path every CI runner and headless agent has to walk, so testing the CLI
// through it is testing the path that will actually be used.
func mintReceiptGrant(t *testing.T, dir, actor, reason string) string {
	t.Helper()
	receipt := filepath.Join(t.TempDir(), "approval.json")
	if err := os.WriteFile(receipt, []byte(`{"approved_by":"`+actor+`","pr":412}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out := mustCLI(t, dir, "authorize", "--actor", actor, "--reason", reason,
		"--receipt", receipt, "--receipt-issuer", "github:pr-412")

	id, ok := grantIDFrom(out)
	if !ok {
		t.Fatalf("authorize must print the grant id it minted:\n%s", out)
	}
	return id
}

// grantIDFrom pulls the id out of "authorization <id> minted".
func grantIDFrom(out string) (string, bool) {
	for line := range strings.SplitSeq(out, "\n") {
		if f := strings.Fields(line); len(f) >= 3 && f[0] == "authorization" && f[2] == "minted" {
			return f[1], true
		}
	}
	return "", false
}

// An agent cannot decide on its own that the steward looks stuck. Seizing the seat
// costs a capability, and the CLI must refuse every way of skipping that.
func TestCLITakeoverRequiresACapability(t *testing.T) {
	dir := t.TempDir()
	mustCLI(t, dir, "claim")

	_, err := cli(t, dir, "takeover")
	if err == nil {
		t.Fatal("a takeover presenting no authorization must be refused — otherwise the capability is decoration")
	}
	if !strings.Contains(err.Error(), "no authorization was presented") {
		t.Fatalf("the refusal must say what was missing: %v", err)
	}

	// An operator ASSERTION minted with --yes is real, and it is still not usable from
	// a process with nobody at the terminal: "a human approved this" is a sentence with
	// no author when there is no human present to be its author.
	assertion := mustCLI(t, dir, "authorize", "--actor", "qiangli", "--reason", "looks stuck", "--yes")
	id, ok := grantIDFrom(assertion)
	if !ok {
		t.Fatalf("authorize --yes must still mint:\n%s", assertion)
	}
	_, err = cli(t, dir, "takeover", "--grant", id)
	if err == nil {
		t.Fatal("an unattended takeover on an operator ASSERTION must be refused")
	}
	if !strings.Contains(err.Error(), "receipt") {
		t.Fatalf("the refusal must point at the receipt that WOULD be auditable: %v", err)
	}

	// With a receipt, the same unattended seizure is allowed — and is recorded forever.
	grant := mintReceiptGrant(t, dir, "qiangli", "wedged on a rate limit")

	// Keep a copy of the capability, the way a backup — or an attacker — would.
	grantFile := filepath.Join(dir, "grants", grant+".json")
	backup, err := os.ReadFile(grantFile)
	if err != nil {
		t.Fatal(err)
	}

	out := mustCLI(t, dir, "takeover", "--grant", grant)
	if !strings.Contains(out, "TOOK OVER") || !strings.Contains(out, "epoch 2") {
		t.Fatalf("an authorized takeover must seize the seat and bump the epoch:\n%s", out)
	}

	hist := mustCLI(t, dir, "history")
	if !strings.Contains(hist, "qiangli") {
		t.Fatalf("history must record who authorized the seizure:\n%s", hist)
	}

	// Single-use — and the JOURNAL is what enforces it, not the grant file. Spending the
	// grant removes the file, so deleting it would be a flimsy control: put the bytes
	// back and a file-based check hands the seat over again. The takeover that consumed
	// this nonce is in the hash chain, and THAT is what refuses.
	if err := os.WriteFile(grantFile, backup, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = cli(t, dir, "takeover", "--grant", grant)
	if err == nil {
		t.Fatal("a capability restored from a backup must not be spendable a second time")
	}
	if !strings.Contains(err.Error(), "already been used") {
		t.Fatalf("the refusal must name the replay, and cite the journal as the reason: %v", err)
	}
}

// A steward that captured its epoch and presents it after being taken over must be
// FENCED from the command line, not merely told it is a stranger. Without --epoch the
// CLI could never reach ErrFenced at all — the most important error in the system
// would be unreachable from the shell.
func TestCLIRecordWithAStaleEpochIsFenced(t *testing.T) {
	dir := t.TempDir()
	mustCLI(t, dir, "claim") // epoch 1 — the token a long-running steward would capture

	// A human authorizes recovery; the seat moves to epoch 2. (Same principal here —
	// which is the sharpest version of the test: even the SAME agent, presenting a
	// superseded token, must be refused. Being yourself is not a credential.)
	grant := mintReceiptGrant(t, dir, "qiangli", "recovery drill")
	mustCLI(t, dir, "takeover", "--grant", grant)

	_, err := cli(t, dir, "record", "-m", "…and then I deployed it",
		"--workstream", "api", "--outcome", "success", "-e", "command:kubectl apply", "--epoch", "1")
	if err == nil {
		t.Fatal("a write presenting a superseded epoch must be rejected")
	}
	var fenced *ErrFenced
	if !errors.As(err, &fenced) {
		t.Fatalf("want ErrFenced, got %T: %v", err, err)
	}
	if fenced.Presented != 1 || fenced.Current != 2 {
		t.Fatalf("fencing error = presented %d, current %d; want 1 and 2", fenced.Presented, fenced.Current)
	}
	if !strings.Contains(err.Error(), "steward log") {
		t.Fatalf("the fencing error must tell the zombie to re-read the journal: %v", err)
	}

	// Nothing of the fenced write reached the journal.
	log := mustCLI(t, dir, "log")
	if strings.Contains(log, "and then I deployed it") {
		t.Fatalf("a fenced write landed in the journal:\n%s", log)
	}
}

func TestCLICheckpointAndVerify(t *testing.T) {
	dir := t.TempDir()
	mustCLI(t, dir, "claim")
	mustCLI(t, dir, "record", "-m", "real work", "--workstream", "api",
		"--outcome", "success", "-e", "commit:abc1234")

	out := mustCLI(t, dir, "--json", "checkpoint", "--note", "before the risky bit")
	var ck Checkpoint
	if err := json.Unmarshal([]byte(out), &ck); err != nil {
		t.Fatalf("checkpoint --json: %v\n%s", err, out)
	}
	if ck.ID == "" || ck.JournalDigest == "" || ck.Board.Digest == "" {
		t.Fatalf("a checkpoint must carry its receipt: %+v", ck)
	}

	verify := mustCLI(t, dir, "checkpoint", "--verify", ck.ID)
	if !strings.Contains(verify, "REPRODUCIBLE") {
		t.Fatalf("a fresh checkpoint must verify:\n%s", verify)
	}

	list := mustCLI(t, dir, "checkpoint", "--list")
	if !strings.Contains(list, ck.ID) {
		t.Fatalf("checkpoint --list must show it:\n%s", list)
	}
}

func TestCLIReconcileReportsDegradedAndCanRecordIt(t *testing.T) {
	dir := t.TempDir()
	mustCLI(t, dir, "claim")
	mustCLI(t, dir, "record", "-m", "shipped, honest", "--workstream", "api", "--outcome", "success")

	out := mustCLI(t, dir, "reconcile")
	if !strings.Contains(out, "DEGRADED") {
		t.Fatalf("reconcile must report a degraded verdict for an unevidenced claim:\n%s", out)
	}
	if !strings.Contains(out, "UNPROVEN") {
		t.Fatalf("reconcile must list the unproven claims by name:\n%s", out)
	}

	// --json carries the same verdict, machine-readably.
	jsonOut := mustCLI(t, dir, "--json", "reconcile")
	var r Reconciliation
	if err := json.Unmarshal([]byte(jsonOut), &r); err != nil {
		t.Fatalf("reconcile --json: %v\n%s", err, jsonOut)
	}
	if r.Health != HealthDegraded || len(r.Unproven) != 1 {
		t.Fatalf("reconcile --json = health %q, %d unproven; want degraded/1", r.Health, len(r.Unproven))
	}

	// --record makes the finding permanent.
	mustCLI(t, dir, "reconcile", "--record")
	log := mustCLI(t, dir, "log", "--kind", "reconcile")
	if !strings.Contains(log, "reconciled") {
		t.Fatalf("--record must append the reconciliation to the journal:\n%s", log)
	}
}

// A non-holder gets the REPORT even though it cannot write it. Refusing to print the
// truth because you lack the seat would be the one genuinely unhelpful failure mode:
// reconcile is what you run BEFORE you have the seat.
func TestCLIReconcileStillReportsWithoutTheSeat(t *testing.T) {
	dir := t.TempDir()
	// Nobody has ever claimed. Reconcile must still work.
	out := mustCLI(t, dir, "reconcile")
	if !strings.Contains(out, "VACANT") {
		t.Fatalf("reconcile on a vacant host must say so:\n%s", out)
	}

	// With --record and no seat, the report is still printed and the failure to
	// write it is explained rather than swallowed.
	out = mustCLI(t, dir, "reconcile", "--record")
	if !strings.Contains(out, "VACANT") {
		t.Fatalf("--record must not suppress the report when the write fails:\n%s", out)
	}
	if !strings.Contains(out, "NOT written") {
		t.Fatalf("the reader must be told the report was not journaled, and why:\n%s", out)
	}
}

func TestCLIWorkstreamOpenAndClose(t *testing.T) {
	dir := t.TempDir()
	mustCLI(t, dir, "claim")
	mustCLI(t, dir, "workstream", "open", "api", "--title", "the API migration")

	out := mustCLI(t, dir, "workstream", "close", "api", "-m", "all done", "--outcome", "success")
	if !strings.Contains(out, "NO EVIDENCE") {
		t.Fatalf("closing with an unevidenced success must warn — closed is not the same fact as verified done:\n%s", out)
	}

	board := mustCLI(t, dir, "board", "api")
	if !strings.Contains(board, "the API migration") {
		t.Fatalf("board <name> must show the title:\n%s", board)
	}
	if !strings.Contains(board, "closed") || !strings.Contains(board, "unknown") {
		t.Fatalf("the workstream must be closed AND unknown:\n%s", board)
	}
	if !strings.Contains(board, "UNPROVEN") {
		t.Fatalf("the detail view must name the unproven claim:\n%s", board)
	}
}

// A transcript is stored, and the CLI says plainly that nothing depends on it.
func TestCLITranscriptIsMarkedNonAuthoritative(t *testing.T) {
	dir := t.TempDir()
	mustCLI(t, dir, "claim")

	cmd := NewStewardCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("human: do the thing\nagent: ok\n"))
	cmd.SetArgs([]string{"--dir", dir, "transcript", "-m", "the conversation", "--workstream", "api"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("transcript: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Non-authoritative") {
		t.Fatalf("the CLI must say a transcript is non-authoritative:\n%s", out.String())
	}

	// It does not appear on the board — transcripts derive nothing.
	board := mustCLI(t, dir, "board")
	if strings.Contains(board, "the conversation") {
		t.Fatalf("a transcript reached the board; nothing may derive from a non-authoritative artifact:\n%s", board)
	}
}

// `take` is an alias for `claim` — the issue asks for either spelling, so both work.
func TestCLITakeIsAnAliasForClaim(t *testing.T) {
	dir := t.TempDir()
	out := mustCLI(t, dir, "take")
	if !strings.Contains(out, "claimed the steward seat") {
		t.Fatalf("`take` must be an alias for `claim`:\n%s", out)
	}
}

func TestCLIReleaseSaysItCapturedNoWork(t *testing.T) {
	dir := t.TempDir()
	mustCLI(t, dir, "claim")

	out := mustCLI(t, dir, "release", "--note", "stopping for the day")
	if !strings.Contains(out, "handoff") {
		t.Fatalf("release must point at the verb that DOES move work, or the two stay conflated:\n%s", out)
	}

	status := mustCLI(t, dir, "status")
	if !strings.Contains(status, "VACANT") {
		t.Fatalf("the seat must be vacant after release:\n%s", status)
	}
}

// A torn journal must be reported by every read verb, not silently truncated —
// printing a partial history with no warning is the one dishonest thing this could do.
func TestCLIWarnsAboutACorruptTail(t *testing.T) {
	dir := t.TempDir()
	mustCLI(t, dir, "claim")
	mustCLI(t, dir, "record", "-m", "real work", "--workstream", "api",
		"--outcome", "success", "-e", "commit:abc")

	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(s.journalPath(), os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"seq":3,"trunc`)
	f.Close()

	out := mustCLI(t, dir, "log")
	if !strings.Contains(out, "WARNING") {
		t.Fatalf("log must warn that its output is a valid PREFIX, not the whole story:\n%s", out)
	}
	if !strings.Contains(out, "real work") {
		t.Fatalf("a torn tail must not hide the valid history before it:\n%s", out)
	}

	// A write refuses until the tail is explicitly repaired.
	_, err = cli(t, dir, "record", "-m", "carrying on", "--outcome", "success", "-e", "commit:def")
	if err == nil {
		t.Fatal("appending onto a corrupt tail must be refused")
	}

	// --plan changes nothing, and says exactly what it WOULD discard, so an operator can
	// see with their own eyes that it is a torn fragment and not a record.
	out = mustCLI(t, dir, "repair", "--plan")
	if !strings.Contains(out, "repairable:     true") {
		t.Fatalf("a torn final append is repairable, and the plan must say so:\n%s", out)
	}
	if !strings.Contains(out, "trunc") {
		t.Fatalf("the plan must show the bytes it would discard:\n%s", out)
	}
	if !strings.Contains(mustCLI(t, dir, "log"), "WARNING") {
		t.Fatal("--plan must change nothing")
	}

	// …and the repair is explicit, and truncates only the unreadable bytes.
	out = mustCLI(t, dir, "repair")
	if !strings.Contains(out, "No valid entry was removed") {
		t.Fatalf("the repair must reassure that it cost only the torn tail:\n%s", out)
	}
	if !strings.Contains(out, "quarantined") {
		t.Fatalf("the repair must say where the discarded bytes went:\n%s", out)
	}

	out = mustCLI(t, dir, "log")
	if strings.Contains(out, "WARNING") {
		t.Fatalf("the journal must be clean after a repair:\n%s", out)
	}
	if !strings.Contains(out, "real work") {
		t.Fatalf("the repair destroyed valid history:\n%s", out)
	}
	// The repair itself is on the record, as a DEGRADED entry. A journal that quietly
	// healed itself is indistinguishable from one somebody edited.
	if !strings.Contains(mustCLI(t, dir, "log", "--kind", "repair"), "quarantined") {
		t.Fatal("the repair must leave a receipt in the journal")
	}
}
