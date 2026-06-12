//go:build windows

package cpcmd

import (
	"os"
	"syscall"
	"time"
)

// atime returns the access time recorded in fi, falling back to the
// modification time when the platform data is unavailable.
func atime(fi os.FileInfo) time.Time {
	if st, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok {
		return time.Unix(0, st.LastAccessTime.Nanoseconds())
	}
	return fi.ModTime()
}
