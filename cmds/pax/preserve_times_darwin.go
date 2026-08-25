//go:build darwin

package paxcmd

import (
	"time"

	"golang.org/x/sys/unix"
)

func defaultSetExtractedTimes(path string, atime time.Time, setA bool, mtime time.Time, setM, symlink bool) error {
	const utimeOmit = -2 // Darwin <sys/stat.h>; x/sys does not export it.
	times := []unix.Timespec{{Nsec: utimeOmit}, {Nsec: utimeOmit}}
	if setA {
		times[0] = unix.NsecToTimespec(atime.UnixNano())
	}
	if setM {
		times[1] = unix.NsecToTimespec(mtime.UnixNano())
	}
	flags := 0
	if symlink {
		flags = unix.AT_SYMLINK_NOFOLLOW
	}
	return unix.UtimesNanoAt(unix.AT_FDCWD, path, times, flags)
}
