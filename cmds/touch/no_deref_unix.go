//go:build darwin || linux

package touchcmd

import (
	"time"

	"golang.org/x/sys/unix"
)

// applyChtimesNoDeref sets the times of path itself, never the symlink target.
// A zero atime/mtime means "leave that timestamp alone" (UTIME_OMIT), matching
// os.Chtimes' documented convention — touch -a and touch -m rely on it. Using
// utimensat also keeps nanosecond precision, which the older utimes(2) pair of
// microsecond timevals would have truncated.
func applyChtimesNoDeref(path string, atime, mtime time.Time) error {
	ts := []unix.Timespec{omitOrTimespec(atime), omitOrTimespec(mtime)}
	return unix.UtimesNanoAt(unix.AT_FDCWD, path, ts, unix.AT_SYMLINK_NOFOLLOW)
}

func omitOrTimespec(t time.Time) unix.Timespec {
	if t.IsZero() {
		return unix.Timespec{Sec: 0, Nsec: utimeOmit}
	}
	return unix.Timespec{Sec: t.Unix(), Nsec: int64(t.Nanosecond())}
}
