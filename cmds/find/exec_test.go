package findcmd

// -exec / -ok tests, hermetic: the utility spawned is this test binary
// re-executed in helper mode (no shell, no system tools), so they run
// identically on every platform.

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestMain(m *testing.M) {
	if os.Getenv("FIND_TEST_HELPER") == "1" {
		// Helper mode: print each argv element on its own line — one
		// line per element proves argv boundaries survived intact.
		if os.Getenv("FIND_HELPER_QUIET") != "1" {
			for _, a := range os.Args[1:] {
				fmt.Println(a)
			}
		}
		code := 0
		if s := os.Getenv("FIND_HELPER_EXIT"); s != "" {
			code, _ = strconv.Atoi(s)
		}
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func helperBin(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	return exe
}

func helperEnv(extra ...string) []string {
	return append(append(os.Environ(), "FIND_TEST_HELPER=1"), extra...)
}

// runFindExec is runFind with an environment and stdin, for the
// spawning surfaces.
func runFindExec(t *testing.T, dir, stdin string, env []string, args ...string) (out, errOut string, code int) {
	t.Helper()
	var o, e bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &o, Err: &e},
	}
	code = cmd.Run(rc, args)
	return o.String(), e.String(), code
}

func TestFindExecSemicolon(t *testing.T) {
	dir := setupTree(t)
	bin := helperBin(t)

	out, errb, code := runFindExec(t, dir, "", helperEnv(),
		".", "-name", "a.txt", "-exec", bin, "ARG1", "{}", ";")
	if out != "ARG1\n./a.txt\n" || code != 0 {
		t.Errorf("-exec ;: out=%q code=%d err=%q", out, code, errb)
	}

	// GNU substitutes {} anywhere inside an argument.
	out, _, code = runFindExec(t, dir, "", helperEnv(),
		".", "-name", "a.txt", "-exec", bin, "pre[{}]post", ";")
	if out != "pre[./a.txt]post\n" || code != 0 {
		t.Errorf("embedded {}: out=%q code=%d", out, code)
	}

	// One invocation per file.
	out, _, code = runFindExec(t, dir, "", helperEnv(),
		".", "-name", "*.go", "-exec", bin, "GO", "{}", ";")
	if out != "GO\n./b.go\nGO\n./sub/deep/d.go\n" || code != 0 {
		t.Errorf("per-file invocations: out=%q code=%d", out, code)
	}
}

func TestFindExecArgvInjectionSafe(t *testing.T) {
	dir := t.TempDir()
	// A name full of shell metacharacters must arrive as ONE argv
	// element, never interpreted: find builds argv directly.
	name := "a b;c $(boom) && rm.txt"
	writeFile(t, dir, name, "x")
	out, errb, code := runFindExec(t, dir, "", helperEnv(),
		".", "-type", "f", "-exec", helperBin(t), "{}", ";")
	if out != "./"+name+"\n" || code != 0 {
		t.Errorf("metacharacter operand: out=%q code=%d err=%q", out, code, errb)
	}
}

func TestFindExecStatusSemantics(t *testing.T) {
	dir := setupTree(t)
	bin := helperBin(t)

	// ; form: child exit status is the primary's truth value…
	out, _, code := runFindExec(t, dir, "", helperEnv("FIND_HELPER_QUIET=1"),
		".", "-name", "a.txt", "-exec", bin, "{}", ";", "-print")
	if out != "./a.txt\n" || code != 0 {
		t.Errorf("-exec true -print: out=%q code=%d", out, code)
	}
	out, _, code = runFindExec(t, dir, "", helperEnv("FIND_HELPER_QUIET=1", "FIND_HELPER_EXIT=1"),
		".", "-name", "a.txt", "-exec", bin, "{}", ";", "-print")
	// …and a failing child does not change find's own exit status.
	if out != "" || code != 0 {
		t.Errorf("-exec false -print: out=%q code=%d, want no output, exit 0", out, code)
	}
}

func TestFindExecPlus(t *testing.T) {
	dir := setupTree(t)
	bin := helperBin(t)

	// One batched invocation: the MARK argument appears once, then
	// every path in traversal order.
	out, errb, code := runFindExec(t, dir, "", helperEnv(),
		".", "-type", "f", "-exec", bin, "MARK", "{}", "+")
	want := "MARK\n./a.txt\n./b.go\n./empty.txt\n./skipme/e.txt\n./sub/c.txt\n./sub/deep/d.go\n"
	if out != want || code != 0 {
		t.Errorf("-exec +: out=%q code=%d err=%q, want %q", out, code, errb, want)
	}

	// POSIX: a non-zero batched invocation makes find itself exit
	// non-zero (unlike the ; form).
	_, _, code = runFindExec(t, dir, "", helperEnv("FIND_HELPER_QUIET=1", "FIND_HELPER_EXIT=3"),
		".", "-type", "f", "-exec", bin, "{}", "+")
	if code != 1 {
		t.Errorf("-exec + with failing child: code=%d, want 1", code)
	}

	// The + form always evaluates true: chained -print still fires.
	out, _, code = runFindExec(t, dir, "", helperEnv("FIND_HELPER_QUIET=1"),
		".", "-name", "a.txt", "-exec", bin, "{}", "+", "-print")
	if out != "./a.txt\n" || code != 0 {
		t.Errorf("-exec + -print: out=%q code=%d", out, code)
	}

	// A '+' not immediately after {} is a plain argument of the ; form.
	out, _, code = runFindExec(t, dir, "", helperEnv(),
		".", "-name", "a.txt", "-exec", bin, "+", "{}", ";")
	if out != "+\n./a.txt\n" || code != 0 {
		t.Errorf("literal + argument: out=%q code=%d", out, code)
	}
}

func TestFindOk(t *testing.T) {
	dir := setupTree(t)
	bin := helperBin(t)

	// "y" runs the utility; the prompt goes to stderr.
	out, errb, code := runFindExec(t, dir, "y\n", helperEnv(),
		".", "-name", "a.txt", "-ok", bin, "{}", ";")
	if out != "./a.txt\n" || code != 0 {
		t.Errorf("-ok yes: out=%q code=%d", out, code)
	}
	if !strings.Contains(errb, bin) || !strings.Contains(errb, "./a.txt") || !strings.Contains(errb, "?") {
		t.Errorf("-ok prompt missing pieces: err=%q", errb)
	}

	// Anything but y/Y declines: utility not run, primary false.
	out, _, code = runFindExec(t, dir, "n\n", helperEnv(),
		".", "-name", "a.txt", "-ok", bin, "{}", ";", "-print")
	if out != "" || code != 0 {
		t.Errorf("-ok no: out=%q code=%d", out, code)
	}

	// One reply per file; EOF declines the rest (and must not hang).
	out, _, code = runFindExec(t, dir, "y\nn\n", helperEnv(),
		".", "-maxdepth", "1", "-type", "f", "-ok", bin, "{}", ";")
	if out != "./a.txt\n" || code != 0 {
		t.Errorf("-ok y,n,EOF: out=%q code=%d", out, code)
	}
}

func TestFindExecCommandNotFound(t *testing.T) {
	dir := setupTree(t)
	out, errb, code := runFindExec(t, dir, "", helperEnv(),
		".", "-maxdepth", "0", "-exec", "definitely-not-a-command-xyz123", "{}", ";", "-print")
	if code != 1 || !strings.Contains(errb, "definitely-not-a-command-xyz123") {
		t.Errorf("missing utility: code=%d err=%q, want diagnostic + exit 1", code, errb)
	}
	if out != "" {
		t.Errorf("missing utility evaluated true: out=%q", out)
	}
}
