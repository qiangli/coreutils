//go:build !unix

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

func signalKill(pid int) error { return signalStop(pid) }
