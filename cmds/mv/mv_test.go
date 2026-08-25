package mvcmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

// runTool is the canonical test harness shape for cmds packages:
// output is captured after Run returns.
func runTool(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
	return runToolInput(t, dir, "", args...)
}

func runToolInput(t *testing.T, dir, input string, args ...string) (stdout, stderr string, code int) {
	return runToolInputDeps(t, dir, input, defaultMoverDeps(), args...)
}

func runToolInputDeps(t *testing.T, dir, input string, deps moverDeps, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &errb},
	}
	code = runWithDeps(rc, args, deps)
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

func TestMvInteractiveRefusalContinuesAndFails(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "new-a")
	write(t, filepath.Join(dir, "b"), "new-b")
	write(t, filepath.Join(dir, "d", "a"), "old-a")
	write(t, filepath.Join(dir, "d", "b"), "old-b")

	_, errb, code := runToolInput(t, dir, "n\ny\n", "-i", "a", "b", "d")
	if code != 0 {
		t.Fatalf("mv -i with one refusal: code=%d err=%q", code, errb)
	}
	if strings.Count(errb, "overwrite '") != 2 {
		t.Fatalf("mv -i should prompt for both destinations: %q", errb)
	}
	if read(t, filepath.Join(dir, "a")) != "new-a" || read(t, filepath.Join(dir, "d", "a")) != "old-a" {
		t.Fatal("refused source should remain and destination should be unchanged")
	}
	if _, err := os.Lstat(filepath.Join(dir, "b")); !os.IsNotExist(err) {
		t.Fatal("accepted source should be removed")
	}
	if read(t, filepath.Join(dir, "d", "b")) != "new-b" {
		t.Fatal("accepted destination should be replaced")
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

func TestMvDebugStripTrailingSlashesContextAndShortBackup(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src", "file"), "x")
	if err := os.Mkdir(filepath.Join(dir, "dst"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "dst", "src"), "old")
	out, errb, code := runTool(t, dir, "--debug", "--strip-trailing-slashes", "-Z", "-b", "-S", ".bak", "src/", "dst")
	if code != 0 || out != "" {
		t.Fatalf("mv debug/backups: code=%d out=%q err=%q", code, out, errb)
	}
	if !strings.Contains(errb, "mv: debug: renamed 'src' -> '"+filepath.Join("dst", "src")+"'") {
		t.Fatalf("missing debug diagnostic: %q", errb)
	}
	if read(t, filepath.Join(dir, "dst", "src", "file")) != "x" {
		t.Fatal("directory not moved into target")
	}
	if read(t, filepath.Join(dir, "dst", "src.bak")) != "old" {
		t.Fatal("-b/-S backup was not created")
	}
}

func TestMvBackupAndProgressAliasesVisible(t *testing.T) {
	out, _, code := runTool(t, t.TempDir(), "--help")
	if code != 0 {
		t.Fatalf("mv --help code = %d", code)
	}
	for _, opt := range []string{"-b, --backup", "-g, --progress"} {
		if !strings.Contains(out, opt) {
			t.Fatalf("mv --help missing %q:\n%s", opt, out)
		}
	}

	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "new")
	write(t, filepath.Join(dir, "dst"), "old")
	_, errb, code := runTool(t, dir, "-bg", "src", "dst")
	if code != 0 || errb != "" {
		t.Fatalf("mv -bg: code=%d err=%q", code, errb)
	}
	if read(t, filepath.Join(dir, "dst")) != "new" || read(t, filepath.Join(dir, "dst~")) != "old" {
		t.Fatalf("mv -bg did not create simple backup")
	}
}

func TestMvNumberedBackupForm(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "new")
	write(t, filepath.Join(dir, "dst"), "old")
	write(t, filepath.Join(dir, "dst.~1~"), "previous")
	_, errb, code := runTool(t, dir, "--backup=numbered", "src", "dst")
	if code != 0 {
		t.Fatalf("mv --backup=numbered: code=%d err=%q", code, errb)
	}
	if read(t, filepath.Join(dir, "dst")) != "new" {
		t.Fatal("destination not moved")
	}
	if read(t, filepath.Join(dir, "dst.~2~")) != "old" {
		t.Fatal("numbered backup was not created")
	}
}

