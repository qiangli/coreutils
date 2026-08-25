//go:build linux

package paxcmd

import (
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func sourceAccessTime(fi os.FileInfo) (time.Time, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, false
	}
	return time.Unix(st.Atim.Sec, st.Atim.Nsec), true
}

func restoreSourceTimes(path string, atime time.Time, symlink bool) error {
	times := []unix.Timespec{
		unix.NsecToTimespec(atime.UnixNano()),
		{Nsec: unix.UTIME_OMIT},
	}
	flags := 0
	if symlink {
		flags = unix.AT_SYMLINK_NOFOLLOW
	}
	return unix.UtimesNanoAt(unix.AT_FDCWD, path, times, flags)
}
