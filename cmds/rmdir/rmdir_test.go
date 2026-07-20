package rmdircmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// runTool is the canonical test harness shape for cmds packages:
// output is captured after Run returns.
func runTool(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
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

func TestRmdirEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, "d")
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("rmdir d: code=%d out=%q err=%q", code, out, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "d")); !os.IsNotExist(err) {
		t.Error("directory still exists")
	}
}

func TestRmdirNonEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "d", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "d")
	if code != 1 || !strings.Contains(errb, "failed to remove 'd'") ||
		!strings.Contains(strings.ToLower(errb), "not empty") {
		t.Errorf("code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "d", "sub")); err != nil {
		t.Error("non-empty directory contents were removed")
	}
}

func TestRmdirIgnoreFailOnNonEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "d", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "--ignore-fail-on-non-empty", "d")
	if code != 0 || errb != "" {
		t.Fatalf("ignore non-empty: code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "d", "sub")); err != nil {
		t.Error("non-empty directory contents were removed")
	}
}

func TestRmdirNotADirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "f")
	if code != 1 || !strings.Contains(errb, "failed to remove 'f': Not a directory") {
		t.Errorf("code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "f")); err != nil {
		t.Error("file was removed")
	}
}

func TestRmdirMissing(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "nope")
	if code != 1 || !strings.Contains(errb, "failed to remove 'nope'") {
		t.Errorf("code=%d err=%q", code, errb)
	}
}

func TestRmdirParents(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a", "b", "c"), 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join("a", "b", "c")
	out, errb, code := runTool(t, dir, "-pv", nested)
	if code != 0 {
		t.Fatalf("rmdir -pv: code=%d err=%q", code, errb)
	}
	want := "rmdir: removing directory, '" + nested + "'\n" +
		"rmdir: removing directory, '" + filepath.Join("a", "b") + "'\n" +
		"rmdir: removing directory, 'a'\n"
	if out != want {
		t.Errorf("out=%q want %q", out, want)
	}
	if _, err := os.Lstat(filepath.Join(dir, "a")); !os.IsNotExist(err) {
		t.Error("ancestors not removed")
	}
}

func TestRmdirParentsExplicitCurrentDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-p", "./a/b")
	if code != 1 || !strings.Contains(errb, "failed to remove '.'") {
		t.Errorf("code=%d err=%q", code, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "a")); !os.IsNotExist(err) {
		t.Error("explicit current-directory path did not remove its empty ancestors")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("working directory was removed: %v", err)
	}
}

func TestRmdirParentsStopsOnNonEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a", "keep"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-p", filepath.Join("a", "b"))
	if code != 1 || !strings.Contains(errb, "failed to remove 'a'") {
		t.Errorf("code=%d err=%q", code, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "a", "b")); !os.IsNotExist(err) {
		t.Error("operand itself not removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "a", "keep")); err != nil {
		t.Error("sibling file lost")
	}
}

func TestRmdirVerbose(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, _, code := runTool(t, dir, "-v", "d")
	if code != 0 || out != "rmdir: removing directory, 'd'\n" {
		t.Errorf("rmdir -v: code=%d out=%q", code, out)
	}
}

func TestRmdirContinuesPastErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "nope", "d")
	if code != 1 || !strings.Contains(errb, "failed to remove 'nope'") {
		t.Errorf("code=%d err=%q", code, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "d")); !os.IsNotExist(err) {
		t.Error("later operand not removed after earlier failure")
	}
}

func TestRmdirUsageErrors(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir)
	if code != 2 || !strings.Contains(errb, "missing operand") {
		t.Errorf("no args: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "--frobnicate", "d")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestRmdirHelpAndVersion(t *testing.T) {
	dir := t.TempDir()
	out, _, code := runTool(t, dir, "--help")
	if code != 0 || !strings.Contains(out, "Usage: rmdir") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, dir, "--version")
	if code != 0 || !strings.Contains(out, "rmdir") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}

// TestRmdirDotDotBare covers the POSIX requirement that rmdir reject a
// final component of "." or ".." with EINVAL, for the bare forms.
func TestRmdirDotDotBare(t *testing.T) {
	dir := t.TempDir()
	for _, op := range []string{".", ".."} {
		_, errb, code := runTool(t, dir, op)
		if code != 1 || !strings.Contains(errb, "failed to remove '"+op+"'") ||
			!strings.Contains(errb, "Invalid argument") {
			t.Errorf("rmdir %s: code=%d err=%q", op, code, errb)
		}
	}
}

