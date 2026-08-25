//go:build aix || linux || darwin || freebsd || openbsd || netbsd || dragonfly || solaris

package mvcmd

import (
	"os"

	"golang.org/x/sys/unix"
)

func preserveLinkMode(dst string, fi os.FileInfo) error {
	want := fi.Mode().Perm()
	current, err := os.Lstat(dst)
	if err != nil {
		return err
	}
	if current.Mode().Perm() == want {
		return nil
	}
	return unix.Fchmodat(unix.AT_FDCWD, dst, uint32(want), unix.AT_SYMLINK_NOFOLLOW)
}
