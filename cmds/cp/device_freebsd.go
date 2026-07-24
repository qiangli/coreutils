//go:build freebsd

package cpcmd

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func deviceNumber(fi os.FileInfo) (uint64, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(st.Rdev), true
}

func makeDeviceNode(path string, mode uint32, device uint64) error {
	return unix.Mknod(path, mode, device)
}
