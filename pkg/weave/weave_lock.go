//go:build !windows

package weave

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// weaveFlock takes an exclusive flock on path, waiting at most `wait` for it.
// It polls with LOCK_NB rather than blocking in LOCK_EX so the wait is
// bounded: a holder that died without releasing (or one that is simply slow)
// can no longer wedge every other weave command in the repo forever.
//
// wait == 0 means "one attempt" — the try-lock used by pull.lock.
//
// Returns errWeaveQueueBusy when the deadline passes with the lock still held.
// The returned func releases the lock and closes the file.
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
		err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() {
				_ = syscall.Flock(int(lf.Fd()), syscall.LOCK_UN)
				_ = lf.Close()
			}, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			_ = lf.Close()
			return nil, fmt.Errorf("lock: flock: %w", err)
		}
		if !time.Now().Before(deadline) {
			_ = lf.Close()
			return nil, errWeaveQueueBusy
		}
		time.Sleep(weaveQueueLockPoll)
	}
}
