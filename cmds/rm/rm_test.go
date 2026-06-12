package rmcmd

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

func TestRmInteractiveRefused(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "x")
	for _, args := range [][]string{
		{"-i", "a"},
		{"-I", "a"},
		{"-ri", "a"},
		{"--interactive", "a"},
		{"--interactive=always", "a"},
	} {
		_, errb, code := runTool(t, dir, args...)
		if code != 2 || !strings.Contains(errb, "not supported") {
			t.Errorf("rm %v: code=%d err=%q", args, code, errb)
		}
		if _, err := os.Stat(filepath.Join(dir, "a")); err != nil {
			t.Fatalf("rm %v removed the file despite refusal", args)
		}
	}
}

func TestRmRootRefused(t *testing.T) {
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
