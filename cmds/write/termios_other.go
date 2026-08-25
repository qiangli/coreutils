//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd

package writecmd

import (
	"errors"

	"golang.org/x/sys/unix"
)

func getTermios(fd int) (*unix.Termios, error) {
	return nil, errors.New("termios unavailable")
}
