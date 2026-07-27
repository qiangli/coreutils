package weave

import (
	"os"
	"strings"
	"testing"
)

// `weave prune --help` claims it "REFUSES to delete a workspace that still
// holds work". A claim in a help string is not a guard; this exercises the
// actual sweep end to end and proves the workspace — the only place an agent
// branch lives until `weave pull` fetches it — survives.
func TestPruneDoesNotSilentlyDiscardUnmergedCommits(t *testing.T) {
	_, workspace, dir := setupPullRefusalFixture(t, 1, "failed", 2)

	out, code := runWeave(t, "prune", "--yes")
	if code != 0 {
		t.Fatalf("prune exit=%d: %s", code, out)
	}
	if _, err := os.Stat(workspace); err != nil {
		t.Fatalf("prune destroyed a workspace holding 2 unmerged commits: %v\n%s", err, out)
	}
	if !strings.Contains(out, "KEPT") {
		t.Fatalf("a sweep that spares work must SAY so — a silent skip is indistinguishable from a silent delete: %s", out)
	}
	if !strings.Contains(out, "unmerged commit") || !strings.Contains(out, "salvage") {
		t.Fatalf("the hold reason must name the loss and the recovery verb: %s", out)
	}
	q, err := loadWeaveQueue(dir)
	if err != nil {
		t.Fatal(err)
	}
	if it := findWeaveItem(q, 1); it == nil || it.Workspace != workspace {
		t.Fatalf("a spared run must keep its workspace pointer: %#v", it)
	}
}

// THE GAP THE CLAIM DID NOT COVER.
//
// prune's guard used to count `main..HEAD` with the base BRANCH NAME as read
// inside the WORKSPACE clone, ignoring the item's clone-point BaseSHA. A
// workspace whose local `main` ref has moved on to include the agent's commits
// (a rebase, a local fast-forward, an agent that tidied up before exiting)
// counts 0 that way — and 0 is exactly the number that lets a destructive sweep
// proceed. The user repo never had those commits.
//
// Both halves now measure through weaveUnmergedAhead, which counts from the
// immutable BaseSHA and asks the ROOT repo what actually landed.
func TestPruneGuardSurvivesWorkspaceBaseRefDrift(t *testing.T) {
	_, workspace, _ := setupPullRefusalFixture(t, 1, "failed", 1)
	// The drift: the workspace's own `main` now points at the agent's commit,
	// so an in-workspace `main..HEAD` count reads 0.
	gitT(t, workspace, "branch", "-f", "main", "HEAD")
	if n := gitT(t, workspace, "rev-list", "--count", "main..HEAD"); n != "0" {
		t.Fatalf("fixture did not reproduce the drift: main..HEAD = %s", n)
	}

	out, code := runWeave(t, "prune", "--yes")
	if code != 0 {
		t.Fatalf("prune exit=%d: %s", code, out)
	}
	if _, err := os.Stat(workspace); err != nil {
		t.Fatalf("prune destroyed work the user repo has never seen, because the workspace's own base ref had drifted: %v\n%s", err, out)
	}
	if !strings.Contains(out, "KEPT") {
		t.Fatalf("prune must report the hold: %s", out)
	}
}

// The same drift, on the verb that is one keystroke from a mistake. abandon's
// guard shares the measurement, so it must refuse here too.
func TestAbandonGuardSurvivesWorkspaceBaseRefDrift(t *testing.T) {
	_, workspace, _ := setupPullRefusalFixture(t, 1, "failed", 1)
	gitT(t, workspace, "branch", "-f", "main", "HEAD")

	out, code := runWeave(t, "abandon", "1", "--yes", "--json")
	if code == 0 {
		t.Fatalf("abandon must refuse a run holding an unmerged commit, got exit 0: %s", out)
	}
	if _, err := os.Stat(workspace); err != nil {
		t.Fatalf("a refused abandon must leave the workspace intact: %v", err)
	}
}

// A clean run is still swept. A guard that holds everything gets forced by
// reflex, and then it is protecting nothing.
func TestPruneStillSweepsCleanRun(t *testing.T) {
	_, workspace, _ := setupPullRefusalFixture(t, 1, "failed", 0)

	out, code := runWeave(t, "prune", "--yes")
	if code != 0 {
		t.Fatalf("prune exit=%d: %s", code, out)
	}
	if _, err := os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatalf("a run with nothing at risk must still be cleaned up: %v\n%s", err, out)
	}
}
