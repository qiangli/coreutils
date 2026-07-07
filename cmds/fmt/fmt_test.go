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

func TestFmtRejectsBadWidth(t *testing.T) {
	_, stderr, code := runFmt(t, "", "-w", "bad")
	if code != 2 || !strings.Contains(stderr, "invalid width") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}
