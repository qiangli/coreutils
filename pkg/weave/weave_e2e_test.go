package weave

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// runWeave drives a FRESH weave command tree (cobra flag state is
// per-construction) with the given args, capturing stdout+stderr into one
// buffer and resolving the propagated exit code. Mirrors how a host
// (bashy) invokes NewWeaveCmd().
func runWeave(t *testing.T, args ...string) (string, int) {
	t.Helper()
	cmd := newWeaveCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	code := 0
	if err != nil {
		var ec interface{ ExitCode() int }
		if errors.As(err, &ec) {
			code = ec.ExitCode()
		} else {
			code = 1
		}
	}
	return buf.String(), code
}

// TestWeaveCommandSurface locks in the post-migration command surface:
// the forge-only init-board is gone, cobra's completion subverb is
// disabled, the local-only open + the check introspection verb are
// present, and every lifecycle verb is still wired. Pure structure — no
// git, no filesystem — so it runs on every platform including the
// Windows CI leg.
func TestWeaveCommandSurface(t *testing.T) {
	cmd := newWeaveCmd()
	have := map[string]bool{}
	for _, c := range cmd.Commands() {
		have[c.Name()] = true
	}
	for _, n := range []string{
		"add", "start", "next", "prio", "point", "list", "pause", "resume",
		"autopilot", "status", "log", "remember", "recall", "memory", "attach", "say", "pull", "reverify", "prune", "abandon",
		"kill", "shell", "open", "reset", "wait", "check",
		"sessions", "join", "note", "steer", "take", "handoff", "roster",
		"share", "shares", "unshare",
	} {
		if !have[n] {
			t.Errorf("missing subverb %q", n)
		}
	}
	if have["init-board"] {
		t.Error("init-board is forge-only and must not be registered")
	}
	if have["completion"] {
		t.Error("cobra completion subverb must be disabled")
	}
	if !cmd.CompletionOptions.DisableDefaultCmd {
		t.Error("CompletionOptions.DisableDefaultCmd must be true")
	}

	// Guard the YCODE_* -> WEAVE_* env rename at the doc level: start's
	// help advertises WEAVE_*, and no subverb help may name the old
	// YCODE_LOOM_* contract.
	var start *cobra.Command
	for _, c := range cmd.Commands() {
		if c.Name() == "start" {
			start = c
		}
		if strings.Contains(c.Long, "YCODE_LOOM") {
			t.Errorf("subverb %q help still references YCODE_LOOM_*", c.Name())
		}
	}
	if start == nil || !strings.Contains(start.Long, "WEAVE_*") {
		t.Error("start help should advertise WEAVE_* env vars")
	}
}

// TestWeaveCheckReportsStatus drives `weave check` and asserts it reports
// open as fully implemented (local-only now) and prio as the lone
// LLM-gated path, with no init-board row. check walks the command tree
// only, so it too needs no git.
func TestWeaveCheckReportsStatus(t *testing.T) {
	t.Setenv("YCODE_AGENT", "") // force human rows, not the agent JSON envelope
	out, code := runWeave(t, "check")
	if code != 0 {
		t.Fatalf("check exit=%d out=%s", code, out)
	}
	lineFor := func(name string) string {
		for _, ln := range strings.Split(out, "\n") {
			if strings.HasPrefix(strings.TrimSpace(ln), name+" ") {
				return ln
			}
		}
		return ""
	}
	if l := lineFor("open"); !strings.Contains(l, "implemented") || strings.Contains(l, "require") {
		t.Errorf("open should be plain 'implemented', got %q", l)
	}
	if l := lineFor("prio"); !strings.Contains(l, "LLM") {
		t.Errorf("prio should name its LLM dependency, got %q", l)
	}
	if strings.Contains(out, "init-board") {
		t.Errorf("check must not list init-board:\n%s", out)
	}
}

