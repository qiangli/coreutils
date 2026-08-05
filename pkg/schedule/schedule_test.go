package schedule

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestUpdateJobsConcurrentWritersDoNotLoseEntries(t *testing.T) {
	withState(t)
	const writers = 24
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			errs <- UpdateJobs(func(jobs []*Job) ([]*Job, error) {
				return append(jobs, &Job{ID: strconv.Itoa(id)}), nil
			})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	jobs, err := LoadJobs()
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != writers {
		t.Fatalf("stored jobs=%d, want %d", len(jobs), writers)
	}
}

func TestScheduleOutputModeEscape(t *testing.T) {
	withState(t)
	t.Setenv("BASHY_AGENTIC", "1")

	// Under agent mode, `list` emits the JSON envelope (consistent with weave/dag).
	cmd := NewScheduleCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"list"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "schema_version") {
		t.Errorf("BASHY_AGENTIC list should be JSON, got %q", out.String())
	}

	// --json=false escapes back to prose even under agent mode.
	cmd = NewScheduleCmd()
	var out2 bytes.Buffer
	cmd.SetOut(&out2)
	cmd.SetErr(&out2)
	cmd.SetArgs([]string{"list", "--json=false"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out2.String(), "schema_version") {
		t.Errorf("--json=false should force prose, got %q", out2.String())
	}
}

func withState(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "schedule.json")
	t.Setenv("BASHY_SCHEDULE_STATE", p)
	return p
}

func TestComputeNext(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)
	cases := []struct {
		kind, spec string
		ok         bool
	}{
		{"cron", "*/15 * * * *", true},
		{"every", "30m", true},
		{"every", "bogus", false},
		{"at", "2099-01-02 03:04", true},
		{"cron", "not a cron", false},
	}
	for _, c := range cases {
		j := &Job{Kind: c.kind, Spec: c.spec, CreatedAt: now}
		next, err := j.computeNext(now)
		if c.ok && err != nil {
			t.Errorf("%s %q: unexpected err %v", c.kind, c.spec, err)
		}
		if !c.ok && err == nil {
			t.Errorf("%s %q: expected error", c.kind, c.spec)
		}
		if c.ok && !next.After(now) {
			t.Errorf("%s %q: next %v not after now", c.kind, c.spec, next)
		}
	}
}

func TestAddListRemoveRoundTrip(t *testing.T) {
	withState(t)
	s, _ := load()
	s.Jobs = append(s.Jobs, &Job{ID: "j1", Kind: "every", Spec: "1h", Command: []string{"true"}, Enabled: true, CreatedAt: time.Now(), NextRun: time.Now().Add(time.Hour)})
	if err := s.save(); err != nil {
		t.Fatal(err)
	}
	s2, err := load()
	if err != nil || len(s2.Jobs) != 1 || s2.Jobs[0].ID != "j1" {
		t.Fatalf("round-trip failed: %+v %v", s2, err)
	}
}

func TestJSONListRedactsCapturedEnvironment(t *testing.T) {
	state := withState(t)
	s, _ := load()
	s.Jobs = append(s.Jobs, &Job{
		ID: "private-at", Kind: "at", Spec: time.Now().Add(time.Hour).Format(time.RFC3339),
		Command: []string{"sh", "-c", "true"}, Env: []string{"TOKEN=top-secret"}, EnvSet: true,
		Enabled: true, CreatedAt: time.Now(), NextRun: time.Now().Add(time.Hour),
	})
	if err := s.save(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(state)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("state mode=%v, want 0600", info.Mode().Perm())
	}

	cmd := NewScheduleCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"list", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "top-secret") || strings.Contains(out.String(), "TOKEN=") {
		t.Fatalf("JSON listing disclosed captured environment: %s", out.String())
	}
	loaded, err := load()
	if err != nil || len(loaded.Jobs) != 1 || !reflect.DeepEqual(loaded.Jobs[0].Env, []string{"TOKEN=top-secret"}) {
		t.Fatalf("private state did not retain environment: jobs=%v err=%v", loaded.Jobs, err)
	}
}

func TestTickFiresDueJobAndReschedules(t *testing.T) {
	withState(t)
	marker := filepath.Join(t.TempDir(), "fired")
	now := time.Now()
	s, _ := load()
	// Due job (NextRun in the past) that creates a marker file when it fires.
	s.Jobs = append(s.Jobs, &Job{
		ID: "due", Kind: "every", Spec: "1h",
		Command: []string{"sh", "-c", "echo $BASHY_SCHEDULE_PROMPT > " + filepath.ToSlash(marker)},
		Prompt:  "hello-prompt", Enabled: true, CreatedAt: now.Add(-2 * time.Hour),
		NextRun: now.Add(-time.Minute),
	})
	// Not-due job.
	s.Jobs = append(s.Jobs, &Job{ID: "future", Kind: "every", Spec: "1h", Command: []string{"true"}, Enabled: true, CreatedAt: now, NextRun: now.Add(time.Hour)})
	if err := s.save(); err != nil {
		t.Fatal(err)
	}

	fired, err := tickOnce(now, os.Stdout)
	if err != nil {
		t.Fatal(err)
	}
	if len(fired) != 1 || fired[0] != "due" {
		t.Fatalf("fired = %v, want [due]", fired)
	}
	// The agentic prompt reached the command's env.
	if b, err := os.ReadFile(marker); err != nil || string(b) != "hello-prompt\n" {
		t.Errorf("prompt not delivered via env: %q %v", b, err)
	}
	// The due job was rescheduled into the future; the other is untouched.
	s3, _ := load()
	due := s3.find("due")
	if due == nil || !due.NextRun.After(now) {
		t.Errorf("due job not rescheduled: %+v", due)
	}
}

func TestTickOneShotAtDisables(t *testing.T) {
	withState(t)
	now := time.Now()
	s, _ := load()
	s.Jobs = append(s.Jobs, &Job{ID: "once", Kind: "at", Spec: "2000-01-01 00:00", Command: []string{"true"}, Enabled: true, CreatedAt: now, NextRun: now.Add(-time.Hour)})
	_ = s.save()
	if _, err := tickOnce(now, os.Stdout); err != nil {
		t.Fatal(err)
	}
	s2, _ := load()
	if j := s2.find("once"); j == nil || j.Enabled {
		t.Errorf("one-shot at job should be disabled after firing: %+v", j)
	}
}
