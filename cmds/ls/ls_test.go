package lscmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

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

func mkTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "b.txt", "bb")
	write(t, dir, "a.txt", "a")
	write(t, dir, ".hidden", "h")
	return dir
}

func TestDefaultSortAndOnePerLine(t *testing.T) {
	dir := mkTree(t)
	want := "a.txt\nb.txt\n"
	out, _, code := runToolAt(t, dir)
	if code != 0 || out != want {
		t.Errorf("ls = (%q, %d), want (%q, 0)", out, code, want)
	}
	// -1 is the default; output identical.
	out1, _, code1 := runToolAt(t, dir, "-1")
	if code1 != 0 || out1 != want {
		t.Errorf("ls -1 = (%q, %d), want (%q, 0)", out1, code1, want)
	}
}

func TestAllAndAlmostAll(t *testing.T) {
	dir := mkTree(t)
	out, _, code := runToolAt(t, dir, "-a")
	if code != 0 || out != ".\n..\n.hidden\na.txt\nb.txt\n" {
		t.Errorf("ls -a = (%q, %d)", out, code)
	}
	out, _, code = runToolAt(t, dir, "-A")
	if code != 0 || out != ".hidden\na.txt\nb.txt\n" {
		t.Errorf("ls -A = (%q, %d)", out, code)
	}
	// GNU last-one-wins: -A after -a means almost-all.
	out, _, _ = runToolAt(t, dir, "-a", "-A")
	if out != ".hidden\na.txt\nb.txt\n" {
		t.Errorf("ls -a -A = %q, want almost-all behavior", out)
	}
	out, _, _ = runToolAt(t, dir, "-A", "-a")
	if !strings.HasPrefix(out, ".\n..\n") {
		t.Errorf("ls -A -a = %q, want all behavior", out)
	}
}

func TestReverse(t *testing.T) {
	dir := mkTree(t)
	out, _, code := runToolAt(t, dir, "-r")
	if code != 0 || out != "b.txt\na.txt\n" {
		t.Errorf("ls -r = (%q, %d)", out, code)
	}
}

func TestSortTime(t *testing.T) {
	dir := t.TempDir()
	old := write(t, dir, "old", "x")
	write(t, dir, "new", "y")
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	out, _, code := runToolAt(t, dir, "-t")
	if code != 0 || out != "new\nold\n" {
		t.Errorf("ls -t = (%q, %d), want newest first", out, code)
	}
	out, _, _ = runToolAt(t, dir, "-tr")
	if out != "old\nnew\n" {
		t.Errorf("ls -tr = %q", out)
	}
}

func TestSortSize(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "small", "1")
	write(t, dir, "big", "123456")
	write(t, dir, "mid", "123")
	out, _, code := runToolAt(t, dir, "-S")
	if code != 0 || out != "big\nmid\nsmall\n" {
		t.Errorf("ls -S = (%q, %d), want largest first", out, code)
	}
}

func TestDirectoryFlag(t *testing.T) {
	dir := mkTree(t)
	out, _, code := runToolAt(t, dir, "-d", ".")
	if code != 0 || out != ".\n" {
		t.Errorf("ls -d . = (%q, %d), want \".\\n\"", out, code)
	}
}

func TestMixedOperandsHeaders(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", "x")
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, sub, "inner", "y")
	out, _, code := runToolAt(t, dir, "f", "sub")
	want := "f\n\nsub:\ninner\n"
	if code != 0 || out != want {
		t.Errorf("ls f sub = (%q, %d), want (%q, 0)", out, code, want)
	}
	// Single dir operand, no -R: no header.
	out, _, _ = runToolAt(t, dir, "sub")
	if out != "inner\n" {
		t.Errorf("ls sub = %q, want \"inner\\n\"", out)
	}
}

