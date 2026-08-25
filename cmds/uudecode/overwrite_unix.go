//go:build darwin || linux || freebsd || netbsd || openbsd

package uudecodecmd

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func openWritableRegular(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_WRONLY|unix.O_NONBLOCK|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("output changed while opening and is not a regular file")
	}
	return os.NewFile(uintptr(fd), path), nil
}

// checkOverwrite intentionally only stats/access-checks the target. Opening a
// FIFO or device merely to ask whether it can be replaced can block or trigger
// device side effects. AT_EACCESS asks for the effective credentials where the
// platform supports them.
func checkOverwrite(path string) error {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return unix.Faccessat(unix.AT_FDCWD, path, unix.W_OK, unix.AT_EACCESS)
}
