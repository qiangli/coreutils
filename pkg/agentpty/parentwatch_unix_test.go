//go:build !windows

package agentpty

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestParentExitKillsPTYTree covers the abrupt supervisor-death path used by
// bounded Meet turns. Context cancellation and Run's defers cannot execute
// after SIGKILL, so the pipe-EOF watcher must remove both the PTY leader and a
// descendant which inherited its process group.
func TestParentExitKillsPTYTree(t *testing.T) {
	if os.Getenv("BASHY_AGENTPTY_PARENT_EXIT_HELPER") == "1" {
		dir := os.Getenv("BASHY_AGENTPTY_PARENT_EXIT_DIR")
		cmd := exec.Command("sh", "-c", "echo $$ > leader.pid; sleep 120 & echo $! > child.pid; wait")
		cmd.Dir = dir
		_, _, _ = Run(cmd, io.Discard, Options{Capture: true, KillOnParentExit: true})
		return
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh on this host")
	}

	dir := t.TempDir()
	helper := exec.Command(os.Args[0], "-test.run=^TestParentExitKillsPTYTree$", "-test.v=false")
	helper.Env = append(os.Environ(),
		"BASHY_AGENTPTY_PARENT_EXIT_HELPER=1",
		"BASHY_AGENTPTY_PARENT_EXIT_DIR="+dir,
	)
	if err := helper.Start(); err != nil {
		t.Fatal(err)
	}
	leader := readParentWatchPID(t, filepath.Join(dir, "leader.pid"), 5*time.Second)
	child := readParentWatchPID(t, filepath.Join(dir, "child.pid"), 5*time.Second)
	defer func() {
		_ = syscall.Kill(leader, syscall.SIGKILL)
		_ = syscall.Kill(child, syscall.SIGKILL)
	}()

	if err := helper.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := helper.Wait(); err == nil {
		t.Fatal("helper should be killed")
	}
	if !waitParentWatchGone(leader, 5*time.Second) {
		t.Errorf("PTY leader %d survived its supervisor dying", leader)
	}
	if !waitParentWatchGone(child, 5*time.Second) {
		t.Errorf("PTY child %d survived its supervisor dying", child)
	}
}

func TestStopParentDeathWatchReapsWatcher(t *testing.T) {
	child := exec.Command("sh", "-c", "sleep 30")
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = syscall.Kill(-child.Process.Pid, syscall.SIGKILL)
		_ = child.Wait()
	}()

	watch := startParentDeathWatch(child.Process.Pid)
	if watch == nil {
		t.Fatal("parent-death watcher did not start")
	}
	stopParentDeathWatch(watch)
	if watch.cmd.ProcessState == nil {
		t.Fatal("normal teardown did not wait for the parent-death watcher")
	}
	if _, err := watch.retain.Write([]byte("still open")); err == nil {
		t.Fatal("normal teardown left the EOF-retention pipe open")
	}
}

func readParentWatchPID(t *testing.T, path string, within time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if raw, err := os.ReadFile(path); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil && pid > 1 {
				return pid
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("helper did not publish pid at %s", path)
	return 0
}

func waitParentWatchGone(pid int, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) == syscall.ESRCH {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return syscall.Kill(pid, 0) == syscall.ESRCH
}
