//go:build darwin

package cpcmd

import (
	"os"
	"syscall"
	"time"
)

// atime returns the access time recorded in fi, falling back to the
// modification time when the platform data is unavailable.
func atime(fi os.FileInfo) time.Time {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return time.Unix(st.Atimespec.Sec, st.Atimespec.Nsec)
	}
	return fi.ModTime()
}
