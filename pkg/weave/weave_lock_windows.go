//go:build windows

package weave

import (
	"fmt"
	"time"
)

// withWeaveQueueLockWait on Windows is best-effort: we simply load, mutate,
// save without an OS-level mutex. The MVP orchestrator flow targets unix;
// concurrent weave writes on Windows have undefined behavior pending a real
// LockFileEx implementation. The `wait` argument is accepted so the call sites
// are identical across platforms.
func withWeaveQueueLockWait(dir string, wait time.Duration, fn func(*weaveQueue) error) error {
	_ = wait
	q, err := loadWeaveQueue(dir)
	if err != nil {
		return fmt.Errorf("queue lock: load: %w", err)
	}
	if err := fn(q); err != nil {
		return err
	}
	return saveWeaveQueue(dir, q)
}

func withWeaveQueueLock(dir string, fn func(*weaveQueue) error) error {
	return withWeaveQueueLockWait(dir, weaveQueueLockWait, fn)
}

// withWeavePullLock has no OS-level mutex on Windows, matching the queue lock
// above; it exists so pull's structure is one code path on every platform.
func withWeavePullLock(dir string, fn func() error) error {
	_ = dir
	return fn()
}
