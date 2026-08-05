//go:build !windows

package batchcmd

import "syscall"

func processUmask() (uint32, bool) {
	mask := syscall.Umask(0)
	syscall.Umask(mask)
	return uint32(mask), true
}
