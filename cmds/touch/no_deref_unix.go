//go:build darwin || linux

package touchcmd

import (
	"time"

	"golang.org/x/sys/unix"
)

// setFileTimes updates the access and/or modification time of path via
// utimensat(2). changeA/changeM select which timestamps to write; the others
// are left untouched (UTIME_OMIT), which is how -a and -m preserve the sibling
// timestamp. When useNow is set the written timestamps are set to the current
// time with UTIME_NOW rather than an explicit value: POSIX requires plain
// "touch FILE" to succeed for anyone with write permission (as utime(path,
// NULL) does), whereas writing an explicit timestamp demands file ownership.
// follow chooses whether a symlink operand is dereferenced (default) or the
// link itself is retimed (-h/--no-dereference). utimensat also preserves
// nanosecond precision, which the older microsecond utimes(2) pair truncated.
func setFileTimes(path string, changeA, changeM, useNow bool, atime, mtime time.Time, follow bool) error {
	ts := []unix.Timespec{
		timespecFor(changeA, useNow, atime),
		timespecFor(changeM, useNow, mtime),
	}
	flags := 0
	if !follow {
		flags = unix.AT_SYMLINK_NOFOLLOW
	}
	return unix.UtimesNanoAt(unix.AT_FDCWD, path, ts, flags)
}

func timespecFor(change, useNow bool, t time.Time) unix.Timespec {
	switch {
	case !change:
		return unix.Timespec{Sec: 0, Nsec: utimeOmit}
	case useNow:
		return unix.Timespec{Sec: 0, Nsec: utimeNow}
	default:
		return unix.Timespec{Sec: t.Unix(), Nsec: int64(t.Nanosecond())}
	}
}
