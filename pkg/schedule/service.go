package schedule

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/lockfile"
	"github.com/qiangli/coreutils/pkg/nudge"
	"github.com/qiangli/coreutils/pkg/weavecli"
)

// EngineOffHint is the stderr suggestion emitted when a job is submitted while
// no schedule daemon holds the admission lock.
const EngineOffHint = "schedule engine is not running: this job will not fire. `bashy service enable schedule`, or `bashy schedule tick` to fire due jobs once."

// EngineStatus is a read-only snapshot of the schedule daemon admission lock
// and overdue enabled jobs in the schedule store.
type EngineStatus struct {
	Running bool       `json:"running"`
	PID     int        `json:"pid,omitempty"`
	Since   *time.Time `json:"since,omitempty"`
	Overdue int        `json:"overdue"`
}

func daemonLockPath(stateFile string) string {
	return filepath.Join(filepath.Dir(stateFile), "schedule-daemon.lock")
}

// claimDaemonAdmission prevents two schedule loops from firing jobs at once.
// Process creation, status, and shutdown belong to the service monitor; this
// lock only protects the scheduler's single-writer job admission invariant.
func claimDaemonAdmission() (*lockfile.Lock, error) {
	path := daemonLockPath(statePath())
	lease, err := lockfile.TryAcquire(path, lockfile.Holder{
		Name: "bashy-schedule-daemon", PID: os.Getpid(), Intent: "run foreground scheduler",
	})
	if err != nil {
		return nil, fmt.Errorf("schedule daemon already running: %w", err)
	}
	return lease, nil
}

// EngineState inspects the daemon admission lock with lockfile.Owner (never
// taking the lock) and counts enabled jobs whose next_run is in the past.
func EngineState() EngineStatus {
	p := statePath()
	s, err := loadPath(p)
	var jobs []*Job
	if err == nil {
		jobs = s.Jobs
	}
	return engineStateFor(p, jobs, time.Now())
}

func engineStateFor(stateFile string, jobs []*Job, now time.Time) EngineStatus {
	st := EngineStatus{
		Overdue: countOverdueJobs(jobs, now),
	}
	if h, ok := lockfile.Owner(daemonLockPath(stateFile)); ok {
		st.Running = true
		st.PID = h.PID
		if !h.Since.IsZero() {
			since := h.Since
			st.Since = &since
		}
	}
	return st
}

func countOverdueJobs(jobs []*Job, now time.Time) int {
	overdue := 0
	for _, j := range jobs {
		if j == nil || !j.Enabled || j.NextRun.IsZero() {
			continue
		}
		if j.NextRun.Before(now) {
			overdue++
		}
	}
	return overdue
}

func formatEngineLine(st EngineStatus) string {
	if st.Running {
		if st.PID > 0 {
			return fmt.Sprintf("engine: running (pid %d, overdue %d)", st.PID, st.Overdue)
		}
		return fmt.Sprintf("engine: running (overdue %d)", st.Overdue)
	}
	return fmt.Sprintf("engine: not running (overdue %d)", st.Overdue)
}

type engineHintLine struct {
	Schema  string `json:"schema_version"`
	Kind    string `json:"kind"`
	Tool    string `json:"tool"`
	Suggest string `json:"suggest"`
	Off     string `json:"off"`
}

func engineHintSuppressed() bool {
	if os.Getenv("POSIXLY_CORRECT") != "" {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("BASHY_HINTS"))) {
	case "0", "false", "off", "no":
		return true
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("BASHY_TELEMETRY_QUIET"))) {
	case "", "0", "false", "off", "no":
	default:
		return true
	}
	return false
}

// HintIfEngineOff writes one bashy-hint-v1 stderr hint when no schedule daemon
// holds the admission lock, unless suppressed by POSIXLY_CORRECT, BASHY_HINTS,
// or BASHY_TELEMETRY_QUIET. It never blocks, never acquires the lock, and never
// alters exit status.
func HintIfEngineOff(w io.Writer, tool string) {
	if w == nil || engineHintSuppressed() {
		return
	}
	if _, running := lockfile.Owner(daemonLockPath(statePath())); running {
		return
	}
	if tool == "" {
		tool = "schedule"
	}
	if weavecli.IsAgentDriven() {
		b, _ := json.Marshal(engineHintLine{
			Schema:  nudge.SchemaVersion,
			Kind:    "hint",
			Tool:    tool,
			Suggest: EngineOffHint,
			Off:     "BASHY_HINTS=off",
		})
		fmt.Fprintf(w, "%s\n", b)
		return
	}
	fmt.Fprintf(w, "─── hint ─── %s (silence: BASHY_HINTS=off)\n", EngineOffHint)
}
