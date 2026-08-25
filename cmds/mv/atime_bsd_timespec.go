//go:build freebsd || netbsd

package mvcmd

import (
	"os"
	"syscall"
	"time"
)

func atime(fi os.FileInfo) time.Time {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return time.Unix(st.Atimespec.Sec, st.Atimespec.Nsec)
	}
	return fi.ModTime()
}
