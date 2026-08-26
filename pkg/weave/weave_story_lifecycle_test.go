package weave

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSprintLifecycleSurface(t *testing.T) {
	cmd := NewSprintCmd()
	have := map[string]bool{}
	for _, sub := range cmd.Commands() {
		have[sub.Name()] = true
	}
	for _, name := range []string{"start", "pause", "resume", "end"} {
		if !have[name] {
			t.Errorf("missing sprint lifecycle verb %q", name)
		}
	}
	end, _, err := cmd.Find([]string{"end"})
	if err != nil {
		t.Fatal(err)
	}
	if end.Flags().Lookup("force") != nil || end.Flags().Lookup("no-verify") != nil {
		t.Error("sprint end must not expose force or no-verify escape hatches")
	}
	if end.Flags().Lookup("gate-timeout") == nil {
		t.Error("sprint end must bound a stuck gate")
	}
}

func TestRunDrainGateHonorsContextDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	out := runDrainGate(ctx, t.TempDir(), "sleep 5")
	if out.Passed {
		t.Fatal("timed-out gate must not pass")
	}
	if time.Since(started) > 2*time.Second {
		t.Fatal("gate ignored its context deadline")
	}
	if !strings.Contains(out.Output, "deadline exceeded") {
		t.Fatalf("timeout reason missing from gate output: %q", out.Output)
	}
}

func TestSprintPauseResumeCarriesContinuityWithoutStoppingBox(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("BASHY_AGENTIC", "")
	t.Setenv("WEAVE_CONDUCTOR", "Ada")

	if out, code := runSprint(t, "add", "lifecycle test"); code != 0 {
		t.Fatalf("add exit=%d: %s", code, out)
	}
	if out, code := runSprint(t, "start", "1", "--for", "1h"); code != 0 {
		t.Fatalf("start exit=%d: %s", code, out)
	}
	if out, code := runSprint(t, "pause", "1"); code == 0 {
		t.Fatalf("pause without continuity must fail, exit=%d: %s", code, out)
	}
	if out, code := runSprint(t, "pause", "1", "-m", "next: inspect the journal"); code != 0 {
		t.Fatalf("pause exit=%d: %s", code, out)
	}

	q, err := loadWeaveQueue(home + "/.bashy/sprint")
	if err != nil {
		t.Fatal(err)
	}
	s := findWeaveStory(q, 1)
	if s == nil || s.Lease != nil || !s.currentBox().Running() {
		t.Fatalf("pause must release only the lease and leave the box running: %+v", s)
	}

	t.Setenv("WEAVE_CONDUCTOR", "Grace")
	out, code := runSprint(t, "resume", "1")
	if code != 0 {
		t.Fatalf("resume exit=%d: %s", code, out)
	}
	if !strings.Contains(out, "next: inspect the journal") {
		t.Fatalf("resume did not display continuity: %s", out)
	}
	q, err = loadWeaveQueue(home + "/.bashy/sprint")
	if err != nil {
		t.Fatal(err)
	}
	s = findWeaveStory(q, 1)
	if s.Lease == nil || s.Lease.Holder != "Grace" || !s.currentBox().Running() {
		t.Fatalf("resume must claim the lease without restarting the box: %+v", s)
	}

	// The next CLI process may not inherit --as. Pause must use the durable
	// lease holder, not silently fall back to the generic conductor identity.
	t.Setenv("WEAVE_CONDUCTOR", "")
	if out, code := runSprint(t, "pause", "1", "-m", "paused again"); code != 0 {
		t.Fatalf("pause after resume --as must use the lease identity, exit=%d: %s", code, out)
	}
}

func TestSprintEndRequiresGateAndClosesLifecycle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("BASHY_AGENTIC", "")
	t.Setenv("WEAVE_CONDUCTOR", "Ada")

	if out, code := runSprint(t, "add", "end test"); code != 0 {
		t.Fatalf("add exit=%d: %s", code, out)
	}
	if out, code := runSprint(t, "start", "1", "--for", "1h"); code != 0 {
		t.Fatalf("start exit=%d: %s", code, out)
	}
	if out, code := runSprint(t, "end", "1"); code == 0 {
		t.Fatalf("end without a gate must fail, exit=%d: %s", code, out)
	}
	if out, code := runSprint(t, "end", "1", "--gate", "true"); code != 0 {
		t.Fatalf("end exit=%d: %s", code, out)
	}

	q, err := loadWeaveQueue(home + "/.bashy/sprint")
	if err != nil {
		t.Fatal(err)
	}
	s := findWeaveStory(q, 1)
	if s == nil || s.Column != "done" || s.Lease != nil || s.currentBox() != nil {
		t.Fatalf("end must close the box, release the lease, and move done: %+v", s)
	}
}
