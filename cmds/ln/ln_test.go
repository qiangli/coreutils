package lncmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// runTool is the canonical test harness shape for cmds packages.
func runTool(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	return runToolIn(t, dir, "", args...)
}

func runToolIn(t *testing.T, dir, input string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

// requireSymlinks skips on platforms where the test user cannot create
// symlinks (Windows without Developer Mode).
func requireSymlinks(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Symlink("probe-target", filepath.Join(dir, "probe")); err != nil {
		t.Skipf("symlinks not available: %v", err)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLnHard(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.txt"), "hello")
	_, errb, code := runTool(t, dir, "a.txt", "b.txt")
	if code != 0 || errb != "" {
		t.Fatalf("ln a b: code=%d err=%q", code, errb)
	}
	got, err := os.ReadFile(filepath.Join(dir, "b.txt"))
	if err != nil || string(got) != "hello" {
		t.Errorf("link content=%q err=%v", got, err)
	}
}

func TestLnSymbolic(t *testing.T) {
	requireSymlinks(t)
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.txt"), "hello")
	_, errb, code := runTool(t, dir, "-s", "a.txt", "l")
	if code != 0 || errb != "" {
		t.Fatalf("ln -s: code=%d err=%q", code, errb)
	}
	// The link stores the target verbatim.
	target, err := os.Readlink(filepath.Join(dir, "l"))
	if err != nil || target != "a.txt" {
		t.Errorf("readlink=%q err=%v", target, err)
	}
}

func TestLnSingleOperand(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "sub", "a.txt"), "x")
	_, errb, code := runTool(t, dir, "sub/a.txt")
	if code != 0 || errb != "" {
		t.Fatalf("ln TARGET: code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); err != nil {
		t.Errorf("basename link missing: %v", err)
	}
}

func TestLnIntoDirectory(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a1"), "1")
	write(t, filepath.Join(dir, "a2"), "2")
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "a1", "a2", "d")
	if code != 0 || errb != "" {
		t.Fatalf("ln a1 a2 d: code=%d err=%q", code, errb)
	}
	for _, n := range []string{"a1", "a2"} {
		if _, err := os.Stat(filepath.Join(dir, "d", n)); err != nil {
			t.Errorf("d/%s missing: %v", n, err)
		}
	}
}

func TestLnTargetDirectoryOption(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a1"), "1")
	write(t, filepath.Join(dir, "a2"), "2")
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-t", "d", "a1", "a2")
	if code != 0 || errb != "" {
		t.Fatalf("ln -t d a1 a2: code=%d err=%q", code, errb)
	}
	for _, n := range []string{"a1", "a2"} {
		if _, err := os.Stat(filepath.Join(dir, "d", n)); err != nil {
			t.Errorf("d/%s missing: %v", n, err)
		}
	}
}

func TestLnNoTargetDirectory(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "a")
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-T", "a", "d")
	if code != 1 || !strings.Contains(errb, "failed to create hard link") {
		t.Errorf("ln -T a d: code=%d err=%q", code, errb)
	}
}

func TestLnNoDereferenceDestinationSymlink(t *testing.T) {
	requireSymlinks(t)
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "a")
	if err := os.Mkdir(filepath.Join(dir, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(dir, "linkdir")); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-n", "a", "linkdir")
	if code != 1 || !strings.Contains(errb, "failed to create hard link") {
		t.Errorf("ln -n a linkdir: code=%d err=%q", code, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "real", "a")); !os.IsNotExist(err) {
		t.Errorf("ln -n unexpectedly linked inside symlinked directory: %v", err)
	}
}

func TestLnRelativeSymbolic(t *testing.T) {
	requireSymlinks(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "links", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "src", "file"), "x")
	_, errb, code := runTool(t, dir, "-sr", "src/file", "links/deep/file-link")
	if code != 0 || errb != "" {
		t.Fatalf("ln -sr: code=%d err=%q", code, errb)
	}
	target, err := os.Readlink(filepath.Join(dir, "links", "deep", "file-link"))
	want := filepath.Join("..", "..", "src", "file")
	if err != nil || target != want {
		t.Errorf("relative symlink target=%q err=%v, want %q", target, err, want)
	}
}

func TestLnForce(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "a")
	write(t, filepath.Join(dir, "b"), "b")
	// Without -f: destination exists -> failure.
	_, errb, code := runTool(t, dir, "a", "b")
	if code != 1 || !strings.Contains(errb, "failed to create hard link") {
		t.Errorf("no -f: code=%d err=%q", code, errb)
	}
	// With -f: replaced.
	_, errb, code = runTool(t, dir, "-f", "a", "b")
	if code != 0 || errb != "" {
		t.Fatalf("-f: code=%d err=%q", code, errb)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "b"))
	if string(got) != "a" {
		t.Errorf("after -f content=%q", got)
	}
}

