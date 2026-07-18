//go:build unix

package nicecmd

import (
	"os/exec"
	"syscall"
)

// signaledExitCode reports the POSIX shell exit code (128+signal) for a
// child that died from an unhandled signal, matching what nice's own exit
// status would be if it were the process to receive that signal.
func signaledExitCode(ee *exec.ExitError) (int, bool) {
	ws, ok := ee.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() {
		return 0, false
	}
	return 128 + int(ws.Signal()), true
}
