//go:build darwin || freebsd || netbsd || openbsd

package sttycmd

import "golang.org/x/sys/unix"

func applyMode(fd int, mode string) error {
	t, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if err != nil {
		return err
	}
	applyTermiosMode(t, mode)
	return unix.IoctlSetTermios(fd, unix.TIOCSETA, t)
}

func applyValue(fd int, name string, value uint8) error {
	t, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if err != nil {
		return err
	}
	applyTermiosValue(t, name, value)
	return unix.IoctlSetTermios(fd, unix.TIOCSETA, t)
}
