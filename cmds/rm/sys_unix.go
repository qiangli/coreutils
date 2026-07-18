//go:build unix

package rmcmd

import "golang.org/x/sys/unix"

func isWriteProtected(path string) bool {
	return unix.Access(path, unix.W_OK) != nil
}
