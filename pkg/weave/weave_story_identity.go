package weave

import (
	"fmt"
	"strings"
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
