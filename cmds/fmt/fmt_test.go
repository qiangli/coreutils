package fmtcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runFmt(t *testing.T, input string, args ...string) (string, string, int) {
	t.Helper()
	var out, err bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Stdio: tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &err}}
	code := run(rc, args)
	return out.String(), err.String(), code
}

func TestFmtWrapsParagraphs(t *testing.T) {
	out, stderr, code := runFmt(t, "alpha beta\ngamma delta\n\nz\n", "-w", "12")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "alpha beta\ngamma delta\n\nz\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFmtSplitOnlyAndFile(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "in.txt")
	if err := os.WriteFile(name, []byte("alpha beta gamma\nshort\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &stderr}}
	code := run(rc, []string{"-s", "-w", "10", "in.txt"})
	if code != 0 || stderr.String() != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	if want := "alpha beta\ngamma\nshort\n"; out.String() != want {
		t.Fatalf("out=%q want %q", out.String(), want)
	}
}

func TestFmtSplitOnlyPreservesIndent(t *testing.T) {
	// -s keeps each line's own indent on its continuation lines.
	out, stderr, code := runFmt(t, "    alpha beta gamma\n", "-s", "-w", "14")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "    alpha beta\n    gamma\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFmtPrefix(t *testing.T) {
	out, stderr, code := runFmt(t, ">hello world again\nkeep\n", "-p", ">", "-w", "10")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := ">hello\n>world\n>again\nkeep\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFmtUniformSpacing(t *testing.T) {
	out, stderr, code := runFmt(t, "one.  two   three\n", "-u", "-w", "20")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "one.  two three\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFmtGoalBreaksEarlier(t *testing.T) {
	// Without -g the goal is 93% of the width, so all four words fit
	// on one 11-column line. With -g 6 the lines break near column 6.
	out, stderr, code := runFmt(t, "aa bb cc dd\n", "-w", "12")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "aa bb cc dd\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
	out, stderr, code = runFmt(t, "aa bb cc dd\n", "-g", "6", "-w", "12")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "aa bb\ncc dd\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFmtGoalWithoutWidth(t *testing.T) {
	// -g without -w is valid: the width becomes goal+10.
	out, stderr, code := runFmt(t, "aa bb cc dd\n", "-g", "6")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "aa bb\ncc dd\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFmtRejectsGoalOverWidth(t *testing.T) {
	_, stderr, code := runFmt(t, "", "-g", "20", "-w", "10")
	if code != 2 || !strings.Contains(stderr, "invalid goal") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	// GNU caps the goal at the default width (75) when -w is absent.
	_, stderr, code = runFmt(t, "", "-g", "80")
	if code != 2 || !strings.Contains(stderr, "invalid goal") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestFmtObsoleteWidthSyntax(t *testing.T) {
	out, stderr, code := runFmt(t, "aa bb cc dd\n", "-6")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "aa bb\ncc dd\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFmtRejectsBadWidth(t *testing.T) {
	_, stderr, code := runFmt(t, "", "-w", "bad")
	if code != 2 || !strings.Contains(stderr, "invalid width") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestFmtPreservesIndentation(t *testing.T) {
	// A paragraph's indent is preserved on every output line.
	out, stderr, code := runFmt(t, "    alpha beta gamma delta\n", "-w", "16")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "    alpha beta\n    gamma delta\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFmtDifferentIndentationNotJoined(t *testing.T) {
	// Successive lines with different indentation start new
	// paragraphs; equal indentation joins.
	out, stderr, code := runFmt(t, "alpha beta\n  gamma delta\n  epsilon\n", "-w", "30")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "alpha beta\n  gamma delta epsilon\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFmtCrownMargin(t *testing.T) {
	out, stderr, code := runFmt(t, "  alpha beta gamma\n    delta epsilon\n", "-c", "-w", "14")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "  alpha beta\n    gamma\n    delta\n    epsilon\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFmtTaggedParagraph(t *testing.T) {
	out, stderr, code := runFmt(t, "Tag alpha beta gamma\n    delta epsilon\n", "-t", "-w", "16")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "Tag alpha beta\n    gamma delta\n    epsilon\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFmtTaggedFirstLineOwnParagraphWhenIndentEqual(t *testing.T) {
	// In tagged mode, a second line with the SAME indent as the first
	// does not join it: the first line is a one-line paragraph.
	out, stderr, code := runFmt(t, "alpha beta\ngamma delta\n", "-t", "-w", "30")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "alpha beta\ngamma delta\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFmtRemovedFlagsFailLoudly(t *testing.T) {
	for _, flag := range []string{"-T", "-m", "-P", "-x", "-X", "-q"} {
		_, stderr, code := runFmt(t, "x\n", flag, "8")
		if code != 2 || !strings.Contains(stderr, strings.TrimPrefix(flag, "-")) {
			t.Fatalf("flag %s: code=%d stderr=%q", flag, code, stderr)
		}
	}
}
