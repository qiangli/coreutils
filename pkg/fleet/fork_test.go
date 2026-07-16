// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package fleet

import (
	"strings"
	"testing"
)

func TestForkArgvSubstitutesSessionModelPrompt(t *testing.T) {
	tool := Tool{CLI: ToolCLI{Launch: ToolLaunch{
		ForkExec:   "claude --resume {session} --fork-session --dangerously-skip-permissions -p {prompt}",
		SessionEnv: []string{"TEST_CLAUDE_SESS"},
	}}}
	if !tool.CanFork() {
		t.Fatal("CanFork should be true for a tool with fork_exec")
	}
	got := strings.Join(tool.ForkArgv("", "sess-123", "do the thing"), " ")
	want := "claude --resume sess-123 --fork-session --dangerously-skip-permissions -p do the thing"
	if got != want {
		t.Fatalf("ForkArgv = %q\nwant %q", got, want)
	}
	// The prefix is what agentlaunch inserts between the binary and the prompt.
	prefix, ok := tool.ForkArgvPrefix("", "sess-123")
	if !ok {
		t.Fatal("ForkArgvPrefix should resolve")
	}
	if p := strings.Join(prefix, " "); p != "--resume sess-123 --fork-session --dangerously-skip-permissions -p" {
		t.Fatalf("ForkArgvPrefix = %q", p)
	}
	// CurrentSession reads the first set SessionEnv var.
	t.Setenv("TEST_CLAUDE_SESS", "abc123")
	if tool.CurrentSession() != "abc123" {
		t.Fatalf("CurrentSession = %q", tool.CurrentSession())
	}
}

func TestNoForkTemplateMeansNoFork(t *testing.T) {
	plain := Tool{CLI: ToolCLI{Launch: ToolLaunch{Exec: "codex exec {prompt}"}}}
	if plain.CanFork() {
		t.Fatal("a tool without fork_exec must not claim to fork (codex: headless resume mutates the parent)")
	}
	if _, ok := plain.ForkArgvPrefix("", "s"); ok {
		t.Fatal("ForkArgvPrefix must be false without a fork template")
	}
	if plain.CurrentSession() != "" {
		t.Fatal("CurrentSession with no SessionEnv should be empty")
	}
}

// The shipped claude tool must declare a real fork; codex must not.
func TestBaselineClaudeForksCodexDoesNot(t *testing.T) {
	cat := New()
	claude, ok := cat.Tool("claude")
	if !ok {
		t.Fatal("baseline claude tool missing")
	}
	if !claude.CanFork() {
		t.Fatal("baseline claude should declare fork_exec (delegate self relies on it)")
	}
	if codex, ok := cat.Tool("codex"); ok && codex.CanFork() {
		t.Fatal("codex must NOT declare a fork — its headless resume mutates the parent thread")
	}
}
