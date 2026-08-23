//go:build unix

package mesgcmd

import (
	"io/fs"
	"syscall"
)

func deviceOf(fi fs.FileInfo) uint64 {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0
	}
	return uint64(st.Rdev)
}
