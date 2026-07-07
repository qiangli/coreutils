//go:build darwin || linux

package touchcmd

import (
	"time"

	"golang.org/x/sys/unix"
)

func applyChtimesNoDeref(path string, atime, mtime time.Time) error {
	tv := []unix.Timeval{
		unix.NsecToTimeval(atime.UnixNano()),
		unix.NsecToTimeval(mtime.UnixNano()),
	}
	if atime.IsZero() && mtime.IsZero() {
		return unix.Lutimes(path, nil)
	}
	return unix.Lutimes(path, tv)
}
