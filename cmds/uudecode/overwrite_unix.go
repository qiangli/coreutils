//go:build darwin || linux || freebsd || netbsd || openbsd

package uudecodecmd

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

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
