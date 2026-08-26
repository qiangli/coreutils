package pinkycmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestPinkyNoUsers(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}, nil)
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errb.String())
	}
}

func testPinkyLongReadsAgentProjectAndPlan(t *testing.T) {
	root := t.TempDir()
	name := "test-agent-w14"
	home := filepath.Join(root, "who", name)
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".project"), []byte("owns profile C\nignored second line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".plan"), []byte("contact: mb\nfree form"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := []string{
		"HOME=" + root, "SHELL=/bin/bashy", "BASHY_AGENT_ID=" + name,
		"BASHY_WHO_FILE=" + filepath.Join(root, "who", "sessions"),
	}
	runLong := func(args ...string) (string, string, int) {
		var out, errb bytes.Buffer
		code := run(&tool.RunContext{Ctx: context.Background(), Env: env, Stdio: tool.Stdio{Out: &out, Err: &errb}}, args)
		return out.String(), errb.String(), code
	}
	out, errOut, code := runLong("-l", name)
	if code != 0 || errOut != "" {
		t.Fatalf("long: code=%d out=%q err=%q", code, out, errOut)
	}
	for _, want := range []string{"Login name: " + name, "Directory: " + home, "Shell: bashy", "Project: owns profile C", "Plan:\ncontact: mb\nfree form\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("long output missing %q: %q", want, out)
		}
	}
	if strings.Contains(out, "ignored second line") {
		t.Fatalf(".project must be one line: %q", out)
	}

	out, _, _ = runLong("-l", "-h", name)
	if strings.Contains(out, "Plan:") || !strings.Contains(out, "Project:") {
		t.Fatalf("-h/--no-plan did not suppress only plan: %q", out)
	}
	out, _, _ = runLong("-l", "--no-project", name)
	if strings.Contains(out, "Project:") || !strings.Contains(out, "Plan:") {
		t.Fatalf("-p/--no-project did not suppress only project: %q", out)
	}

	posixEnv := append(append([]string{}, env...), "POSIXLY_CORRECT=1")
	var posixOut, posixErr bytes.Buffer
	code = run(&tool.RunContext{Ctx: context.Background(), Env: posixEnv, Stdio: tool.Stdio{Out: &posixOut, Err: &posixErr}}, []string{"-l", name})
	if code == 0 || !strings.Contains(posixErr.String(), "no such user") {
		t.Fatalf("agent directory leaked into POSIX mode: code=%d out=%q err=%q", code, posixOut.String(), posixErr.String())
	}
}

func TestPinkyHelp(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}, []string{"--help"})
	if code != 0 || !strings.Contains(out.String(), "Usage: pinky") {
		t.Fatalf("--help: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}

func TestPinkyVersion(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}, []string{"-V"})
	if code != 0 || !strings.Contains(out.String(), "qiangli/coreutils") {
		t.Fatalf("-V: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}

func TestPinkyHFlag(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}, []string{"-h"})
	if code != 0 {
		t.Fatalf("-h (no-plan): code=%d err=%q", code, errb.String())
	}
	t.Run("long reads agent project and plan", testPinkyLongReadsAgentProjectAndPlan)
}

func TestPinkyQuick(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}, []string{"-q"})
	if code != 0 {
		t.Fatalf("-q: code=%d err=%q", code, errb.String())
	}
}

func TestPinkyLookup(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}, []string{"-i"})
	if code != 0 {
		t.Fatalf("-i: code=%d err=%q", code, errb.String())
	}
	out2, errb2 := bytes.Buffer{}, bytes.Buffer{}
	code2 := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out2, Err: &errb2}}, []string{"--lookup"})
	if code2 != 0 {
		t.Fatalf("--lookup: code=%d err=%q", code2, errb2.String())
	}
}
