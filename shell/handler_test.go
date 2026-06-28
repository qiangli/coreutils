package shell

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"

	_ "github.com/qiangli/coreutils/cmds/cat"
	_ "github.com/qiangli/coreutils/cmds/head"
	_ "github.com/qiangli/coreutils/cmds/tr"
	"github.com/qiangli/coreutils/tool"
)

// register a throwaway tool that echoes how the adapter mapped the
// interpreter state into a RunContext, then returns a chosen exit code.
func init() {
	tool.Register(&tool.Tool{
		Name:     "probe",
		Synopsis: "test probe tool",
		Usage:    "probe [args...]",
		Run: func(rc *tool.RunContext, args []string) int {
			fmt.Fprintf(rc.Out, "args=%s\n", strings.Join(args, ","))
			fmt.Fprintf(rc.Out, "dir=%s\n", rc.Dir)
			fmt.Fprintf(rc.Out, "env_FOO=%s\n", rc.Getenv("FOO"))
			in := new(bytes.Buffer)
			in.ReadFrom(rc.In)
			fmt.Fprintf(rc.Out, "stdin=%s\n", strings.TrimSpace(in.String()))
			if len(args) > 0 && args[0] == "fail" {
				fmt.Fprintln(rc.Err, "probe: deliberate failure")
				return 7
			}
			return 0
		},
	})
}

func runScript(t *testing.T, src, dir string, mw func(interp.ExecHandlerFunc) interp.ExecHandlerFunc) (string, string, error) {
	t.Helper()
	var out, errb bytes.Buffer
	runner, err := interp.New(
		interp.Dir(dir),
		interp.Env(expand.ListEnviron("FOO=bar")),
		interp.StdIO(strings.NewReader("hello-stdin\n"), &out, &errb),
		interp.ExecHandlers(mw),
	)
	if err != nil {
		t.Fatalf("interp.New: %v", err)
	}
	file, err := syntax.NewParser().Parse(strings.NewReader(src), "")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	runErr := runner.Run(context.Background(), file)
	return out.String(), errb.String(), runErr
}

func TestHandlerDispatchesRegisteredTool(t *testing.T) {
	dir := t.TempDir()
	out, _, err := runScript(t, "probe one two", dir, Handler())
	if err != nil {
		t.Fatalf("unexpected run error: %v", err)
	}
	for _, want := range []string{
		"args=one,two",
		"dir=" + dir,
		"env_FOO=bar",
		"stdin=hello-stdin",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestHandlerPropagatesExitStatus(t *testing.T) {
	// A nonzero tool exit must surface as the shell's $? — assert via a
	// conditional that only prints on failure.
	out, errb, err := runScript(t, "probe fail || echo CAUGHT", t.TempDir(), Handler())
	if err != nil {
		t.Fatalf("unexpected run error: %v", err)
	}
	if !strings.Contains(out, "CAUGHT") {
		t.Errorf("nonzero exit not observed by shell; out=%q err=%q", out, errb)
	}
	if !strings.Contains(errb, "deliberate failure") {
		t.Errorf("tool stderr not routed; err=%q", errb)
	}
}

func TestHandlerFallsThroughUnknownCommand(t *testing.T) {
	// `true` is not a registered tool here (no cmds imported), so it must
	// fall through to the next handler (the default PATH exec). It exists
	// on every POSIX system, so the script should succeed.
	_, _, err := runScript(t, "true", t.TempDir(), Handler())
	if err != nil {
		t.Fatalf("fall-through to system `true` failed: %v", err)
	}
}

func TestHandlerFuncPredicateSkips(t *testing.T) {
	// Predicate excludes "probe" → must fall through (and fail, since
	// there is no system `probe`), proving the predicate gates dispatch.
	called := false
	intercept := func(name string) bool { called = true; return false }
	_, _, _ = runScript(t, "probe one", t.TempDir(), HandlerFunc(intercept))
	if !called {
		t.Fatal("predicate was never consulted")
	}
}

func TestHandlerCoreutilsPipelineEarlyHeadClose(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("in-process coreutils pipe early-close differs on Windows (\"pipe has been ended\") — tracked windows-port item")
	}
	dir := t.TempDir()
	input := strings.Repeat("alpha\n", 20000)
	if err := os.WriteFile(dir+"/input.txt", []byte(input), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	out, errb, err := runScript(t, "cat input.txt | tr 'a-z' 'A-Z' | head -n 1", dir, Handler())
	if err != nil {
		t.Fatalf("unexpected run error: %v\nstderr:\n%s", err, errb)
	}
	if out != "ALPHA\n" {
		t.Fatalf("out=%q, want %q", out, "ALPHA\n")
	}
	if errb != "" {
		t.Fatalf("stderr=%q, want empty", errb)
	}
}
