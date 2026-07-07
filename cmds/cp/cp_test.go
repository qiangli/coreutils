package cpcmd

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

func TestCpFile(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "hello")
	out, errb, code := runTool(t, dir, "a", "b")
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("cp a b: code=%d out=%q err=%q", code, out, errb)
	}
	if got := read(t, filepath.Join(dir, "b")); got != "hello" {
		t.Errorf("content = %q, want %q", got, "hello")
	}
	// source still present
	if _, err := os.Stat(filepath.Join(dir, "a")); err != nil {
		t.Errorf("source removed: %v", err)
	}
}

func TestCpIntoDir(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "x")
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "a", "d")
	if code != 0 {
		t.Fatalf("cp a d: code=%d err=%q", code, errb)
	}
	if got := read(t, filepath.Join(dir, "d", "a")); got != "x" {
		t.Errorf("d/a = %q", got)
	}
}

func TestCpTargetDirectoryAndNoTargetDirectory(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "x")
	write(t, filepath.Join(dir, "b"), "y")
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-t", "d", "a", "b")
	if code != 0 {
		t.Fatalf("cp -t: code=%d err=%q", code, errb)
	}
	if read(t, filepath.Join(dir, "d", "a")) != "x" || read(t, filepath.Join(dir, "d", "b")) != "y" {
		t.Fatal("-t did not copy both sources into directory")
	}
	_, errb, code = runTool(t, dir, "-T", "a", "d")
	if code != 1 || !strings.Contains(errb, "cannot overwrite directory 'd' with non-directory") {
		t.Errorf("cp -T file dir: code=%d err=%q", code, errb)
	}
}

func TestCpMultipleToNonDir(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "1")
	write(t, filepath.Join(dir, "b"), "2")
	_, errb, code := runTool(t, dir, "a", "b", "c")
	if code != 1 || !strings.Contains(errb, "target 'c' is not a directory") {
		t.Errorf("code=%d err=%q", code, errb)
	}
}

func TestCpOmitsDirWithoutR(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "d", "e")
	if code != 1 || !strings.Contains(errb, "-r not specified; omitting directory 'd'") {
		t.Errorf("code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "e")); err == nil {
		t.Error("destination created despite omitted directory")
	}
}

func TestCpRecursive(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src", "f1"), "one")
	write(t, filepath.Join(dir, "src", "sub", "f2"), "two")

	// GNU edge: dest does not exist -> src tree copied AS dest.
	_, errb, code := runTool(t, dir, "-r", "src", "dst")
	if code != 0 {
		t.Fatalf("cp -r src dst: code=%d err=%q", code, errb)
	}
	if read(t, filepath.Join(dir, "dst", "f1")) != "one" || read(t, filepath.Join(dir, "dst", "sub", "f2")) != "two" {
		t.Error("tree not copied as new name")
	}

	// GNU edge: dest exists as dir -> src copied INTO it under its basename.
	if err := os.Mkdir(filepath.Join(dir, "exist"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code = runTool(t, dir, "-r", "src", "exist")
	if code != 0 {
		t.Fatalf("cp -r src exist: code=%d err=%q", code, errb)
	}
	if read(t, filepath.Join(dir, "exist", "src", "sub", "f2")) != "two" {
		t.Error("tree not copied into existing dir under basename")
	}
}

func TestCpCapitalRAlias(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src", "f"), "z")
	_, errb, code := runTool(t, dir, "-R", "src", "dst")
	if code != 0 {
		t.Fatalf("cp -R: code=%d err=%q", code, errb)
	}
	if read(t, filepath.Join(dir, "dst", "f")) != "z" {
		t.Error("-R did not copy recursively")
	}
	// clustered: -Rv
	_, errb, code = runTool(t, dir, "-Rv", "src", "dst2")
	if code != 0 {
		t.Fatalf("cp -Rv: code=%d err=%q", code, errb)
	}
}

func TestCpNoClobber(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "new")
	write(t, filepath.Join(dir, "b"), "old")
	out, errb, code := runTool(t, dir, "-n", "a", "b")
	if code != 0 || out != "" || errb != "" {
		t.Errorf("cp -n: code=%d out=%q err=%q", code, out, errb)
	}
	if got := read(t, filepath.Join(dir, "b")); got != "old" {
		t.Errorf("destination overwritten despite -n: %q", got)
	}
	// -n then -f: final one takes effect (GNU rule) -> overwrite.
	_, _, code = runTool(t, dir, "-n", "-f", "a", "b")
	if code != 0 || read(t, filepath.Join(dir, "b")) != "new" {
		t.Errorf("-n -f should overwrite (last wins)")
	}
}

func TestCpBackupSuffixUpdateAndInteractive(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a")
	dst := filepath.Join(dir, "b")
	write(t, src, "new")
	write(t, dst, "old")
	_, errb, code := runTool(t, dir, "--backup=simple", "-S", ".bak", "a", "b")
	if code != 0 {
		t.Fatalf("cp backup: code=%d err=%q", code, errb)
	}
	if read(t, dst) != "new" || read(t, filepath.Join(dir, "b.bak")) != "old" {
		t.Fatalf("backup/suffix did not preserve old destination")
	}

	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local)
	newer := time.Date(2021, 1, 1, 0, 0, 0, 0, time.Local)
	write(t, src, "older")
	write(t, dst, "newer")
	if err := os.Chtimes(src, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dst, newer, newer); err != nil {
		t.Fatal(err)
	}
	_, errb, code = runTool(t, dir, "-u", "a", "b")
	if code != 0 || errb != "" || read(t, dst) != "newer" {
		t.Fatalf("cp -u should skip newer destination: code=%d err=%q", code, errb)
	}

	write(t, src, "prompted")
	write(t, dst, "keep")
	_, errb, code = runTool(t, dir, "-i", "a", "b")
	if code != 0 || !strings.Contains(errb, "overwrite 'b'?") || read(t, dst) != "keep" {
		t.Fatalf("cp -i without yes should skip: code=%d err=%q", code, errb)
	}
}

