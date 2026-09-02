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
