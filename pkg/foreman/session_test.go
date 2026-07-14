package foreman

import (
	"context"
	"strings"
	"sync"
	"testing"
)

// stubRunner is written by the async apply goroutine and read by the test, so it
// carries its own lock. `foreman tell` acks "accepted" and applies in the
// background -- the ack has a 3s deadline and a turn runs an LLM, so it could
// never have honestly carried the result.
type stubRunner struct {
	mu      sync.Mutex
	prompts []string
	out     string
	code    int
	err     error
}

func (s *stubRunner) Run(ctx context.Context, agent string, args []string, cwd string) (string, int, error) {
	s.mu.Lock()
	if len(args) > 0 {
		s.prompts = append(s.prompts, args[len(args)-1])
	}
	out, code, err := s.out, s.code, s.err
	s.mu.Unlock()
	return out, code, err
}

// Prompts is a snapshot of what the runner has been asked, safe to read while a
// turn is in flight.
func (s *stubRunner) Prompts() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.prompts...)
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
