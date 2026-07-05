//go:build windows

package schedule

import (
	"os"
	"os/exec"
)

// applyBackgroundProcAttrs is a no-op on non-unix (no process groups).
func applyBackgroundProcAttrs(cmd *exec.Cmd) {}

// signalStop terminates the process (no process groups on non-unix).
func signalStop(pid int) error {
	if pid <= 0 {
		return nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}

// processAlive reports whether pid is a live process. os.FindProcess on Windows
// fails for a dead pid, so a successful lookup is a good-enough liveness signal.
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = proc.Release()
	return true
}
