//go:build windows

package paxcmd

import (
	"errors"
	"os"
	"syscall"
	"time"
)

func sourceAccessTime(fi os.FileInfo) (time.Time, bool) {
	st, ok := fi.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return time.Time{}, false
	}
	return time.Unix(0, st.LastAccessTime.Nanoseconds()), true
}

func restoreSourceTimes(path string, atime, mtime time.Time, symlink bool) error {
	if symlink {
		return errors.New("restoring a symbolic link access time is not supported on windows")
	}
	return os.Chtimes(path, atime, mtime)
}
