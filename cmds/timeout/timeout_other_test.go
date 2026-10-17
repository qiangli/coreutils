//go:build !windows

package timeoutcmd

import (
	"os/exec"
	"syscall"
)

func isPidAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil
}

func killPid(pid int) {
	_ = syscall.Kill(pid, syscall.SIGKILL)
}

func setBreakawaySysProcAttr(cmd *exec.Cmd) {}
