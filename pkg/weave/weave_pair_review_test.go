package weave

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestWeavePullPairEvidenceBlocksMerge(t *testing.T) {
	root := setupIsolationFixture(t)
	t.Chdir(root)
	if out, code := runWeave(t, "add", "buggy work", "--verify", "test ! -f adversarial_test.go", "--json"); code != 0 {
		t.Fatalf("add exit=%d: %s", code, out)
	}
	script := `set -e
echo buggy > feature.txt
git add feature.txt
git -c user.email=a@a -c user.name=a commit -qm "buggy feature"`
	if out, code := runWeave(t, "start", "--issue", "1", "--json", "--", "sh", "-c", script); code != 0 {
		t.Fatalf("start exit=%d: %s", code, out)
	}

	original := weavePairReviewRunner
	t.Cleanup(func() { weavePairReviewRunner = original })
	weavePairReviewRunner = func(workspace, diffRef, gateCommand, requested string, it *weaveItem) (weavePairReviewResult, error) {
		if requested != "reviewer" || diffRef == "" || gateCommand == "" {
			t.Fatalf("pair args requested=%q diff=%q gate=%q", requested, diffRef, gateCommand)
		}
		if err := os.WriteFile(filepath.Join(workspace, "adversarial_test.go"), []byte("package adversarial\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		gitT(t, workspace, "add", "adversarial_test.go")
		gitT(t, workspace, "commit", "-qm", "pair proof")
		return weavePairReviewResult{CodingAgent: "sh", ReviewAgent: "codex:gpt", AddedTest: true, ExitCode: weavePairProvedExit}, nil
	}

	out, code := runWeave(t, "pull", "1", "--review-agent", "reviewer", "--json")
	if code != 0 {
		t.Fatalf("pull exit=%d: %s", code, out)
	}
	if !strings.Contains(out, `"status": "verify-failed"`) || !strings.Contains(out, `"review_added_test": true`) {
		t.Fatalf("pair proof did not block merge with durable evidence: %s", out)
	}
	dir, _ := weaveQueueDir(root)
	q, err := loadWeaveQueue(dir)
	if err != nil {
		t.Fatal(err)
	}
	it := findWeaveItem(q, 1)
	if it.State != "failed" || it.CodingAgent != "sh" || it.ReviewAgent != "codex:gpt" || !it.ReviewAddedTest {
		t.Fatalf("item evidence/state = %#v", it)
	}
	if got := gitT(t, it.Workspace, "show", "--format=", "--name-only", "HEAD"); !strings.Contains(got, "adversarial_test.go") {
		t.Fatalf("pair proof was not committed: %s", got)
	}
	if got := gitT(t, root, "log", "--format=%s", "-1"); got != "seed" {
		t.Fatalf("buggy branch merged despite red gate: %s", got)
	}
}

func TestWeavePullReviewOptInAndCleanMerge(t *testing.T) {
	root := setupIsolationFixture(t)
	t.Chdir(root)
	if out, code := runWeave(t, "add", "clean work", "--verify", "test -f feature.txt", "--json"); code != 0 {
		t.Fatalf("add exit=%d: %s", code, out)
	}
	script := `set -e
echo clean > feature.txt
git add feature.txt
git -c user.email=a@a -c user.name=a commit -qm "clean feature"`
	if out, code := runWeave(t, "start", "--issue", "1", "--json", "--", "sh", "-c", script); code != 0 {
		t.Fatalf("start exit=%d: %s", code, out)
	}

	original := weavePairReviewRunner
	t.Cleanup(func() { weavePairReviewRunner = original })
	called := 0
	weavePairReviewRunner = func(workspace, diffRef, gateCommand, requested string, it *weaveItem) (weavePairReviewResult, error) {
		called++
		return weavePairReviewResult{CodingAgent: "sh", ReviewAgent: "claude:opus", AddedTest: false}, nil
	}
	out, code := runWeave(t, "pull", "1", "--review-agent", "reviewer", "--json")
	if code != 0 || !strings.Contains(out, `"status": "merged"`) {
		t.Fatalf("clean reviewed run did not merge (exit %d): %s", code, out)
	}
	if called != 1 {
		t.Fatalf("pair calls=%d, want 1", called)
	}
}

func TestWeaveSelectReviewAgentReplacesCoder(t *testing.T) {
	agents, _ := fleetCatalog().Agents()
	if len(agents) < 2 {
		t.Skip("fleet registry has fewer than two agents")
	}
	coder := agents[0]
	it := &weaveItem{Tool: coder.Tool, LaunchSpec: &weaveLaunchSpec{Tool: coder.Tool, Agent: coder.Name, Model: coder.Model}}
	reviewer, coding, err := weaveSelectReviewAgent(coder.Name, it)
	if err != nil {
		t.Fatal(err)
	}
	if reviewer == "" || reviewer == coding || reviewer == coder.Tool+":"+coder.Model {
		t.Fatalf("self-review was not replaced: coder=%q reviewer=%q", coding, reviewer)
	}
}

func TestReviewAgentIsDefaultOffAndThreadsIntoAutopilot(t *testing.T) {
	for name, cmd := range map[string]*cobra.Command{
		"pull":      newWeavePullCmd(),
		"autopilot": newWeaveAutopilotCmd(),
		"heartbeat": newWeaveHeartbeatCmd(),
	} {
		flag := cmd.Flag("review-agent")
		if flag == nil || flag.DefValue != "" {
			t.Fatalf("%s --review-agent default = %#v, want present and off", name, flag)
		}
	}

	dir := t.TempDir()
	if err := saveWeaveQueue(dir, &weaveQueue{}); err != nil {
		t.Fatal(err)
	}
	prompt, err := buildWeaveAutopilotPrompt(t.TempDir(), dir, "", "codex:gpt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "pull <issue> --review-agent codex:gpt") {
		t.Fatalf("autopilot did not receive fleet-wide review requirement: %s", prompt)
	}
}
