//go:build unix

package mvcmd

import "golang.org/x/sys/unix"

func isWritable(path string) bool {
	if unix.Access(path, unix.W_OK) != nil {
		return false
	}
	var stat unix.Stat_t
	if unix.Stat(path, &stat) == nil {
		if stat.Mode&0222 == 0 {
			return false
		}
	}
	return true
}
