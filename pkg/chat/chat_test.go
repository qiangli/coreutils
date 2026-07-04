package chat

import (
	"context"
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