// TestWeaveQueueLifecycleE2E exercises the real command tree end-to-end
// against an isolated HOME and a throwaway git repo: seed -> inspect ->
// reprioritize -> allocate a workspace -> open (local-only file:// URL) ->
// error paths -> prune. weave shells out to system git, so this skips
// cleanly where git is absent (e.g. the hermetic Windows leg).
func TestWeaveQueueLifecycleE2E(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("system git not available; weave lifecycle needs it")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir on Windows
	t.Setenv("YCODE_AGENT", "")

	repo := t.TempDir()
	gitE2E(t, repo, "init", "-q", "-b", "main")
	gitE2E(t, repo, "config", "user.email", "e2e@test.local")
	gitE2E(t, repo, "config", "user.name", "E2E")
	if err := writeFile(filepath.Join(repo, "README.md"), "hello\n"); err != nil {
		t.Fatal(err)
	}
	gitE2E(t, repo, "add", "-A")
	gitE2E(t, repo, "commit", "-q", "-m", "init")
	t.Chdir(repo)

	mustOK := func(label string, args ...string) {
		t.Helper()
		out, code := runWeave(t, append(args, "--json")...)
		if code != 0 || !strings.Contains(out, `"status": "ok"`) {
			t.Fatalf("%s: exit=%d out=%s", label, code, out)
		}
	}

	// seed three issues at mixed priorities
	mustOK("add p1", "add", "alpha", "--priority", "p1")
	mustOK("add p2", "add", "beta")
	mustOK("add p0", "add", "gamma", "--priority", "p0")

	// list sees all three
	if out, code := runWeave(t, "list", "--json"); code != 0 || strings.Count(out, `"id"`) < 3 {
		t.Fatalf("list: exit=%d out=%s", code, out)
	}
	// next peeks the p0 (gamma) without mutating
	if out, _ := runWeave(t, "next", "--json"); !strings.Contains(out, "gamma") {
		t.Fatalf("next should peek the p0 issue, got %s", out)
	}
	// mutate: points + manual priority
	mustOK("point", "point", "1", "3")
	mustOK("prio", "prio", "2", "p0")
	// prio --auto degrades to a dependency_unhealthy envelope (no LLM here)
	if out, code := runWeave(t, "prio", "--auto", "--json"); code == 0 || !strings.Contains(out, "dependency_unhealthy") {
		t.Fatalf("prio --auto should degrade: exit=%d out=%s", code, out)
	}
	// invalid combo: `prio <issue> --auto` must emit a structured
	// invalid_arg envelope, not silently exit non-zero with no message.
	if out, code := runWeave(t, "prio", "1", "--auto", "--json"); code == 0 || !strings.Contains(out, "invalid_arg") {
		t.Fatalf("prio <issue> --auto should be invalid_arg: exit=%d out=%s", code, out)
	}
	if out, code := runWeave(t, "prio", "1", "--auto"); code == 0 || strings.TrimSpace(out) == "" {
		t.Fatalf("prio <issue> --auto must print an error, not exit silently: exit=%d out=%q", code, out)
	}

	// allocate a workspace for issue 1 without spawning a tool
	if out, code := runWeave(t, "start", "--issue", "1", "--no-spawn", "--json"); code != 0 || !strings.Contains(out, `"status": "ok"`) {
		t.Fatalf("start --no-spawn: exit=%d out=%s", code, out)
	}
	// open is local-only: a file:// workspace URL, never a forge field
	out, code := runWeave(t, "open", "1", "--json")
	if code != 0 || !strings.Contains(out, `"workspace_url": "file://`) {
		t.Fatalf("open 1: exit=%d out=%s", code, out)
	}
	if strings.Contains(out, "forge") {
		t.Errorf("open output must not mention a forge: %s", out)
	}
	// error paths
	if out, code := runWeave(t, "open", "999", "--json"); code == 0 || !strings.Contains(strings.ToLower(out), "not found") {
		t.Fatalf("open 999 should be not-found: exit=%d out=%s", code, out)
	}
	if _, code := runWeave(t, "open"); code == 0 {
		t.Error("open with no issue arg should fail (usage)")
	}

	// prune terminal/workspace state without error
	if _, code := runWeave(t, "prune", "--yes"); code != 0 {
		t.Errorf("prune exit=%d", code)
	}
}

// gitE2E runs `git -C dir <args...>` and fails the test on error. Used
// only to stand up / inspect the throwaway repo; the weave commands under
// test do their own git work.
func gitE2E(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", full...)
	// Deterministic identity so the test doesn't depend on global git config.
	cmd.Env = append(cmd.Environ(),
		"GIT_AUTHOR_NAME=E2E", "GIT_AUTHOR_EMAIL=e2e@test.local",
		"GIT_COMMITTER_NAME=E2E", "GIT_COMMITTER_EMAIL=e2e@test.local",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
