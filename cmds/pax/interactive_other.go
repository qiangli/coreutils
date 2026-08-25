//go:build !unix

package paxcmd

import (
	"errors"
	"io"
)

func defaultOpenInteractiveTTY() (io.ReadWriteCloser, error) {
	return nil, errors.New("controlling terminal (/dev/tty) is not available on this platform")
}
