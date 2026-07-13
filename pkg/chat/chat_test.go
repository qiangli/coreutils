package chat

import (
	"context"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"
)

type fakeRunner struct {
	agent string
	args  []string
	cwd   string
}

func (f *fakeRunner) Run(ctx context.Context, agent string, args []string, cwd string) (string, int, error) {
	f.agent, f.args, f.cwd = agent, append([]string{}, args...), cwd
	return "ok\n", 0, nil
}

func TestResolveAgentRole(t *testing.T) {
	got, err := ResolveAgent("", "conductor")
	if err != nil {
		t.Fatal(err)
	}
	if got != "claude" {
		t.Fatalf("conductor role resolved to %q, want claude", got)
	}
}

func TestBuildPromptIncludesContext(t *testing.T) {
	prompt, err := BuildPrompt(Options{
		Instruction: "implement the issue",
		Context:     []string{"deployment target: staging"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "implement the issue") || !strings.Contains(prompt, "deployment target: staging") {
		t.Fatalf("prompt missing expected content:\n%s", prompt)
	}
}

func TestInvokeUsesSeededHeadlessContract(t *testing.T) {
	r := &fakeRunner{}
	res, err := Invoke(context.Background(), Options{
		Agent:       "codex",
		Instruction: "review this",
		Cwd:         "/tmp/work",
	}, r)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 0 || r.agent != "codex" {
		t.Fatalf("unexpected result=%+v runner.agent=%q", res, r.agent)
	}
	if len(r.args) < 5 || r.args[0] != "exec" || r.args[1] != "--skip-git-repo-check" {
		t.Fatalf("missing codex headless contract: %#v", r.args)
	}
	if r.args[len(r.args)-1] != "review this" {
		t.Fatalf("last arg should be prompt, got %#v", r.args)
	}
}

func TestInvokeCanOverrideCodexSandbox(t *testing.T) {
	// A non-danger sandbox override sets --sandbox <value>.
	r := &fakeRunner{}
	_, err := Invoke(context.Background(), Options{
		Agent: "codex", Instruction: "commit this", Sandbox: "workspace-write",
	}, r)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(r.args, " "); !strings.Contains(got, "--sandbox workspace-write") {
		t.Fatalf("sandbox override missing from args: %#v", r.args)
	}
}

func TestInvokeCodexDangerFullAccessIsNonInteractive(t *testing.T) {
	// danger-full-access → the fully non-interactive bypass flag (no approval/trust
	// popup that would hang a headless runner), and NOT a plain --sandbox value.
	// Asserts the RENDERING; guardUnsafeArgs (tested in TestUnsafeLaunch*) is what
	// decides whether this rendering is permitted to run at all.
	permitUnsafeLaunch(t)
	r := &fakeRunner{}
	_, err := Invoke(context.Background(), Options{
		Agent: "codex", Instruction: "commit this", Sandbox: "danger-full-access",
	}, r)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(r.args, " ")
	if !strings.Contains(got, "--dangerously-bypass-approvals-and-sandbox") {
		t.Fatalf("expected non-interactive bypass flag: %#v", r.args)
	}
	if strings.Contains(got, "--sandbox") {
		t.Fatalf("danger-full-access must not emit --sandbox: %#v", r.args)
	}
}

func TestInvokeAiderHeadlessProfile(t *testing.T) {
	// aider must be driven headlessly with --message (prompt appended as its
	// value) + --yes-always + --no-git; bare `aider <prompt>` opens the TUI.
	// --yes-always is aider's approval-gate kill-switch, so it is emitted only
	// when unsafe launches are permitted — this test asserts that full headless
	// argv, so it opts in (the default now launches aider under its own gate).
	permitUnsafeLaunch(t)
	r := &fakeRunner{}
	_, err := Invoke(context.Background(), Options{
		Agent: "aider", Instruction: "review this",
	}, r)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(r.args, " ")
	if !strings.Contains(got, "--yes-always") || !strings.Contains(got, "--no-git") {
		t.Fatalf("aider headless flags missing: %#v", r.args)
	}
	if n := len(r.args); n < 2 || r.args[n-2] != "--message" || r.args[n-1] != "review this" {
		t.Fatalf("prompt must be the --message value (last two args): %#v", r.args)
	}
}

func TestForcedShellEnv(t *testing.T) {
	base := []string{
		"PATH=/usr/bin:/bin",
		"HOME=/home/u",
		"SHELL=/bin/zsh",             // should be replaced
		"CLAUDE_CODE_SHELL=/bin/zsh", // should be replaced
	}
	got := forcedShellEnv(base, "/opt/bashy", "/shims")

	find := func(prefix string) string {
		var v string
		n := 0
		for _, kv := range got {
			if strings.HasPrefix(kv, prefix) {
				v = kv[len(prefix):]
				n++
			}
		}
		if n != 1 {
			t.Fatalf("expected exactly one %q, found %d in %#v", prefix, n, got)
		}
		return v
	}

	if p := find("PATH="); p != "/shims"+string(os.PathListSeparator)+"/usr/bin:/bin" {
		t.Fatalf("shim dir not prepended to PATH: %q", p)
	}
	if runtime.GOOS == "windows" {
		for _, kv := range got {
			if strings.HasPrefix(kv, "SHELL=") {
				t.Fatalf("SHELL should not be set on Windows: %#v", got)
			}
		}
	} else {
		if s := find("SHELL="); s != "/opt/bashy" {
			t.Fatalf("SHELL not pinned to bashy: %q", s)
		}
	}
	if s := find("CLAUDE_CODE_SHELL="); s != "/opt/bashy" {
		t.Fatalf("CLAUDE_CODE_SHELL not pinned to bashy: %q", s)
	}
	if h := find("HOME="); h != "/home/u" {
		t.Fatalf("unrelated var mangled: HOME=%q", h)
	}
}

func TestForcedShellEnvNoShimNoPath(t *testing.T) {
	// With no shim dir, PATH is left untouched and no PATH entry is invented.
	got := forcedShellEnv([]string{"PATH=/usr/bin", "HOME=/h"}, "/opt/bashy", "")
	if !slices.Contains(got, "PATH=/usr/bin") {
		t.Fatalf("PATH should be unchanged when shimDir empty: %#v", got)
	}
}
