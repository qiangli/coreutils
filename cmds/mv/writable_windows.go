//go:build windows

package mvcmd

import "os"

func isWritable(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return fi.Mode().Perm()&0222 != 0
}
