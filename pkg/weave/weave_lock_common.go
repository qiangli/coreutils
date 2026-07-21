package weave

import (
	"encoding/json"
	"errors"
	"time"
)

// LOCK DISCIPLINE — why this file exists.
//
// weave used to hold ONE coarse exclusive flock on <queueDir>/queue.lock across
// whole operations, including the multi-minute ones: `weave pull
// --review-agent` ran an adversarial-review agent subprocess and a suite gate
// INSIDE the lock. Everything else in the repo — `weave list`, `weave show`,
// `weave add`, another `weave pull` — then blocked for the entire cycle. A
// steward could not read the board or file an issue while the autopilot ran,
// and the only escape found in practice was killing the autopilot. That is a
// governance failure, not a performance nit.
//
// The discipline now:
//
//   - READS ARE LOCK-FREE. saveWeaveQueue writes a temp file and renames it,
//     so every reader either sees the whole old queue or the whole new one —
//     never a torn one. A read therefore never needs to wait for a writer.
//     readWeaveQueue is that path, and it is what `weave list`/`show` use.
//   - WRITES HOLD THE LOCK ONLY AROUND THE MUTATION. load → mutate → save,
//     measured in milliseconds. Any long-running work (agent subprocess, suite
//     gate, git merge) happens OUTSIDE the lock and re-acquires it briefly to
//     record its outcome onto the freshly re-read queue.
//   - MAINTENANCE PASSES ARE OPPORTUNISTIC. The reaper runs on read paths; if
//     the lock is momentarily held it skips this round rather than blocking a
//     `weave list`. It is idempotent, so the next read reaps.
//   - LONG EXCLUSIVE OPERATIONS TAKE THEIR OWN NON-BLOCKING LOCK. `weave pull`
//     merges into the shared checkout, so two of them must not overlap — but a
//     second caller gets an immediate "busy, retry" instead of an indefinite
//     block. That is pull.lock, not queue.lock.

// errWeaveQueueBusy is returned when the queue lock could not be taken within
// the caller's patience. Callers either report it (write paths) or degrade to
// a lock-free read (maintenance paths).
var errWeaveQueueBusy = errors.New("weave: queue busy — another weave command holds the queue lock; retry")

// errWeavePullBusy is returned when another merge/pull already owns this
// repo's pull lock. A pull mutates the shared live checkout, so it is
// genuinely exclusive; reporting busy immediately is the contract, because
// waiting means waiting minutes.
var errWeavePullBusy = errors.New("weave: a merge is already in progress in this repo (pull.lock held) — retry when it finishes")

// weaveQueueLockWait bounds how long an ordinary write waits for the lock. It
// is a safety valve against a crashed-but-not-yet-reaped holder, not a tuning
// knob: with long operations moved out of the lock, real contention is
// milliseconds. A var so tests can shorten it.
var weaveQueueLockWait = 120 * time.Second

// weaveReapLockWait is the reaper's patience on read paths. Short by design:
// `weave list` must return promptly even mid-merge, and a skipped reap costs
// nothing because the pass is idempotent.
var weaveReapLockWait = 250 * time.Millisecond

// weaveQueueLockPoll is the retry interval while waiting for the lock.
const weaveQueueLockPoll = 20 * time.Millisecond

// readWeaveQueue is the LOCK-FREE read path onto the queue. It exists to make
// the contract explicit at call sites: reads never take queue.lock, because
// saveWeaveQueue's write-temp-then-rename makes every visible queue.json a
// complete one. Never pair it with a write — that is a lost-update race; use
// withWeaveQueueLock for read-modify-write.
func readWeaveQueue(dir string) (*weaveQueue, error) { return loadWeaveQueue(dir) }

// weaveIsBusy reports whether err is a lock-contention refusal rather than a
// real failure. Callers surface it as a retryable state conflict.
func weaveIsBusy(err error) bool {
	return errors.Is(err, errWeaveQueueBusy) || errors.Is(err, errWeavePullBusy)
}

// weaveItemFingerprints records each item's serialized form, keyed by ID.
// Paired with weaveWriteBackChangedItems it lets a long operation work on a
// private copy of the queue and then persist ONLY what it actually changed —
// the alternative, writing the whole stale copy back, silently drops every
// `weave add` / `weave comment` that landed while the operation ran.
func weaveItemFingerprints(q *weaveQueue) map[int64]string {
	fp := make(map[int64]string, len(q.Items))
	if q == nil {
		return fp
	}
	for _, it := range q.Items {
		if b, err := json.Marshal(it); err == nil {
			fp[it.ID] = string(b)
		}
	}
	return fp
}

// weaveWriteBackChangedItems re-acquires the queue lock briefly and copies the
// items whose content differs from their fingerprint onto the queue as it
// stands on disk NOW. Items the caller never touched are left alone, and items
// that appeared meanwhile are preserved.
//
// An item the caller changed but that has since vanished from disk (abandoned,
// reset) is not resurrected: it is gone by a deliberate act, and re-adding it
// would undo that.
func weaveWriteBackChangedItems(dir string, work *weaveQueue, before map[int64]string) error {
	if work == nil {
		return nil
	}
	changed := make(map[int64]*weaveItem)
	for _, it := range work.Items {
		b, err := json.Marshal(it)
		if err != nil {
			continue
		}
		if prev, ok := before[it.ID]; !ok || prev != string(b) {
			cp := *it
			changed[it.ID] = &cp
		}
	}
	if len(changed) == 0 {
		return nil
	}
	return withWeaveQueueLock(dir, func(fresh *weaveQueue) error {
		for i, it := range fresh.Items {
			if upd, ok := changed[it.ID]; ok {
				cp := *upd
				fresh.Items[i] = &cp
			}
		}
		return nil
	})
}
