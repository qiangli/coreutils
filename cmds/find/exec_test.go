package findcmd

// -exec / -ok tests, hermetic: the utility spawned is this test binary
// re-executed in helper mode (no shell, no system tools), so they run
// identically on every platform.

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestMain(m *testing.M) {
	if os.Getenv("FIND_TEST_HELPER") == "1" {
		// Helper mode: print each argv element on its own line — one
		// line per element proves argv boundaries survived intact.
		if os.Getenv("FIND_HELPER_ARGV0") == "1" {
			fmt.Println(os.Args[0])
		}
		if os.Getenv("FIND_HELPER_QUIET") != "1" {
			for _, a := range os.Args[1:] {
				fmt.Println(a)
			}
		}
		// FIND_HELPER_LOG=<path>: append every argument after argv[0],
		// one per line, to the named file — a real filesystem side
		// effect the parent can count per invocation and per path.
		if p := os.Getenv("FIND_HELPER_LOG"); p != "" {
			if f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
				for _, a := range os.Args[1:] {
					fmt.Fprintln(f, a)
				}
				f.Close()
			}
		}
		// Optionally kill ourselves with a signal so the parent can
		// assert the 128+signal exit-status mapping (no-op on Windows,
		// which has no POSIX signals — the test that uses it is unix).
		if s := os.Getenv("FIND_HELPER_SIGNAL"); s != "" {
			n, _ := strconv.Atoi(s)
			raiseSelfSignal(n)
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

// TestFindOkAffirmationAnchoredAtLineStart pins that the -ok reply is
// matched anchored at the start of the line, per the POSIX locale's
// yesexpr "^[yY]" (GNU rpmatch and BSD find agree): a reply whose FIRST
// character is not affirmative — including leading whitespace before a
// 'y' — declines, the utility is not invoked, and the primary is false.
// Trailing text after the leading 'y' is irrelevant ("yes", "y "), and a
// declined -ok leaves find's own exit status 0.
func TestFindOkAffirmationAnchoredAtLineStart(t *testing.T) {
	dir := setupTree(t)
	bin := helperBin(t)

	decline := []string{" y\n", "\ty\n", " yes\n", " j\n"}
	for _, reply := range decline {
		out, _, code := runFindExec(t, dir, reply, helperEnv(),
			".", "-name", "a.txt", "-ok", bin, "{}", ";", "-print")
		if out != "" || code != 0 {
			t.Errorf("-ok reply %q: out=%q code=%d, want declined (no output, exit 0)", reply, out, code)
		}
	}

	affirm := []string{"y\n", "Y\n", "yes\n", "y \n"}
	for _, reply := range affirm {
		out, _, code := runFindExec(t, dir, reply, helperEnv(),
			".", "-name", "a.txt", "-ok", bin, "{}", ";")
		if out != "./a.txt\n" || code != 0 {
			t.Errorf("-ok reply %q: out=%q code=%d, want utility run", reply, out, code)
		}
	}

	// The German affirmative is anchored the same way.
	german := append(helperEnv(), "LC_ALL=de_DE.iso88591")
	out, _, code := runFindExec(t, dir, " j\n", german,
		".", "-name", "a.txt", "-ok", bin, "{}", ";", "-print")
	if out != "" || code != 0 {
		t.Errorf("de_DE -ok reply \" j\": out=%q code=%d, want declined", out, code)
	}
	out, _, code = runFindExec(t, dir, "ja\n", german,
		".", "-name", "a.txt", "-ok", bin, "{}", ";")
	if out != "./a.txt\n" || code != 0 {
		t.Errorf("de_DE -ok reply \"ja\": out=%q code=%d, want utility run", out, code)
	}
}

// TestFindExecChildArgvZeroIsNameAsGiven pins the execvp convention GNU
// and BSD find follow: the invoked utility's argv[0] is the utility name
// exactly as written on the find command line, while the file executed is
// the PATH-resolved binary. Rewriting argv[0] to the resolved path breaks
// command composition with argv[0]-dispatched children (busybox-style
// multicall binaries, shells reporting $0).
func TestFindExecChildArgvZeroIsNameAsGiven(t *testing.T) {
	dir := setupTree(t)
	bin := helperBin(t)
	name := filepath.Base(bin)

	// Resolve the bare helper name through PATH so the typed name and the
	// resolved path genuinely differ.
	env := append(helperEnv("FIND_HELPER_ARGV0=1", "FIND_HELPER_QUIET=1"),
		"PATH="+filepath.Dir(bin))
	out, errb, code := runFindExec(t, dir, "", env,
		".", "-name", "a.txt", "-exec", name, "{}", ";")
	if out != name+"\n" || code != 0 {
		t.Errorf("PATH-resolved utility argv[0]: out=%q code=%d err=%q, want %q",
			out, code, errb, name+"\n")
	}

	// A name given as a path keeps that exact spelling as argv[0] too.
	out, _, code = runFindExec(t, dir, "", env,
		".", "-name", "a.txt", "-exec", bin, "{}", ";")
	if out != bin+"\n" || code != 0 {
		t.Errorf("absolute utility argv[0]: out=%q code=%d, want %q", out, code, bin+"\n")
	}
}

// TestFindExecPlusGrammar pins the strict POSIX grammar of the batched
// '{} +' form: exactly one standalone '{}', immediately before '+', with
// a preceding utility. Anything else is a usage error (exit 2), never a
// silent pass of literal braces.
func TestFindExecPlusGrammar(t *testing.T) {
	dir := setupTree(t)
	bin := helperBin(t)

	bad := []struct {
		desc string
		args []string
	}{
		{"second standalone {}", []string{".", "-name", "a.txt", "-exec", bin, "{}", "{}", "+"}},
		{"leading standalone {}", []string{".", "-name", "a.txt", "-exec", bin, "{}", "x", "{}", "+"}},
		{"embedded {} in fixed arg", []string{".", "-name", "a.txt", "-exec", bin, "pre{}post", "{}", "+"}},
		{"no utility before {}", []string{".", "-name", "a.txt", "-exec", "{}", "+"}},
	}
	for _, tc := range bad {
		out, errb, code := runFindExec(t, dir, "", helperEnv(), tc.args...)
		if code != 2 {
			t.Errorf("%s: code=%d, want 2 (usage error); out=%q err=%q", tc.desc, code, out, errb)
		}
		if !strings.Contains(errb, "find") {
			t.Errorf("%s: err=%q, want a find diagnostic", tc.desc, errb)
		}
		if out != "" {
			t.Errorf("%s: produced output %q, want none", tc.desc, out)
		}
	}

	// The one valid shape still works and batches every match once.
	out, errb, code := runFindExec(t, dir, "", helperEnv(),
		".", "-name", "*.go", "-exec", bin, "MARK", "{}", "+")
	if out != "MARK\n./b.go\n./sub/deep/d.go\n" || code != 0 {
		t.Errorf("valid {} +: out=%q code=%d err=%q", out, code, errb)
	}
}

// TestOwnerCacheConcurrent exercises the per-invocation -nouser/-nogroup
// lookup cache from many goroutines at once. Run under -race it proves
// the cache is concurrency-safe (the former package-global maps were
// unsynchronized). It also confirms fresh caches are independent, so a
// lookup outcome cannot leak across unrelated find invocations.
func TestOwnerCacheConcurrent(t *testing.T) {
	oc := newOwnerCache()
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(base uint32) {
			defer wg.Done()
			for i := uint32(0); i < 256; i++ {
				oc.nameExists(base+(i%32), i%2 == 0)
			}
		}(uint32(g) * 1000)
	}
	wg.Wait()

	// A second cache shares no state with the first — the guarantee that
	// replaced the leaky package globals.
	other := newOwnerCache()
	if &oc.uids == &other.uids || &oc.gids == &other.gids {
		t.Fatal("newOwnerCache returned shared maps")
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
