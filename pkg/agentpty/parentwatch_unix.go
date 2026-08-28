//go:build !windows

package agentpty

import (
	"io"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

func startParentDeathWatch(childPID int) *exec.Cmd {
	watch := exec.Command("sh", "-c", `
parent=$1
group=$2
while kill -0 "$parent" 2>/dev/null; do
  state=$(ps -p "$parent" -o stat= 2>/dev/null)
  case "$state" in ""|Z*) break ;; esac
  sleep 0.05
done
kill -KILL -"$group" 2>/dev/null || true
`, "bashy-parent-watch", strconv.Itoa(os.Getpid()), strconv.Itoa(childPID))
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
