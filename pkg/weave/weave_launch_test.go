package weave

import (
	"strings"
	"testing"
)

// A conductor commonly starts weave from a detached, reviewed commit while a
// shared main ref advances under another campaign.  The workspace must fork
// the checkout the conductor actually used, not whichever commit `main`
// resolves to inside a local clone.
func TestWeaveStartPinsDetachedSourceHEAD(t *testing.T) {
	root := setupIsolationFixture(t)
	old := strings.TrimSpace(gitT(t, root, "rev-parse", "HEAD"))
	gitT(t, root, "checkout", "-q", "main")
	mustWrite(t, root+"/new-main.txt", "new\n")
	gitT(t, root, "add", "new-main.txt")
	gitT(t, root, "commit", "-qm", "advance main")
	gitT(t, root, "checkout", "-q", "--detach", old)
	t.Chdir(root)

	if _, code := runWeave(t, "add", "pin detached base", "--json"); code != 0 {
		t.Fatal("weave add failed")
	}
	if out, code := runWeave(t, "start", "--issue", "1", "--no-spawn", "--", "sh", "-c", "true"); code != 0 {
		t.Fatalf("weave start failed (exit %d): %s", code, out)
	}
	dir, _ := weaveQueueDir(root)
	q, err := loadWeaveQueue(dir)
	if err != nil {
		t.Fatal(err)
	}
	it := findWeaveItem(q, 1)
	if it == nil || it.BaseSHA != old {
		t.Fatalf("base SHA = %#v, want detached source HEAD %s", it, old)
	}
	// --no-spawn leaves the provisioned item allocated. This is the
	// pre-agent observability contract: a conductor can see the workspace,
	// immutable base, and current provisioning phase even if hydration is
	// still running in another start invocation.
	if it.State != "allocated" || it.Workspace == "" || it.LaunchPhase == "" {
		t.Fatalf("pre-agent provisioning was not durably observable: state=%q workspace=%q phase=%q", it.State, it.Workspace, it.LaunchPhase)
	}
	if got := strings.TrimSpace(gitT(t, it.Workspace, "rev-parse", "HEAD")); got != old {
		t.Fatalf("workspace HEAD = %s, want detached source HEAD %s", got, old)
	}
}

// Provisioning happens before an agent exists to report trouble. Its failure
// must therefore become durable queue state rather than returning to a silent
// todo item that looks like no worker was ever launched.
func TestWeaveProvisioningFailureIsTerminalAndObservable(t *testing.T) {
	root := setupIsolationFixture(t)
	t.Chdir(root)
	if _, code := runWeave(t, "add", "hydration timeout", "--json"); code != 0 {
		t.Fatal("weave add failed")
	}
	dir, _ := weaveQueueDir(root)
	weaveMarkLaunchFailed(dir, 1, errHydrationTestFailure{})
	q, err := loadWeaveQueue(dir)
	if err != nil {
		t.Fatal(err)
	}
	it := findWeaveItem(q, 1)
	if it == nil || it.State != "failed" || it.FinishedAt.IsZero() || !strings.Contains(it.LaunchPhase, "hydration test timeout") {
		t.Fatalf("provisioning failure was not durable terminal evidence: %#v", it)
	}
}

type errHydrationTestFailure struct{}

func (errHydrationTestFailure) Error() string { return "hydration test timeout" }
