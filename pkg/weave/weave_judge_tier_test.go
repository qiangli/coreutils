package weave

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/fleet"
)

// pinTierFleet installs a scratch fleet catalog holding the given models and
// agents, so a test can exercise the real band/family eligibility logic without
// depending on the shipped baseline's live pegs.
func pinTierFleet(t *testing.T, models []fleet.Model, agents []fleet.Agent) {
	t.Helper()
	root := t.TempDir()
	cat := fleet.New(fleet.WithRoot(root))
	for _, m := range models {
		if err := cat.SaveModel(m); err != nil {
			t.Fatalf("save model %s: %v", m.Name, err)
		}
	}
	for _, a := range agents {
		if err := cat.SaveAgent(a); err != nil {
			t.Fatalf("save agent %s: %v", a.Name, err)
		}
	}
	prev := fleetCatalog
	fleetCatalog = func() *fleet.Catalog { return fleet.New(fleet.WithRoot(root)) }
	t.Cleanup(func() { fleetCatalog = prev })
}

func tierModel(name, family string, band int) fleet.Model {
	return fleet.Model{
		Name:       name,
		Family:     family,
		Version:    "1",
		Band:       band,
		Kind:       fleet.ModelKindAPI,
		Provider:   "openai-compat",
		BaseURL:    "https://example.invalid",
		APIKeyRef:  "tier",
		UpstreamID: name,
	}
}

// patchWeaveItem mutates one queued item in place and persists it.
func patchWeaveItem(t *testing.T, dir string, id int64, fn func(*weaveItem)) {
	t.Helper()
	if err := withWeaveQueueLock(dir, func(q *weaveQueue) error {
		it := findWeaveItem(q, id)
		if it == nil {
			return fmt.Errorf("item %d not found", id)
		}
		fn(it)
		return nil
	}); err != nil {
		t.Fatalf("patch item %d: %v", id, err)
	}
}

// setupSubmittedRun files an issue, runs it to a clean submitted state with a
// committed feature, and returns the repo root.
func setupSubmittedRun(t *testing.T, verify string) string {
	t.Helper()
	root := setupIsolationFixture(t)
	t.Chdir(root)
	if out, code := runWeave(t, "add", "tiered work", "--verify", verify, "--json"); code != 0 {
		t.Fatalf("add exit=%d: %s", code, out)
	}
	script := `set -e
echo clean > feature.txt
git add feature.txt
git -c user.email=a@a -c user.name=a commit -qm "clean feature"`
	if out, code := runWeave(t, "start", "--issue", "1", "--json", "--", "sh", "-c", script); code != 0 {
		t.Fatalf("start exit=%d: %s", code, out)
	}
	return root
}

// (a) A Judge=="none" item with a green gate MERGES, and the pair runner is
// NEVER invoked — a deterministically-verifiable unit needs no LLM arbiter.
func TestWeavePullJudgeNoneSkipsPairAndMerges(t *testing.T) {
	root := setupSubmittedRun(t, "test -f feature.txt")
	dir, _ := weaveQueueDir(root)
	patchWeaveItem(t, dir, 1, func(it *weaveItem) { it.Judge = weaveJudgeNone })

	original := weavePairReviewRunner
	t.Cleanup(func() { weavePairReviewRunner = original })
	called := 0
	weavePairReviewRunner = func(workspace, diffRef, gateCommand, requested string, it *weaveItem) (weavePairReviewResult, error) {
		called++
		return weavePairReviewResult{}, nil
	}

	out, code := runWeave(t, "pull", "1", "--review-agent", "reviewer", "--json")
	if code != 0 || !strings.Contains(out, `"status": "merged"`) {
		t.Fatalf("judge=none item did not merge on the probe alone (exit %d): %s", code, out)
	}
	if called != 0 {
		t.Fatalf("pair runner invoked %d times for a judge=none item; must be 0", called)
	}
}

// (b) A Judge=="required" item with NO eligible judge available: the autopilot
// merge loop HALTS loudly and exits non-zero — it never loops into a merge that
// cannot produce a verdict.
func TestWeaveAutopilotHaltsWhenNoEligibleJudge(t *testing.T) {
	// The reviewer resolves, but serves band L2 — below the L3 verdict floor.
	pinTierFleet(t,
		[]fleet.Model{tierModel("judge-m", "judgefam", 2)},
		[]fleet.Agent{{Name: "judgeagent", Tool: "codex", Model: "judge-m"}},
	)
	dir := testWeaveAutopilotQueue(t)
	patchWeaveItem(t, dir, 1, func(it *weaveItem) {
		it.State = "submitted"
		it.Judge = weaveJudgeRequired
	})

	runner := &testWeaveAutopilotRunner{}
	_, err := runWeaveAutopilotLoop(context.Background(), weaveAutopilotLoopOptions{
		queueDir: dir, repoRoot: "/repo", fleet: testMembers("codex"),
		leaseTTL: time.Second, heartbeat: 10 * time.Millisecond, backoff: time.Millisecond,
		runner: runner, holder: "halt-test", maxRuns: 1, reviewAgent: "judgeagent",
	})
	if err == nil {
		t.Fatalf("autopilot proceeded despite no eligible judge; want a fail-closed halt")
	}
	if !strings.Contains(err.Error(), "no eligible judge") {
		t.Fatalf("halt reason = %q, want it to name the missing eligible judge", err.Error())
	}
	if len(runner.runList()) != 0 {
		t.Fatalf("autopilot ran a merge member (%v) before halting; must halt first", runner.runList())
	}
}

