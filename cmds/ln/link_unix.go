//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package lncmd

import "golang.org/x/sys/unix"

func hardLinkPhysical(targetPath, destPath string) error {
	return unix.Linkat(unix.AT_FDCWD, targetPath, unix.AT_FDCWD, destPath, 0)
}