func TestLnBackupAndSuffix(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "new")
	write(t, filepath.Join(dir, "dest"), "old")
	_, errb, code := runTool(t, dir, "-b", "-S", ".bak", "src", "dest")
	if code != 0 || errb != "" {
		t.Fatalf("ln -b -S: code=%d err=%q", code, errb)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "dest"))
	if string(got) != "new" {
		t.Errorf("dest content=%q", got)
	}
	backup, err := os.ReadFile(filepath.Join(dir, "dest.bak"))
	if err != nil || string(backup) != "old" {
		t.Errorf("backup content=%q err=%v", backup, err)
	}
}

func TestLnBackupExistingUsesNumberedWhenPresent(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "new")
	write(t, filepath.Join(dir, "dest"), "old")
	write(t, filepath.Join(dir, "dest.~1~"), "older")
	_, errb, code := runTool(t, dir, "--backup", "src", "dest")
	if code != 0 || errb != "" {
		t.Fatalf("ln --backup: code=%d err=%q", code, errb)
	}
	backup, err := os.ReadFile(filepath.Join(dir, "dest.~2~"))
	if err != nil || string(backup) != "old" {
		t.Errorf("numbered backup content=%q err=%v", backup, err)
	}
}

func TestLnInteractiveAcceptsAndDeclines(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "new")
	write(t, filepath.Join(dir, "dest"), "old")
	_, errb, code := runToolIn(t, dir, "", "-i", "src", "dest")
	if code != 0 || !strings.Contains(errb, "replace 'dest'?") || strings.Contains(errb, "cannot read response") {
		t.Fatalf("ln -i default EOF: code=%d err=%q", code, errb)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "dest"))
	if string(got) != "old" {
		t.Errorf("declined replacement content=%q", got)
	}
	_, errb, code = runToolIn(t, dir, "n\n", "-i", "src", "dest")
	if code != 0 || !strings.Contains(errb, "replace 'dest'?") {
		t.Fatalf("ln -i decline: code=%d err=%q", code, errb)
	}
	got, _ = os.ReadFile(filepath.Join(dir, "dest"))
	if string(got) != "old" {
		t.Errorf("declined replacement content=%q", got)
	}
	_, errb, code = runToolIn(t, dir, "y\n", "-i", "src", "dest")
	if code != 0 || !strings.Contains(errb, "replace 'dest'?") {
		t.Fatalf("ln -i accept: code=%d err=%q", code, errb)
	}
	got, _ = os.ReadFile(filepath.Join(dir, "dest"))
	if string(got) != "new" {
		t.Errorf("accepted replacement content=%q", got)
	}
}

func TestLnForceAndInteractiveLastOptionWins(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "new")
	write(t, filepath.Join(dir, "dest"), "old")
	_, errb, code := runToolIn(t, dir, "n\n", "-f", "-i", "src", "dest")
	if code != 0 || !strings.Contains(errb, "replace 'dest'?") {
		t.Fatalf("ln -f -i: code=%d err=%q", code, errb)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "dest"))
	if string(got) != "old" {
		t.Errorf("-f -i should obey prompt, got %q", got)
	}
	_, errb, code = runToolIn(t, dir, "n\n", "-i", "-f", "src", "dest")
	if code != 0 || strings.Contains(errb, "replace 'dest'?") {
		t.Fatalf("ln -i -f: code=%d err=%q", code, errb)
	}
	got, _ = os.ReadFile(filepath.Join(dir, "dest"))
	if string(got) != "new" {
		t.Errorf("-i -f should force replacement, got %q", got)
	}
}

func TestLnLogicalAndPhysicalSourceSymlink(t *testing.T) {
	requireSymlinks(t)
	dir := t.TempDir()
	write(t, filepath.Join(dir, "real"), "real")
	if err := os.Symlink("real", filepath.Join(dir, "sym")); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-L", "sym", "logical")
	if code != 0 || errb != "" {
		t.Fatalf("ln -L: code=%d err=%q", code, errb)
	}
	if target, err := os.Readlink(filepath.Join(dir, "logical")); err == nil {
		t.Fatalf("ln -L created symlink hard link to %q, want regular file", target)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "logical"))
	if string(got) != "real" {
		t.Errorf("logical content=%q", got)
	}
	_, errb, code = runTool(t, dir, "-P", "sym", "physical")
	if code != 0 || errb != "" {
		t.Fatalf("ln -P: code=%d err=%q", code, errb)
	}
	target, err := os.Readlink(filepath.Join(dir, "physical"))
	if err != nil || target != "real" {
		t.Errorf("physical readlink=%q err=%v", target, err)
	}
}

