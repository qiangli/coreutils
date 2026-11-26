package schedule

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/lockfile"
	"github.com/qiangli/coreutils/pkg/nudge"
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

func clearHintEnv(t *testing.T) {
	t.Helper()
	t.Setenv("BASHY_AGENTIC", "")
	t.Setenv("BASHY_HINTS", "")
	t.Setenv("POSIXLY_CORRECT", "")
	t.Setenv("BASHY_TELEMETRY_QUIET", "")
}

func TestEngineStateTracksDaemonLockAndOverdueJobs(t *testing.T) {
	state := withState(t)
	now := time.Now()
	if err := SaveJobs([]*Job{
		{ID: "overdue-on", Kind: "every", Spec: "1h", Command: []string{"true"}, Enabled: true, CreatedAt: now.Add(-2 * time.Hour), NextRun: now.Add(-time.Minute)},
		{ID: "overdue-off", Kind: "every", Spec: "1h", Command: []string{"true"}, Enabled: false, CreatedAt: now.Add(-2 * time.Hour), NextRun: now.Add(-time.Minute)},
		{ID: "future-on", Kind: "every", Spec: "1h", Command: []string{"true"}, Enabled: true, CreatedAt: now, NextRun: now.Add(time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}

	st := EngineState()
	if st.Running || st.PID != 0 || st.Since != nil || st.Overdue != 1 {
		t.Fatalf("EngineState (off) = %+v, want running=false pid=0 since=nil overdue=1", st)
	}
	if _, err := os.Stat(daemonLockPath(state)); !os.IsNotExist(err) {
		t.Fatalf("EngineState created daemon lock file: %v", err)
	}

	lease, err := claimDaemonAdmission()
	if err != nil {
		t.Fatal(err)
	}
	st = EngineState()
	if !st.Running || st.PID != os.Getpid() || st.Since == nil || st.Overdue != 1 {
		lease.Release()
		t.Fatalf("EngineState (running) = %+v, want running=true pid=%d since!=nil overdue=1", st, os.Getpid())
	}
	if _, err := claimDaemonAdmission(); !errors.Is(err, lockfile.ErrHeld) {
		lease.Release()
		t.Fatalf("EngineState disturbed held daemon lock: second claim err = %v", err)
	}
	if err := lease.Release(); err != nil {
		t.Fatal(err)
	}

	// Stale metadata left behind in schedule-daemon.lock after release/crash
	// must still report running=false.
	st = EngineState()
	if st.Running || st.PID != 0 || st.Since != nil || st.Overdue != 1 {
		t.Fatalf("EngineState (after release) = %+v, want running=false pid=0 since=nil overdue=1", st)
	}
}

func TestScheduleListIncludesEngineState(t *testing.T) {
	withState(t)
	clearHintEnv(t)

	// Empty store, engine off.
	cmd := NewScheduleCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"list"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "engine: not running (overdue 0)\n") || !strings.Contains(out.String(), "no scheduled jobs\n") {
		t.Fatalf("empty list output = %q", out.String())
	}

	// Store with 1 overdue and 1 future job.
	now := time.Now()
	if err := SaveJobs([]*Job{
		{ID: "due1", Kind: "every", Spec: "1h", Command: []string{"true"}, Enabled: true, CreatedAt: now.Add(-2 * time.Hour), NextRun: now.Add(-time.Minute)},
		{ID: "fut1", Kind: "every", Spec: "1h", Command: []string{"true"}, Enabled: true, CreatedAt: now, NextRun: now.Add(time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}

	// Text list with engine off.
	out.Reset()
	errOut.Reset()
	cmd = NewScheduleCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"list"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "engine: not running (overdue 1)\n") {
		t.Fatalf("list (off) missing engine line: %q", out.String())
	}

	// JSON list with engine off.
	out.Reset()
	errOut.Reset()
	cmd = NewScheduleCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"list", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var payload struct {
		SchemaVersion string       `json:"schema_version"`
		Kind          string       `json:"kind"`
		Engine        EngineStatus `json:"engine"`
		Jobs          []*Job       `json:"jobs"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("list --json invalid JSON: %v (%q)", err, out.String())
	}
	if payload.SchemaVersion != "bashy-schedule-v1" || payload.Kind != "list" || payload.Engine.Running || payload.Engine.Overdue != 1 || len(payload.Jobs) != 2 {
		t.Fatalf("list --json (off) = %+v", payload)
	}

	// Hold daemon lock and verify both text and JSON show running + PID.
	lease, err := claimDaemonAdmission()
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()

	out.Reset()
	cmd = NewScheduleCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"list"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	wantLine := fmt.Sprintf("engine: running (pid %d, overdue 1)\n", os.Getpid())
	if !strings.Contains(out.String(), wantLine) {
		t.Fatalf("list (running) = %q, want %q", out.String(), wantLine)
	}

	out.Reset()
	cmd = NewScheduleCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"list", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	payload = struct {
		SchemaVersion string       `json:"schema_version"`
		Kind          string       `json:"kind"`
		Engine        EngineStatus `json:"engine"`
		Jobs          []*Job       `json:"jobs"`
	}{}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("list --json (running) invalid JSON: %v", err)
	}
	if !payload.Engine.Running || payload.Engine.PID != os.Getpid() || payload.Engine.Since == nil || payload.Engine.Overdue != 1 {
		t.Fatalf("list --json (running) engine = %+v, want running=true pid=%d since!=nil overdue=1", payload.Engine, os.Getpid())
	}
}

func TestScheduleAddEmitsHintWhenEngineOffAndStoresJob(t *testing.T) {
	withState(t)
	clearHintEnv(t)

	// Plain/human mode.
	cmd := NewScheduleCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"add", "--every", "1h", "--", "true"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("schedule add failed: %v", err)
	}
	if !strings.Contains(out.String(), "scheduled ") {
		t.Fatalf("stdout = %q, want scheduled confirmation", out.String())
	}
	if strings.Contains(out.String(), "hint") {
		t.Fatalf("hint leaked to stdout: %q", out.String())
	}
	if !strings.Contains(errOut.String(), "bashy service enable schedule") || !strings.Contains(errOut.String(), "bashy schedule tick") {
		t.Fatalf("stderr = %q, want hint naming service enable and schedule tick", errOut.String())
	}
	jobs, err := LoadJobs()
	if err != nil || len(jobs) != 1 {
		t.Fatalf("stored jobs = %v err = %v, want 1 job", jobs, err)
	}

	// Agentic mode emits a bashy-hint-v1 JSON line on stderr while keeping stdout pure JSON.
	t.Setenv("BASHY_AGENTIC", "1")
	out.Reset()
	errOut.Reset()
	cmd = NewScheduleCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"add", "--every", "2h", "--", "true"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("schedule add (agentic) failed: %v", err)
	}
	var added map[string]any
	if err := json.Unmarshal(out.Bytes(), &added); err != nil {
		t.Fatalf("agentic stdout not valid JSON: %v (%q)", err, out.String())
	}
	if added["schema_version"] != "bashy-schedule-v1" || added["kind"] != "added" {
		t.Fatalf("agentic stdout = %v", added)
	}
	var hint engineHintLine
	if err := json.Unmarshal(errOut.Bytes(), &hint); err != nil {
		t.Fatalf("agentic stderr not valid bashy-hint-v1 JSON: %v (%q)", err, errOut.String())
	}
	if hint.Schema != nudge.SchemaVersion || hint.Kind != "hint" || hint.Tool != "schedule" || hint.Off != "BASHY_HINTS=off" {
		t.Fatalf("unexpected hint envelope: %+v", hint)
	}
	if !strings.Contains(hint.Suggest, "bashy service enable schedule") || !strings.Contains(hint.Suggest, "bashy schedule tick") {
		t.Fatalf("hint suggest = %q", hint.Suggest)
	}
	jobs, err = LoadJobs()
	if err != nil || len(jobs) != 2 {
		t.Fatalf("stored jobs = %v err = %v, want 2 jobs", jobs, err)
	}
}

func TestScheduleAddNoHintWhenDaemonRunning(t *testing.T) {
	withState(t)
	clearHintEnv(t)

	lease, err := claimDaemonAdmission()
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()

	for _, agentic := range []string{"", "1"} {
		t.Setenv("BASHY_AGENTIC", agentic)
		cmd := NewScheduleCmd()
		var out, errOut bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		cmd.SetArgs([]string{"add", "--every", "1h", "--", "true"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("schedule add (agentic=%q) failed: %v", agentic, err)
		}
		if errOut.Len() != 0 {
			t.Fatalf("schedule add (agentic=%q) emitted unexpected stderr while daemon running: %q", agentic, errOut.String())
		}
	}
	jobs, err := LoadJobs()
	if err != nil || len(jobs) != 2 {
		t.Fatalf("stored jobs = %v err = %v, want 2 jobs", jobs, err)
	}
}

func TestScheduleAddHintSuppressedByEnv(t *testing.T) {
	cases := []struct {
		name string
		key  string
		val  string
	}{
		{name: "BASHY_HINTS=off", key: "BASHY_HINTS", val: "off"},
		{name: "POSIXLY_CORRECT=1", key: "POSIXLY_CORRECT", val: "1"},
		{name: "BASHY_TELEMETRY_QUIET=1", key: "BASHY_TELEMETRY_QUIET", val: "1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withState(t)
			clearHintEnv(t)
			t.Setenv(tc.key, tc.val)

			for _, agentic := range []string{"", "1"} {
				t.Setenv("BASHY_AGENTIC", agentic)
				cmd := NewScheduleCmd()
				var out, errOut bytes.Buffer
				cmd.SetOut(&out)
				cmd.SetErr(&errOut)
				cmd.SetArgs([]string{"add", "--every", "1h", "--", "true"})
				if err := cmd.Execute(); err != nil {
					t.Fatalf("schedule add (%s, agentic=%q) failed: %v", tc.name, agentic, err)
				}
				if errOut.Len() != 0 {
					t.Fatalf("schedule add (%s, agentic=%q) emitted stderr despite suppression: %q", tc.name, agentic, errOut.String())
				}
			}
			jobs, err := LoadJobs()
			if err != nil || len(jobs) != 2 {
				t.Fatalf("stored jobs = %v err = %v, want 2 jobs", jobs, err)
			}
		})
	}
}
