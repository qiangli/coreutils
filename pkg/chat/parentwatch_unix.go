//go:build !windows

package chat

import (
	"io"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

// startParentDeathWatch puts a tiny watcher in the child's process group. If
// the meet process is killed (including SIGKILL, which cannot be handled), the
// watcher notices that the recorded parent PID is gone and kills the group.
// This is needed on macOS as well as Linux: unlike Linux, Darwin has no
// portable Go SysProcAttr parent-death signal.
func startParentDeathWatch(childPID int) *exec.Cmd {
	parentPID := os.Getpid()
	watch := exec.Command("sh", "-c", `
parent=$1
group=$2
while kill -0 "$parent" 2>/dev/null; do
  state=$(ps -p "$parent" -o stat= 2>/dev/null)
  case "$state" in
    ""|Z*) break ;;
  esac
  sleep 0.05
done
kill -KILL -"$group" 2>/dev/null || true
`, "bashy-parent-watch", strconv.Itoa(parentPID), strconv.Itoa(childPID))
	// Keep the watcher out of the participant group. It must survive long
	// enough to observe the participant's parent becoming a zombie, then signal
	// the participant group and exit itself. Normal cancellation stops it.
	watch.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	watch.Stdout = io.Discard
	watch.Stderr = io.Discard
	if err := watch.Start(); err != nil {
		return nil
	}
	return watch
}

func stopParentDeathWatch(watch *exec.Cmd) {
	if watch == nil || watch.Process == nil {
		return
	}
	_ = watch.Process.Kill()
	_ = watch.Wait()
}
