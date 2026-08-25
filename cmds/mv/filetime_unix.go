//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly || solaris

package mvcmd

import (
	"os"

	"golang.org/x/sys/unix"
)

func preserveFileTimes(dst *os.File, fi os.FileInfo) error {
	times := []unix.Timeval{
		unix.NsecToTimeval(atime(fi).UnixNano()),
		unix.NsecToTimeval(fi.ModTime().UnixNano()),
	}
	return unix.Futimes(int(dst.Fd()), times)
}
