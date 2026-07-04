//go:build unix

package sdlc

import (
	"os/exec"
	"syscall"
)

func applyBackgroundProcAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// signalStop asks the process (and its group, since Serve is a group leader via
// Setpgid) to terminate.
func signalStop(pid int) error {
	if pid <= 0 {
		return nil
	}
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	return syscall.Kill(pid, syscall.SIGTERM)
}