func TestCpCompatibilityAliasesVisible(t *testing.T) {
	out, _, code := runTool(t, t.TempDir(), "--help")
	if code != 0 {
		t.Fatalf("cp --help code = %d", code)
	}
	for _, opt := range []string{"-b, --backup", "-g, --progress", "--copy-contents", "--preserve-default-attributes"} {
		if !strings.Contains(out, opt) {
			t.Fatalf("cp --help missing %q:\n%s", opt, out)
		}
	}

	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "new")
	write(t, filepath.Join(dir, "b"), "old")
	_, errb, code := runTool(t, dir, "-bg", "a", "b")
	if code != 0 || errb != "" {
		t.Fatalf("cp -bg: code=%d err=%q", code, errb)
	}
	if read(t, filepath.Join(dir, "b")) != "new" || read(t, filepath.Join(dir, "b~")) != "old" {
		t.Fatalf("cp -bg did not create simple backup")
	}

	write(t, filepath.Join(dir, "c"), "compat")
	_, errb, code = runTool(t, dir, "--copy-contents", "--preserve-default-attributes", "c", "d")
	if code != 0 || errb != "" || read(t, filepath.Join(dir, "d")) != "compat" {
		t.Fatalf("cp compatibility flags: code=%d err=%q", code, errb)
	}
}

func TestCpClusteredPreserveArchiveUpdateAliases(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
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
	_, errb, code := runTool(t, dir, "-pu", "src", "dst")
	if code != 0 || errb != "" || read(t, dst) != "newer" {
		t.Fatalf("cp -pu should preserve parser behavior and skip newer destination: code=%d err=%q", code, errb)
	}

	write(t, filepath.Join(dir, "tree", "f"), "x")
	_, errb, code = runTool(t, dir, "-au", "tree", "copy")
	if code != 0 {
		t.Fatalf("cp -au should parse as archive+update: code=%d err=%q", code, errb)
	}
	if read(t, filepath.Join(dir, "copy", "f")) != "x" {
		t.Fatalf("cp -au did not copy recursively")
	}
}

func TestCpVerbose(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "x")
	out, _, code := runTool(t, dir, "-v", "a", "b")
	if code != 0 || out != "'a' -> 'b'\n" {
		t.Errorf("cp -v: code=%d out=%q", code, out)
	}
}

func TestCpPreserve(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a")
	write(t, src, "x")
	when := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := os.Chtimes(src, when, when); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(src, 0o640); err != nil {
			t.Fatal(err)
		}
	}
	_, errb, code := runTool(t, dir, "-p", "a", "b")
	if code != 0 {
		t.Fatalf("cp -p: code=%d err=%q", code, errb)
	}
	fi, err := os.Stat(filepath.Join(dir, "b"))
	if err != nil {
		t.Fatal(err)
	}
	if !fi.ModTime().Equal(when) {
		t.Errorf("mtime = %v, want %v", fi.ModTime(), when)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o640 {
		t.Errorf("mode = %o, want 640", fi.Mode().Perm())
	}
}

