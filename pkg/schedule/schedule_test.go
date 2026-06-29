package schedule

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

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

func TestTickFiresDueJobAndReschedules(t *testing.T) {
	withState(t)
	marker := filepath.Join(t.TempDir(), "fired")
	now := time.Now()
	s, _ := load()
	// Due job (NextRun in the past) that creates a marker file when it fires.
	s.Jobs = append(s.Jobs, &Job{
		ID: "due", Kind: "every", Spec: "1h",
		Command: []string{"sh", "-c", "echo $BASHY_SCHEDULE_PROMPT > " + marker},
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
