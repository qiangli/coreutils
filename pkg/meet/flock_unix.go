//go:build !windows

package meet

import (
	"errors"
	"os"
	"syscall"
)

// tryLockFile takes an exclusive advisory lock on f without blocking. It
// reports whether the lock was granted; an error means the attempt itself
// failed, which is distinct from losing to another holder.
//
// flock — not fcntl/POSIX record locking — because the lock must be held by the
// DESCRIPTOR. POSIX locks are owned by the process, so a second acquisition
// within one process would silently succeed (and closing any descriptor to the
// file would drop every lock the process held on it). flock's per-open-file-
// description ownership is exactly the semantics a lease needs: two runners in
// one process contend the same as two runners in two processes.
func tryLockFile(f *os.File) (bool, error) {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, syscall.EWOULDBLOCK), errors.Is(err, syscall.EAGAIN), errors.Is(err, syscall.EACCES):
		return false, nil // held by someone else
	default:
		return false, err
	}
}

// unlockFile releases the lock held by this descriptor. Closing f would release
// it too; unlocking first makes the intent explicit and keeps Release correct
// even if a caller later holds the file open for another reason.
func unlockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
