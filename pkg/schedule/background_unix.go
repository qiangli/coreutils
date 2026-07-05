//go:build !windows

package schedule

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// applyBackgroundProcAttrs detaches the daemon into its own process group so a
// closed shell / parent exit doesn't take it down, and so signalStop can reap the
// whole group.
func applyBackgroundProcAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// processAlive reports whether pid is a live process. On unix os.FindProcess
// always succeeds, so probe with signal 0 (EPERM still means it exists).
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

// signalStop terminates the daemon's process group, then the process itself.
func signalStop(pid int) error {
	if pid <= 0 {
		return nil
	}
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	return syscall.Kill(pid, syscall.SIGTERM)
}
