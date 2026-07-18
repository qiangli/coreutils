package weave

import (
	"strings"
	"testing"
	"time"
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

// An external kill can happen while clone/hydration is running, before the
// item becomes working.  That must not leave a permanently allocated phantom
// worker with no wrapper and no terminal evidence.
func TestWeaveStatusRecoversDeadProvisioningLauncher(t *testing.T) {
	root := setupIsolationFixture(t)
	t.Chdir(root)
	dir, _ := weaveQueueDir(root)
	if err := saveWeaveQueue(dir, &weaveQueue{Root: root, Items: []*weaveItem{{
		ID:          1,
		Title:       "orphaned hydration",
		State:       "allocated",
		Workspace:   root + "/workspace",
		BaseSHA:     "immutable-base",
		LaunchPhase: "hydrating submodules",
		WrapperPid:  2147483647, // deliberately impossible PID on supported hosts
	}}}); err != nil {
		t.Fatal(err)
	}
	out, code := runWeave(t, "status", "1", "--json")
	if code != 0 {
		t.Fatalf("status failed (exit %d): %s", code, out)
	}
	for _, want := range []string{
		`"state": "failed"`,
		`"launch_phase": "failed: provisioning launcher exited before agent launch"`,
		`"base_sha": "immutable-base"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("status missing %s:\n%s", want, out)
		}
	}
	q, err := loadWeaveQueue(dir)
	if err != nil {
		t.Fatal(err)
	}
	it := findWeaveItem(q, 1)
	if it == nil || it.FinishedAt.IsZero() || it.WrapperPid != 0 {
		t.Fatalf("orphan recovery was not durable: %#v", it)
	}
}

// Older launchers recorded allocated/hydrating state before they recorded a
// wrapper PID. Once the bounded provisioning interval elapses that is durable
// orphan evidence, but deliberate no-spawn/manual allocations remain available
// because they carry no active provisioning start time.
func TestWeaveRecoverOrphanedAllocationsWithoutPID(t *testing.T) {
	root := setupIsolationFixture(t)
	dir, _ := weaveQueueDir(root)
	if err := saveWeaveQueue(dir, &weaveQueue{Root: root, Items: []*weaveItem{
		{
			ID:          1,
			Title:       "old hydrating launcher disappeared",
			State:       "allocated",
			LaunchPhase: "hydrating submodules",
			StartedAt:   time.Now().UTC().Add(-weaveProvisioningTimeout - time.Second),
		},
		{
			ID:          2,
			Title:       "recent hydrating launcher",
			State:       "allocated",
			LaunchPhase: "hydrating submodules",
			StartedAt:   time.Now().UTC(),
		},
		{
			ID:          3,
			Title:       "manual allocation",
			State:       "allocated",
			LaunchPhase: "hydrating submodules",
		},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := weaveRecoverOrphanedAllocations(dir); err != nil {
		t.Fatal(err)
	}
	q, err := loadWeaveQueue(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := findWeaveItem(q, 1); got == nil || got.State != "failed" || got.FinishedAt.IsZero() {
		t.Fatalf("missing-PID hydrating allocation was not recovered: %#v", got)
	}
	if got := findWeaveItem(q, 2); got == nil || got.State != "allocated" || !got.FinishedAt.IsZero() {
		t.Fatalf("recent missing-PID allocation must remain observable: %#v", got)
	}
	if got := findWeaveItem(q, 3); got == nil || got.State != "allocated" || !got.FinishedAt.IsZero() {
		t.Fatalf("manual allocation must remain resumable: %#v", got)
	}
}
