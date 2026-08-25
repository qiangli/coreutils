//go:build unix

package schedule

import (
	"os/exec"
	"syscall"
)

func applyJobProcAttrs(cmd *exec.Cmd) {
	// setsid(2) gives the at-job both properties required by POSIX: a new
	// process group and a new session with no controlling terminal. Setpgid
	// must not also be requested: a session leader cannot subsequently change
	// its process group.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
