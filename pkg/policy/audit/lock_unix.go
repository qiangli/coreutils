// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build linux || darwin

package audit

import (
	"os"

	"golang.org/x/sys/unix"
)

// lockFile takes an exclusive advisory lock on f, blocking until it is
// available, and returns a function that releases it. Concurrent bashy
// processes writing one shared audit log serialize on this, so the hash chain
// never forks and a line is never torn.
func lockFile(f *os.File) (func(), error) {
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		return nil, err
	}
	return func() { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN) }, nil
}
