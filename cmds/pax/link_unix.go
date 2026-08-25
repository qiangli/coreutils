//go:build unix

package paxcmd

import "golang.org/x/sys/unix"

// linkat without AT_SYMLINK_FOLLOW links the source directory entry itself.
// That distinction is load-bearing for copy -l's default physical treatment
// of symbolic links; os.Link follows the source symlink on some Unix hosts.
func defaultLinkSource(source, target string) error {
	return unix.Linkat(unix.AT_FDCWD, source, unix.AT_FDCWD, target, 0)
}
