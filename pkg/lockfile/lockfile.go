// Package lockfile provides process-scoped file locks with blocking,
// try-once, and bounded-wait acquisition.
package lockfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Holder is recorded beside the locked sentinel so a contender can report who
// owns the lock. The record is diagnostic only; the kernel lock is authoritative.
type Holder struct {
	Name   string    `json:"name"`
	PID    int       `json:"pid"`
	Intent string    `json:"intent"`
	Since  time.Time `json:"since"`
}

// Lock is a held kernel lock.
type Lock struct {
	file       *os.File
	unlock     func() error
	once       sync.Once
	releaseErr error
}

// ErrHeld is returned when a try-once or bounded acquisition finds another
// process holding the lock.
var ErrHeld = errors.New("lock held")

type heldError struct {
	holder Holder
	has    bool
}

func (e *heldError) Error() string {
	if !e.has {
		return ErrHeld.Error()
	}
	if e.holder.Name != "" {
		return fmt.Sprintf("%s by %s", ErrHeld, e.holder.Name)
	}
	if e.holder.PID > 0 {
		return fmt.Sprintf("%s by pid %d", ErrHeld, e.holder.PID)
	}
	return ErrHeld.Error()
}

func (e *heldError) Unwrap() error { return ErrHeld }

const pollInterval = 20 * time.Millisecond

// Acquire blocks until the lock is available.
func Acquire(path string, h Holder) (*Lock, error) {
	f, err := open(path)
	if err != nil {
		return nil, err
	}
	unlock, err := lockBlocking(f)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	l := &Lock{file: f, unlock: unlock}
	l.record(normalize(h))
	return l, nil
}

// TryAcquire makes exactly one attempt. It returns ErrHeld if another process
// holds the lock.
func TryAcquire(path string, h Holder) (*Lock, error) {
	return acquireWithin(path, 0, h)
}

// AcquireWithin polls until d elapses. It returns ErrHeld on deadline.
func AcquireWithin(path string, d time.Duration, h Holder) (*Lock, error) {
	return acquireWithin(path, d, h)
}

func acquireWithin(path string, d time.Duration, h Holder) (*Lock, error) {
	f, err := open(path)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(d)
	for {
		ok, unlock, err := tryLock(f)
		if err != nil {
			_ = f.Close()
			return nil, err
		}
		if ok {
			l := &Lock{file: f, unlock: unlock}
			l.record(normalize(h))
			return l, nil
		}
		if d <= 0 || !time.Now().Before(deadline) {
			_ = f.Close()
			holder, has := readHolder(path)
			return nil, &heldError{holder: holder, has: has}
		}
		sleep := time.Until(deadline)
		if sleep > pollInterval {
			sleep = pollInterval
		}
		if sleep > 0 {
			time.Sleep(sleep)
		}
	}
}

func open(path string) (*os.File, error) {
	if path == "" {
		return nil, fmt.Errorf("lockfile: empty path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("lockfile: ensure directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("lockfile: open %s: %w", path, err)
	}
	return f, nil
}

func normalize(h Holder) Holder {
	if h.PID == 0 {
		h.PID = os.Getpid()
	}
	if h.Since.IsZero() {
		h.Since = time.Now()
	}
	return h
}

// record replaces the diagnostic record without replacing the lock inode.
// Failure is deliberately best effort: ownership was already decided by the
// kernel, and a metadata failure must not create a second winner.
func (l *Lock) record(h Holder) {
	b, err := json.Marshal(h)
	if err != nil {
		return
	}
	b = append(b, '\n')
	if err := l.file.Truncate(0); err != nil {
		return
	}
	if _, err := l.file.WriteAt(b, 0); err != nil {
		return
	}
	_ = l.file.Sync()
}

// Release unlocks and closes. It is safe to call twice.
func (l *Lock) Release() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		if l.unlock != nil {
			l.releaseErr = l.unlock()
		}
		if err := l.file.Close(); l.releaseErr == nil {
			l.releaseErr = err
		}
	})
	return l.releaseErr
}

// Owner reports who holds path right now, best effort, without acquiring it
// for the caller. Stale metadata is ignored when the kernel says the lock is
// currently free.
func Owner(path string) (Holder, bool) {
	f, err := os.OpenFile(path, os.O_RDWR, 0o644)
	if err != nil {
		return Holder{}, false
	}
	ok, unlock, err := tryLock(f)
	if err != nil {
		_ = f.Close()
		return Holder{}, false
	}
	if ok {
		_ = unlock()
		_ = f.Close()
		return Holder{}, false
	}
	h, has := readHolder(path)
	_ = f.Close()
	return h, has
}

func readHolder(path string) (Holder, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Holder{}, false
	}
	var h Holder
	if json.Unmarshal(b, &h) != nil || (h.Name == "" && h.PID <= 0) {
		return Holder{}, false
	}
	return h, true
}

// HeldBy returns the recorded holder carried by an ErrHeld error.
func HeldBy(err error) (Holder, bool) {
	var held *heldError
	if !errors.As(err, &held) || !held.has {
		return Holder{}, false
	}
	return held.holder, true
}
