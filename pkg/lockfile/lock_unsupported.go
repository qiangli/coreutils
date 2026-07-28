//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly && !solaris && !windows

package lockfile

import (
	"errors"
	"os"
)

var errUnsupported = errors.New("lockfile: advisory file locking is unsupported on this platform")

func lockBlocking(*os.File) (func() error, error) { return nil, errUnsupported }

func tryLock(*os.File) (bool, func() error, error) { return false, nil, errUnsupported }
