//go:build windows

package lockfile

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// Lock a sentinel byte at offset 2^62. LockFileEx range locks are mandatory,
// so keeping the range past EOF leaves the holder JSON at byte zero readable
// by contenders.
const (
	lockOffsetLow  = 0
	lockOffsetHigh = 0x4000_0000
	lockLenLow     = 1
	lockLenHigh    = 0
)

func lockOverlapped() *windows.Overlapped {
	return &windows.Overlapped{Offset: lockOffsetLow, OffsetHigh: lockOffsetHigh}
}

func lockBlocking(f *os.File) (func() error, error) {
	h := windows.Handle(f.Fd())
	ol := lockOverlapped()
	if err := windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, lockLenLow, lockLenHigh, ol); err != nil {
		return nil, fmt.Errorf("lockfile: LockFileEx: %w", err)
	}
	return func() error {
		return windows.UnlockFileEx(h, 0, lockLenLow, lockLenHigh, ol)
	}, nil
}

func tryLock(f *os.File) (bool, func() error, error) {
	h := windows.Handle(f.Fd())
	ol := lockOverlapped()
	err := windows.LockFileEx(
		h,
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, lockLenLow, lockLenHigh, ol,
	)
	switch {
	case err == nil:
		return true, func() error {
			return windows.UnlockFileEx(h, 0, lockLenLow, lockLenHigh, ol)
		}, nil
	case errors.Is(err, windows.ERROR_LOCK_VIOLATION), errors.Is(err, windows.ERROR_IO_PENDING):
		return false, nil, nil
	default:
		return false, nil, fmt.Errorf("lockfile: LockFileEx: %w", err)
	}
}
