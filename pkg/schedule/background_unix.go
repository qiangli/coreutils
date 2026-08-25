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