// TestRmdirTrailingDotComponent covers the POSIX EINVAL guarantee for a
// path whose final component is "." — including non-bare forms like
// "a/." and "a/./" where filepath.Clean would otherwise hide the dot.
func TestRmdirTrailingDotComponent(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, op := range []string{"a/.", "a/./"} {
		_, errb, code := runTool(t, dir, op)
		if code != 1 || !strings.Contains(errb, "failed to remove '"+op+"'") ||
			!strings.Contains(errb, "Invalid argument") {
			t.Errorf("rmdir %s: code=%d err=%q", op, code, errb)
		}
	}
	// Directory must not have been removed.
	if _, err := os.Stat(filepath.Join(dir, "a")); err != nil {
		t.Errorf("rmdir a/. removed the directory: %v", err)
	}
}

// TestRmdirTrailingDotDotComponent covers the POSIX EINVAL guarantee for
// a path whose final component is ".." — including non-bare forms like
// "a/.." and "a/b/.." where filepath.Clean would otherwise resolve away
// the dotdot.
func TestRmdirTrailingDotDotComponent(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, op := range []string{"a/..", "a/b/.."} {
		_, errb, code := runTool(t, dir, op)
		if code != 1 || !strings.Contains(errb, "failed to remove '"+op+"'") ||
			!strings.Contains(errb, "Invalid argument") {
			t.Errorf("rmdir %s: code=%d err=%q", op, code, errb)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "a")); err != nil {
		t.Errorf("rmdir a/.. removed the directory: %v", err)
	}
}

// TestRmdirTrailingSlash covers rmdir of a path with a trailing slash —
// a valid empty directory must be removable, and the diagnostic (if any)
// reports the operand as given.
func TestRmdirTrailingSlash(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, _, code := runTool(t, dir, "-v", "d/")
	if code != 0 || !strings.Contains(out, "removing directory, 'd/'") {
		t.Errorf("rmdir -v d/: code=%d out=%q", code, out)
	}
	if _, err := os.Lstat(filepath.Join(dir, "d")); !os.IsNotExist(err) {
		t.Error("trailing-slash directory not removed")
	}
}

// TestRmdirIgnoreNonEmptyWithParents verifies that --ignore-fail-on-non-empty
// suppresses the error for a non-empty ancestor during a -p walk without
// affecting directories that cannot be removed for other reasons.
func TestRmdirIgnoreNonEmptyWithParents(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a", "b", "c"), 0o755); err != nil {
		t.Fatal(err)
	}
	// a is non-empty even after removing b/c chain is attempted: put a
	// sibling file in a so that the -p walk hits a non-empty ancestor.
	if err := os.WriteFile(filepath.Join(dir, "a", "keep"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-p", "--ignore-fail-on-non-empty", filepath.Join("a", "b", "c"))
	// c and b are removed; a is non-empty → ignored (no error, exit 0).
	if code != 0 || errb != "" {
		t.Errorf("ignore-non-empty -p: code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "a", "b")); !os.IsNotExist(err) {
		t.Error("b not removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "a", "keep")); err != nil {
		t.Error("sibling file lost")
	}
}

// TestRmdirIgnoreNonEmptyDoesNotIgnoreOtherErrors verifies that
// --ignore-fail-on-non-empty only suppresses ENOTEMPTY/EEXIST, not
// missing-directory or not-a-directory failures.
func TestRmdirIgnoreNonEmptyDoesNotIgnoreOtherErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Missing directory → still an error even with the flag.
	_, errb, code := runTool(t, dir, "--ignore-fail-on-non-empty", "nope")
	if code != 1 || !strings.Contains(errb, "failed to remove 'nope'") {
		t.Errorf("missing dir not reported: code=%d err=%q", code, errb)
	}
	// Regular file → still an error even with the flag.
	_, errb, code = runTool(t, dir, "--ignore-fail-on-non-empty", "f")
	if code != 1 || !strings.Contains(errb, "Not a directory") {
		t.Errorf("not-a-directory not reported: code=%d err=%q", code, errb)
	}
}

// TestRmdirDoubleDashOperand verifies that -- terminates option parsing
// so operands beginning with - are treated as directory names.
func TestRmdirDoubleDashOperand(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"-p", "-v"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
		_, _, code := runTool(t, dir, "--", name)
		if code != 0 {
			t.Errorf("rmdir -- %s: code=%d", name, code)
		}
		if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("rmdir -- %s did not remove the directory", name)
		}
	}
}

// TestRmdirDashOperand verifies that a lone "-" is treated as a filename,
// not as an option introducer (matching POSIX utility syntax).
func TestRmdirDashOperand(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "-"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, _, code := runTool(t, dir, "-")
	if code != 0 {
		t.Errorf("rmdir -: code=%d", code)
	}
	if _, err := os.Lstat(filepath.Join(dir, "-")); !os.IsNotExist(err) {
		t.Error("rmdir - did not remove the directory")
	}
}
