package foreman

import (
	"context"
	"strings"
	"testing"
)

type stubRunner struct {
	prompts []string
	out     string
	code    int
	err     error
}

func (s *stubRunner) Run(ctx context.Context, agent string, args []string, cwd string) (string, int, error) {
	if len(args) > 0 {
		s.prompts = append(s.prompts, args[len(args)-1])
	}
	return s.out, s.code, s.err
}

func TestSessionStateMachine(t *testing.T) {
	dir := t.TempDir()
	r := &stubRunner{out: "ack"}
	s, err := Start(context.Background(), Options{
		ID:     "s1",
		Goal:   "ship foreman",
		Agent:  "stub",
		Root:   dir,
		Runner: r,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := s.State().Status; got != StatusIdle {
		t.Fatalf("initial status = %q, want %q", got, StatusIdle)
	}

	if err := s.Enqueue(Command{Verb: CommandTell, Message: "first instruction"}); err != nil {
		t.Fatalf("enqueue tell: %v", err)
	}
	if err := s.ProcessPending(context.Background()); err != nil {
		t.Fatalf("process tell: %v", err)
	}
	if got := s.State().Status; got != StatusIdle {
		t.Fatalf("after tell status = %q, want %q", got, StatusIdle)
	}
	if len(r.prompts) != 1 || !strings.Contains(r.prompts[0], "first instruction") {
		t.Fatalf("runner prompts = %#v, want first instruction", r.prompts)
	}

	for _, tc := range []struct {
		verb string
		want string
	}{
		{CommandPause, StatusBlocked},
		{CommandResume, StatusIdle},
		{CommandStop, StatusDone},
	} {
		if err := s.Enqueue(Command{Verb: tc.verb}); err != nil {
			t.Fatalf("enqueue %s: %v", tc.verb, err)
		}
		if err := s.ProcessPending(context.Background()); err != nil {
			t.Fatalf("process %s: %v", tc.verb, err)
		}
		if got := s.State().Status; got != tc.want {
			t.Fatalf("after %s status = %q, want %q", tc.verb, got, tc.want)
		}
	}

	reopened, err := Open(dir, "s1", r)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !reopened.State().Stopped || reopened.State().Status != StatusDone {
		t.Fatalf("persisted state = %+v, want stopped done", reopened.State())
	}
}
