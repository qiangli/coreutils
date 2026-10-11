package schedule

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/qiangli/coreutils/pkg/lockfile"
)

func TestDaemonAdmissionAndMonitorOwnedCommands(t *testing.T) {
	state := filepath.Join(t.TempDir(), "schedule.json")
	t.Setenv("BASHY_SCHEDULE_STATE", state)

	lease, err := claimDaemonAdmission()
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	if _, err := claimDaemonAdmission(); !errors.Is(err, lockfile.ErrHeld) {
		t.Fatalf("second scheduler admission error = %v, want ErrHeld", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(state), "schedule-daemon.pid")); !os.IsNotExist(err) {
		t.Fatalf("scheduler created a pidfile: %v", err)
	}

	root := NewScheduleCmd()
	for _, name := range []string{"start", "status", "stop"} {
		if _, _, err := root.Find([]string{name}); err == nil {
			t.Errorf("retired schedule %s command remains", name)
		}
	}
	if _, _, err := root.Find([]string{"daemon"}); err != nil {
		t.Errorf("foreground daemon command missing: %v", err)
	}
}
