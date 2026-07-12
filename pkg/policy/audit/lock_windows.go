// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build windows

package audit

import (
	"os"

	"golang.org/x/sys/windows"
)

// lockFile takes an exclusive lock on f, blocking until it is available, and
// returns a function that releases it. This is the Windows counterpart to the
// unix flock: concurrent bashy processes (and goroutines holding separate
// handles) writing one shared audit log serialize on it, so the hash chain
// never forks and a line is never torn.
//
// LockFileEx locks a byte range; the whole file (0..0xFFFFFFFFFFFFFFFF) is
// locked so any writer contends. Without LOCKFILE_FAIL_IMMEDIATELY the call
// blocks until the range is free — matching the unix LOCK_EX semantics.
func lockFile(f *os.File) (func(), error) {
	h := windows.Handle(f.Fd())
	ol := new(windows.Overlapped)
	if err := windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, ^uint32(0), ^uint32(0), ol); err != nil {
		return nil, err
	}
	return func() { _ = windows.UnlockFileEx(h, 0, ^uint32(0), ^uint32(0), ol) }, nil
}