func TestMvBackupControlAndSameFile(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "new")
	write(t, filepath.Join(dir, "dst"), "old")
	_, errb, code := runTool(t, dir, "--backup=none", "src", "dst")
	if code != 0 || errb != "" || read(t, filepath.Join(dir, "dst")) != "new" {
		t.Fatalf("--backup=none: code=%d err=%q", code, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "dst~")); !os.IsNotExist(err) {
		t.Fatal("--backup=none created a backup")
	}

	write(t, filepath.Join(dir, "src"), "newer")
	write(t, filepath.Join(dir, "dst.~1~"), "previous")
	_, errb, code = runTool(t, dir, "--backup=existing", "src", "dst")
	if code != 0 || errb != "" || read(t, filepath.Join(dir, "dst.~2~")) != "new" {
		t.Fatalf("--backup=existing: code=%d err=%q", code, errb)
	}

	write(t, filepath.Join(dir, "same"), "keep")
	_, errb, code = runTool(t, dir, "--backup", "same", "same")
	if code != 1 || !strings.Contains(errb, "'same' and 'same' are the same file") {
		t.Fatalf("same-file backup: code=%d err=%q", code, errb)
	}
	if read(t, filepath.Join(dir, "same")) != "keep" {
		t.Fatal("same-file move changed the source")
	}
	if _, err := os.Lstat(filepath.Join(dir, "same~")); !os.IsNotExist(err) {
		t.Fatal("same-file move created a backup")
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
// using an injected EXDEV failure rather than directly calling internal methods.
func TestMvCopyFallback(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src", "f1"), "one")
	write(t, filepath.Join(dir, "src", "sub", "f2"), "two")
	if runtime.GOOS != "windows" {
		if err := os.Symlink("f1", filepath.Join(dir, "src", "link")); err != nil {
			t.Fatal(err)
		}
	}

	deps := defaultMoverDeps()
	deps.rename = func(oldpath, newpath string) error {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: syscall.EXDEV}
	}

	_, errb, code := runToolInputDeps(t, dir, "", deps, "src", "dst")
	if code != 0 {
		t.Fatalf("EXDEV fallback failed: code=%d err=%q", code, errb)
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

func TestMvTrailingSlashOnRegularFile(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "NEWCONTENT")
	write(t, filepath.Join(dir, "targetfile"), "ORIGINAL")

	_, errb, code := runTool(t, dir, "src", "targetfile/")
	if code != 1 {
		t.Fatalf("mv src targetfile/ must fail: code=%d", code)
	}
	if !strings.Contains(errb, "Not a directory") {
		t.Errorf("expected 'Not a directory' in stderr, got %q", errb)
	}
	if read(t, filepath.Join(dir, "targetfile")) != "ORIGINAL" {
		t.Error("destination file was overwritten")
	}
	if _, err := os.Stat(filepath.Join(dir, "src")); err != nil {
		t.Error("source file was destroyed")
	}
}

func TestMvNoTargetDirectoryTrailingSlashOnExistingDir(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "NEWCONTENT")
	if err := os.Mkdir(filepath.Join(dir, "somedir"), 0o755); err != nil {
		t.Fatal(err)
	}

	// -T disables target-directory treatment, but "somedir/" DOES resolve
	// to an existing directory, so this must not be misdiagnosed as
	// "Not a directory". The move itself still fails (a regular file
	// cannot be renamed onto an existing directory), but the source must
	// survive and the directory must be untouched.
	_, errb, code := runTool(t, dir, "-T", "src", "somedir/")
	if code != 1 {
		t.Fatalf("mv -T src somedir/ must fail: code=%d", code)
	}
	if strings.Contains(errb, "Not a directory") {
		t.Errorf("existing directory misdiagnosed as not-a-directory: %q", errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "src")); err != nil {
		t.Error("source file was destroyed")
	}
	if fi, err := os.Stat(filepath.Join(dir, "somedir")); err != nil || !fi.IsDir() {
		t.Error("destination directory was disturbed")
	}
}

