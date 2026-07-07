package mvcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestMvRenameFile(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "hello")
	out, errb, code := runTool(t, dir, "a", "b")
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("mv a b: code=%d out=%q err=%q", code, out, errb)
	}
	if read(t, filepath.Join(dir, "b")) != "hello" {
		t.Error("content not moved")
	}
	if _, err := os.Lstat(filepath.Join(dir, "a")); !os.IsNotExist(err) {
		t.Error("source still exists")
	}
}

func TestMvIntoDir(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "x")
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "a", "d")
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if read(t, filepath.Join(dir, "d", "a")) != "x" {
		t.Error("not moved into directory")
	}
}

func TestMvTargetDirectoryAndNoTargetDirectory(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "x")
	write(t, filepath.Join(dir, "b"), "y")
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-t", "d", "a", "b")
	if code != 0 {
		t.Fatalf("mv -t: code=%d err=%q", code, errb)
	}
	if read(t, filepath.Join(dir, "d", "a")) != "x" || read(t, filepath.Join(dir, "d", "b")) != "y" {
		t.Fatal("-t did not move both sources into directory")
	}
	write(t, filepath.Join(dir, "c"), "z")
	_, errb, code = runTool(t, dir, "-T", "c", "d")
	if code != 1 || !strings.Contains(errb, "cannot move 'c' to 'd'") {
		t.Errorf("mv -T file dir: code=%d err=%q", code, errb)
	}
}

func TestMvDirRename(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src", "f"), "x")
	_, errb, code := runTool(t, dir, "src", "dst")
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if read(t, filepath.Join(dir, "dst", "f")) != "x" {
		t.Error("directory not renamed")
	}
}

func TestMvMultipleToNonDir(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "1")
	write(t, filepath.Join(dir, "b"), "2")
	_, errb, code := runTool(t, dir, "a", "b", "c")
	if code != 1 || !strings.Contains(errb, "target 'c' is not a directory") {
		t.Errorf("code=%d err=%q", code, errb)
	}
}

func TestMvNoClobber(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "new")
	write(t, filepath.Join(dir, "b"), "old")
	out, errb, code := runTool(t, dir, "-n", "a", "b")
	if code != 0 || out != "" || errb != "" {
		t.Errorf("mv -n: code=%d out=%q err=%q", code, out, errb)
	}
	if read(t, filepath.Join(dir, "b")) != "old" {
		t.Error("destination overwritten despite -n")
	}
	if _, err := os.Lstat(filepath.Join(dir, "a")); err != nil {
		t.Error("source removed despite -n skip")
	}
	// -n then -f: final one takes effect (GNU rule) -> overwrite.
	_, _, code = runTool(t, dir, "-n", "-f", "a", "b")
	if code != 0 || read(t, filepath.Join(dir, "b")) != "new" {
		t.Error("-n -f should overwrite (last wins)")
	}
}

func TestMvBackupSuffixUpdateAndInteractive(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "new")
	write(t, filepath.Join(dir, "b"), "old")
	_, errb, code := runTool(t, dir, "--backup=simple", "-S", ".bak", "a", "b")
	if code != 0 {
		t.Fatalf("mv backup: code=%d err=%q", code, errb)
	}
	if read(t, filepath.Join(dir, "b")) != "new" || read(t, filepath.Join(dir, "b.bak")) != "old" {
		t.Fatalf("backup/suffix did not preserve old destination")
	}
	if _, err := os.Lstat(filepath.Join(dir, "a")); !os.IsNotExist(err) {
		t.Fatal("source should be removed after move")
	}

	src := filepath.Join(dir, "a")
	dst := filepath.Join(dir, "b")
	write(t, src, "older")
	write(t, dst, "newer")
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local)
	newer := time.Date(2021, 1, 1, 0, 0, 0, 0, time.Local)
	if err := os.Chtimes(src, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dst, newer, newer); err != nil {
		t.Fatal(err)
	}
	_, errb, code = runTool(t, dir, "-u", "a", "b")
	if code != 0 || errb != "" || read(t, dst) != "newer" {
		t.Fatalf("mv -u should skip newer destination: code=%d err=%q", code, errb)
	}
	if _, err := os.Lstat(src); err != nil {
		t.Fatal("source should remain after -u skip")
	}

	write(t, src, "prompted")
	write(t, dst, "keep")
	_, errb, code = runTool(t, dir, "-i", "a", "b")
	if code != 0 || !strings.Contains(errb, "overwrite 'b'?") || read(t, dst) != "keep" {
		t.Fatalf("mv -i without yes should skip: code=%d err=%q", code, errb)
	}
}

func TestMvVerbose(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "x")
	out, _, code := runTool(t, dir, "-v", "a", "b")
	if code != 0 || out != "renamed 'a' -> 'b'\n" {
		t.Errorf("mv -v: code=%d out=%q", code, out)
	}
}

func TestMvMissingSource(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "nope", "b")
	if code != 1 || !strings.Contains(errb, "cannot stat 'nope'") {
		t.Errorf("code=%d err=%q", code, errb)
	}
}

func TestMvSameFile(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "x")
	_, errb, code := runTool(t, dir, "a", "a")
	if code != 1 || !strings.Contains(errb, "'a' and 'a' are the same file") {
		t.Errorf("code=%d err=%q", code, errb)
	}
}

// TestMvCopyFallback exercises the cross-device copy+remove path
// directly (a real EXDEV needs two filesystems, which is not
// hermetic). Same logic, same code path.
func TestMvCopyFallback(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src", "f1"), "one")
	write(t, filepath.Join(dir, "src", "sub", "f2"), "two")
	if runtime.GOOS != "windows" {
		if err := os.Symlink("f1", filepath.Join(dir, "src", "link")); err != nil {
			t.Fatal(err)
		}
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	m := &mover{rc: rc}
	if !m.copyMove("src", "dst") || m.failed {
		t.Fatalf("copyMove failed: %s", errb.String())
	}
	if read(t, filepath.Join(dir, "dst", "f1")) != "one" || read(t, filepath.Join(dir, "dst", "sub", "f2")) != "two" {
		t.Error("tree not copied")
	}
	if runtime.GOOS != "windows" {
		if target, err := os.Readlink(filepath.Join(dir, "dst", "link")); err != nil || target != "f1" {
			t.Errorf("symlink not preserved: %q %v", target, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(dir, "src")); !os.IsNotExist(err) {
		t.Error("source not removed after fallback copy")
	}
}

func TestMvUsageErrors(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir)
	if code != 2 || !strings.Contains(errb, "missing file operand") {
		t.Errorf("no args: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "only")
	if code != 2 || !strings.Contains(errb, "missing destination file operand after 'only'") {
		t.Errorf("one arg: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "--frobnicate", "a", "b")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestMvHelpAndVersion(t *testing.T) {
	dir := t.TempDir()
	out, _, code := runTool(t, dir, "--help")
	if code != 0 || !strings.Contains(out, "Usage: mv") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, dir, "--version")
	if code != 0 || !strings.Contains(out, "mv") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}
