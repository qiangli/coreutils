package rmcmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// runTool is the canonical test harness shape for cmds packages:
// output is captured after Run returns.
func runTool(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	return runToolIn(t, dir, "", args...)
}

func runToolIn(t *testing.T, dir, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
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

func TestRmFile(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "x")
	out, errb, code := runTool(t, dir, "a")
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("rm a: code=%d out=%q err=%q", code, out, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "a")); !os.IsNotExist(err) {
		t.Error("file still exists")
	}
}

func TestRmVerbose(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "x")
	out, _, code := runTool(t, dir, "-v", "a")
	if code != 0 || out != "removed 'a'\n" {
		t.Errorf("rm -v: code=%d out=%q", code, out)
	}
}

func TestRmMissing(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "nope")
	if code != 1 || !strings.Contains(errb, "cannot remove 'nope'") {
		t.Errorf("code=%d err=%q", code, errb)
	}
	// -f silences nonexistent operands
	out, errb, code := runTool(t, dir, "-f", "nope")
	if code != 0 || out != "" || errb != "" {
		t.Errorf("rm -f nope: code=%d out=%q err=%q", code, out, errb)
	}
}

func TestRmDirWithoutR(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "d")
	if code != 1 || !strings.Contains(errb, "cannot remove 'd': Is a directory") {
		t.Errorf("code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "d")); err != nil {
		t.Error("directory was removed without -r")
	}
}

func TestRmDirFlagRemovesEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-d", "d")
	if code != 0 {
		t.Fatalf("rm -d: code=%d err=%q", code, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "d")); !os.IsNotExist(err) {
		t.Error("empty directory still exists")
	}
}

func TestRmRecursive(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "d", "f"), "x")
	write(t, filepath.Join(dir, "d", "sub", "g"), "y")
	_, errb, code := runTool(t, dir, "-r", "d")
	if code != 0 {
		t.Fatalf("rm -r: code=%d err=%q", code, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "d")); !os.IsNotExist(err) {
		t.Error("tree still exists")
	}
}

func TestRmRecursiveVerbosePostOrder(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "d", "f"), "x")
	out, _, code := runTool(t, dir, "-rv", "d")
	want := "removed '" + filepath.Join("d", "f") + "'\nremoved directory 'd'\n"
	if code != 0 || out != want {
		t.Errorf("rm -rv: code=%d out=%q want %q", code, out, want)
	}
}

func TestRmCapitalRAlias(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "d", "f"), "x")
	_, errb, code := runTool(t, dir, "-R", "d")
	if code != 0 {
		t.Fatalf("rm -R: code=%d err=%q", code, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "d")); !os.IsNotExist(err) {
		t.Error("-R did not remove recursively")
	}
}

func TestRmInteractivePrompt(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "x")
	_, errb, code := runTool(t, dir, "-i", "a")
	if code != 0 || !strings.Contains(errb, "remove 'a'?") {
		t.Fatalf("rm -i no input: code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "a")); err != nil {
		t.Fatal("rm -i without yes removed the file")
	}
	_, errb, code = runToolIn(t, dir, "y\n", "-i", "a")
	if code != 0 || !strings.Contains(errb, "remove 'a'?") {
		t.Fatalf("rm -i yes: code=%d err=%q", code, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "a")); !os.IsNotExist(err) {
		t.Fatal("rm -i yes did not remove the file")
	}
}

func TestRmCompatibilityNoOps(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "x")
	_, errb, code := runTool(t, dir, "-go", "a")
	if code != 0 || errb != "" {
		t.Fatalf("compat flags: code=%d err=%q", code, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "a")); !os.IsNotExist(err) {
		t.Fatal("file still exists")
	}

	out, _, code := runTool(t, dir, "--help")
	if code != 0 || !strings.Contains(out, "-g, --progress") || !strings.Contains(out, "-o, --one-file-system") {
		t.Fatalf("help missing compatibility aliases: code=%d out=%q", code, out)
	}
}

func TestRmRootRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the test's `\\` is not an absolute root on Windows; rm's preserve-root guard fires on real roots like C:\\ (windows-port item)")
	}
	dir := t.TempDir()
	root := string(filepath.Separator)
	_, errb, code := runTool(t, dir, "-rf", root)
	if code != 1 || !strings.Contains(errb, "it is dangerous to operate recursively on") {
		t.Errorf("rm -rf /: code=%d err=%q", code, errb)
	}
}

func TestRmOperandErrors(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir)
	if code != 2 || !strings.Contains(errb, "missing operand") {
		t.Errorf("no args: code=%d err=%q", code, errb)
	}
	// GNU: rm -f with no operands exits 0
	out, errb, code := runTool(t, dir, "-f")
	if code != 0 || out != "" || errb != "" {
		t.Errorf("rm -f: code=%d out=%q err=%q", code, out, errb)
	}
	_, errb, code = runTool(t, dir, "--frobnicate", "x")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestRmContinuesPastErrors(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "b"), "x")
	_, errb, code := runTool(t, dir, "nope", "b")
	if code != 1 || !strings.Contains(errb, "cannot remove 'nope'") {
		t.Errorf("code=%d err=%q", code, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "b")); !os.IsNotExist(err) {
		t.Error("later operand not removed after earlier failure")
	}
}

func TestRmHelpAndVersion(t *testing.T) {
	dir := t.TempDir()
	out, _, code := runTool(t, dir, "--help")
	if code != 0 || !strings.Contains(out, "Usage: rm") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, dir, "--version")
	if code != 0 || !strings.Contains(out, "rm") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}

func TestRmPOSIXWriteProtectPrompt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "readonly")
	write(t, path, "x")
	os.Chmod(path, 0444)

	// Mock terminal check
	origIsTerminal := isTerminalFunc
	isTerminalFunc = func(r io.Reader) bool { return true }
	defer func() { isTerminalFunc = origIsTerminal }()

	// If user says no
	_, errb, code := runToolIn(t, dir, "n\n", "readonly")
	if code != 0 {
		t.Errorf("expected 0 exit code on 'n', got %d", code)
	}
	if !strings.Contains(errb, "remove 'readonly'?") {
		t.Errorf("missing prompt, errb=%q", errb)
	}
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		t.Error("file removed despite 'n' answer")
	}

	// If user says yes
	_, _, code = runToolIn(t, dir, "y\n", "readonly")
	if code != 0 {
		t.Errorf("expected 0 exit code on 'y', got %d", code)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Error("file not removed after 'y' answer")
	}
}

func TestRmRecursiveInteractivePrompts(t *testing.T) {
	dir := t.TempDir()
	d := filepath.Join(dir, "d")
	os.MkdirAll(d, 0755)
	write(t, filepath.Join(d, "f"), "x")

	// Answer 'y' to all prompts
	input := "y\ny\ny\n"
	_, errb, code := runToolIn(t, dir, input, "-ri", "d")
	if code != 0 {
		t.Errorf("expected 0 exit code, got %d", code)
	}

	// Should prompt for:
	// 1. descend into directory (or just "remove 'd'?" based on our generic prompt)
	// 2. remove file 'd/f'?
	// 3. remove directory 'd'?
	if strings.Count(errb, "remove '") != 3 {
		t.Errorf("expected 3 prompts, got %d in %q", strings.Count(errb, "remove '"), errb)
	}
	if !strings.Contains(errb, filepath.Join("d", "f")) {
		t.Errorf("did not prompt for file inside directory: %q", errb)
	}

	if _, err := os.Lstat(d); !os.IsNotExist(err) {
		t.Error("directory not removed after 'y' answers")
	}
}