func TestMvTrailingSlashOnNonexistentDestination(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "NEWCONTENT")

	_, errb, code := runTool(t, dir, "src", "nosuchdir/")
	if code != 1 {
		t.Fatalf("mv src nosuchdir/ must fail: code=%d", code)
	}
	if !strings.Contains(errb, "No such file or directory") {
		t.Errorf("expected 'No such file or directory' in stderr, got %q", errb)
	}
	if strings.Contains(errb, "Not a directory") {
		t.Errorf("nonexistent destination misdiagnosed as not-a-directory: %q", errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "src")); err != nil {
		t.Error("source file was destroyed")
	}
}

func TestMvTrailingSlashChecksMissingSourceFirst(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(string)
		dest  string
	}{
		{name: "missing destination", dest: "missing/"},
		{name: "non-directory destination", dest: "dst/", setup: func(dir string) {
			write(t, filepath.Join(dir, "dst"), "keep")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.setup != nil {
				tc.setup(dir)
			}
			_, errb, code := runTool(t, dir, "nosrc", tc.dest)
			if code != 1 || !strings.Contains(errb, "cannot stat 'nosrc': No such file or directory") {
				t.Fatalf("code=%d err=%q", code, errb)
			}
		})
	}
}

func TestMvDirectoryToMissingTrailingSlash(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src", "file"), "content")

	_, errb, code := runTool(t, dir, "src", "newdir/")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if got := read(t, filepath.Join(dir, "newdir", "file")); got != "content" {
		t.Fatalf("moved content=%q", got)
	}
	if _, err := os.Lstat(filepath.Join(dir, "src")); !os.IsNotExist(err) {
		t.Fatalf("source still exists: %v", err)
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

func TestMvCopyFallbackFailures(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src_fail_copy"), "one")
	write(t, filepath.Join(dir, "src_fail_remove"), "two")

	deps := defaultMoverDeps()
	deps.rename = func(oldpath, newpath string) error {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: syscall.EXDEV}
	}

	deps.removeAll = func(path string) error {
		if filepath.Base(path) == "src_fail_remove" {
			return os.ErrPermission
		}
		return os.RemoveAll(path)
	}

	// 1. Fail during copy (source is unreadable)
	os.Chmod(filepath.Join(dir, "src_fail_copy"), 0o000)
	_, _, code := runToolInputDeps(t, dir, "", deps, "src_fail_copy", "dst1")
	if code == 0 {
		t.Errorf("expected failure when copy fails")
	}
	if _, err := os.Lstat(filepath.Join(dir, "src_fail_copy")); os.IsNotExist(err) {
		t.Errorf("source should not be removed if copy fails")
	}
	os.Chmod(filepath.Join(dir, "src_fail_copy"), 0o644) // restore for cleanup

	// 2. Fail during remove
	_, _, code2 := runToolInputDeps(t, dir, "", deps, "src_fail_remove", "dst2")
	if code2 == 0 {
		t.Errorf("expected failure when remove fails")
	}
	if read(t, filepath.Join(dir, "dst2")) != "two" {
		t.Errorf("copy should have completed before remove failed")
	}
}

func TestMvInteractiveRefusal(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "one")
	write(t, filepath.Join(dir, "dst"), "two")

	// Provide an explicit negative response
	out, errb, code := runToolInput(t, dir, "n\n", "-i", "src", "dst")

	if code != 0 {
		t.Errorf("expected 0 exit code on refusal, got %d. stderr=%q", code, errb)
	}
	if out != "" {
		t.Errorf("unexpected output: %q", out)
	}
	if read(t, filepath.Join(dir, "src")) != "one" {
		t.Errorf("source should still exist")
	}
	if read(t, filepath.Join(dir, "dst")) != "two" {
		t.Errorf("destination should still be 'two'")
	}
}

