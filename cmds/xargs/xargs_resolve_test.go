package xargscmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// runXargsEnv runs xargs with full control over Env and Dir, so the
// rc.Dir ≠ process-cwd and custom-PATH cases are exercisable.
func runXargsEnv(t *testing.T, dir string, env []string, in string, args ...string) (out, errOut string, code int) {
	t.Helper()
	var o, e bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(in), Out: &o, Err: &e},
	}
	code = cmd.Run(rc, args)
	return o.String(), e.String(), code
}

func writeXargsTool(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" && filepath.Ext(path) == "" {
		path += ".bat"
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho ran-tool\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// PATH-unset must fall back to the default search path and find `echo`.
func TestXargsPathUnsetFindsCommand(t *testing.T) {
	out, _, code := runXargsEnv(t, t.TempDir(), []string{"HOME=/tmp"}, "x\n", "echo", "hi")
	if code != 0 {
		t.Fatalf("PATH-unset xargs echo: code=%d, want 0", code)
	}
	if !strings.Contains(out, "hi") {
		t.Errorf("PATH-unset xargs echo output=%q, want it to contain hi", out)
	}
}

// A relative PATH entry is resolved against rc.Dir, not the process cwd.
func TestXargsRelativePathEntry(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeXargsTool(t, filepath.Join(dir, "bin", "mytool"))
	out, _, code := runXargsEnv(t, dir, []string{"PATH=bin"}, "x\n", "mytool")
	if code != 0 {
		t.Fatalf("relative-PATH xargs: code=%d, want 0", code)
	}
	if !strings.Contains(out, "ran-tool") {
		t.Errorf("relative-PATH xargs output=%q", out)
	}
}

// An empty PATH component means the working directory.
func TestXargsEmptyPathComponent(t *testing.T) {
	dir := t.TempDir()
	writeXargsTool(t, filepath.Join(dir, "mytool"))
	out, _, code := runXargsEnv(t, dir, []string{"PATH=:/nonexistent"}, "x\n", "mytool")
	if code != 0 {
		t.Fatalf("empty-component xargs: code=%d, want 0", code)
	}
	if !strings.Contains(out, "ran-tool") {
		t.Errorf("empty-component xargs output=%q", out)
	}
}

// A command found in PATH but not executable yields POSIX 126, not 127.
func TestXargsFoundNotExecutableIs126(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "noexec"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, code := runXargsEnv(t, dir, []string{"PATH=."}, "x\n", "noexec")
	if code != 126 {
		t.Errorf("non-executable xargs: code=%d, want 126 (found but not executable)", code)
	}
}

// A command absent from PATH yields 127.
func TestXargsNotFoundIs127(t *testing.T) {
	_, _, code := runXargsEnv(t, t.TempDir(), []string{"PATH=/nonexistent"}, "x\n", "no-such-cmd-xyz")
	if code != 127 {
		t.Errorf("not-found xargs: code=%d, want 127", code)
	}
}

func TestXargsNotFoundStopsBeforeLaterBatches(t *testing.T) {
	_, errOut, code := runXargsEnv(t, t.TempDir(), []string{"PATH=/nonexistent"}, "a b c\n", "-n1", "no-such-cmd-xyz")
	if code != 127 {
		t.Fatalf("not-found xargs: code=%d, want 127", code)
	}
	if count := strings.Count(errOut, "command not found"); count != 1 {
		t.Fatalf("not-found diagnostics=%d stderr=%q, want one attempted batch", count, errOut)
	}
}

func TestXargsStartFailureStopsBeforeLaterBatches(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "cannot-run"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errOut, code := runXargsEnv(t, dir, []string{"PATH=."}, "a b c\n", "-n1", "cannot-run")
	if code != 126 {
		t.Fatalf("start-failure xargs: code=%d stderr=%q, want 126", code, errOut)
	}
	if count := strings.Count(errOut, "xargs: cannot-run:"); count != 1 {
		t.Fatalf("start-failure diagnostics=%d stderr=%q, want one attempted batch", count, errOut)
	}
}

// A child exit of 1..125 propagates as xargs status 123 (GNU semantics).
func TestXargsChildExitMapsTo123(t *testing.T) {
	_, _, code := runXargsEnv(t, t.TempDir(), []string{"PATH=/bin:/usr/bin"}, "x\n", "sh", "-c", "exit 7")
	if code != 123 {
		t.Errorf("child exit 7: xargs code=%d, want 123", code)
	}
}

// `--` ends xargs options so the command's own flags are not consumed.
func TestXargsDoubleDashStopsOptionParsing(t *testing.T) {
	out, _, code := runXargsEnv(t, t.TempDir(), []string{"PATH=/bin:/usr/bin"}, "x\n", "--", "echo", "-n", "kept")
	if code != 0 {
		t.Fatalf("xargs -- echo: code=%d, want 0", code)
	}
	if !strings.Contains(out, "kept") {
		t.Errorf("xargs -- output=%q, want echo to receive its own -n", out)
	}
}
