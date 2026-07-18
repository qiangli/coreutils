//go:build !unix

package rmcmd

import "os"

func isWriteProtected(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return fi.Mode().Perm()&0222 == 0
}
