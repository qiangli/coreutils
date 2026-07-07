package dircmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
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

func write(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDirDefaultsToCompactListing(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "b")
	write(t, dir, "a")

	out, errb, code := runToolAt(t, dir)
	if code != 0 || errb != "" || out != "a\nb\n" {
		t.Fatalf("dir = (%q, %q, %d), want compact sorted listing", out, errb, code)
	}
}

func TestDirDelegatesLsFlags(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a")
	write(t, dir, ".hidden")

	out, errb, code := runToolAt(t, dir, "-A")
	if code != 0 || errb != "" || out != ".hidden\na\n" {
		t.Fatalf("dir -A = (%q, %q, %d), want ls -A behavior", out, errb, code)
	}
}

func TestDirRegistered(t *testing.T) {
	if tool.Lookup("dir") != cmd {
		t.Fatalf("dir is not registered")
	}
}

// --help/--version answer as dir, not as the ls delegate.
func TestDirHelpAndVersionIdentity(t *testing.T) {
	out, _, code := runToolAt(t, t.TempDir(), "--help")
	if code != 0 || !strings.Contains(out, "Usage: dir") || strings.Contains(out, "Usage: ls") {
		t.Fatalf("--help: code=%d out=%q", code, out)
	}

	// Verify representative options are enumerated
	for _, opt := range []string{"-1", "-A", "--all", "--format", "--sort", "--zero"} {
		if !strings.Contains(out, opt) {
			t.Errorf("--help output missing representative option %q", opt)
		}
	}

	out, _, code = runToolAt(t, t.TempDir(), "--version")
	if code != 0 || !strings.HasPrefix(out, "dir (qiangli/coreutils)") {
		t.Fatalf("--version: code=%d out=%q", code, out)
	}

	out, _, code = runToolAt(t, t.TempDir(), "-V")
	if code != 0 || !strings.HasPrefix(out, "dir (qiangli/coreutils)") {
		t.Fatalf("-V: code=%d out=%q", code, out)
	}
}
