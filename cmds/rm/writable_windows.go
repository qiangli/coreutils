//go:build windows

package rmcmd

import "os"

func isWritable(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return fi.Mode().Perm()&0222 != 0
}