func TestCpArchiveParentsAttributesAndNoPreserve(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "src", "sub", "file")
	write(t, src, "new-data")
	when := time.Date(2022, 2, 3, 4, 5, 6, 0, time.UTC)
	if err := os.Chmod(src, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(src, when, when); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "out"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-a", "--parents", "src/sub/file", "out")
	if code != 0 || errb != "" {
		t.Fatalf("cp -a --parents: code=%d err=%q", code, errb)
	}
	dst := filepath.Join(dir, "out", "src", "sub", "file")
	if got := read(t, dst); got != "new-data" {
		t.Fatalf("content = %q", got)
	}
	fi, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o640 || !fi.ModTime().Equal(when) {
		t.Fatalf("archive did not preserve mode/time: mode=%o time=%v", fi.Mode().Perm(), fi.ModTime())
	}

	write(t, filepath.Join(dir, "attrs-dst"), "keep")
	_, errb, code = runTool(t, dir, "--attributes-only", "--preserve=mode,timestamps", "src/sub/file", "attrs-dst")
	if code != 0 || errb != "" {
		t.Fatalf("cp --attributes-only: code=%d err=%q", code, errb)
	}
	if got := read(t, filepath.Join(dir, "attrs-dst")); got != "keep" {
		t.Fatalf("--attributes-only changed data: %q", got)
	}
	fi, err = os.Stat(filepath.Join(dir, "attrs-dst"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o640 || !fi.ModTime().Equal(when) {
		t.Fatalf("--attributes-only did not preserve selected attrs: mode=%o time=%v", fi.Mode().Perm(), fi.ModTime())
	}

	_, errb, code = runTool(t, dir, "-p", "--no-preserve=timestamps", "src/sub/file", "no-time")
	if code != 0 || errb != "" {
		t.Fatalf("cp --no-preserve: code=%d err=%q", code, errb)
	}
	fi, err = os.Stat(filepath.Join(dir, "no-time"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o640 {
		t.Fatalf("--no-preserve=timestamps should still preserve mode: %o", fi.Mode().Perm())
	}
	if fi.ModTime().Equal(when) {
		t.Fatalf("--no-preserve=timestamps still preserved source timestamp")
	}
}

func TestCpDebugRemoveDestinationParsingAndZ(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "new")
	write(t, filepath.Join(dir, "linked"), "old")
	if err := os.Link(filepath.Join(dir, "linked"), filepath.Join(dir, "dst")); err != nil {
		t.Skipf("hard links unsupported here: %v", err)
	}
	out, errb, code := runTool(t, dir, "--debug", "--remove-destination", "--reflink=auto", "--sparse=always", "-Z", "src", "dst")
	if code != 0 || out != "" {
		t.Fatalf("cp compatibility flags: code=%d out=%q err=%q", code, out, errb)
	}
	if !strings.Contains(errb, "cp: debug: copied 'src' -> 'dst'") {
		t.Fatalf("missing debug diagnostic: %q", errb)
	}
	if read(t, filepath.Join(dir, "dst")) != "new" {
		t.Fatal("destination not copied")
	}
	if read(t, filepath.Join(dir, "linked")) != "old" {
		t.Fatal("--remove-destination wrote through hard link")
	}
}

func TestCpStripTrailingSlashesAndH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privilege on windows")
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, "real", "file"), "x")
	if err := os.Symlink("real", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-R", "-H", "--strip-trailing-slashes", "link/", "dst/")
	if code != 0 {
		t.Fatalf("cp -H --strip-trailing-slashes: code=%d err=%q", code, errb)
	}
	if got := read(t, filepath.Join(dir, "dst", "file")); got != "x" {
		t.Fatalf("did not follow command-line symlink to directory: %q", got)
	}
}

func TestCpForceUnwritableDest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("POSIX write-permission semantics")
	}
	if os.Geteuid() == 0 {
		t.Skipf("root ignores permission bits")
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "new")
	dst := filepath.Join(dir, "b")
	write(t, dst, "old")
	if err := os.Chmod(dst, 0o444); err != nil {
		t.Fatal(err)
	}
	// without -f: cannot open destination
	_, errb, code := runTool(t, dir, "a", "b")
	if code != 1 || !strings.Contains(errb, "cannot create regular file 'b'") {
		t.Errorf("no -f: code=%d err=%q", code, errb)
	}
	// with -f: remove and retry
	_, errb, code = runTool(t, dir, "-f", "a", "b")
	if code != 0 {
		t.Fatalf("cp -f: code=%d err=%q", code, errb)
	}
	if read(t, dst) != "new" {
		t.Error("-f did not replace unwritable destination")
	}
}

func TestCpSameFile(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "x")
	_, errb, code := runTool(t, dir, "a", "a")
	if code != 1 || !strings.Contains(errb, "'a' and 'a' are the same file") {
		t.Errorf("code=%d err=%q", code, errb)
	}
}

func TestCpIntoItself(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "d", "f"), "x")
	_, errb, code := runTool(t, dir, "-r", "d", filepath.Join("d", "x"))
	if code != 1 || !strings.Contains(errb, "into itself") {
		t.Errorf("code=%d err=%q", code, errb)
	}
}

func TestCpSymlinkInTree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("symlink creation needs privilege on windows")
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src", "f1"), "one")
	if err := os.Symlink("f1", filepath.Join(dir, "src", "link")); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-r", "src", "dst")
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	target, err := os.Readlink(filepath.Join(dir, "dst", "link"))
	if err != nil || target != "f1" {
		t.Errorf("symlink not preserved: target=%q err=%v", target, err)
	}
}

func TestCpMissingSource(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "nope", "b")
	if code != 1 || !strings.Contains(errb, "cannot stat 'nope'") {
		t.Errorf("code=%d err=%q", code, errb)
	}
}

func TestCpUsageErrors(t *testing.T) {
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

func TestCpHelpAndVersion(t *testing.T) {
	dir := t.TempDir()
	out, _, code := runTool(t, dir, "--help")
	if code != 0 || !strings.Contains(out, "Usage: cp") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, dir, "--version")
	if code != 0 || !strings.Contains(out, "cp") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}
