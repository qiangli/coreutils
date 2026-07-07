//go:build !windows

package tailcmd

import (
	"os"
	"syscall"
)

func processExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil
}

func inodeKey(fi os.FileInfo) uint64 {
	return 0
}
