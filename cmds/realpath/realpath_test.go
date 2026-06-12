package realpathcmd

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

// resolvedTempDir returns a t.TempDir() with its own symlinks
// resolved (on darwin /var -> /private/var), so expected canonical
// outputs can be built by joining.
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

func sep() string { return string(filepath.Separator) }

func TestRealpathDefault(t *testing.T) {
	dir := resolvedTempDir(t)
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	// existing file, relative operand resolved against rc.Dir
	out, _, code := runIn(t, dir, "f")
	if code != 0 || out != filepath.Join(dir, "f")+"\n" {
		t.Errorf("realpath f = (%q, %d)", out, code)
	}
	// missing final component is fine by default
	out, _, code = runIn(t, dir, "missing")
	if code != 0 || out != filepath.Join(dir, "missing")+"\n" {
		t.Errorf("realpath missing = (%q, %d)", out, code)
	}
	// missing non-final component is an error
	_, errb, code := runIn(t, dir, "no"+sep()+"such")
	if code != 1 || !strings.Contains(errb, "realpath: no"+sep()+"such: ") {
		t.Errorf("realpath no/such = (code=%d, err=%q)", code, errb)
	}
	// missing non-final component still errors when ".." follows it
	_, _, code = runIn(t, dir, "sub"+sep()+".."+sep()+"f")
	if code != 1 {
		t.Errorf("realpath sub/../f with missing sub: code=%d, want 1", code)
	}
}

func TestRealpathSymlinks(t *testing.T) {
	dir := resolvedTempDir(t)
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	mkSymlink(t, "f", filepath.Join(dir, "lnk"))

	out, _, code := runIn(t, dir, "lnk")
	if code != 0 || out != filepath.Join(dir, "f")+"\n" {
		t.Errorf("realpath lnk = (%q, %d), want %q", out, code, filepath.Join(dir, "f"))
	}

	// dangling symlink: fine by default, error with -e
	mkSymlink(t, "gone", filepath.Join(dir, "dangling"))
	out, _, code = runIn(t, dir, "dangling")
	if code != 0 || out != filepath.Join(dir, "gone")+"\n" {
		t.Errorf("realpath dangling = (%q, %d)", out, code)
	}
	_, _, code = runIn(t, dir, "-e", "dangling")
	if code != 1 {
		t.Errorf("realpath -e dangling: code=%d, want 1", code)
	}

	// symlink loop
	mkSymlink(t, "loopB", filepath.Join(dir, "loopA"))
	mkSymlink(t, "loopA", filepath.Join(dir, "loopB"))
	_, errb, code := runIn(t, dir, "loopA")
	if code != 1 || !strings.Contains(errb, "too many levels of symbolic links") {
		t.Errorf("realpath loopA = (code=%d, err=%q)", code, errb)
	}
}

func TestRealpathModes(t *testing.T) {
	dir := resolvedTempDir(t)

	// -m: nothing needs to exist
	out, _, code := runIn(t, dir, "-m", "a"+sep()+"b"+sep()+"c")
	if code != 0 || out != filepath.Join(dir, "a", "b", "c")+"\n" {
		t.Errorf("realpath -m a/b/c = (%q, %d)", out, code)
	}
	// -e: everything must exist
	_, _, code = runIn(t, dir, "-e", "missing")
	if code != 1 {
		t.Errorf("realpath -e missing: code=%d, want 1", code)
	}
	out, _, code = runIn(t, dir, "-e", ".")
	if code != 0 || out != dir+"\n" {
		t.Errorf("realpath -e . = (%q, %d), want %q", out, code, dir)
	}
	// last of -e/-m wins (GNU rule)
	out, _, code = runIn(t, dir, "-e", "-m", "missing")
	if code != 0 || out != filepath.Join(dir, "missing")+"\n" {
		t.Errorf("realpath -e -m missing = (%q, %d), want success", out, code)
	}
	_, _, code = runIn(t, dir, "-m", "-e", "missing")
	if code != 1 {
		t.Errorf("realpath -m -e missing: code=%d, want 1", code)
	}
	// multiple operands: failures don't stop the rest
	out, _, code = runIn(t, dir, "-e", "missing", ".")
	if code != 1 || out != dir+"\n" {
		t.Errorf("realpath -e missing . = (%q, %d), want (%q, 1)", out, code, dir+"\n")
	}
}

