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

func TestUnexpandFirstOnlyOverridesAll(t *testing.T) {
	out, stderr, code := runUnexpand(t, "x   y\n", "-a", "-f", "-t", "4")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "x   y\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestUnexpandNoUTF8CountsBytes(t *testing.T) {
	out, stderr, code := runUnexpand(t, "é  z\n", "-U", "-a", "-t", "4")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "é\tz\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestUnexpandRejectsBadTabs(t *testing.T) {
	_, stderr, code := runUnexpand(t, "", "--tabs=4,2")
	if code != 2 || !strings.Contains(stderr, "invalid tab size") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}
