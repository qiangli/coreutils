package unexpandcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runUnexpand(t *testing.T, input string, args ...string) (string, string, int) {
	t.Helper()
	var out, err bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Stdio: tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &err}}
	code := run(rc, args)
	return out.String(), err.String(), code
}

func TestUnexpandLeadingBlanks(t *testing.T) {
	out, stderr, code := runUnexpand(t, "        x\nx        y\n")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "\tx\nx        y\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestUnexpandAllAndFile(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "in.txt")
	if err := os.WriteFile(name, []byte("x   y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &stderr}}
	code := run(rc, []string{"-a", "-t", "4", "in.txt"})
	if code != 0 || stderr.String() != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	if want := "x\ty\n"; out.String() != want {
		t.Fatalf("out=%q want %q", out.String(), want)
	}
}

func TestUnexpandTabsImpliesAll(t *testing.T) {
	out, stderr, code := runUnexpand(t, "x   y\n", "-t", "4")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "x\ty\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestUnexpandNoUTF8CountsBytes(t *testing.T) {
	out, stderr, code := runUnexpand(t, "é  x\n", "-a", "-t", "4")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "é  x\n"; out != want {
		t.Fatalf("default UTF-8 out=%q want %q", out, want)
	}
	out, stderr, code = runUnexpand(t, "é  x\n", "-U", "-a", "-t", "4")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "é\tx\n"; out != want {
		t.Fatalf("-U out=%q want %q", out, want)
	}
}

func TestUnexpandFirstOnlyOverridesAll(t *testing.T) {
	out, stderr, code := runUnexpand(t, "x   y\n", "-a", "-f", "-t", "4")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "x   y\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestUnexpandSingleInteriorSpaceStays(t *testing.T) {
	// -a converts only runs of two or more blanks before a stop; a
	// single interior space is never converted.
	out, stderr, code := runUnexpand(t, "aaaaaaa b\n", "-a")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "aaaaaaa b\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
	// Two blanks reaching the stop do convert.
	out, _, _ = runUnexpand(t, "aaaaaa  b\n", "-a")
	if want := "aaaaaa\tb\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestUnexpandSingleLeadingBlankConverts(t *testing.T) {
	// A single blank at the start of a line may convert (the line
	// start acts as if preceded by a blank).
	out, stderr, code := runUnexpand(t, " x\n", "--first-only", "-t", "1")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "\tx\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestUnexpandBlanksBeyondLastStopUnchanged(t *testing.T) {
	out, stderr, code := runUnexpand(t, "      x\n", "-t", "2,4")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "\t\t  x\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestUnexpandSpaceBeforeTabAbsorbed(t *testing.T) {
	// A maximal run of blanks is one unit: the space is absorbed into
	// the following tab.
	out, stderr, code := runUnexpand(t, "a \tb\n", "-a")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "a\tb\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestUnexpandBackspaceDecrementsColumn(t *testing.T) {
	// After "ab\b" the column is 1, so two blanks reach the stop at 4
	// only when they span columns 2..4.
	out, stderr, code := runUnexpand(t, "ab\b   c\n", "-a", "-t", "4")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "ab\b\tc\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestUnexpandRepeatedTabsAccumulate(t *testing.T) {
	out, stderr, code := runUnexpand(t, "      x\n", "-t", "2", "-t", "4")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "\t\t  x\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestUnexpandRejectsBadTabs(t *testing.T) {
	_, stderr, code := runUnexpand(t, "", "--tabs=4,2")
	if code != 2 || !strings.Contains(stderr, "tab sizes must be ascending") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}