// (c) A Judge=="required" item, judged by an L2 agent for an L3 issue: REFUSED
// on the band floor, and the base branch never moves.
func TestWeavePullRequiredRefusesBelowBandFloor(t *testing.T) {
	pinTierFleet(t,
		[]fleet.Model{
			tierModel("coder-m", "coderfam", 3),
			tierModel("judge-m", "judgefam", 2),
		},
		[]fleet.Agent{{Name: "judgeagent", Tool: "codex", Model: "judge-m"}},
	)
	root := setupSubmittedRun(t, "test -f feature.txt")
	dir, _ := weaveQueueDir(root)
	patchWeaveItem(t, dir, 1, func(it *weaveItem) {
		it.Judge = weaveJudgeRequired
		it.Band = 3
		it.LaunchSpec = &weaveLaunchSpec{Tool: "claude", Model: "coder-m"}
	})

	original := weavePairReviewRunner
	t.Cleanup(func() { weavePairReviewRunner = original })
	weavePairReviewRunner = func(workspace, diffRef, gateCommand, requested string, it *weaveItem) (weavePairReviewResult, error) {
		t.Fatalf("pair runner invoked despite an ineligible (band-floor) judge")
		return weavePairReviewResult{}, nil
	}

	out, code := runWeave(t, "pull", "1", "--review-agent", "judgeagent", "--json")
	if code == 0 {
		t.Fatalf("pull merged an L3 issue with an L2 judge: %s", out)
	}
	if !strings.Contains(out, "band") && !strings.Contains(out, "floor") {
		t.Fatalf("refusal did not name the band floor: %s", out)
	}
	if got := gitT(t, root, "log", "--format=%s", "-1"); got != "seed" {
		t.Fatalf("base branch moved despite band-floor refusal: %s", got)
	}
}

// (d) A Judge=="required" item, judged by an agent of the coder's OWN family:
// REFUSED on separation of duties.
func TestWeavePullRequiredRefusesSameFamilyJudge(t *testing.T) {
	pinTierFleet(t,
		[]fleet.Model{
			tierModel("coder-m", "sharedfam", 3),
			tierModel("judge-m", "sharedfam", 4),
		},
		[]fleet.Agent{{Name: "judgeagent", Tool: "codex", Model: "judge-m"}},
	)
	root := setupSubmittedRun(t, "test -f feature.txt")
	dir, _ := weaveQueueDir(root)
	patchWeaveItem(t, dir, 1, func(it *weaveItem) {
		it.Judge = weaveJudgeRequired
		it.Band = 3
		it.LaunchSpec = &weaveLaunchSpec{Tool: "claude", Model: "coder-m"}
	})

	original := weavePairReviewRunner
	t.Cleanup(func() { weavePairReviewRunner = original })
	weavePairReviewRunner = func(workspace, diffRef, gateCommand, requested string, it *weaveItem) (weavePairReviewResult, error) {
		t.Fatalf("pair runner invoked despite a same-family judge")
		return weavePairReviewResult{}, nil
	}

	out, code := runWeave(t, "pull", "1", "--review-agent", "judgeagent", "--json")
	if code == 0 {
		t.Fatalf("pull merged with a judge of the coder's own family: %s", out)
	}
	if !strings.Contains(out, "family") {
		t.Fatalf("refusal did not name the family-separation rule: %s", out)
	}
	if got := gitT(t, root, "log", "--format=%s", "-1"); got != "seed" {
		t.Fatalf("base branch moved despite same-family refusal: %s", got)
	}
}

// (e) A Judge=="required" item with a passing verdict from an eligible
// (band-ok, different-family) judge MERGES.
func TestWeavePullRequiredMergesWithEligibleVerdict(t *testing.T) {
	pinTierFleet(t,
		[]fleet.Model{tierModel("judge-m", "judgefam", 3)},
		[]fleet.Agent{{Name: "judgeagent", Tool: "codex", Model: "judge-m"}},
	)
	root := setupSubmittedRun(t, "test -f feature.txt")
	dir, _ := weaveQueueDir(root)
	patchWeaveItem(t, dir, 1, func(it *weaveItem) { it.Judge = weaveJudgeRequired })

	original := weavePairReviewRunner
	t.Cleanup(func() { weavePairReviewRunner = original })
	called := 0
	weavePairReviewRunner = func(workspace, diffRef, gateCommand, requested string, it *weaveItem) (weavePairReviewResult, error) {
		called++
		return weavePairReviewResult{
			CodingAgent: "sh", ReviewAgent: "codex:judge-m", AddedTest: false,
			Verdict: weavePairPass, Reason: "pair attacked the change and the gate stayed green", ExitCode: weavePairPassExit,
		}, nil
	}

	out, code := runWeave(t, "pull", "1", "--review-agent", "judgeagent", "--json")
	if code != 0 || !strings.Contains(out, `"status": "merged"`) || !strings.Contains(out, `"pair_verdict": "pass"`) {
		t.Fatalf("eligible+passing required merge did not land (exit %d): %s", code, out)
	}
	if called != 1 {
		t.Fatalf("pair runner calls=%d, want exactly 1", called)
	}
}
