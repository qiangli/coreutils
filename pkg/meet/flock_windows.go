//go:build windows

package meet

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// Windows locks byte RANGES, not whole files, so the lease is a single sentinel
// byte at a fixed offset every holder agrees on.
//
// That offset is far past EOF deliberately. Locking beyond the end of a file is
// legal, and keeping the locked region disjoint from byte 0 leaves the
// diagnostic metadata readable by contenders — on Windows a mandatory range
// lock would otherwise fail their os.ReadFile with ERROR_LOCK_VIOLATION, and
// every busy message would lose the owner's identity.
const (
	lockOffsetLow  = 0
	lockOffsetHigh = 0x4000_0000 // byte 2^62
	lockLenLow     = 1
	lockLenHigh    = 0
)

func lockOverlapped() *windows.Overlapped {
	return &windows.Overlapped{Offset: lockOffsetLow, OffsetHigh: lockOffsetHigh}
}

// tryLockFile takes an exclusive advisory lock on f without blocking. It
// reports whether the lock was granted; an error means the attempt itself
// failed, which is distinct from losing to another holder.
//
// LockFileEx ties the lock to the HANDLE, matching flock's descriptor
// ownership on unix: the lock disappears when the handle closes or the process
// dies, and a second handle in the same process contends normally.
func tryLockFile(f *os.File) (bool, error) {
	err := windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, lockLenLow, lockLenHigh, lockOverlapped(),
	)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, windows.ERROR_LOCK_VIOLATION), errors.Is(err, windows.ERROR_IO_PENDING):
		return false, nil // held by someone else
	default:
		return false, err
	}
}

// unlockFile releases the lock held by this handle.
func unlockFile(f *os.File) error {
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, lockLenLow, lockLenHigh, lockOverlapped())
}