func TestRecursive(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(filepath.Join(sub, "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "top", "x")
	write(t, sub, "mid", "y")
	out, _, code := runToolAt(t, dir, "-R", ".")
	want := ".:\nsub\ntop\n\n./sub:\ndeep\nmid\n\n./sub/deep:\n"
	if code != 0 || out != want {
		t.Errorf("ls -R . = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestLongFormat(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", "hello")
	out, _, code := runToolAt(t, dir, "-l")
	if code != 0 {
		t.Fatalf("ls -l code = %d", code)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("ls -l lines = %q", lines)
	}
	if !regexp.MustCompile(`^total \d+$`).MatchString(lines[0]) {
		t.Errorf("ls -l total line = %q", lines[0])
	}
	// mode nlink owner group size month day time name
	if !regexp.MustCompile(`^[-dl][rwxsStT-]{9} +\d+ .* 5 [A-Z][a-z]{2} [ \d]\d [ \d\d:]{5} f$`).MatchString(lines[1]) {
		t.Errorf("ls -l entry line = %q", lines[1])
	}
	if runtime.GOOS != "windows" && !strings.HasPrefix(lines[1], "-rw-") {
		t.Errorf("ls -l mode = %q, want -rw- prefix", lines[1])
	}
}

func TestLongHuman(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "two-k", strings.Repeat("x", 2048))
	out, _, code := runToolAt(t, dir, "-lh")
	if code != 0 || !strings.Contains(out, " 2.0K ") {
		t.Errorf("ls -lh = (%q, %d), want 2.0K size", out, code)
	}
}

func TestLongSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("symlink creation needs privileges on windows")
	}
	dir := t.TempDir()
	write(t, dir, "target", "x")
	if err := os.Symlink("target", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	out, _, code := runToolAt(t, dir, "-l")
	if code != 0 || !strings.Contains(out, "link -> target") {
		t.Errorf("ls -l = (%q, %d), want link -> target", out, code)
	}
	if !strings.Contains(out, "\nl") && !strings.HasPrefix(out, "l") {
		// the symlink line must carry the 'l' type char
		if !regexp.MustCompile(`(?m)^l`).MatchString(out) {
			t.Errorf("ls -l = %q, want a line starting with 'l'", out)
		}
	}
}

func TestInode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("inode numbers are a documented 0 fallback on windows")
	}
	dir := t.TempDir()
	write(t, dir, "f", "x")
	out, _, code := runToolAt(t, dir, "-i")
	if code != 0 {
		t.Fatalf("ls -i code = %d", code)
	}
	fields := strings.Fields(out)
	if len(fields) != 2 || fields[1] != "f" {
		t.Fatalf("ls -i = %q", out)
	}
	if n, err := strconv.ParseUint(fields[0], 10, 64); err != nil || n == 0 {
		t.Errorf("ls -i inode = %q, want nonzero number", fields[0])
	}
}

func TestNonexistentOperand(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "real", "x")
	out, errb, code := runToolAt(t, dir, "nope", "real")
	if code != 2 {
		t.Errorf("code = %d, want 2", code)
	}
	if !strings.Contains(errb, "cannot access 'nope'") {
		t.Errorf("stderr = %q", errb)
	}
	if !strings.Contains(out, "real") {
		t.Errorf("stdout = %q, want surviving operand listed", out)
	}
}

func TestUnknownFlag(t *testing.T) {
	_, errb, code := runTool(t, "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, "--help")
	if code != 0 || !strings.Contains(out, "Usage: ls") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "--version")
	if code != 0 || !strings.Contains(out, "ls") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}

func TestHumanSize(t *testing.T) {
	cases := []struct {
		n    uint64
		want string
	}{
		{0, "0"},
		{1023, "1023"},
		{1024, "1.0K"},
		{1025, "1.1K"},
		{2048, "2.0K"},
		{10 * 1024, "10K"},
		{1047552, "1023K"}, // 1023 * 1024
		{1048063, "1.0M"},  // ceils past 1023K into the next unit
		{1048576, "1.0M"},  // 1024K
		{1536 * 1024, "1.5M"},
	}
	for _, c := range cases {
		if got := humanSize(c.n); got != c.want {
			t.Errorf("humanSize(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}
