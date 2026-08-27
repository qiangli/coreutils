//go:build unix

package cpcmd

import (
	"time"

	"golang.org/x/sys/unix"
)

// preserveLinkTimes updates the link inode, never its referent.
func preserveLinkTimes(path string, atime, mtime time.Time) error {
	times := []unix.Timespec{
		unix.NsecToTimespec(atime.UnixNano()),
		unix.NsecToTimespec(mtime.UnixNano()),
	}
	return unix.UtimesNanoAt(unix.AT_FDCWD, path, times, unix.AT_SYMLINK_NOFOLLOW)
}
