package shell

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"

	_ "github.com/qiangli/coreutils/cmds/cat"
	_ "github.com/qiangli/coreutils/cmds/head"
	_ "github.com/qiangli/coreutils/cmds/tail"
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
			fmt.Fprintf(rc.Out, "sigpipe_ignored=%v\n", rc.SIGPIPEIgnored)
			fmt.Fprintf(rc.Out, "dir_is_process_cwd=%v\n", rc.DirIsProcessCwd)
			if len(args) > 0 && args[0] == "fail" {
				fmt.Fprintln(rc.Err, "probe: deliberate failure")
				return 7
			}
			return 0
		},
	})
	tool.Register(&tool.Tool{
		Name:     "pathprobe",
		Synopsis: "test path mapping",
		Usage:    "pathprobe OPERAND",
		Run: func(rc *tool.RunContext, args []string) int {
			fmt.Fprintln(rc.Out, rc.Path(args[0]))
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

func TestHandlerDeclaresMatchingProcessCwd(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	out, errb, err := runScript(t, "probe", cwd, Handler())
	if err != nil {
		t.Fatalf("probe: %v; stderr=%q", err, errb)
	}
	if !strings.Contains(out, "dir_is_process_cwd=true\n") {
		t.Fatalf("matching process cwd was not declared native:\n%s", out)
	}
}

func TestHandlerPreservesLongRelativeOperandAtMatchingProcessCwd(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	operand := strings.Repeat("x", 40000)
	out, errb, err := runScript(t, "pathprobe "+operand, cwd, Handler())
	if err != nil {
		t.Fatalf("pathprobe: %v; stderr=%q", err, errb)
	}
	if out != operand+"\n" {
		t.Fatalf("long relative operand was materialized against cwd: got length %d, want %d", len(strings.TrimSuffix(out, "\n")), len(operand))
	}
}

func TestHandlerDoesNotDeclareVirtualCwdNative(t *testing.T) {
	out, errb, err := runScript(t, "probe", t.TempDir(), Handler())
	if err != nil {
		t.Fatalf("probe: %v; stderr=%q", err, errb)
	}
	if !strings.Contains(out, "dir_is_process_cwd=false\n") {
		t.Fatalf("virtual cwd was declared native:\n%s", out)
	}
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

// TestHandlerTailOptionBoundaryAndParentOperand exercises the same public
// applet contract through the embedded-shell adapter used by Bashy.  It pins
// both argv preservation across -- and resolution of ../FILE after the shell
// changes only its logical working directory.
func TestHandlerTailOptionBoundaryAndParentOperand(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	data := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\nline11\nline12\n"
	if err := os.WriteFile(filepath.Join(root, "parent-input.txt"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	script := "POSIXLY_CORRECT=1; export POSIXLY_CORRECT\n" +
		"tail -n 1 -- parent-input.txt\n" +
		"cd child\n" +
		"tail ../parent-input.txt\n"
	out, errb, err := runScript(t, script, root, Handler())
	if err != nil {
		t.Fatalf("embedded tail invocation: %v; stderr=%q", err, errb)
	}
	want := "line12\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\nline11\nline12\n"
	if out != want || errb != "" {
		t.Fatalf("embedded tail output=(%q, %q), want (%q, empty stderr)", out, errb, want)
	}
}

func TestHandlerSIGPIPEIgnored(t *testing.T) {
	const script = `
probe
trap '' PIPE
probe
trap 'echo caught' PIPE
probe
trap - PIPE
probe
`
	out, errb, err := runScript(t, script, t.TempDir(), Handler())
	if err != nil {
		t.Fatalf("sequential signal disposition script: %v\nstderr:\n%s", err, errb)
	}
	if errb != "" {
		t.Fatalf("sequential signal disposition stderr=%q, want empty", errb)
	}
	var got []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "sigpipe_ignored=") {
			got = append(got, strings.TrimPrefix(line, "sigpipe_ignored="))
		}
	}
	want := []string{"false", "true", "false", "false"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("sequential SIGPIPE dispositions=%v, want %v\nfull output:\n%s", got, want, out)
	}
}
