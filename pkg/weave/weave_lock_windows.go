//go:build windows

package weave

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

// weaveFlock takes an exclusive LockFileEx lock on byte 0 of path, waiting at
// most wait for it. It polls with LOCKFILE_FAIL_IMMEDIATELY rather than
// blocking in LockFileEx so the wait stays bounded: a holder that is simply
// slow must not wedge every other weave command in the repo forever.
//
// wait == 0 means one attempt, the try-lock used by pull.lock.
//
// pull.lock is a pure sentinel; its readable holder metadata lives in the
// separate weavePullHolderPath sidecar. Unlike meet's lease lock, this lock
// therefore does not need a range past EOF to leave byte-zero metadata
// readable.
//
// Returns errWeaveQueueBusy when the deadline passes with the lock still held.
// The returned func releases the handle-scoped lock and closes the file.
func weaveFlock(path string, wait time.Duration) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("lock: ensure dir: %w", err)
	}
	lf, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("lock: open: %w", err)
	}
	deadline := time.Now().Add(wait)
	for {
		ol := new(windows.Overlapped)
		err := windows.LockFileEx(
			windows.Handle(lf.Fd()),
			windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
			0, 1, 0, ol,
		)
		if err == nil {
			return func() {
				_ = windows.UnlockFileEx(windows.Handle(lf.Fd()), 0, 1, 0, ol)
				_ = lf.Close()
			}, nil
		}
		if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) &&
			!errors.Is(err, windows.ERROR_IO_PENDING) {
			_ = lf.Close()
			return nil, fmt.Errorf("lock: LockFileEx: %w", err)
		}
		if !time.Now().Before(deadline) {
			_ = lf.Close()
			return nil, errWeaveQueueBusy
		}
		time.Sleep(weaveQueueLockPoll)
	}
}
