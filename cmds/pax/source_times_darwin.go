//go:build darwin

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
	return time.Unix(st.Atimespec.Sec, st.Atimespec.Nsec), true
}

func restoreSourceTimes(path string, atime time.Time, symlink bool) error {
	// Darwin's <sys/stat.h> defines UTIME_OMIT as -2, but x/sys/unix does
	// not export the macro on this target.
	const utimeOmit = -2
	times := []unix.Timespec{
		unix.NsecToTimespec(atime.UnixNano()),
		{Nsec: utimeOmit},
	}
	flags := 0
	if symlink {
		flags = unix.AT_SYMLINK_NOFOLLOW
	}
	return unix.UtimesNanoAt(unix.AT_FDCWD, path, times, flags)
}
