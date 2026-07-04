//go:build !unix

package sdlc

import (
	"os"
	"os/exec"
)

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
