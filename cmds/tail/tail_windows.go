//go:build windows

package tailcmd

import "os"

func processExists(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil || p == nil {
		return false
	}
	return true
}

func inodeKey(fi os.FileInfo) uint64 {
	return 0
}
