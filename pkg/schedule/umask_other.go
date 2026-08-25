//go:build !unix

package schedule

import "os/exec"

func commandWithUmask(command []string, _ uint32, _ bool) *exec.Cmd {
	return exec.Command(command[0], command[1:]...)
}
