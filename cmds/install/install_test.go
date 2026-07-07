package installcmd

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

func perm(t *testing.T, path string) os.FileMode {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}

func TestInstallFileDefaultExecutableMode(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "hello")
	out, errb, code := runTool(t, dir, "src", "dst")
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("install src dst: code=%d out=%q err=%q", code, out, errb)
	}
	if got := read(t, filepath.Join(dir, "dst")); got != "hello" {
		t.Fatalf("content = %q", got)
	}
	if runtime.GOOS != "windows" && perm(t, filepath.Join(dir, "dst")) != 0o755 {
		t.Fatalf("mode = %#o, want 0755", perm(t, filepath.Join(dir, "dst")))
	}
}

func TestInstallModeAndVerbose(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits")
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "x")
	out, errb, code := runTool(t, dir, "-v", "-m", "640", "src", "dst")
	if code != 0 || errb != "" {
		t.Fatalf("install -m: code=%d out=%q err=%q", code, out, errb)
	}
	if out != "'src' -> 'dst'\n" {
		t.Fatalf("verbose = %q", out)
	}
	if got := perm(t, filepath.Join(dir, "dst")); got != 0o640 {
		t.Fatalf("mode = %#o, want 0640", got)
	}
}

func TestInstallIntoTargetDirectory(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "1")
	write(t, filepath.Join(dir, "b"), "2")
	if err := os.Mkdir(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-t", "bin", "a", "b")
	if code != 0 {
		t.Fatalf("install -t: code=%d err=%q", code, errb)
	}
	if read(t, filepath.Join(dir, "bin", "a")) != "1" || read(t, filepath.Join(dir, "bin", "b")) != "2" {
		t.Fatal("sources were not installed into target directory")
	}
}

func TestInstallDParentsAndMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits")
	}
	dir := t.TempDir()
	out, errb, code := runTool(t, dir, "-d", "-v", "-m", "700", "a/b/c")
	if code != 0 || errb != "" {
		t.Fatalf("install -d: code=%d out=%q err=%q", code, out, errb)
	}
	if out != "install: creating directory 'a/b/c'\n" {
		t.Fatalf("verbose = %q", out)
	}
	if got := perm(t, filepath.Join(dir, "a", "b", "c")); got != 0o700 {
		t.Fatalf("mode = %#o, want 0700", got)
	}
}

func TestInstallDCreatesParentForDestination(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "x")
	_, errb, code := runTool(t, dir, "-D", "src", "usr/local/bin/tool")
	if code != 0 {
		t.Fatalf("install -D: code=%d err=%q", code, errb)
	}
	if got := read(t, filepath.Join(dir, "usr", "local", "bin", "tool")); got != "x" {
		t.Fatalf("content = %q", got)
	}
}

func TestInstallMultipleRequiresDirectory(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "1")
	write(t, filepath.Join(dir, "b"), "2")
	_, errb, code := runTool(t, dir, "a", "b", "missing")
	if code != 1 || !strings.Contains(errb, "target 'missing' is not a directory") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
}

func TestInstallTargetDirectoryMustExistWithoutD(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "x")
	_, errb, code := runTool(t, dir, "-t", "missing", "src")
	if code != 1 || !strings.Contains(errb, "cannot create regular file 'missing/src'") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "missing")); !os.IsNotExist(statErr) {
		t.Fatalf("target directory was created without -D: %v", statErr)
	}
}

func TestInstallSameFileRefused(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "x")
	_, errb, code := runTool(t, dir, "src", "src")
	if code != 1 || !strings.Contains(errb, "'src' and 'src' are the same file") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if got := read(t, filepath.Join(dir, "src")); got != "x" {
		t.Fatalf("source content changed: %q", got)
	}
}

func TestInstallTDoesNotTreatExistingDirAsDirectory(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "x")
	if err := os.Mkdir(filepath.Join(dir, "dest"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-T", "src", "dest")
	if code != 1 || !strings.Contains(errb, "cannot overwrite directory 'dest' with non-directory") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if fi, err := os.Stat(filepath.Join(dir, "dest")); err != nil || !fi.IsDir() {
		t.Fatalf("existing directory was removed: %v", err)
	}
}

func TestInstallOwnershipFlagsUnsupported(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "x")
	_, errb, code := runTool(t, dir, "-o", "root", "src", "dst")
	if code != 2 || !strings.Contains(errb, "-o/--owner and -g/--group") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
}

// GNU install removes an existing destination before copying, so a
// read-only destination is replaced rather than failing on open.
func TestInstallReplacesReadOnlyDest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("read-only files block os.Remove on windows")
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "new")
	write(t, filepath.Join(dir, "dst"), "old")
	if err := os.Chmod(filepath.Join(dir, "dst"), 0o444); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "src", "dst")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if got := read(t, filepath.Join(dir, "dst")); got != "new" {
		t.Fatalf("dst content = %q, want new", got)
	}
}