func TestMvInteractiveLeadingWhitespaceIsNotAffirmative(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "new")
	write(t, filepath.Join(dir, "dst"), "old")
	_, errb, code := runToolInput(t, dir, " yes\n", "-i", "src", "dst")
	if code != 0 || !strings.Contains(errb, "overwrite 'dst'?") || read(t, filepath.Join(dir, "dst")) != "old" {
		t.Fatalf("leading-space reply = (_, %q, %d), destination was not preserved", errb, code)
	}
}

func TestMvPromptsBeforeSameFileResolution(t *testing.T) {
	for _, tc := range []struct {
		name, reply string
		wantCode    int
		wantSame    bool
	}{
		{"declined", "n\n", 0, false},
		{"accepted", "y\n", 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, filepath.Join(dir, "same"), "keep")
			_, errb, code := runToolInput(t, dir, tc.reply, "-i", "same", "same")
			if code != tc.wantCode || !strings.Contains(errb, "overwrite 'same'?") || strings.Contains(errb, "same file") != tc.wantSame {
				t.Fatalf("same-file prompt = (_, %q, %d)", errb, code)
			}
		})
	}
}

func TestMvUnsupportedLCMessagesFailsClosed(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "new")
	write(t, filepath.Join(dir, "dst"), "old")
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: []string{"LC_MESSAGES=en_US.UTF-8"}, Stdio: tool.Stdio{In: strings.NewReader("yes\n"), Out: &out, Err: &errb}}
	if code := runWithDeps(rc, []string{"-i", "src", "dst"}, defaultMoverDeps()); code != 1 || !strings.Contains(errb.String(), "unsupported LC_MESSAGES locale") || read(t, filepath.Join(dir, "dst")) != "old" {
		t.Fatalf("unsupported LC_MESSAGES = (_, %q, %d)", errb.String(), code)
	}
}

func TestMvLastOverwriteOptionWins(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		options     []string
		wantMoved   bool
	}{
		{"short force last", "", []string{"-if"}, true},
		{"short interactive last", "n\n", []string{"-fi"}, false},
		{"long force last", "", []string{"--interactive", "--force"}, true},
		{"long interactive last", "n\n", []string{"--force", "--interactive"}, false},
		{"long no-clobber last", "", []string{"--force", "--no-clobber"}, false},
		{"abbreviated force last", "", []string{"--inter", "--for"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, filepath.Join(dir, "src"), "new")
			write(t, filepath.Join(dir, "dst"), "old")
			args := append(append([]string(nil), tc.options...), "src", "dst")
			_, errb, code := runToolInput(t, dir, tc.input, args...)
			if code != 0 {
				t.Fatalf("options %v = (_, %q, %d)", tc.options, errb, code)
			}
			got := read(t, filepath.Join(dir, "dst")) == "new"
			if got != tc.wantMoved {
				t.Fatalf("moved=%v, want %v; stderr=%q", got, tc.wantMoved, errb)
			}
		})
	}

	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "new")
	write(t, filepath.Join(dir, "dst"), "old")
	_, errb, code := runTool(t, dir, "--interactive=always", "src", "dst")
	if code != 2 || !strings.Contains(errb, "option does not take a value") {
		t.Fatalf("unsupported mv --interactive=always = (_, %q, %d), want usage error", errb, code)
	}
}

func TestMvPromptUnwritable(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "one")
	write(t, filepath.Join(dir, "dst"), "two")
	os.Chmod(filepath.Join(dir, "dst"), 0o444)

	deps := defaultMoverDeps()
	deps.terminal = func(r io.Reader) bool { return true }
	deps.writable = func(string) bool { return false }

	_, errb, code := runToolInputDeps(t, dir, "y\n", deps, "src", "dst")

	if code != 0 {
		t.Errorf("expected 0, got %d. err=%q", code, errb)
	}
	if !strings.Contains(errb, "override mode?") && !strings.Contains(errb, "replace") {
		t.Errorf("expected prompt for unwritable destination, got %q", errb)
	}
}

