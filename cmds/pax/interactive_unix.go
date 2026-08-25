//go:build unix

package paxcmd

import (
	"io"
	"os"
	"syscall"
)

func defaultOpenInteractiveTTY() (io.ReadWriteCloser, error) {
	return os.OpenFile("/dev/tty", os.O_RDWR|syscall.O_NOCTTY, 0)
}
