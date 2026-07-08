//go:build linux

package lscmd

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// sysTime returns fi's access, status-change, or birth time (mtime is
// served portably from os.FileInfo and never reaches here). Birth time
// needs statx(2); a filesystem that does not record it makes
// --time=birth fail loudly rather than approximate.
func sysTime(fi os.FileInfo, path string, sel timeSel) (time.Time, error) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, fmt.Errorf("no system stat data")
	}
	switch sel {
	case selAtime:
		return time.Unix(st.Atim.Unix()), nil
	case selCtime:
		return time.Unix(st.Ctim.Unix()), nil
	case selBirth:
		// Match how fi was obtained: stat the link itself only when fi
		// describes a symlink.
		flags := 0
		if fi.Mode()&os.ModeSymlink != 0 {
			flags = unix.AT_SYMLINK_NOFOLLOW
		}
		var stx unix.Statx_t
		if err := unix.Statx(unix.AT_FDCWD, path, flags, unix.STATX_BTIME, &stx); err != nil {
			return time.Time{}, err
		}
		if stx.Mask&unix.STATX_BTIME == 0 {
			return time.Time{}, fmt.Errorf("filesystem does not report a birth time")
		}
		return time.Unix(stx.Btime.Sec, int64(stx.Btime.Nsec)), nil
	}
	return fi.ModTime(), nil
}
