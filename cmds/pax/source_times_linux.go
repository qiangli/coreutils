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

func restoreSourceTimes(path string, atime, mtime time.Time, symlink bool) error {
	if symlink {
		times := []unix.Timespec{unix.NsecToTimespec(atime.UnixNano()), unix.NsecToTimespec(mtime.UnixNano())}
		return unix.UtimesNanoAt(unix.AT_FDCWD, path, times, unix.AT_SYMLINK_NOFOLLOW)
	}
	return os.Chtimes(path, atime, mtime)
}
