package meet

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// A meeting has ONE floor, so it may have only one runner.
//
// The append-only files made every individual write safe, and that was mistaken
// for the whole problem being solved. It is not: `st.Round++`, the turn loop,
// and the transcript append are a read-modify-write over shared state, and two
// processes running rounds against the same meeting interleave them. The
// 2026-07-18 artifact shows exactly that — a round-2 speaker still holding the
// floor when a round-3 speaker took it 22.6 seconds later, and events from
// different rounds interleaved afterwards. Nothing was corrupt line-by-line and
// the meeting was incoherent anyway.
//
// So round execution takes a lease. It is per-MEETING (the lock lives in the
// meeting's own directory), so independent meetings never contend, and a second
// runner is REFUSED with a clear message naming the owner rather than allowed
// to interleave. Refusing is the right failure: a busy meeting is a temporary,
// legible condition an operator can act on, where a corrupted lifecycle is
// neither.

// ErrMeetingBusy is returned when another process holds the meeting's run lease.
var ErrMeetingBusy = errors.New("meet: meeting is already being run by another process")

const (
	// leaseHeartbeat is how often the owner refreshes the lock's mtime.
	leaseHeartbeat = 20 * time.Second
	// leaseStale is how long a lock may go unrefreshed before another process may
	// break it. Generous relative to the heartbeat, because breaking a LIVE
	// lease is the one outcome worse than refusing to start: a turn can block a
	// process for the whole per-turn budget under load, and the heartbeat runs on
	// its own goroutine precisely so that does not look like death.
	leaseStale = 2 * time.Minute
)

// leaseInfo is what a lock file says about its owner. It is written for the
// benefit of the NEXT process (and of a human reading the directory), so it
// records who and since when, not just that the file exists.
type leaseInfo struct {
	PID   int       `json:"pid"`
	Host  string    `json:"host"`
	Since time.Time `json:"since"`
}

// runLease is a held lease. Release is idempotent.
type runLease struct {
	path string
	stop chan struct{}
	once sync.Once
}

func leasePath(id string) (string, error) {
	dir, err := storeDir(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "run.lock"), nil
}

// acquireRunLease takes the meeting's run lease, or reports why it could not.
//
// Ordinary case: O_EXCL creates the lock and we own it. Contended case: the
// existing owner is inspected, and only a DEMONSTRABLY dead one is broken —
// either its pid is gone on this same host, or its heartbeat has aged past
// leaseStale (the cross-host / crashed-and-pid-reused case). Breaking is done by
// removing the lock and racing for the O_EXCL create again, so two processes
// that decide to break the same stale lock cannot both end up believing they
// won: exactly one create succeeds and the other is told the meeting is busy.
func acquireRunLease(id string) (*runLease, error) {
	path, err := leasePath(id)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	if l, err := createLease(path); err == nil {
		return l, nil
	} else if !os.IsExist(err) {
		return nil, err
	}

	prev, stale := inspectLease(path)
	if !stale {
		return nil, fmt.Errorf("%w (pid %d on %s since %s); wait for it to finish, or remove %s if you are certain it is gone",
			ErrMeetingBusy, prev.PID, prev.Host, prev.Since.Format(time.RFC3339), path)
	}
	// Break it and race for the create. A failure here is not fatal on its own —
	// the create below is what actually decides the winner.
	_ = os.Remove(path)
	l, err := createLease(path)
	if err != nil {
		return nil, fmt.Errorf("%w (another process claimed the stale lease first)", ErrMeetingBusy)
	}
	return l, nil
}

func createLease(path string) (*runLease, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	host, _ := os.Hostname()
	b, _ := json.Marshal(leaseInfo{PID: os.Getpid(), Host: host, Since: nowFn()})
	_, _ = f.Write(append(b, '\n'))
	if cerr := f.Close(); cerr != nil {
		_ = os.Remove(path)
		return nil, cerr
	}

	l := &runLease{path: path, stop: make(chan struct{})}
	// The heartbeat is what separates "still working" from "died holding the
	// lock". Without it a long turn would be indistinguishable from a crash, and
	// the stale window would have to be longer than the longest legal turn —
	// which would mean a crashed meeting stays unrunnable for 20 minutes.
	go l.beat()
	return l, nil
}

func (l *runLease) beat() {
	t := time.NewTicker(leaseHeartbeat)
	defer t.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-t.C:
			now := time.Now()
			_ = os.Chtimes(l.path, now, now)
		}
	}
}

// Release drops the lease. Safe to call more than once, so callers can defer it
// unconditionally.
func (l *runLease) Release() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		close(l.stop)
		_ = os.Remove(l.path)
	})
}

// inspectLease reads a lock file and decides whether it may be broken.
//
// An UNREADABLE or malformed lock is treated as stale: a truncated file is the
// signature of a process that died mid-write, and refusing forever on a file
// nobody can parse would make a crash permanently unrecoverable without manual
// cleanup.
func inspectLease(path string) (leaseInfo, bool) {
	var info leaseInfo
	b, err := os.ReadFile(path)
	if err != nil {
		return info, true
	}
	if err := json.Unmarshal(b, &info); err != nil {
		return info, true
	}
	host, _ := os.Hostname()
	if info.Host == host && info.PID > 0 && !processAlive(info.PID) {
		return info, true // same host, owner gone: certain, and immediate
	}
	fi, err := os.Stat(path)
	if err != nil {
		return info, true
	}
	return info, time.Since(fi.ModTime()) > leaseStale
}
