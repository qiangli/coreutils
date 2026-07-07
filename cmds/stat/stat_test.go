package statcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// runToolAt is the canonical test harness shape for cmds packages,
// with an explicit working directory.
func runToolAt(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
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

func runTool(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	return runToolAt(t, t.TempDir(), args...)
}

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFormatNameSize(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", "hello")
	out, _, code := runToolAt(t, dir, "-c", "%n %s", "f")
	if code != 0 || out != "f 5\n" {
		t.Errorf("stat -c '%%n %%s' = (%q, %d), want (\"f 5\\n\", 0)", out, code)
	}
}

func TestFormatFileType(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", "hello")
	write(t, dir, "empty", "")
	cases := []struct{ op, want string }{
		{"f", "regular file\n"},
		{"empty", "regular empty file\n"},
		{".", "directory\n"},
	}
	for _, c := range cases {
		out, _, code := runToolAt(t, dir, "-c", "%F", c.op)
		if code != 0 || out != c.want {
			t.Errorf("stat -c %%F %s = (%q, %d), want (%q, 0)", c.op, out, code, c.want)
		}
	}
}

func TestFormatSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("symlink creation needs privileges on windows")
	}
	dir := t.TempDir()
	write(t, dir, "target", "x")
	if err := os.Symlink("target", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	// GNU stat does not follow symlinks by default.
	out, _, code := runToolAt(t, dir, "-c", "%F", "link")
	if code != 0 || out != "symbolic link\n" {
		t.Errorf("stat -c %%F link = (%q, %d)", out, code)
	}
	out, _, _ = runToolAt(t, dir, "link")
	if !strings.Contains(out, "link -> target") {
		t.Errorf("stat link default block = %q, want 'link -> target'", out)
	}
}

func TestFormatPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("POSIX permission bits are not faithful on windows")
	}
	dir := t.TempDir()
	p := write(t, dir, "f", "x")
	if err := os.Chmod(p, 0o640); err != nil {
		t.Fatal(err)
	}
	out, _, code := runToolAt(t, dir, "-c", "%a", "f")
	if code != 0 || out != "640\n" {
		t.Errorf("stat -c %%a = (%q, %d), want (\"640\\n\", 0)", out, code)
	}
}

func TestFormatInodeLinks(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", "x")
	out, _, code := runToolAt(t, dir, "-c", "%i %h", "f")
	if code != 0 {
		t.Fatalf("stat -c '%%i %%h' code = %d", code)
	}
	if runtime.GOOS == "windows" {
		if out != "0 1\n" {
			t.Errorf("windows fallback = %q, want \"0 1\\n\"", out)
		}
		return
	}
	if !regexp.MustCompile(`^[1-9]\d* 1\n$`).MatchString(out) {
		t.Errorf("stat -c '%%i %%h' = %q, want nonzero inode and 1 link", out)
	}
}

func TestFormatUidGidTimes(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", "x")
	out, _, code := runToolAt(t, dir, "-c", "%u %g", "f")
	if code != 0 || !regexp.MustCompile(`^\d+ \d+\n$`).MatchString(out) {
		t.Errorf("stat -c '%%u %%g' = (%q, %d)", out, code)
	}
	out, _, code = runToolAt(t, dir, "-c", "%y", "f")
	if code != 0 || !regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{9} [+-]\d{4}\n$`).MatchString(out) {
		t.Errorf("stat -c %%y = (%q, %d), want full-precision timestamp", out, code)
	}
}

func TestFormatLiteralsAndPercent(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", "x")
	out, _, code := runToolAt(t, dir, "-c", "100%% %n", "f")
	if code != 0 || out != "100% f\n" {
		t.Errorf("stat -c '100%%%% %%n' = (%q, %d)", out, code)
	}
}

func TestDefaultBlock(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", "hello")
	out, _, code := runToolAt(t, dir, "f")
	if code != 0 {
		t.Fatalf("stat f code = %d", code)
	}
	for _, want := range []string{"  File: f\n", "  Size: 5", "IO Block:", "regular file", "Inode:", "Links:", "Access: (", "Uid: (", "Gid: (", "Access: ", "Modify: ", "Change: ", " Birth: "} {
		if !strings.Contains(out, want) {
			t.Errorf("default block missing %q in %q", want, out)
		}
	}
}

func TestMultipleOperands(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a", "1")
	write(t, dir, "b", "22")
	out, _, code := runToolAt(t, dir, "-c", "%n %s", "a", "b")
	if code != 0 || out != "a 1\nb 2\n" {
		t.Errorf("stat two operands = (%q, %d)", out, code)
	}
}

func TestUnsupportedDirective(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", "x")
	out, errb, code := runToolAt(t, dir, "-c", "%q", "f")
	if code != 2 || !strings.Contains(errb, "format directive '%q'") || !strings.Contains(errb, "not supported") {
		t.Errorf("unsupported directive: code=%d err=%q", code, errb)
	}
	if out != "" {
		t.Errorf("unsupported directive wrote output: %q", out)
	}
}

func TestErrors(t *testing.T) {
	_, errb, code := runTool(t)
	if code != 2 || !strings.Contains(errb, "missing operand") {
		t.Errorf("no args: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "nope")
	if code != 1 || !strings.Contains(errb, "cannot stat 'nope'") {
		t.Errorf("missing file: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "--frobnicate", "x")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, "--help")
	if code != 0 || !strings.Contains(out, "Usage: stat") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "--version")
	if code != 0 || !strings.Contains(out, "stat") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}

func TestDereference(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("symlinks not available on windows")
	}
	dir := t.TempDir()
	write(t, dir, "target", "x")
	if err := os.Symlink("target", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	// Without -L: reports symlink
	out, _, code := runToolAt(t, dir, "-c", "%F", "link")
	if code != 0 || out != "symbolic link\n" {
		t.Errorf("without -L: got=%q", out)
	}
	// With -L: reports regular file
	out, _, code = runToolAt(t, dir, "-L", "-c", "%F", "link")
	if code != 0 || out != "regular file\n" {
		t.Errorf("with -L: got=%q", out)
	}
}

func TestPrintf(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", "hello")
	// --printf should NOT append a newline
	out, _, code := runToolAt(t, dir, "--printf", "%n", "f")
	if code != 0 || out != "f" {
		t.Errorf("--printf: got=%q code=%d", out, code)
	}
}

func TestTerse(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", "hello")
	out, _, code := runToolAt(t, dir, "-t", "f")
	if code != 0 {
		t.Fatalf("-t: code=%d err=%q", code, out)
	}
	if !strings.HasPrefix(out, "f ") {
		t.Errorf("-t: got=%q, want 'f ' prefix", out)
	}
}

func TestFileSystem(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("--file-system not supported on windows")
	}
	dir := t.TempDir()
	out, _, code := runToolAt(t, dir, "-f", ".")
	if code != 0 {
		t.Fatalf("-f: code=%d err=%q", code, out)
	}
	if !strings.Contains(out, "Block size") {
		t.Errorf("-f default: got=%q", out)
	}
	// Terse mode for -f
	out2, _, code := runToolAt(t, dir, "-f", "-t", ".")
	if code != 0 {
		t.Fatalf("-f -t: code=%d err=%q", code, out2)
	}
	if !strings.Contains(out2, " ") {
		t.Errorf("-f -t: got=%q", out2)
	}
}
