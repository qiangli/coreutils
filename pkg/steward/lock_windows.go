// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build windows

package steward

import (
	"os"

	"golang.org/x/sys/windows"
)

// lockFile takes an exclusive lock on f, blocking until it is available, and
// returns a function that releases it.
//
// This is a REAL lock, not a best-effort no-op. The older claim registry
// (pkg/policy/coord) documents an honest Windows gap — no flock, so two agents
// that interleave read-decide-write can both conclude a project is free — and the
// steward seat cannot afford that gap: the entire contract is that exactly one seat
// exists per host, and a singleton enforced by a racy acquisition is not a
// singleton. pkg/policy/audit already proved LockFileEx works for this, so the seat
// uses it rather than inheriting an apology.
//
// LockFileEx locks a byte range; the whole file (0..0xFFFFFFFFFFFFFFFF) is locked
// so any writer contends. Without LOCKFILE_FAIL_IMMEDIATELY the call blocks until
// the range is free, matching the unix LOCK_EX semantics.
func lockFile(f *os.File) (func(), error) {
	h := windows.Handle(f.Fd())
	ol := new(windows.Overlapped)
	if err := windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, ^uint32(0), ^uint32(0), ol); err != nil {
		return nil, err
	}
	return func() { _ = windows.UnlockFileEx(h, 0, ^uint32(0), ^uint32(0), ol) }, nil
}

// LockSupported reports whether this platform can host a steward seat.
func LockSupported() bool { return true }
