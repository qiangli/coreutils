//go:build windows

package schedule

import "os/exec"

// Windows has no POSIX process umask. Keep execution behavior unchanged; the
// captured value remains portable state if the job store moves to a POSIX host.
func commandWithUmask(command []string, _ uint32, _ bool) *exec.Cmd {
	return exec.Command(command[0], command[1:]...)
}
