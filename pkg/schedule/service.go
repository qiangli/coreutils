package schedule

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/qiangli/coreutils/pkg/lockfile"
)

// claimDaemonAdmission prevents two schedule loops from firing jobs at once.
// Process creation, status, and shutdown belong to the service monitor; this
// lock only protects the scheduler's single-writer job admission invariant.
func claimDaemonAdmission() (*lockfile.Lock, error) {
	path := filepath.Join(filepath.Dir(statePath()), "schedule-daemon.lock")
	lease, err := lockfile.TryAcquire(path, lockfile.Holder{
		Name: "bashy-schedule-daemon", PID: os.Getpid(), Intent: "run foreground scheduler",
	})
	if err != nil {
		return nil, fmt.Errorf("schedule daemon already running: %w", err)
	}
	return lease, nil
}
