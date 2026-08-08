package timecmd

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

func runTimeEnv(t *testing.T, dir string, env []string, args ...string) (out, errOut string, code int) {
	t.Helper()
	var o, e bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &o, Err: &e},
	}
	code = cmd.Run(rc, args)
	return o.String(), e.String(), code
}

func writeTimeTool(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" && filepath.Ext(path) == "" {
		path += ".bat"
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho ran-tool\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// PATH-unset must fall back to the default search path and find `echo`.
func TestTimePathUnsetFindsCommand(t *testing.T) {
	_, _, code := runTimeEnv(t, t.TempDir(), []string{"HOME=/tmp"}, "echo", "hi")
	if code != 0 {
		t.Fatalf("PATH-unset time echo: code=%d, want 0", code)
	}
}

// A relative PATH entry is resolved against rc.Dir.
func TestTimeRelativePathEntry(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTimeTool(t, filepath.Join(dir, "bin", "mytool"))
	out, _, code := runTimeEnv(t, dir, []string{"PATH=bin"}, "mytool")
	if code != 0 {
		t.Fatalf("relative-PATH time: code=%d, want 0", code)
	}
	if !strings.Contains(out, "ran-tool") {
		t.Errorf("relative-PATH time output=%q", out)
	}
}

// An empty PATH component means the working directory.
func TestTimeEmptyPathComponent(t *testing.T) {
	dir := t.TempDir()
	writeTimeTool(t, filepath.Join(dir, "mytool"))
	out, _, code := runTimeEnv(t, dir, []string{"PATH=:/nonexistent"}, "mytool")
	if code != 0 {
		t.Fatalf("empty-component time: code=%d, want 0", code)
	}
	if !strings.Contains(out, "ran-tool") {
		t.Errorf("empty-component time output=%q", out)
	}
}

// Found but not executable yields POSIX 126.
func TestTimeFoundNotExecutableIs126(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "noexec"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, code := runTimeEnv(t, dir, []string{"PATH=."}, "noexec")
	if code != 126 {
		t.Errorf("non-executable time: code=%d, want 126", code)
	}
}

// Absent from PATH yields 127.
func TestTimeNotFoundIs127(t *testing.T) {
	_, _, code := runTimeEnv(t, t.TempDir(), []string{"PATH=/nonexistent"}, "no-such-cmd-xyz")
	if code != 127 {
		t.Errorf("not-found time: code=%d, want 127", code)
	}
}

// time propagates the command's own exit status (not remapped).
func TestTimeChildExitPropagates(t *testing.T) {
	_, _, code := runTimeEnv(t, t.TempDir(), []string{"PATH=/bin:/usr/bin"}, "sh", "-c", "exit 7")
	if code != 7 {
		t.Errorf("child exit 7: time code=%d, want 7", code)
	}
}

// `--` ends time's options so the command's flags reach the command.
func TestTimeDoubleDashStopsOptionParsing(t *testing.T) {
	out, _, code := runTimeEnv(t, t.TempDir(), []string{"PATH=/bin:/usr/bin"}, "--", "echo", "kept")
	if code != 0 {
		t.Fatalf("time -- echo: code=%d, want 0", code)
	}
	if !strings.Contains(out, "kept") {
		t.Errorf("time -- output=%q", out)
	}
}
