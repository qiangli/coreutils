package chat

import "testing"

// TestPickAgentConflict — --agent with --band/--tool is a contradiction, not a
// silent precedence.
func TestPickAgentConflict(t *testing.T) {
	if _, err := PickAgent(Selector{Agent: "claude", Band: 3}); err == nil {
		t.Fatal("expected an error for --agent + --band, got nil")
	}
	if _, err := PickAgent(Selector{Agent: "claude", Tool: "codex"}); err == nil {
		t.Fatal("expected an error for --agent + --tool, got nil")
	}
}

// TestPickAgentSpecific — a specific name passes through (canonicalized when the
// catalog knows it, verbatim when it is a bare tool).
func TestPickAgentSpecific(t *testing.T) {
	got, err := PickAgent(Selector{Agent: "claude"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "claude" {
		t.Fatalf("bare tool should pass through unchanged, got %q", got)
	}
}

// TestPickAgentEmpty — no selector means "use the default", signalled by "".
func TestPickAgentEmpty(t *testing.T) {
	got, err := PickAgent(Selector{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("empty selector should return \"\", got %q", got)
	}
}

// TestPickAgentBandRange — an out-of-range band is rejected loudly.
func TestPickAgentBandRange(t *testing.T) {
	if _, err := PickAgent(Selector{Band: 99}); err == nil {
		t.Fatal("expected out-of-range band error, got nil")
	}
}
