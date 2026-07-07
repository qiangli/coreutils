package teecmd

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

func runToolDir(t *testing.T, dir, stdin string, args ...string) (stdout, stderr string, code int) {
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

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestTeeStdoutOnly(t *testing.T) {
	out, errb, code := runToolDir(t, t.TempDir(), "hello\nworld\n")
	if out != "hello\nworld\n" || errb != "" || code != 0 {
		t.Errorf("no files: out=%q err=%q code=%d", out, errb, code)
	}
}

func TestTeeWritesFiles(t *testing.T) {
	dir := t.TempDir()
	out, _, code := runToolDir(t, dir, "data\n", "f1", "f2")
	if out != "data\n" || code != 0 {
		t.Errorf("out=%q code=%d", out, code)
	}
	// relative operands resolve against rc.Dir
	for _, f := range []string{"f1", "f2"} {
		if got := readFile(t, filepath.Join(dir, f)); got != "data\n" {
			t.Errorf("%s = %q, want %q", f, got, "data\n")
		}
	}
}

func TestTeeTruncatesByDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	if err := os.WriteFile(path, []byte("old contents that are long\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, code := runToolDir(t, dir, "new\n", "f")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if got := readFile(t, path); got != "new\n" {
		t.Errorf("file = %q, want %q", got, "new\n")
	}
}

func TestTeeAppend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	if err := os.WriteFile(path, []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runToolDir(t, dir, "second\n", "-a", "f")
	if out != "second\n" || code != 0 {
		t.Errorf("out=%q code=%d", out, code)
	}
	if got := readFile(t, path); got != "first\nsecond\n" {
		t.Errorf("file = %q, want %q", got, "first\nsecond\n")
	}
}

func TestTeeIgnoreInterruptsAccepted(t *testing.T) {
	// -i is accepted (and a documented no-op in this in-process userland)
	dir := t.TempDir()
	out, errb, code := runToolDir(t, dir, "x\n", "-i", "f")
	if out != "x\n" || errb != "" || code != 0 {
		t.Errorf("-i: out=%q err=%q code=%d", out, errb, code)
	}
	if got := readFile(t, filepath.Join(dir, "f")); got != "x\n" {
		t.Errorf("file = %q", got)
	}
}

func TestTeeOutputErrorOptionsAccepted(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{
		{"-p", "f"},
		{"--output-error", "f"},
		{"--output-error=warn", "f"},
		{"--output-error=exit-nopipe", "f"},
	} {
		out, errb, code := runToolDir(t, dir, "x\n", args...)
		if code != 0 || errb != "" || out != "x\n" {
			t.Errorf("tee %v: out=%q err=%q code=%d", args, out, errb, code)
		}
	}
	_, errb, code := runToolDir(t, dir, "", "--output-error=bad")
	if code != 2 || !strings.Contains(errb, "invalid argument") {
		t.Errorf("bad --output-error: err=%q code=%d", errb, code)
	}
}

func TestTeeOpenErrorContinues(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("relies on opening a path under a missing directory failing the same way")
	}
	dir := t.TempDir()
	// unopenable path (missing parent dir): diagnose, keep copying to the rest
	out, errb, code := runToolDir(t, dir, "x\n", "missing/sub/f", "ok")
	if code != 1 || out != "x\n" || !strings.Contains(errb, "tee: missing/sub/f:") {
		t.Errorf("open error: out=%q err=%q code=%d", out, errb, code)
	}
	if got := readFile(t, filepath.Join(dir, "ok")); got != "x\n" {
		t.Errorf("ok file = %q", got)
	}
}

func TestTeeDashIsLiteralFileName(t *testing.T) {
	// GNU tee does not special-case "-": it names a file
	dir := t.TempDir()
	out, _, code := runToolDir(t, dir, "x\n", "-")
	if out != "x\n" || code != 0 {
		t.Errorf("dash: out=%q code=%d", out, code)
	}
	if got := readFile(t, filepath.Join(dir, "-")); got != "x\n" {
		t.Errorf("dash file = %q", got)
	}
}

func TestTeeUnknownFlag(t *testing.T) {
	_, errb, code := runToolDir(t, t.TempDir(), "", "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestTeeHelpAndVersion(t *testing.T) {
	out, _, code := runToolDir(t, t.TempDir(), "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: tee") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runToolDir(t, t.TempDir(), "", "--version")
	if code != 0 || !strings.Contains(out, "tee") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}
