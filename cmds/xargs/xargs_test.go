package xargscmd

import (
	"bytes"
	"context"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runXargs(t *testing.T, in string, args ...string) (out, errOut string, code int) {
	t.Helper()
	var o, e bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Env:   os.Environ(),
		Stdio: tool.Stdio{In: strings.NewReader(in), Out: &o, Err: &e},
	}
	code = cmd.Run(rc, args)
	return o.String(), e.String(), code
}

func TestXargsDefaultEcho(t *testing.T) {
	// No command ⇒ echo; all items on one line.
	if out, _, _ := runXargs(t, "a b c\n"); out != "a b c\n" {
		t.Errorf("default echo = %q, want 'a b c'", out)
	}
}

func TestXargsMaxArgsBatches(t *testing.T) {
	// -n2 ⇒ two items per echo invocation.
	out, _, _ := runXargs(t, "1 2 3 4 5\n", "-n2", "echo")
	if out != "1 2\n3 4\n5\n" {
		t.Errorf("-n2 = %q, want batches of 2", out)
	}
}

func TestXargsNullDelimited(t *testing.T) {
	out, _, _ := runXargs(t, "a\x00b c\x00", "-0", "echo")
	if out != "a b c\n" { // "b c" stays one item (NUL only)
		t.Errorf("-0 = %q, want 'a b c'", out)
	}
}

func TestXargsQuotesAndBackslash(t *testing.T) {
	out, _, _ := runXargs(t, `'a b' c\ d`, "-n1", "echo")
	// items: "a b", "c d" (quote groups; backslash escapes the space)
	if out != "a b\nc d\n" {
		t.Errorf("quote/backslash split = %q, want 'a b' then 'c d'", out)
	}
}

func TestXargsReplace(t *testing.T) {
	out, _, _ := runXargs(t, "x\ny\n", "-I", "{}", "echo", "[{}]")
	if out != "[x]\n[y]\n" {
		t.Errorf("-I {} = %q, want [x] then [y]", out)
	}
}

func TestXargsDelimiter(t *testing.T) {
	out, _, _ := runXargs(t, "a,b,c", "-d", ",", "echo")
	if out != "a b c\n" {
		t.Errorf("-d , = %q, want 'a b c'", out)
	}
}

func TestXargsEOFString(t *testing.T) {
	out, _, _ := runXargs(t, "a\nb\n_END_\nc\n", "-E", "_END_", "echo")
	if out != "a b\n" {
		t.Errorf("-E stop = %q, want 'a b'", out)
	}
}

func TestXargsNoRunIfEmpty(t *testing.T) {
	// Without -r, empty input still runs the command once (no extra args).
	if out, _, _ := runXargs(t, "   \n", "echo", "ran"); out != "ran\n" {
		t.Errorf("empty without -r = %q, want 'ran'", out)
	}
	// With -r, empty input runs nothing.
	if out, _, _ := runXargs(t, "   \n", "-r", "echo", "ran"); out != "" {
		t.Errorf("empty with -r = %q, want no output", out)
	}
}

func TestXargsParallelRunsAll(t *testing.T) {
	// -P4 -n1: every item runs; order is unspecified, so compare as a set.
	out, _, _ := runXargs(t, "1 2 3 4 5 6\n", "-P4", "-n1", "echo")
	got := strings.Fields(strings.TrimSpace(out))
	sort.Strings(got)
	want := []string{"1", "2", "3", "4", "5", "6"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("-P4 -n1 ran %v, want all of %v", got, want)
	}
}

func TestXargsCommandNotFound(t *testing.T) {
	_, errOut, code := runXargs(t, "x\n", "definitely-not-a-real-cmd-xyz")
	if code != 127 {
		t.Errorf("missing command exit = %d, want 127", code)
	}
	if !strings.Contains(errOut, "command not found") {
		t.Errorf("error wording = %q", errOut)
	}
}

func TestXargsTrace(t *testing.T) {
	_, errOut, _ := runXargs(t, "a\n", "-t", "echo")
	if !strings.Contains(errOut, "echo a") {
		t.Errorf("-t trace = %q, want the command echoed to stderr", errOut)
	}
}

func TestXargsInteractiveUnsupported(t *testing.T) {
	if _, _, code := runXargs(t, "a\n", "-p", "echo"); code == 0 {
		t.Error("-p should fail loudly (unsupported)")
	}
}