func TestRealpathStrip(t *testing.T) {
	dir := t.TempDir() // NOT resolved: -s must keep symlinks (incl. /var on darwin)
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	mkSymlink(t, "f", filepath.Join(dir, "lnk"))

	// no symlink expansion
	out, _, code := runIn(t, dir, "-s", "lnk")
	if code != 0 || out != filepath.Join(dir, "lnk")+"\n" {
		t.Errorf("realpath -s lnk = (%q, %d), want %q", out, code, filepath.Join(dir, "lnk"))
	}
	out, _, code = runIn(t, dir, "--no-symlinks", "lnk")
	if code != 0 || out != filepath.Join(dir, "lnk")+"\n" {
		t.Errorf("realpath --no-symlinks lnk = (%q, %d)", out, code)
	}
	// "." and ".." resolve lexically
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, _, code = runIn(t, dir, "-s", "sub"+sep()+".."+sep()+"lnk")
	if code != 0 || out != filepath.Join(dir, "lnk")+"\n" {
		t.Errorf("realpath -s sub/../lnk = (%q, %d)", out, code)
	}
	// default existence rule still applies under -s
	_, _, code = runIn(t, dir, "-s", "no"+sep()+"such")
	if code != 1 {
		t.Errorf("realpath -s no/such: code=%d, want 1", code)
	}
	out, _, code = runIn(t, dir, "-s", "-m", "no"+sep()+"such")
	if code != 0 || out != filepath.Join(dir, "no", "such")+"\n" {
		t.Errorf("realpath -s -m no/such = (%q, %d)", out, code)
	}
	_, _, code = runIn(t, dir, "-s", "-e", "missing")
	if code != 1 {
		t.Errorf("realpath -s -e missing: code=%d, want 1", code)
	}
}

func TestRealpathRelativeTo(t *testing.T) {
	dir := resolvedTempDir(t)
	if err := os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	out, _, code := runIn(t, dir, "--relative-to=.", "f")
	if code != 0 || out != "f\n" {
		t.Errorf("realpath --relative-to=. f = (%q, %d), want \"f\"", out, code)
	}
	out, _, code = runIn(t, dir, "--relative-to=sub"+sep()+"deep", "f")
	want := filepath.Join("..", "..", "f") + "\n"
	if code != 0 || out != want {
		t.Errorf("realpath --relative-to=sub/deep f = (%q, %d), want %q", out, code, want)
	}
	out, _, code = runIn(t, dir, "--relative-to=.", ".")
	if code != 0 || out != ".\n" {
		t.Errorf("realpath --relative-to=. . = (%q, %d), want \".\"", out, code)
	}
	// DIR is canonicalized with the same mode; missing DIR is fatal by default
	_, errb, code := runIn(t, dir, "--relative-to=no"+sep()+"such", "f")
	if code != 1 || !strings.Contains(errb, "realpath: no"+sep()+"such") {
		t.Errorf("missing --relative-to dir: code=%d err=%q", code, errb)
	}
}

func TestRealpathErrors(t *testing.T) {
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
	// empty operand never exists
	_, errb, code = runIn(t, dir, "")
	if code != 1 || !strings.Contains(errb, "no such file or directory") {
		t.Errorf("empty operand: code=%d err=%q", code, errb)
	}
}

func TestRealpathHelpAndVersion(t *testing.T) {
	out, _, code := runIn(t, t.TempDir(), "--help")
	if code != 0 || !strings.Contains(out, "Usage: realpath") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runIn(t, t.TempDir(), "--version")
	if code != 0 || !strings.Contains(out, "realpath") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}
