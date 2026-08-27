//go:build darwin

package talkcmd

import "golang.org/x/sys/unix"

func ioctlTermiosForTest(fd int) (*unix.Termios, error) {
	return unix.IoctlGetTermios(fd, unix.TIOCGETA)
}
