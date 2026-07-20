//go:build !darwin && !linux

package touchcmd

import (
	"errors"
	"os"
	"time"
)

// setFileTimes has no symlink-aware or UTIME_NOW primitive on this platform, so
// it falls back to os.Chtimes. A zero time.Time leaves that timestamp alone,
// matching os.Chtimes' documented convention, so -a and -m still preserve the
// sibling timestamp. When useNow is set the current time is substituted for the
// written timestamps (the ownership subtlety UTIME_NOW addresses on unix does
// not map cleanly here). os.Chtimes follows symlinks, so a genuine symlink
// operand under -h fails loudly rather than retiming the wrong inode; a plain
// file (where -h is a no-op) still works.
func setFileTimes(path string, changeA, changeM, useNow bool, atime, mtime time.Time, follow bool) error {
	if !follow {
		if fi, err := os.Lstat(path); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			return errors.New("--no-dereference on a symbolic link is not supported on this platform")
		}
	}
	if useNow {
		now := time.Now()
		atime, mtime = now, now
	}
	at, mt := atime, mtime
	if !changeA {
		at = time.Time{}
	}
	if !changeM {
		mt = time.Time{}
	}
	return os.Chtimes(path, at, mt)
}
