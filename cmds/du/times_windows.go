//go:build windows

package ducmd

import (
	"os"
	"syscall"
	"time"
)

const haveSysTimes = true

// sysTime on Windows: the access time is the last-access time. There
// is no POSIX status-change time; following cmds/stat, ctime maps to
// the last write time (documented platform note).
func sysTime(fi os.FileInfo, sel timeSel) (time.Time, bool) {
	d, ok := fi.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return time.Time{}, false
	}
	switch sel {
	case timeAtime:
		return time.Unix(0, d.LastAccessTime.Nanoseconds()), true
	case timeCtime:
		return time.Unix(0, d.LastWriteTime.Nanoseconds()), true
	}
	return time.Time{}, false
}
