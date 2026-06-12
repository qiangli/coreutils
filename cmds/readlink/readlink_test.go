package readlinkcmd

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

func runIn(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
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

func resolvedTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func mkSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink creation not permitted: %v", err)
		}
		t.Fatal(err)
	}
}

func TestReadlinkPlain(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	mkSymlink(t, "f", filepath.Join(dir, "rel"))
	mkSymlink(t, filepath.Join(dir, "f"), filepath.Join(dir, "abs"))

	// relative target printed verbatim, not resolved
	out, errb, code := runIn(t, dir, "rel")
	if code != 0 || out != "f\n" || errb != "" {
		t.Errorf("readlink rel = (%q, %q, %d), want (\"f\\n\", \"\", 0)", out, errb, code)
	}
	out, _, code = runIn(t, dir, "abs")
	if code != 0 || out != filepath.Join(dir, "f")+"\n" {
		t.Errorf("readlink abs = (%q, %d)", out, code)
	}
	// multiple operands, one per line; failure is silent + exit 1
	out, errb, code = runIn(t, dir, "rel", "f", "abs")
	if code != 1 || out != "f\n"+filepath.Join(dir, "f")+"\n" || errb != "" {
		t.Errorf("readlink rel f abs = (%q, %q, %d)", out, errb, code)
	}
}

func TestReadlinkPlainNonSymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	// not a symlink: exit 1, no output, no error message (GNU default quiet)
	out, errb, code := runIn(t, dir, "f")
	if code != 1 || out != "" || errb != "" {
		t.Errorf("readlink f = (%q, %q, %d), want silent failure", out, errb, code)
	}
	out, errb, code = runIn(t, dir, "missing")
	if code != 1 || out != "" || errb != "" {
		t.Errorf("readlink missing = (%q, %q, %d), want silent failure", out, errb, code)
	}
}

func TestReadlinkNoNewline(t *testing.T) {
	dir := t.TempDir()
	mkSymlink(t, "target", filepath.Join(dir, "lnk"))
	mkSymlink(t, "target", filepath.Join(dir, "lnk2"))

	out, _, code := runIn(t, dir, "-n", "lnk")
	if code != 0 || out != "target" {
		t.Errorf("readlink -n lnk = (%q, %d), want (\"target\", 0)", out, code)
	}
	// -n with multiple operands: warned and ignored
	out, errb, code := runIn(t, dir, "-n", "lnk", "lnk2")
	if code != 0 || out != "target\ntarget\n" ||
		!strings.Contains(errb, "ignoring --no-newline with multiple arguments") {
		t.Errorf("readlink -n lnk lnk2 = (%q, %q, %d)", out, errb, code)
	}
}

func TestReadlinkCanonicalize(t *testing.T) {
	dir := resolvedTempDir(t)
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	mkSymlink(t, "f", filepath.Join(dir, "lnk"))
	mkSymlink(t, "gone", filepath.Join(dir, "dangling"))

	// -f resolves recursively; works on non-symlinks too
	out, _, code := runIn(t, dir, "-f", "lnk")
	if code != 0 || out != filepath.Join(dir, "f")+"\n" {
		t.Errorf("readlink -f lnk = (%q, %d)", out, code)
	}
	out, _, code = runIn(t, dir, "-f", "f")
	if code != 0 || out != filepath.Join(dir, "f")+"\n" {
		t.Errorf("readlink -f f = (%q, %d)", out, code)
	}
	// -f: missing final component ok, dangling resolves to target
	out, _, code = runIn(t, dir, "-f", "dangling")
	if code != 0 || out != filepath.Join(dir, "gone")+"\n" {
		t.Errorf("readlink -f dangling = (%q, %d)", out, code)
	}
	// -f: missing non-final component fails (silently)
	out, errb, code := runIn(t, dir, "-f", "no"+string(filepath.Separator)+"such")
	if code != 1 || out != "" || errb != "" {
		t.Errorf("readlink -f no/such = (%q, %q, %d), want silent failure", out, errb, code)
	}
	// -e: everything must exist
	_, _, code = runIn(t, dir, "-e", "dangling")
	if code != 1 {
		t.Errorf("readlink -e dangling: code=%d, want 1", code)
	}
	out, _, code = runIn(t, dir, "-e", "lnk")
	if code != 0 || out != filepath.Join(dir, "f")+"\n" {
		t.Errorf("readlink -e lnk = (%q, %d)", out, code)
	}
	// -m: nothing needs to exist
	out, _, code = runIn(t, dir, "-m", "a"+string(filepath.Separator)+"b")
	if code != 0 || out != filepath.Join(dir, "a", "b")+"\n" {
		t.Errorf("readlink -m a/b = (%q, %d)", out, code)
	}
	// last mode flag wins
	out, _, code = runIn(t, dir, "-e", "-m", "a"+string(filepath.Separator)+"b")
	if code != 0 || out != filepath.Join(dir, "a", "b")+"\n" {
		t.Errorf("readlink -e -m a/b = (%q, %d), want -m to win", out, code)
	}
	_, _, code = runIn(t, dir, "-m", "-e", "dangling")
	if code != 1 {
		t.Errorf("readlink -m -e dangling: code=%d, want -e to win", code)
	}
}

func TestReadlinkErrors(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runIn(t, dir)
	if code != 2 || !strings.Contains(errb, "missing operand") {
		t.Errorf("no args: code=%d err=%q", code, errb)
	}
	_, errb, code = runIn(t, dir, "--frobnicate", "x")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
	// GNU flags deliberately not implemented fail loudly
	_, errb, code = runIn(t, dir, "-z", "x")
	if code != 2 || !strings.Contains(errb, "z") {
		t.Errorf("-z: code=%d err=%q", code, errb)
	}
}

func TestReadlinkHelpAndVersion(t *testing.T) {
	out, _, code := runIn(t, t.TempDir(), "--help")
	if code != 0 || !strings.Contains(out, "Usage: readlink") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runIn(t, t.TempDir(), "--version")
	if code != 0 || !strings.Contains(out, "readlink") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}
