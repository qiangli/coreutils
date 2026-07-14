package chat

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Start must REFUSE a tool it cannot steer, rather than quietly handing back a
// one-shot.
//
// This is the whole contract. A one-shot runs its prompt and exits, so a steer
// sent to it arrives after the agent is gone — and every symptom of that looks
// like success: the socket accepts the bytes, the command prints "sent", the state
// goes to working, an answer comes back. `meet say` shipped in exactly that
// condition for months.
//
// A caller that asked for a conversation and got a monologue must be told.
func TestStartRefusesAToolWithNoInteractiveLaunch(t *testing.T) {
	_, err := Start(context.Background(), "definitely-not-a-registered-tool", SessionOptions{
		Prompt:  "hello",
		Timeout: time.Second,
	})
	if err == nil {
		t.Fatal("Start accepted an unknown agent — a session with nothing on the other end " +
			"must fail loudly, not silently")
	}
}

// CanSteer is how a caller degrades LOUDLY. foreman consults it before falling
// back to replay; meet consults it before promising a chair it can interrupt.
func TestCanSteerNamesTheReason(t *testing.T) {
	ok, why := CanSteer("definitely-not-a-registered-tool")
	if ok {
		t.Fatal("CanSteer said yes to an agent that does not exist")
	}
	if strings.TrimSpace(why) == "" {
		t.Error("CanSteer refused without saying why — an operator who cannot steer " +
			"needs to know whether the tool lacks an interactive launch, the platform " +
			"lacks a pty, or the agent simply is not installed")
	}
}

// A turn boundary must exist even when the agent says NOTHING.
//
// WaitIdle keys off silence, so a naive implementation waits forever on the one
// failure it most needs to report: an agent that launched and never spoke. The
// idle clock is therefore seeded at launch, not at first output.
func TestWaitIdleReturnsOnATotallySilentAgent(t *testing.T) {
	s := &Session{done: make(chan struct{}), lastWrite: time.Now()}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	if err := s.WaitIdle(ctx, 600*time.Millisecond); err != nil {
		t.Fatalf("WaitIdle on a silent agent: %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Error("WaitIdle hung on an agent that never spoke")
	}
}

// Turn returns only what was said SINCE the last turn — otherwise a foreman's
// history and a meeting's minutes would re-record the whole session on every
// message.
func TestTurnReturnsOnlyTheDelta(t *testing.T) {
	s := &Session{done: make(chan struct{}), lastWrite: time.Now()}
	w := &sessionWriter{s: s}

	_, _ = w.Write([]byte("first answer"))
	if got := s.Turn(); got != "first answer" {
		t.Fatalf("Turn 1 = %q", got)
	}
	_, _ = w.Write([]byte("second answer"))
	if got := s.Turn(); got != "second answer" {
		t.Errorf("Turn 2 = %q — it must not replay the first", got)
	}
	if got := s.Turn(); got != "" {
		t.Errorf("Turn 3 = %q — nothing was said, so nothing is the answer", got)
	}
	if got := s.Output(); got != "first answersecond answer" {
		t.Errorf("Output must still hold the WHOLE transcript: %q", got)
	}
}

// A session with no control channel cannot be steered, and must say so rather than
// returning nil.
func TestSayWithoutAControlChannelFails(t *testing.T) {
	s := &Session{Nick: "Ada", done: make(chan struct{})}
	if err := s.Say("stop"); err == nil {
		t.Fatal("Say succeeded with no control socket")
	}
	if err := s.Say("   "); err == nil {
		t.Fatal("Say accepted an empty steer")
	}
}
