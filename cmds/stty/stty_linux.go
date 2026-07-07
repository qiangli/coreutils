//go:build linux

package sttycmd

import "golang.org/x/sys/unix"

func applyMode(fd int, mode string) error {
	t, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return err
	}
	applyTermiosMode(t, mode)
	return unix.IoctlSetTermios(fd, unix.TCSETS, t)
}

func applyValue(fd int, name string, value uint8) error {
	t, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return err
	}
	applyTermiosValue(t, name, value)
	return unix.IoctlSetTermios(fd, unix.TCSETS, t)
}
