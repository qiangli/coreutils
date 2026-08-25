//go:build aix || openbsd || dragonfly || solaris

package mvcmd

import (
	"os"
	"syscall"
	"time"
)

func atime(fi os.FileInfo) time.Time {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return time.Unix(st.Atim.Sec, int64(st.Atim.Nsec))
	}
	return fi.ModTime()
}
