package weave

import (
	"fmt"
	"strings"
	"time"
)

// SprintClaimIdentity resolves the exact durable address a start/take command
// will claim. Bashy's external-harness adapter uses it before mutation so the
// foreground inbox stream is already live when claim validation runs.
func SprintClaimIdentity(id int64, explicit string, takeover bool) (string, error) {
	dir, err := sprintStoreDir()
	if err != nil {
		return "", err
	}
	q, err := readWeaveQueue(dir)
	if err != nil {
		return "", err
	}
	s := findWeaveStory(q, id)
	if s == nil {
		return "", fmt.Errorf("sprint #%d not found", id)
	}
	if takeover {
		return sprintTakeoverIdentity(s, explicit), nil
	}
	return weaveStoryConductorName(s, strings.TrimSpace(explicit)), nil
}

// RefreshSprintManagerLease records proven activity by the current sprint
// manager. Attached inbox acknowledgement uses this so a manager that is
// actively consuming its delivery stream cannot simultaneously age stale.
func RefreshSprintManagerLease(id int64, owner string) error {
	dir, err := sprintStoreDir()
	if err != nil {
		return err
	}
	owner = strings.TrimSpace(owner)
	return withWeaveQueueLock(dir, func(q *weaveQueue) error {
		s := findWeaveStory(q, id)
		if s == nil {
			return fmt.Errorf("sprint #%d not found", id)
		}
		if s.Lease == nil || !strings.EqualFold(s.Lease.Holder, owner) {
			return fmt.Errorf("sprint #%d is not held by %s", id, owner)
		}
		now := time.Now().UTC()
		s.Lease.At = now
		s.UpdatedAt = now
		return nil
	})
}

// RefreshSprintOwnerActivity records that this agent just READ ITS INBOX, on
// every sprint it owns.
//
// This is the whole liveness model, and it replaces holding a process open.
// Reading your mail is the act that proves both things a seat needs to be
// true — the agent is running, and it is paying attention to this channel —
// and it is something an agent already does at a turn boundary rather than a
// ritual invented for the tool. A held foreground stream proved neither: it
// survived an agent that had stopped reading, and it died on an agent that had
// not.
//
// Best-effort and silent. An inbox read must never fail because of bookkeeping
// it did not ask for, and an agent that owns no sprint is the ordinary case.
// Only leases this exact name already holds are touched, so reading mail can
// never take a seat from somebody else.
func RefreshSprintOwnerActivity(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	dir, err := sprintStoreDir()
	if err != nil {
		return
	}
	// Read first, without the lock: the overwhelmingly common case is an agent
	// that holds no lease, and that must cost nothing.
	q, err := readWeaveQueue(dir)
	if err != nil || q == nil {
		return
	}
	var held bool
	for _, s := range q.Stories {
		if s != nil && s.Lease != nil && strings.EqualFold(s.Lease.Holder, name) {
			held = true
			break
		}
	}
	if !held {
		return
	}
	now := time.Now().UTC()
	_ = withWeaveQueueLock(dir, func(q *weaveQueue) error {
		for _, s := range q.Stories {
			if s != nil && s.Lease != nil && strings.EqualFold(s.Lease.Holder, name) {
				s.Lease.At = now
				s.UpdatedAt = now
			}
		}
		return nil
	})
}

// SprintOwnerLastRead is when this owner last read its inbox, as recorded by
// the lease heartbeat. ok is false when the name holds no sprint lease.
func SprintOwnerLastRead(name string) (at time.Time, ok bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return time.Time{}, false
	}
	dir, err := sprintStoreDir()
	if err != nil {
		return time.Time{}, false
	}
	q, err := readWeaveQueue(dir)
	if err != nil || q == nil {
		return time.Time{}, false
	}
	for _, s := range q.Stories {
		if s != nil && s.Lease != nil && strings.EqualFold(s.Lease.Holder, name) {
			if at.IsZero() || s.Lease.At.After(at) {
				at, ok = s.Lease.At, true
			}
		}
	}
	return at, ok
}
