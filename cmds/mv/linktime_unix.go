//go:build aix || linux || darwin || freebsd || openbsd || netbsd || dragonfly || solaris

package mvcmd

import (
	"os"

	"golang.org/x/sys/unix"
)

func preserveLinkTimes(dst string, fi os.FileInfo) error {
	times := []unix.Timeval{
		unix.NsecToTimeval(atime(fi).UnixNano()),
		unix.NsecToTimeval(fi.ModTime().UnixNano()),
	}
	return unix.Lutimes(dst, times)
}
