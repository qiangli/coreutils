package expandcmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runExpand(t *testing.T, input string, args ...string) (string, string, int) {
	t.Helper()
	var out, err bytes.Buffer
	// Pin a UTF-8 locale so display-column expectations do not depend on
	// the runner's environment; the C/POSIX byte model has its own tests.
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(), Env: []string{"LC_ALL=C.UTF-8"},
		Stdio: tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &err},
	}
	code := run(rc, args)
	return out.String(), err.String(), code
}

func TestExpandDefaultTabsFromStdin(t *testing.T) {
	out, stderr, code := runExpand(t, "a\tb\n\tz\n")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "a       b\n        z\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestExpandIssue7AttachedTablistAndRepeatedStdinOperands(t *testing.T) {
	out, stderr, code := runExpand(t, "a\tb\nc\td\n", "-t4", "-", "-")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "a   b\nc   d\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestExpandCustomTabsAndFile(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "in.txt")
	if err := os.WriteFile(name, []byte("a\tb\tc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &stderr}}
	code := run(rc, []string{"-t", "4", "in.txt"})
	if code != 0 || stderr.String() != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	if want := "a   b   c\n"; out.String() != want {
		t.Fatalf("out=%q want %q", out.String(), want)
	}
}

func TestExpandOptionPermutationModes(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []struct{ name, content string }{
		{"input", "a\tb\n"},
		{"-t", "middle\n"},
		{"4", "tail\n"},
	} {
		if err := os.WriteFile(filepath.Join(dir, f.name), []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name string
		env  []string
		want string
	}{
		{"GNU default permutes options", []string{"LC_ALL=C"}, "a   b\n"},
		{"POSIX stops at operand", []string{"LC_ALL=C", "POSIXLY_CORRECT=1"}, "a       b\nmiddle\ntail\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, stderr bytes.Buffer
			rc := &tool.RunContext{
				Ctx: context.Background(), Dir: dir, Env: tc.env,
				Stdio: tool.Stdio{Out: &out, Err: &stderr},
			}
			code := run(rc, []string{"input", "-t", "4"})
			if code != 0 || stderr.String() != "" || out.String() != tc.want {
				t.Fatalf("option parsing mode = (%q, %q, %d), want %q", out.String(), stderr.String(), code, tc.want)
			}
		})
	}
}

func TestExpandInitialOnly(t *testing.T) {
	out, stderr, code := runExpand(t, "\t x\t y\n", "-i", "-t", "4")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "     x\t y\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestExpandInitialBackspaceEndsInitialRegion(t *testing.T) {
	// A backspace is a non-blank: it ends the initial region under -i,
	// so tabs after it stay tabs.
	out, stderr, code := runExpand(t, "\b\tx\n", "-i", "-t", "4")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "\b\tx\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestExpandNoUTF8CountsBytes(t *testing.T) {
	out, stderr, code := runExpand(t, "é\tx\n", "-t", "4")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "é   x\n"; out != want {
		t.Fatalf("default UTF-8 out=%q want %q", out, want)
	}
	out, stderr, code = runExpand(t, "é\tx\n", "-U", "-t", "4")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "é  x\n"; out != want {
		t.Fatalf("-U out=%q want %q", out, want)
	}
}

func TestExpandWideRuneCountsDisplayColumns(t *testing.T) {
	out, stderr, code := runExpand(t, "漢\tx\n", "-t", "8")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "漢      x\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestExpandCombiningMarkIsZeroWidth(t *testing.T) {
	out, stderr, code := runExpand(t, "e\u0301\tx\n", "-t", "8")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "e\u0301       x\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestExpandBackspaceDecrementsColumn(t *testing.T) {
	// "ab\b\t": the backspace moves back to column 1, so the tab
	// expands from there to column 4 (3 spaces).
	out, stderr, code := runExpand(t, "ab\b\tx\n", "-t", "4")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "ab\b   x\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestExpandTabListIncrement(t *testing.T) {
	// GNU --tabs=1,+8 sets stops at 1, 9, 17, ...
	out, stderr, code := runExpand(t, "\ta\tb\tc\n", "--tabs=1,+8")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := " a       b       c\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestExpandTabListExtend(t *testing.T) {
	// GNU --tabs=2,4,/8 sets stops at 2, 4, and every multiple of 8.
	out, stderr, code := runExpand(t, "\ta\tb\tc\td\n", "--tabs=2,4,/8")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "  a b   c       d\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestExpandRepeatedTabsAccumulate(t *testing.T) {
	// expand -t2 -t4 is the same as -t2,4.
	out, stderr, code := runExpand(t, "\ta\tb\tc\n", "-t", "2", "-t", "4")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "  a b c\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestExpandBlankSeparatedTabList(t *testing.T) {
	out, stderr, code := runExpand(t, "\ta\tb\n", "--tabs=2 4")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "  a b\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestExpandTabsBeyondLastStopBecomeSingleSpaces(t *testing.T) {
	out, stderr, code := runExpand(t, "\ta\tb\tc\n", "-t", "2,4")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "  a b c\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestExpandRejectsBadTabs(t *testing.T) {
	_, stderr, code := runExpand(t, "", "-t", "0")
	if code != 2 || !strings.Contains(stderr, "tab size cannot be 0") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	_, stderr, code = runExpand(t, "", "-t", "4,2")
	if code != 2 || !strings.Contains(stderr, "tab sizes must be ascending") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	_, stderr, code = runExpand(t, "", "-t", "x")
	if code != 2 || !strings.Contains(stderr, "invalid character") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	_, stderr, code = runExpand(t, "", "-t", "+2,8")
	if code != 2 || !strings.Contains(stderr, "'+' specifier only allowed with the last value") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	_, stderr, code = runExpand(t, "", "-t", "2,,4")
	if code != 2 || !strings.Contains(stderr, "invalid character") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	_, stderr, code = runExpand(t, "", "-t", "")
	if code != 2 || !strings.Contains(stderr, "invalid character") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	_, stderr, code = runExpand(t, "", "-t", "999999999999999999999999999999")
	if code != 2 || !strings.Contains(stderr, "tab stop is too large") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

// POSIX requires immediate termination on difficulty accessing an operand;
// GNU's default extension diagnoses it and continues with later operands.
func TestExpandOperandAccessFailureModes(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  []string
		want string
	}{
		{name: "GNU default continues", want: "a   b\n"},
		{name: "POSIX terminates", env: []string{"POSIXLY_CORRECT="}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "good.txt"), []byte("a\tb\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			var out, stderr bytes.Buffer
			rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: tc.env, Stdio: tool.Stdio{Out: &out, Err: &stderr}}
			code := run(rc, []string{"-t", "4", "nosuch.txt", "good.txt"})
			if code != 1 || !strings.Contains(stderr.String(), "expand: nosuch.txt:") {
				t.Fatalf("missing operand = (%q, %d), want exit 1 naming the operand", stderr.String(), code)
			}
			if out.String() != tc.want {
				t.Fatalf("output after inaccessible operand = %q, want %q", out.String(), tc.want)
			}
		})
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write(p []byte) (int, error) { return 0, w.err }

func TestExpandStandardOutputWriteError(t *testing.T) {
	var stderr bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader("a\tb\n"), Out: failingWriter{err: errors.New("device full")}, Err: &stderr},
	}
	code := run(rc, []string{"-t", "4"})
	if code != 1 || !strings.Contains(stderr.String(), "expand: write error:") || !strings.Contains(stderr.String(), "device full") {
		t.Fatalf("write error = (%q, %d), want exit 1 with write-error diagnostic", stderr.String(), code)
	}
}

func TestExpandParseTabStops(t *testing.T) {
	cases := []struct {
		in   []string
		cols []int // column before each tab
		want []int // resulting next stop
	}{
		{[]string{"8"}, []int{0, 7, 8}, []int{8, 8, 16}},
		{[]string{"1,+8"}, []int{0, 1, 5, 9}, []int{1, 9, 9, 17}},
		{[]string{"2,4,/8"}, []int{0, 3, 4, 9}, []int{2, 4, 8, 16}},
		{[]string{"+8"}, []int{0, 8}, []int{8, 16}},
	}
	for _, c := range cases {
		ts, err := parseTabStops(c.in)
		if err != nil {
			t.Fatalf("parseTabStops(%v): %v", c.in, err)
		}
		for i, col := range c.cols {
			got, _ := ts.next(col)
			if got != c.want[i] {
				t.Errorf("tabs %v next(%d)=%d want %d", c.in, col, got, c.want[i])
			}
		}
	}
}
