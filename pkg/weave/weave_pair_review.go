package weave

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	weavePairProvedExit       = 4
	weavePairBrokenBeforeExit = 5
)

type weavePairReviewResult struct {
	CodingAgent string
	ReviewAgent string
	AddedTest   bool
	ExitCode    int
	Output      string
}

// weavePairReviewRunner is replaceable in tests so pull's merge behavior can
// be exercised without spending an agent invocation.
var weavePairReviewRunner = runWeavePairReview

func weaveCodingIdentity(it *weaveItem) (identity, tool, model string) {
	if it == nil {
		return "", "", ""
	}
	tool = it.Tool
	if it.LaunchSpec != nil {
		model = it.LaunchSpec.Model
		if it.LaunchSpec.Tool != "" && tool == "" {
			tool = it.LaunchSpec.Tool
		}
		if it.LaunchSpec.Agent != "" {
			identity = it.LaunchSpec.Agent
			// LaunchSpec.Model is the provider-side target, while separation
			// compares registry bindings. Resolve the recorded agent so aliases
			// and provider target names cannot disguise self-review.
			if l, err := weaveResolveAgent(it.LaunchSpec.Agent); err == nil && l != nil {
				identity, tool, model = l.Binding(), l.ToolName, l.ModelName
			}
		}
	}
	if tool != "" && model != "" {
		identity = tool + ":" + model
	}
	if identity == "" {
		identity = tool
	}
	return identity, tool, model
}

// weaveSelectReviewAgent resolves the requested reviewer and enforces the
// load-bearing separation: the acting pair cannot be the coder. "auto" is an
// explicit opt-in that asks weave to choose from the fleet registry.
func weaveSelectReviewAgent(requested string, it *weaveItem) (reviewer, coder string, err error) {
	coder, coderTool, coderModel := weaveCodingIdentity(it)
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return "", coder, nil
	}

	resolve := func(name string) (*weaveAgentLaunch, error) {
		l, err := weaveResolveAgent(name)
		if err != nil {
			return nil, err
		}
		if l == nil {
			return nil, fmt.Errorf("reviewer %q is not a fleet agent; use an agent nickname or tool:model binding", name)
		}
		return l, nil
	}

	if requested != "auto" {
		l, rerr := resolve(requested)
		if rerr != nil {
			return "", coder, rerr
		}
		sameBinding := coderTool != "" && l.ToolName == coderTool && (coderModel == "" || l.ModelName == coderModel)
		sameName := it != nil && it.LaunchSpec != nil && it.LaunchSpec.Agent != "" &&
			(requested == it.LaunchSpec.Agent || l.Nick == it.LaunchSpec.Agent)
		if !sameBinding && !sameName {
			return l.Binding(), coder, nil
		}
	}

	agents, _ := fleetCatalog().Agents()
	// Prefer diversity on both axes. If the local registry cannot supply that,
	// accept a different binding on either axis; it is still not self-review.
	for pass := 0; pass < 2; pass++ {
		for _, a := range agents {
			if a.Tool == coderTool && a.Model == coderModel {
				continue
			}
			if pass == 0 && (a.Tool == coderTool || (coderModel != "" && a.Model == coderModel)) {
				continue
			}
			l, rerr := resolve(a.Name)
			if rerr == nil {
				return l.Binding(), coder, nil
			}
		}
	}
	return "", coder, fmt.Errorf("review agent %q resolves to the coder %q and no different fleet agent is available", requested, coder)
}

func runWeavePairReview(workspace, diffRef, gateCommand, requested string, it *weaveItem) (weavePairReviewResult, error) {
	reviewer, coder, err := weaveSelectReviewAgent(requested, it)
	res := weavePairReviewResult{CodingAgent: coder, ReviewAgent: reviewer}
	if err != nil || reviewer == "" {
		return res, err
	}
	if strings.TrimSpace(gateCommand) == "" {
		return res, fmt.Errorf("run #%d has no verify or suite gate for adversarial review; a pair cannot become the arbiter", it.ID)
	}

	before, _ := gitOut(workspace, "rev-parse", "HEAD")
	before = strings.TrimSpace(before)
	task := fmt.Sprintf("Adversarially test run #%d (%s). Write and leave a failing test for every real defect; do not fix or approve the code.", it.ID, it.Title)
	args := []string{"pair", task, "--diff", diffRef, "--agent", reviewer, "--verify", gateCommand}
	cmd := exec.Command("bashy", args...)
	cmd.Dir = workspace
	out, runErr := cmd.CombinedOutput()
	res.Output = strings.TrimSpace(string(out))
	if runErr != nil {
		var ee *exec.ExitError
		if errors.As(runErr, &ee) {
			res.ExitCode = ee.ExitCode()
		} else {
			return res, fmt.Errorf("launch bashy pair: %w", runErr)
		}
	}

	// Pair evidence must survive the workspace. Commit whatever the acting pair
	// left; this does not approve it—the next verify/suite gate still decides.
	if committed, cerr := weaveCommitPairEvidence(workspace, fmt.Sprintf("weave: adversarial review evidence for run #%d by %s", it.ID, reviewer)); cerr != nil {
		return res, fmt.Errorf("commit pair evidence: %w", cerr)
	} else if committed {
		// The changed-path check below observes the new commit.
	}
	res.AddedTest = weaveReviewChangedTest(workspace, before)

	switch res.ExitCode {
	case 0, weavePairProvedExit, weavePairBrokenBeforeExit:
		return res, nil
	default:
		return res, fmt.Errorf("bashy pair failed (exit %d): %s", res.ExitCode, res.Output)
	}
}

func weaveCommitPairEvidence(workspace, message string) (bool, error) {
	weaveEnsureBashyExclude(workspace)
	dirty, _, untracked := weaveMeasureDirtiness(workspace)
	if !dirty && untracked == 0 {
		return false, nil
	}
	if out, err := exec.Command("git", "-C", workspace, "add", "-A").CombinedOutput(); err != nil {
		return false, fmt.Errorf("git add pair evidence: %w: %s", err, strings.TrimSpace(string(out)))
	}
	// A proof is intentionally red. A pre-commit hook that runs the suite must
	// not erase that durable evidence by refusing its commit.
	if out, err := exec.Command("git", "-C", workspace, "commit", "--no-verify", "-m", message).CombinedOutput(); err != nil {
		return false, fmt.Errorf("git commit pair evidence: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return true, nil
}

func weaveReviewChangedTest(workspace, before string) bool {
	if before == "" {
		return false
	}
	out, err := exec.Command("git", "-C", workspace, "diff", "--name-only", before+"...HEAD").Output()
	if err != nil {
		return false
	}
	for _, name := range strings.Fields(string(out)) {
		base := strings.ToLower(filepath.Base(name))
		if strings.HasSuffix(base, "_test.go") || strings.HasPrefix(base, "test_") ||
			strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") || strings.Contains(base, "_test.") ||
			strings.Contains(strings.ToLower(filepath.ToSlash(name)), "/test/") ||
			strings.Contains(strings.ToLower(filepath.ToSlash(name)), "/tests/") {
			return true
		}
	}
	return false
}
