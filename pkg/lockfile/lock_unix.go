//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly || solaris

package lockfile

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func lockBlocking(f *os.File) (func() error, error) {
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		return nil, fmt.Errorf("lockfile: flock: %w", err)
	}
	return func() error { return unix.Flock(int(f.Fd()), unix.LOCK_UN) }, nil
}

func tryLock(f *os.File) (bool, func() error, error) {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	switch {
	case err == nil:
		return true, func() error { return unix.Flock(int(f.Fd()), unix.LOCK_UN) }, nil
	case errors.Is(err, unix.EWOULDBLOCK), errors.Is(err, unix.EAGAIN), errors.Is(err, unix.EACCES):
		return false, nil, nil
	default:
		return false, nil, fmt.Errorf("lockfile: flock: %w", err)
	}
}