func TestMvEXDEVCharacteristicFailuresDiagnoseButStillMove(t *testing.T) {
	tests := []struct {
		name, diagnostic string
		fail             func(*moverDeps)
	}{
		{"ownership", "preserving ownership", func(d *moverDeps) {
			d.preserveFileOwner = func(*os.File, os.FileInfo) error { return errors.New("owner failed") }
		}},
		{"permissions", "preserving permissions", func(d *moverDeps) { d.fchmod = func(*os.File, os.FileMode) error { return errors.New("mode failed") } }},
		{"times", "preserving times", func(d *moverDeps) {
			d.preserveFileTimes = func(*os.File, os.FileInfo) error { return errors.New("time failed") }
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, filepath.Join(dir, "src"), "payload")
			deps := exdevDeps()
			tc.fail(&deps)
			_, errb, code := runToolInputDeps(t, dir, "", deps, "src", "dst")
			if code != 0 || !strings.Contains(errb, tc.diagnostic) || !strings.Contains(errb, "failed") {
				t.Fatalf("metadata failure = (_, %q, %d), want %q diagnostic", errb, code, tc.diagnostic)
			}
			if _, err := os.Lstat(filepath.Join(dir, "src")); !os.IsNotExist(err) {
				t.Fatalf("source was not removed: %v", err)
			}
			if got := read(t, filepath.Join(dir, "dst")); got != "payload" {
				t.Fatalf("destination content = %q", got)
			}
		})
	}
}

func TestMvEXDEVDirectoryCharacteristicFailureDiagnosesButStillMoves(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	deps := exdevDeps()
	deps.chmod = func(string, os.FileMode) error { return errors.New("directory mode failed") }
	_, errb, code := runToolInputDeps(t, dir, "", deps, "src", "dst")
	if code != 0 || !strings.Contains(errb, "preserving permissions") || !strings.Contains(errb, "Directory mode failed") {
		t.Fatalf("directory characteristic failure = (_, %q, %d)", errb, code)
	}
	if _, err := os.Lstat(filepath.Join(dir, "src")); !os.IsNotExist(err) {
		t.Fatalf("source directory was not removed: %v", err)
	}
	if fi, err := os.Stat(filepath.Join(dir, "dst")); err != nil || !fi.IsDir() {
		t.Fatalf("destination directory missing: %v, %v", fi, err)
	}
}

func TestMvEXDEVSymlinkCharacteristicFailuresDiagnoseButStillMove(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on some Windows hosts")
	}
	for _, tc := range []struct {
		name, diagnostic string
		fail             func(*moverDeps)
	}{
		{"ownership", "preserving symbolic link ownership", func(d *moverDeps) {
			d.preserveLinkOwner = func(string, os.FileInfo) error { return errors.New("link owner failed") }
		}},
		{"permissions", "preserving symbolic link permissions", func(d *moverDeps) {
			d.preserveLinkMode = func(string, os.FileInfo) error { return errors.New("link mode failed") }
		}},
		{"times", "preserving symbolic link times", func(d *moverDeps) {
			d.preserveLinkTimes = func(string, os.FileInfo) error { return errors.New("link time failed") }
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Symlink("target", filepath.Join(dir, "src")); err != nil {
				t.Fatal(err)
			}
			deps := exdevDeps()
			tc.fail(&deps)
			_, errb, code := runToolInputDeps(t, dir, "", deps, "src", "dst")
			if code != 0 || !strings.Contains(errb, tc.diagnostic) {
				t.Fatalf("symlink metadata failure = (_, %q, %d)", errb, code)
			}
			if _, err := os.Lstat(filepath.Join(dir, "src")); !os.IsNotExist(err) {
				t.Fatalf("source symlink was not removed: %v", err)
			}
			if target, err := os.Readlink(filepath.Join(dir, "dst")); err != nil || target != "target" {
				t.Fatalf("destination symlink = (%q, %v)", target, err)
			}
		})
	}
}

