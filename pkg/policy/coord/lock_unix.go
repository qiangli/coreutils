// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !windows

package coord

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// withLock serialises read-modify-write on the claim registry with a real
// advisory file lock. Extracted from the same pattern weave has used for its queue,
// where it was written after "background N starts in parallel" produced
// last-write-wins races.
//
// The lock is essential precisely here: Acquire must READ every claim, decide there
// is no conflict, and WRITE its own — and if two agents interleave those steps, both
// conclude the project is free and both take it. That is the exact race this whole
// package exists to prevent, reproduced inside the mechanism meant to prevent it.
func withLock(dir string, fn func() (*Claim, error)) (*Claim, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("claim lock: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "claims.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("claim lock: %w", err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return nil, fmt.Errorf("claim lock: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}
