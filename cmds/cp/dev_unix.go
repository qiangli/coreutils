//go:build unix

package cpcmd

import (
	"os"
	"syscall"
)

type devID uint64

func fileDev(fi os.FileInfo) (devID, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return devID(st.Dev), true
}
