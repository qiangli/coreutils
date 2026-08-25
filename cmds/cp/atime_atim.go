//go:build aix || dragonfly || openbsd || solaris

package cpcmd

import (
	"os"
	"syscall"
	"time"
)

func atime(fi os.FileInfo) (time.Time, bool) {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return time.Unix(int64(st.Atim.Sec), int64(st.Atim.Nsec)), true
	}
	return time.Time{}, false
}
