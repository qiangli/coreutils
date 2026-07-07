//go:build unix

package mkfifocmd

import "golang.org/x/sys/unix"

func makeFIFO(path string, mode uint32) error {
	return unix.Mkfifo(path, mode)
}
