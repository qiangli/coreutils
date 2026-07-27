// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build windows

package coord

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// withLock serialises read-modify-write on the claim registry with a real,
// handle-scoped Windows lock. LockFileEx without FAIL_IMMEDIATELY blocks until
// the range is available, matching the unix LOCK_EX contract.
func withLock(dir string, fn func() (*Claim, error)) (*Claim, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("claim lock: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "claims.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("claim lock: %w", err)
	}
	defer f.Close()

	h := windows.Handle(f.Fd())
	ol := new(windows.Overlapped)
	if err := windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, ^uint32(0), ^uint32(0), ol); err != nil {
		return nil, fmt.Errorf("claim lock: %w", err)
	}
	defer func() { _ = windows.UnlockFileEx(h, 0, ^uint32(0), ^uint32(0), ol) }()
	return fn()
}
