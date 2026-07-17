package chat

import (
	"os"
	"path/filepath"
	"testing"
)

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
		// A bare tool names no agent, so it is returned untouched.
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

// TestSessionRegistryRoundTrip — register, list, find, deregister, against an
// isolated HOME so it never touches the real board.
func TestSessionRegistryRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	s := LiveSession{
		ID:      "codex-" + fmtPid(),
		Binding: "codex:gpt-5.5",
		Nick:    "Bruno",
		Tool:    "codex",
		CtlSock: filepath.Join(home, "ctl.sock"),
		PID:     os.Getpid(), // alive: this test process
		Started: "2026-07-17T00:00:00Z",
	}
	if err := registerSession(s); err != nil {
		t.Fatalf("register: %v", err)
	}

	got, err := listSessions()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].ID != s.ID {
		t.Fatalf("list returned %+v, want the one session", got)
	}

	if f, err := findSession(s.ID); err != nil || f.Binding != s.Binding {
		t.Fatalf("find by id: %v / %+v", err, f)
	}
	if f, err := findSession("Bruno"); err != nil || f.ID != s.ID {
		t.Fatalf("find by nick: %v / %+v", err, f)
	}

	deregisterSession(s.ID)
	got, _ = listSessions()
	if len(got) != 0 {
		t.Fatalf("after deregister, want 0 sessions, got %d", len(got))
	}
}

// TestSessionRegistryPrunesDead — a file whose launcher pid is gone is pruned on
// read, so the board never asserts a dead session is live (absence-of-evidence).
func TestSessionRegistryPrunesDead(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dead := LiveSession{
		ID:      "codex-dead",
		Binding: "codex:gpt-5.5",
		Tool:    "codex",
		PID:     2147483000, // a pid that is not running
		Started: "2026-07-17T00:00:00Z",
	}
	if err := registerSession(dead); err != nil {
		t.Fatalf("register: %v", err)
	}
	got, err := listSessions()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("a dead session should be pruned on read, got %d", len(got))
	}
	dir, _ := sessionsDir()
	if _, err := os.Stat(filepath.Join(dir, "codex-dead.json")); !os.IsNotExist(err) {
		t.Fatal("the stale file should have been removed on read")
	}
}

func fmtPid() string { return "1" }
