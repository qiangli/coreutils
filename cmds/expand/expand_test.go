package expandcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runExpand(t *testing.T, input string, args ...string) (string, string, int) {
	t.Helper()
	var out, err bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Stdio: tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &err}}
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

func TestExpandNoUTF8FlagRemoved(t *testing.T) {
	_, stderr, code := runExpand(t, "x\n", "-U")
	if code != 2 || !strings.Contains(stderr, "U") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
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
