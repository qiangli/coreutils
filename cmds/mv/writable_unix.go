//go:build unix

package mvcmd

import "golang.org/x/sys/unix"

func isWritable(path string) bool {
	return unix.Access(path, unix.W_OK) == nil
}