func TestMvEXDEVRegularReplacementDoesNotFollowDestinationSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on some Windows hosts")
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "new")
	write(t, filepath.Join(dir, "referent"), "must stay")
	if err := os.Symlink("referent", filepath.Join(dir, "dst")); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runToolInputDeps(t, dir, "", exdevDeps(), "src", "dst")
	if code != 0 || errb != "" {
		t.Fatalf("EXDEV symlink replacement = (_, %q, %d)", errb, code)
	}
	if got := read(t, filepath.Join(dir, "referent")); got != "must stay" {
		t.Fatalf("symlink referent was truncated: %q", got)
	}
	if fi, err := os.Lstat(filepath.Join(dir, "dst")); err != nil || fi.Mode()&os.ModeSymlink != 0 || read(t, filepath.Join(dir, "dst")) != "new" {
		t.Fatalf("destination was not replaced by regular file: info=%v err=%v", fi, err)
	}
}

func TestMvEXDEVReplacesEmptyDestinationDirectoryBeforeDuplication(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src", "fresh"), "new")
	if err := os.MkdirAll(filepath.Join(dir, "target", "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runToolInputDeps(t, dir, "", exdevDeps(), "src", "target")
	if code != 0 || errb != "" {
		t.Fatalf("EXDEV directory replacement = (_, %q, %d)", errb, code)
	}
	if got := read(t, filepath.Join(dir, "target", "src", "fresh")); got != "new" {
		t.Fatalf("fresh child = %q", got)
	}
	if _, err := os.Lstat(filepath.Join(dir, "src")); !os.IsNotExist(err) {
		t.Fatalf("source hierarchy was not removed: %v", err)
	}
}

func TestMvEXDEVDoesNotRecursivelyRemoveNonemptyDestination(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src", "fresh"), "new")
	write(t, filepath.Join(dir, "target", "src", "stale"), "old")
	_, errb, code := runToolInputDeps(t, dir, "", exdevDeps(), "src", "target")
	if code != 1 || !strings.Contains(errb, "cannot remove 'target/src'") {
		t.Fatalf("nonempty destination = (_, %q, %d)", errb, code)
	}
	if got := read(t, filepath.Join(dir, "target", "src", "stale")); got != "old" {
		t.Fatalf("stale destination child was modified: %q", got)
	}
	if got := read(t, filepath.Join(dir, "src", "fresh")); got != "new" {
		t.Fatalf("source hierarchy was modified: %q", got)
	}
}

func TestMvEXDEVDestinationRemovalFailureRetainsSource(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "new")
	write(t, filepath.Join(dir, "dst"), "old")
	deps := exdevDeps()
	deps.remove = func(path string) error {
		if filepath.Base(path) == "dst" {
			return errors.New("remove destination failed")
		}
		return os.Remove(path)
	}
	_, errb, code := runToolInputDeps(t, dir, "", deps, "src", "dst")
	if code != 1 || !strings.Contains(errb, "cannot remove 'dst'") || read(t, filepath.Join(dir, "src")) != "new" || read(t, filepath.Join(dir, "dst")) != "old" {
		t.Fatalf("destination removal failure = (_, %q, %d)", errb, code)
	}
}

func TestMvEXDEVRegularCharacteristicsStayBoundToCreatedDescriptor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on some Windows hosts")
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "payload")
	referent := filepath.Join(dir, "referent")
	write(t, referent, "guard")
	wantReferentTime := time.Unix(1_500_000_000, 0)
	if err := os.Chtimes(referent, wantReferentTime, wantReferentTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(referent, 0o640); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(dir, "dst")
	deps := exdevDeps()
	originalOwner := deps.preserveFileOwner
	deps.preserveFileOwner = func(f *os.File, fi os.FileInfo) error {
		if err := os.Remove(dst); err != nil {
			return err
		}
		if err := os.Symlink("referent", dst); err != nil {
			return err
		}
		return originalOwner(f, fi)
	}
	_, errb, code := runToolInputDeps(t, dir, "", deps, "src", "dst")
	if code != 0 || errb != "" {
		t.Fatalf("descriptor-bound preservation = (_, %q, %d)", errb, code)
	}
	fi, err := os.Stat(referent)
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, referent); got != "guard" || fi.Mode().Perm() != 0o640 || !fi.ModTime().Equal(wantReferentTime) {
		t.Fatalf("swapped symlink referent changed: content=%q mode=%o mtime=%v", got, fi.Mode().Perm(), fi.ModTime())
	}
}

