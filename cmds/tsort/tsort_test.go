package tsortcmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// runTool is the canonical test harness shape for cmds packages.
func runTool(t *testing.T, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	return runToolDir(t, t.TempDir(), stdin, args...)
}

func runToolDir(t *testing.T, dir, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

// assertTopological checks out lists exactly the given items once each
// and respects every pair ordering in pairs.
func assertTopological(t *testing.T, out string, items []string, pairs [][2]string) {
	t.Helper()
	got := strings.Fields(out)
	pos := map[string]int{}
	for i, it := range got {
		if _, dup := pos[it]; dup {
			t.Fatalf("duplicate item %q in output %q", it, out)
		}
		pos[it] = i
	}
	if len(got) != len(items) {
		t.Fatalf("output %q: got %d items, want %d", out, len(got), len(items))
	}
	for _, it := range items {
		if _, ok := pos[it]; !ok {
			t.Fatalf("output %q missing item %q", out, it)
		}
	}
	for _, p := range pairs {
		if pos[p[0]] >= pos[p[1]] {
			t.Errorf("output %q: %q must precede %q", out, p[0], p[1])
		}
	}
}

func TestTsort(t *testing.T) {
	// Simple chain.
	out, errb, code := runTool(t, "a b\nb c\n")
	if code != 0 || errb != "" {
		t.Fatalf("chain: code=%d err=%q", code, errb)
	}
	if out != "a\nb\nc\n" {
		t.Errorf("chain: out=%q", out)
	}

	// POSIX example: pairs of identical items declare presence only.
	out, _, code = runTool(t, "a b c c d e\ng g\nf g e f\nh h\n")
	if code != 0 {
		t.Fatalf("posix example: code=%d", code)
	}
	assertTopological(t, out,
		[]string{"a", "b", "c", "d", "e", "f", "g", "h"},
		[][2]string{{"a", "b"}, {"d", "e"}, {"e", "f"}, {"f", "g"}})

	// Duplicate edges are harmless.
	out, _, code = runTool(t, "a b\na b\n")
	if code != 0 || out != "a\nb\n" {
		t.Errorf("duplicate edge: out=%q code=%d", out, code)
	}

	// Empty input.
	out, errb, code = runTool(t, "")
	if code != 0 || out != "" || errb != "" {
		t.Errorf("empty: out=%q err=%q code=%d", out, errb, code)
	}
}

func TestTsortLoop(t *testing.T) {
	out, errb, code := runTool(t, "a b\nb a\n")
	if code != 1 {
		t.Fatalf("loop: code=%d", code)
	}
	if !strings.Contains(errb, "tsort: -: input contains a loop:") {
		t.Errorf("loop header: err=%q", errb)
	}
	if !strings.Contains(errb, "tsort: a\n") || !strings.Contains(errb, "tsort: b\n") {
		t.Errorf("loop members: err=%q", errb)
	}
	// All items still appear in the output.
	assertTopological(t, out, []string{"a", "b"}, nil)

	// A cycle plus a dependent tail: the tail still comes after the
	// cycle members it depends on where possible.
	out, errb, code = runTool(t, "a b\nb c\nc a\nc d\n")
	if code != 1 || !strings.Contains(errb, "input contains a loop:") {
		t.Fatalf("cycle+tail: code=%d err=%q", code, errb)
	}
	assertTopological(t, out, []string{"a", "b", "c", "d"}, [][2]string{{"c", "d"}})

	// Two independent cycles: each is reported.
	_, errb, code = runTool(t, "a b\nb a\nc d\nd c\n")
	if code != 1 || strings.Count(errb, "input contains a loop:") != 2 {
		t.Errorf("two cycles: code=%d err=%q", code, errb)
	}
}

func TestTsortFileOperand(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "g.txt"), []byte("x y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runToolDir(t, dir, "", "g.txt")
	if code != 0 || out != "x\ny\n" {
		t.Errorf("file operand: out=%q code=%d", out, code)
	}
	// Loop diagnostics name the operand as given.
	if err := os.WriteFile(filepath.Join(dir, "loop.txt"), []byte("a b\nb a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runToolDir(t, dir, "", "loop.txt")
	if code != 1 || !strings.Contains(errb, "tsort: loop.txt: input contains a loop:") {
		t.Errorf("file loop: code=%d err=%q", code, errb)
	}
	// "-" reads standard input.
	out, _, code = runToolDir(t, dir, "p q\n", "-")
	if code != 0 || out != "p\nq\n" {
		t.Errorf("dash operand: out=%q code=%d", out, code)
	}
	_, errb, code = runToolDir(t, dir, "", "nonexistent")
	if code != 1 || !strings.Contains(errb, "nonexistent") {
		t.Errorf("missing file: code=%d err=%q", code, errb)
	}
}

func TestTsortErrors(t *testing.T) {
	// Odd token count.
	_, errb, code := runTool(t, "a b\nc\n")
	if code != 1 || !strings.Contains(errb, "tsort: -: input contains an odd number of tokens") {
		t.Errorf("odd tokens: code=%d err=%q", code, errb)
	}
	// Extra operand.
	_, errb, code = runTool(t, "", "one", "two")
	if code != 2 || !strings.Contains(errb, "extra operand 'two'") {
		t.Errorf("extra operand: code=%d err=%q", code, errb)
	}
	// Unknown flag: contract error, exit 2, names the flag.
	_, errb, code = runTool(t, "", "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestTsortPOSIXExample(t *testing.T) {
	// POSIX.1-2024 example; the standard lists this exact order.
	out, errb, code := runTool(t, "a b c c d e\ng g\nf g e f\nh h\n")
	if code != 0 || errb != "" {
		t.Fatalf("posix example: code=%d err=%q", code, errb)
	}
	want := "a\nb\nc\nd\ne\nf\ng\nh\n"
	if out != want {
		t.Errorf("posix example: out=%q, want %q", out, want)
	}
}

func TestTsortWarn(t *testing.T) {
	// -w with no cycles exits 0.
	out, errb, code := runTool(t, "a b\n", "-w")
	if code != 0 || errb != "" || out != "a\nb\n" {
		t.Errorf("no cycles -w: out=%q err=%q code=%d", out, errb, code)
	}

	// -w with one cycle exits 1 and still emits the diagnostic.
	_, errb, code = runTool(t, "a b\nb a\n", "-w")
	if code != 1 || !strings.Contains(errb, "input contains a loop:") {
		t.Errorf("one cycle -w: code=%d err=%q", code, errb)
	}

	// -w with multiple cycles exits the cycle count.
	_, errb, code = runTool(t, "a b\nb a\nc d\nd c\n", "-w")
	if code != 2 || strings.Count(errb, "input contains a loop:") != 2 {
		t.Errorf("two cycles -w: code=%d err=%q", code, errb)
	}

	// -w caps the exit status at the implementation-defined maximum.
	var b strings.Builder
	for i := 0; i < 130; i++ {
		fmt.Fprintf(&b, "%d %d\n%d %d\n", i*2, i*2+1, i*2+1, i*2)
	}
	_, errb, code = runTool(t, b.String(), "-w")
	if code != 124 {
		t.Errorf("many cycles -w: code=%d, want 124", code)
	}
	if strings.Count(errb, "input contains a loop:") != 130 {
		t.Errorf("many cycles -w: reported %d loops, want 130", strings.Count(errb, "input contains a loop:"))
	}
}

func TestTsortHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: tsort") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "", "--version")
	if code != 0 || !strings.Contains(out, "tsort") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}

	// -h and -V are accepted as aliases for --help and --version.
	out, _, code = runTool(t, "", "-h")
	if code != 0 || !strings.Contains(out, "Usage: tsort") {
		t.Errorf("-h: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "", "-V")
	if code != 0 || !strings.Contains(out, "tsort") {
		t.Errorf("-V: code=%d out=%q", code, out)
	}
}
