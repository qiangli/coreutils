//go:build unix

package cpcmd

import (
	"time"

	"golang.org/x/sys/unix"
)

// preserveLinkTimes updates the link inode, never its referent.
func preserveLinkTimes(path string, atime, mtime time.Time) error {
	times := []unix.Timespec{
		{Sec: atime.Unix(), Nsec: int64(atime.Nanosecond())},
		{Sec: mtime.Unix(), Nsec: int64(mtime.Nanosecond())},
	}
	return unix.UtimesNanoAt(unix.AT_FDCWD, path, times, unix.AT_SYMLINK_NOFOLLOW)
}