// POSIX: an error on one source_file is diagnosed, affects the exit status,
// and does not stop processing of the remaining source_files.
func TestMvContinuesPastOperandErrors(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "good"), "payload")
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "nope", "good", "d")
	if code != 1 || !strings.Contains(errb, "cannot stat 'nope'") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if read(t, filepath.Join(dir, "d", "good")) != "payload" {
		t.Fatal("later operand not moved after earlier failure")
	}
	if _, err := os.Lstat(filepath.Join(dir, "good")); !os.IsNotExist(err) {
		t.Fatal("moved source still present")
	}
}

// rename() equivalence for the target_dir form: a source directory replaces
// an existing EMPTY directory of the same name inside the target, and a
// NON-EMPTY one is a diagnosed per-operand error (rename fails ENOTEMPTY /
// EEXIST), leaving both hierarchies intact.
func TestMvDirectoryOntoExistingDirectoryInTarget(t *testing.T) {
	t.Run("empty destination replaced", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("MoveFileEx cannot replace an existing directory on Windows")
		}
		dir := t.TempDir()
		write(t, filepath.Join(dir, "src", "fresh"), "new")
		if err := os.MkdirAll(filepath.Join(dir, "target", "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		_, errb, code := runTool(t, dir, "src", "target")
		if code != 0 || errb != "" {
			t.Fatalf("code=%d err=%q", code, errb)
		}
		if read(t, filepath.Join(dir, "target", "src", "fresh")) != "new" {
			t.Fatal("source hierarchy did not replace empty destination directory")
		}
		if _, err := os.Lstat(filepath.Join(dir, "src")); !os.IsNotExist(err) {
			t.Fatal("source directory still present")
		}
	})
	t.Run("nonempty destination diagnosed", func(t *testing.T) {
		dir := t.TempDir()
		write(t, filepath.Join(dir, "src", "fresh"), "new")
		write(t, filepath.Join(dir, "target", "src", "stale"), "old")
		_, errb, code := runTool(t, dir, "src", "target")
		if code != 1 || !strings.Contains(errb, "cannot move 'src'") {
			t.Fatalf("code=%d err=%q", code, errb)
		}
		if read(t, filepath.Join(dir, "target", "src", "stale")) != "old" {
			t.Fatal("nonempty destination child modified")
		}
		if read(t, filepath.Join(dir, "src", "fresh")) != "new" {
			t.Fatal("source hierarchy modified after failed rename")
		}
	})
}

// mv moves a symlink operand itself (the link, never its referent).
func TestMvSymlinkOperandMovesLinkItself(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on some Windows hosts")
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, "referent"), "stay")
	if err := os.Symlink("referent", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "link", "moved")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if target, err := os.Readlink(filepath.Join(dir, "moved")); err != nil || target != "referent" {
		t.Fatalf("moved entry is not the original link: %q %v", target, err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "link")); !os.IsNotExist(err) {
		t.Fatal("source link still present")
	}
	if read(t, filepath.Join(dir, "referent")) != "stay" {
		t.Fatal("referent disturbed")
	}
}

func exdevDeps() moverDeps {
	deps := defaultMoverDeps()
	deps.rename = func(oldpath, newpath string) error {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: syscall.EXDEV}
	}
	return deps
}
