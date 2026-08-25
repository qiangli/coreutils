//go:build unix

package schedule

import (
	"os/exec"
	"syscall"
)

// applyBackgroundProcAttrs detaches the daemon into its own session, without a
// controlling terminal, and makes its PID the process-group ID used by stop.
func applyBackgroundProcAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// signalStop terminates the daemon's process group, then the process itself.
func signalStop(pid int) error {
	if pid <= 0 {
		return nil
	}
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	return syscall.Kill(pid, syscall.SIGTERM)
}

func signalKill(pid int) error {
	if pid <= 0 {
		return nil
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	return syscall.Kill(pid, syscall.SIGKILL)
}
