//go:build windows

package lscmd

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

// sysTime on Windows: the access time is the last-access time and the
// birth time is the file's creation time. There is no POSIX
// status-change time; following cmds/stat, ctime maps to the last
// write time (documented platform note).
func sysTime(fi os.FileInfo, _ string, sel timeSel) (time.Time, error) {
	d, ok := fi.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return time.Time{}, fmt.Errorf("no system stat data")
	}
	switch sel {
	case selAtime:
		return time.Unix(0, d.LastAccessTime.Nanoseconds()), nil
	case selCtime:
		return time.Unix(0, d.LastWriteTime.Nanoseconds()), nil
	case selBirth:
		return time.Unix(0, d.CreationTime.Nanoseconds()), nil
	}
	return fi.ModTime(), nil
}
