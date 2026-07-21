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

// withWeaveQueueLockWait takes the exclusive queue lock (waiting at most
// `wait`), loads the queue, hands it to fn for mutation, saves it back, then
// releases. This is the ONLY sanctioned read-modify-write path.
//
// fn must be short. Anything that shells out to an agent, runs a suite gate or
// merges must happen outside this call and re-enter it to record the outcome —
// see the lock-discipline note in weave_lock_common.go.
func withWeaveQueueLockWait(dir string, wait time.Duration, fn func(*weaveQueue) error) error {
	release, err := weaveFlock(filepath.Join(dir, "queue.lock"), wait)
	if err != nil {
		if errors.Is(err, errWeaveQueueBusy) {
			return err
		}
		return fmt.Errorf("queue %w", err)
	}
	defer release()

	q, err := loadWeaveQueue(dir)
	if err != nil {
		return fmt.Errorf("queue lock: load: %w", err)
	}
	if err := fn(q); err != nil {
		return err
	}
	if err := saveWeaveQueue(dir, q); err != nil {
		return fmt.Errorf("queue lock: save: %w", err)
	}
	return nil
}

// withWeaveQueueLock is the ordinary write path: bounded wait, then
// load/mutate/save. See withWeaveQueueLockWait.
func withWeaveQueueLock(dir string, fn func(*weaveQueue) error) error {
	return withWeaveQueueLockWait(dir, weaveQueueLockWait, fn)
}

// withWeavePullLock guards the genuinely exclusive part of a pull: merging
// into the ONE shared live checkout. Non-blocking on purpose — a second caller
// is told to retry instead of blocking for the minutes an agent review takes.
// It does NOT hold queue.lock, so `weave list`/`add`/`comment` stay live for
// the whole merge cycle.
func withWeavePullLock(dir string, fn func() error) error {
	release, err := weaveFlock(filepath.Join(dir, "pull.lock"), 0)
	if err != nil {
		if errors.Is(err, errWeaveQueueBusy) {
			return errWeavePullBusy
		}
		return fmt.Errorf("pull %w", err)
	}
	defer release()
	return fn()
}
