// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly || solaris

package steward

import (
	"os"

	"golang.org/x/sys/unix"
)

// aix is deliberately NOT in the tag above, and the omission is the point.
//
// It was listed, on the reasonable-sounding grounds that aix is a unix. But listing a
// platform here is a claim that unix.Flock WORKS there, and a platform that compiles the
// call but cannot honour it gets the one outcome this package refuses: a lock that
// silently does nothing, a read-decide-write cycle that is not serialized, and two
// stewards on one host each believing they are the only one. The fencing epoch does not
// save that case either — both claims mint their epoch from the same replayed head, so
// they COLLIDE rather than supersede, and neither steward is fenced.
//
// A platform earns its place in this tag by being TESTED, not by being a unix. Anything
// not listed falls through to lock_unsupported.go and fails closed, loudly, which is the
// honest answer for a platform nobody has checked: reads keep working, mutations refuse.
// Adding aix back is a one-line change for whoever has an aix box to prove it on.

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
