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

func TestFmtGoalAndTabWidthValidate(t *testing.T) {
	out, stderr, code := runFmt(t, "a\tb c\n", "-g", "6", "-T", "4", "-w", "8")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "a b c\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFmtRejectsBadWidth(t *testing.T) {
	_, stderr, code := runFmt(t, "", "-w", "bad")
	if code != 2 || !strings.Contains(stderr, "invalid width") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
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

func TestFmtPreserveHeaders(t *testing.T) {
	out, stderr, code := runFmt(t, "Subject: alpha beta gamma\nbody text here\n", "-m", "-w", "10")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "Subject: alpha beta gamma\nbody text\nhere\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFmtSkipPrefix(t *testing.T) {
	out, stderr, code := runFmt(t, "# keep this line\nalpha beta gamma\n", "-P", "#", "-w", "10")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "# keep this line\nalpha beta\ngamma\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFmtExactPrefix(t *testing.T) {
	out, stderr, code := runFmt(t, " >alpha beta\n>gamma delta\n", "-p", ">", "-x", "-w", "8")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := " >alpha beta\n>gamma\n>delta\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFmtQuickAccepted(t *testing.T) {
	out, stderr, code := runFmt(t, "alpha beta gamma\n", "-q", "-w", "10")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "alpha beta\ngamma\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}
