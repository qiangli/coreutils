package installcmd

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
	if code != 1 || !strings.Contains(errb, "cannot create regular file 'dest'") {
		t.Fatalf("code=%d err=%q", code, errb)
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
