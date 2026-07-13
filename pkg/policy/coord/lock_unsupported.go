// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !windows && !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

package coord

import (
	"errors"
	"fmt"
	"os"
)

// ErrLockUnsupported is returned by every claim MUTATION on a platform with no advisory
// file locking (aix, solaris, js/wasm, plan9 — anything not in lock_unix.go's tag).
//
// This file exists because the flock build tag used to say `!windows`, which is a claim
// that every non-Windows OS is a unix with syscall.Flock. They are not — aix and solaris
// lock through fcntl, js/wasm and plan9 have nothing — so the package did not merely
// mislabel those targets, it FAILED TO COMPILE on them.
//
// The fix is not a no-op lock. Acquire must READ every claim, decide there is no conflict,
// and WRITE its own; if two agents interleave those steps, both conclude the project is
// free and both take it — the exact race this package exists to prevent, reproduced inside
// the mechanism meant to prevent it. A lock that silently does nothing is worse than no
// lock, because the caller believes it is protected.
//
// So a platform that cannot serialize cannot register a claim, and says so. Reads are
// unaffected: they never take the lock.
var ErrLockUnsupported = errors.New(
	"coord: this platform has no advisory file locking, so a claim's read-decide-write cycle cannot be serialized — " +
		"refusing to mutate rather than let two agents both acquire the same project")

func withLock(dir string, _ func() (*Claim, error)) (*Claim, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("claim lock: %w", err)
	}
	return nil, ErrLockUnsupported
}
