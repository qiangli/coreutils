//go:build linux

package cpcmd

import (
	"os"
	"syscall"
	"time"
)

// atime returns the access time recorded in fi. False means the platform stat
// representation is unavailable; callers must not substitute another time.
func atime(fi os.FileInfo) (time.Time, bool) {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return time.Unix(st.Atim.Sec, st.Atim.Nsec), true
	}
	return time.Time{}, false
}
