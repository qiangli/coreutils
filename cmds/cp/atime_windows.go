//go:build windows

package cpcmd

import (
	"os"
	"syscall"
	"time"
)

func atime(fi os.FileInfo) (time.Time, bool) {
	if st, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok {
		return time.Unix(0, st.LastAccessTime.Nanoseconds()), true
	}
	return time.Time{}, false
}
