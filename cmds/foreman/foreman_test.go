package foremancmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

type stubRunner struct {
	prompts []string
	out     string
}

func (s *stubRunner) Run(ctx context.Context, agent string, args []string, cwd string) (string, int, error) {
	if len(args) > 0 {
		s.prompts = append(s.prompts, args[len(args)-1])
	}
	return s.out, 0, nil
}

func TestRegistered(t *testing.T) {
	if tool.Lookup("foreman") == nil {
		t.Fatal("foreman is not registered")
	}
}

func TestOnceUsesChatInvokePath(t *testing.T) {
	old := runner
	r := &stubRunner{out: "done"}
	runner = r
	t.Cleanup(func() { runner = old })
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := run(rc, []string{"--once", "--agent", "stub", "--instruction", "hello"})
	if code != 0 {
		t.Fatalf("code = %d, err = %s", code, errb.String())
	}
	if got := strings.TrimSpace(out.String()); got != "done" {
		t.Fatalf("out = %q, want done", got)
	}
	if len(r.prompts) != 1 || !strings.Contains(r.prompts[0], "hello") {
		t.Fatalf("prompts = %#v, want hello", r.prompts)
	}
}

func TestStartDetachStatusRoundTrip(t *testing.T) {
	t.Setenv("BASHY_FOREMAN_DIR", t.TempDir())
	t.Setenv("BASHY_FOREMAN_NO_SPAWN", "1")
	old := runner
	runner = &stubRunner{out: "ack"}
	t.Cleanup(func() { runner = old })
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := run(rc, []string{"start", "--id", "cli", "--detach", "--goal", "round trip"})
	if code != 0 {
		t.Fatalf("start code = %d, err = %s", code, errb.String())
	}
	out.Reset()
	code = run(rc, []string{"status", "cli"})
	if code != 0 {
		t.Fatalf("status code = %d, err = %s", code, errb.String())
	}
	if got := out.String(); !strings.Contains(got, "cli\tidle\tround trip") {
		t.Fatalf("status = %q", got)
	}
}

func TestScriptedREPL(t *testing.T) {
	t.Setenv("BASHY_FOREMAN_DIR", t.TempDir())
	old := runner
	r := &stubRunner{out: "ack"}
	runner = r
	t.Cleanup(func() { runner = old })
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(),
		Dir: t.TempDir(),
		Stdio: tool.Stdio{
			In:  strings.NewReader("plain steering\nstatus\nstop\n"),
			Out: &out,
			Err: &errb,
		},
	}
	code := run(rc, []string{"run", "--id", "repl", "--goal", "scripted", "--agent", "stub"})
	if code != 0 {
		t.Fatalf("code = %d, err = %s", code, errb.String())
	}
	if len(r.prompts) != 1 || !strings.Contains(r.prompts[0], "plain steering") {
		t.Fatalf("prompts = %#v, want plain steering", r.prompts)
	}
	if got := out.String(); !strings.Contains(got, "repl\tidle\tscripted") || !strings.Contains(got, "done") {
		t.Fatalf("repl output = %q", got)
	}
}