// Removing the destination first breaks hard links instead of writing
// through them.
func TestInstallBreaksHardLinkedDest(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "new")
	write(t, filepath.Join(dir, "other"), "old")
	if err := os.Link(filepath.Join(dir, "other"), filepath.Join(dir, "dst")); err != nil {
		t.Skipf("hard links unsupported here: %v", err)
	}
	_, errb, code := runTool(t, dir, "src", "dst")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if got := read(t, filepath.Join(dir, "dst")); got != "new" {
		t.Fatalf("dst content = %q, want new", got)
	}
	if got := read(t, filepath.Join(dir, "other")); got != "old" {
		t.Fatalf("hard link was written through: other = %q, want old", got)
	}
}

func TestInstallUsage(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "src")
	if code != 2 || !strings.Contains(errb, "missing destination file operand after 'src'") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	out, _, code := runTool(t, dir, "--help")
	if code != 0 || !strings.Contains(out, "Usage: install") || !strings.Contains(out, "-D") {
		t.Fatalf("help code=%d out=%q", code, out)
	}
}

func TestInstallBackup(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "new")
	write(t, filepath.Join(dir, "dst"), "old")
	out, errb, code := runTool(t, dir, "-b", "src", "dst")
	if code != 0 || errb != "" {
		t.Fatalf("install -b: code=%d err=%q", code, errb)
	}
	_ = out
	if read(t, filepath.Join(dir, "dst")) != "new" {
		t.Fatal("dst was not updated")
	}
	bakContent, err := os.ReadFile(filepath.Join(dir, "dst~"))
	if err != nil || string(bakContent) != "old" {
		t.Fatalf("backup not created or has wrong content: err=%v", err)
	}
}

func TestInstallBackupWithSuffix(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "new")
	write(t, filepath.Join(dir, "dst"), "old")
	_, errb, code := runTool(t, dir, "-b", "-S", ".bak", "src", "dst")
	if code != 0 || errb != "" {
		t.Fatalf("install -b -S: code=%d err=%q", code, errb)
	}
	bakContent, err := os.ReadFile(filepath.Join(dir, "dst.bak"))
	if err != nil || string(bakContent) != "old" {
		t.Fatalf("backup with suffix not created: err=%v", err)
	}
}

func TestInstallCompareSkipsIdentical(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "same")
	write(t, filepath.Join(dir, "dst"), "same")
	out, errb, code := runTool(t, dir, "-C", "-v", "src", "dst")
	if code != 0 || errb != "" {
		t.Fatalf("install -C: code=%d err=%q", code, errb)
	}
	if !strings.Contains(out, "unchanged, skipped") {
		t.Errorf("expected skip message for identical files, got: %q", out)
	}
}

func TestInstallCompareCopiesDifferent(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "new")
	write(t, filepath.Join(dir, "dst"), "different")
	_, errb, code := runTool(t, dir, "-C", "src", "dst")
	if code != 0 || errb != "" {
		t.Fatalf("install -C: code=%d err=%q", code, errb)
	}
	if read(t, filepath.Join(dir, "dst")) != "new" {
		t.Fatal("different files not copied with -C")
	}
}

func TestInstallPreserveTimestamps(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	write(t, src, "hi")
	past := "1999-01-01T00:00:00Z"
	if err := os.Chtimes(src, parseTime(t, past), parseTime(t, past)); err != nil {
		t.Skipf("Chtimes failed (may need newer Go): %v", err)
	}
	dst := filepath.Join(dir, "dst")
	_, errb, code := runTool(t, dir, "-p", "src", dst)
	if code != 0 || errb != "" {
		t.Fatalf("install -p: code=%d err=%q", code, errb)
	}
	sfi, _ := os.Stat(src)
	dfi, _ := os.Stat(dst)
	if sfi != nil && dfi != nil && !sfi.ModTime().Equal(dfi.ModTime()) {
		t.Errorf("source=%v dest=%v, timestamps should match", sfi.ModTime(), dfi.ModTime())
	}
}

func TestInstallStripFlag(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "hi")
	out, errb, code := runTool(t, dir, "-s", "src", "dst")
	if code != 0 || errb != "" {
		t.Fatalf("install -s: code=%d err=%q", code, errb)
	}
	if out != "" {
		t.Logf("install -s output: %q", out)
	}
	if read(t, filepath.Join(dir, "dst")) != "hi" {
		t.Fatal("content corrupted after -s")
	}
}

func TestInstallContextFlag(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "hi")
	out, errb, code := runTool(t, dir, "-Z", "unconfined_u:object_r:default_t:s0", "src", "dst")
	if code != 0 || errb != "" {
		t.Fatalf("install -Z: code=%d err=%q", code, errb)
	}
	_ = out
}

func parseTime(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return tm
}