func TestLnVerbose(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "a")
	out, _, code := runTool(t, dir, "-v", "a", "b")
	if code != 0 || out != "'b' => 'a'\n" {
		t.Errorf("hard -v: code=%d out=%q", code, out)
	}
	requireSymlinks(t)
	out, _, code = runTool(t, dir, "-sv", "a", "l")
	if code != 0 || out != "'l' -> 'a'\n" {
		t.Errorf("sym -v: code=%d out=%q", code, out)
	}
}

func TestLnErrors(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir)
	if code != 2 || !strings.Contains(errb, "missing file operand") {
		t.Errorf("no args: code=%d err=%q", code, errb)
	}
	write(t, filepath.Join(dir, "a"), "a")
	write(t, filepath.Join(dir, "b"), "b")
	_, errb, code = runTool(t, dir, "a", "b", "nodir")
	if code != 1 || !strings.Contains(errb, "is not a directory") {
		t.Errorf("last not dir: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "missing-target", "x")
	if code != 1 || !strings.Contains(errb, "failed to create hard link") {
		t.Errorf("missing target: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "--frobnicate", "a", "b")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "-t", "d", "-T", "a")
	if code != 2 || !strings.Contains(errb, "cannot combine -t and -T") {
		t.Errorf("ln -t -T: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "-r", "a", "c")
	if code != 2 || !strings.Contains(errb, "--relative can only be used") {
		t.Errorf("ln -r without -s: code=%d err=%q", code, errb)
	}
}

func TestLnSameFile(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "keep")

	// Hard link to the same path without -f.
	_, errb, code := runTool(t, dir, "a", "a")
	if code != 1 || !strings.Contains(errb, "'a' and 'a' are the same file") {
		t.Errorf("ln a a: code=%d err=%q", code, errb)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "a")); string(got) != "keep" {
		t.Errorf("ln a a modified the file: %q", got)
	}

	// Hard link to the same path with -f: POSIX says do nothing and diagnose.
	_, errb, code = runTool(t, dir, "-f", "a", "a")
	if code != 1 || !strings.Contains(errb, "'a' and 'a' are the same file") {
		t.Errorf("ln -f a a: code=%d err=%q", code, errb)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "a")); string(got) != "keep" {
		t.Errorf("ln -f a a modified the file: %q", got)
	}
}

func TestLnSymbolicSameFile(t *testing.T) {
	requireSymlinks(t)
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "keep")

	// Symbolic link to the same path without -f.
	_, errb, code := runTool(t, dir, "-s", "a", "a")
	if code != 1 || !strings.Contains(errb, "'a' and 'a' are the same file") {
		t.Errorf("ln -s a a: code=%d err=%q", code, errb)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "a")); string(got) != "keep" {
		t.Errorf("ln -s a a modified the file: %q", got)
	}

	// Symbolic link to the same path with -f: must not create a self-loop.
	_, errb, code = runTool(t, dir, "-sf", "a", "a")
	if code != 1 || !strings.Contains(errb, "'a' and 'a' are the same file") {
		t.Errorf("ln -sf a a: code=%d err=%q", code, errb)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "a")); string(got) != "keep" {
		t.Errorf("ln -sf a a modified the file: %q", got)
	}
}

func TestLnSameFileDirectoryForm(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "keep")

	// Linking a file into the current directory produces the same destination.
	_, errb, code := runTool(t, dir, "a", ".")
	if code != 1 || !strings.Contains(errb, "'a' and 'a' are the same file") {
		t.Errorf("ln a .: code=%d err=%q", code, errb)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "a")); string(got) != "keep" {
		t.Errorf("ln a . modified the file: %q", got)
	}

	// Same through -t.
	_, errb, code = runTool(t, dir, "-t", ".", "a")
	if code != 1 || !strings.Contains(errb, "'a' and 'a' are the same file") {
		t.Errorf("ln -t . a: code=%d err=%q", code, errb)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "a")); string(got) != "keep" {
		t.Errorf("ln -t . a modified the file: %q", got)
	}
}

func TestLnHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, t.TempDir(), "--help")
	if code != 0 || !strings.Contains(out, "Usage: ln") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, t.TempDir(), "--version")
	if code != 0 || !strings.Contains(out, "ln") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, t.TempDir(), "-V")
	if code != 0 || !strings.Contains(out, "ln") {
		t.Errorf("-V: code=%d out=%q", code, out)
	}
}
