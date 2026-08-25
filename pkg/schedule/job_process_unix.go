//go:build unix

package schedule

import (
	"os/exec"
	"syscall"
)

func applyJobProcAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
