// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly || solaris || aix

package steward

import (
	"os"

	"golang.org/x/sys/unix"
)

// lockFile takes an exclusive advisory lock on f, blocking until it is available,
// and returns a function that releases it.
func lockFile(f *os.File) (func(), error) {
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		return nil, err
	}
	return func() { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN) }, nil
}

// LockSupported reports whether this platform can host a steward seat. Mutations
// fail closed where it is false; reads still work.
func LockSupported() bool { return true }
