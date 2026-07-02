//go:build unix

package sdlc

import (
	"os/exec"
	"syscall"
)

func applyBackgroundProcAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
