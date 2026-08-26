//go:build unix

package paxcmd

import "golang.org/x/sys/unix"

func fifoSupported() bool { return true }

func makeFIFO(path string, mode uint32) error {
	return unix.Mkfifo(path, mode)
}
