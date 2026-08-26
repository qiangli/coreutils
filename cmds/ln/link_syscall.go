//go:build aix || solaris

package lncmd

import "syscall"

// AIX and Solaris expose the POSIX link(2) operation through syscall rather
// than x/sys/unix.Linkat. POSIX link(2) links the source directory entry
// itself, so it provides the required -P behavior for a symbolic-link source.
func hardLinkPhysical(targetPath, destPath string) error {
	return syscall.Link(targetPath, destPath)
}
