package vdircmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runToolAt(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestVdirDefaultsToLongListing(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", "hello")

	out, errb, code := runToolAt(t, dir)
	if code != 0 || errb != "" {
		t.Fatalf("vdir code=%d stderr=%q", code, errb)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("vdir lines = %q, want total plus entry", lines)
	}
	if !regexp.MustCompile(`^total \d+$`).MatchString(lines[0]) {
		t.Fatalf("vdir total line = %q", lines[0])
	}
	if !regexp.MustCompile(`^[-dl][rwxsStT-]{9} +\d+ .* 5 [A-Z][a-z]{2} [ \d]\d [ \d\d:]{5} f$`).MatchString(lines[1]) {
		t.Fatalf("vdir entry line = %q", lines[1])
	}
}

func TestVdirDelegatesLsFlags(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, ".hidden", "x")
	write(t, dir, "shown", "x")

	out, errb, code := runToolAt(t, dir, "-A")
	if code != 0 || errb != "" {
		t.Fatalf("vdir -A code=%d stderr=%q", code, errb)
	}
	if !strings.Contains(out, ".hidden") || !strings.Contains(out, "shown") {
		t.Fatalf("vdir -A output = %q, want hidden and shown entries", out)
	}
}

func TestVdirRegistered(t *testing.T) {
	if tool.Lookup("vdir") != cmd {
		t.Fatalf("vdir is not registered")
	}
}

// --help/--version answer as vdir, not as the ls delegate.
func TestVdirHelpAndVersionIdentity(t *testing.T) {
	out, _, code := runToolAt(t, t.TempDir(), "--help")
	if code != 0 || !strings.Contains(out, "Usage: vdir") || strings.Contains(out, "Usage: ls") {
		t.Fatalf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runToolAt(t, t.TempDir(), "--version")
	if code != 0 || !strings.HasPrefix(out, "vdir (qiangli/coreutils)") {
		t.Fatalf("--version: code=%d out=%q", code, out)
	}
}
