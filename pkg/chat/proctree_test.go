//go:build !windows

package chat

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The defect these encode: exec.CommandContext kills exactly one pid, so a
// wedged agent's grandchildren survived the per-turn deadline still holding the
// stdout pipe. The turn ran past its budget (the 2026-07-18 artifact shows a
// ycode turn that persisted exit 124 after 20 minutes) and the descendants
// orphaned.
//
// A real child tree is the only honest way to test this — a fake Runner never
// forks — so these use /bin/sh to build a two-level tree. They are Unix-only:
// process groups are the mechanism under test.

// alive reports whether a pid is still running. ESRCH means gone; EPERM means
// alive but not ours.
func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// waitGone waits for a pid to disappear, bounded by a deadline. A reap is
// inherently asynchronous — the kill is delivered, the process must then be
// scheduled and torn down — so this polls rather than sleeping a fixed guess,
// and fails at the deadline instead of hoping a sleep was long enough.
func waitGone(t *testing.T, pid int, within time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if !alive(pid) {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return !alive(pid)
}

// readPID waits for the helper to publish its grandchild's pid, bounded.
func readPID(t *testing.T, path string, within time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("helper never published a grandchild pid at %s", path)
	return 0
}

// hungTreeScript builds a shell command that forks a long-lived GRANDCHILD
// (which inherits our stdout pipe) and then hangs itself. Killing only the
// direct child leaves the grandchild holding the pipe — the exact shape that
// wedged a turn past its budget.
func hungTreeScript(pidFile string) string {
	return "sleep 120 & echo $! > " + pidFile + "; sleep 120"
}

//  3. A hung child must return within its budget, report a timeout, and take its
//     descendants with it.
func TestHungChildTimesOutAndKillsDescendants(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh on this host")
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "grandchild.pid")

	ctx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, exit, err := execRunner{}.Run(ctx, "sh", []string{"-c", hungTreeScript(pidFile)}, dir)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("a killed turn must report an error")
	}
	if exit != 124 {
		t.Errorf("exit = %d, want 124 (timeout)", exit)
	}
	// Bounded return. The pre-fix behaviour blocked on the grandchild's pipe
	// until WaitDelay (5s) expired; the group kill closes it immediately.
	if elapsed > 3*time.Second {
		t.Errorf("Run took %s — it waited on a grandchild's pipe instead of killing the tree", elapsed)
	}

	// The whole point: no orphan.
	grandchild := readPID(t, pidFile, 2*time.Second)
	if !waitGone(t, grandchild, 3*time.Second) {
		_ = syscall.Kill(grandchild, syscall.SIGKILL) // do not leak into the test host
		t.Errorf("grandchild %d survived the turn's deadline — it orphaned", grandchild)
	}
}

//  4. Caller cancellation is the same requirement by a different trigger: the
//     tree goes, and the call returns promptly.
func TestCancelKillsDescendants(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh on this host")
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "grandchild.pid")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var exit int
	var err error
	go func() {
		defer close(done)
		_, exit, err = execRunner{}.Run(ctx, "sh", []string{"-c", hungTreeScript(pidFile)}, dir)
	}()

	// Cancel only once the tree actually exists, so this tests teardown rather
	// than racing the launch.
	grandchild := readPID(t, pidFile, 5*time.Second)
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
	if err == nil {
		t.Error("a cancelled turn must report an error")
	}
	if exit != 124 {
		t.Errorf("exit = %d, want 124", exit)
	}
	if !waitGone(t, grandchild, 3*time.Second) {
		_ = syscall.Kill(grandchild, syscall.SIGKILL)
		t.Errorf("grandchild %d survived cancellation — it orphaned", grandchild)
	}
}

// The teardown must not change what a NORMAL turn returns. Putting the child in
// its own process group is invisible to a command that simply runs and exits.
func TestProcessGroupDoesNotChangeNormalRuns(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh on this host")
	}
	out, exit, err := execRunner{}.Run(context.Background(), "sh",
		[]string{"-c", "echo hello from the agent"}, t.TempDir())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if exit != 0 {
		t.Errorf("exit = %d, want 0", exit)
	}
	if strings.TrimSpace(out) != "hello from the agent" {
		t.Errorf("out = %q", out)
	}
}

// A non-zero exit is still classified as one, not swallowed by the new cancel
// path.
func TestNonZeroExitIsPreserved(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh on this host")
	}
	_, exit, err := execRunner{}.Run(context.Background(), "sh",
		[]string{"-c", "exit 3"}, t.TempDir())
	if err == nil {
		t.Fatal("a non-zero exit must report an error")
	}
	if exit != 3 {
		t.Errorf("exit = %d, want 3", exit)
	}
}

// setProcessGroup must actually take effect, or the group kill silently
// degrades to a single-pid kill and everything above passes for the wrong
// reason.
func TestSetProcessGroupPutsChildInItsOwnGroup(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh on this host")
	}
	cmd := exec.Command("sh", "-c", "sleep 5")
	setProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = killProcessTree(cmd); _, _ = cmd.Process.Wait() }()

	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("Getpgid: %v", err)
	}
	if pgid != cmd.Process.Pid {
		t.Errorf("child pgid = %d, want its own pid %d", pgid, cmd.Process.Pid)
	}
	if pgid == syscall.Getpgrp() {
		t.Error("child shares the parent's process group; a group kill would signal the test runner")
	}
}
