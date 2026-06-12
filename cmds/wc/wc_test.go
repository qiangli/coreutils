package wccmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, dir, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	if dir == "" {
		dir = t.TempDir()
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWcStdin(t *testing.T) {
	// GNU pads stdin counts to width 7.
	out, _, code := runTool(t, "", "hi there\n")
	if out != "      1       2       9\n" || code != 0 {
		t.Errorf("wc stdin: got (%q, %d)", out, code)
	}

	// A single selected count on stdin prints unpadded.
	out, _, _ = runTool(t, "", "a\nb\n", "-l")
	if out != "2\n" {
		t.Errorf("wc -l stdin: got %q", out)
	}
}

func TestWcFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f", "hello\n") // 6 bytes -> width 1

	out, _, code := runTool(t, dir, "", "f")
	if out != "1 1 6 f\n" || code != 0 {
		t.Errorf("wc f: got (%q, %d)", out, code)
	}

	out, _, _ = runTool(t, dir, "", "-l", "f")
	if out != "1 f\n" {
		t.Errorf("wc -l f: got %q", out)
	}
}

func TestWcMultipleAndTotal(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a", "one two\nthree\n") // 14 bytes
	writeFile(t, dir, "b", "x\n")              // 2 bytes; total 16 -> width 2

	out, _, code := runTool(t, dir, "", "a", "b")
	want := " 2  3 14 a\n 1  1  2 b\n 3  4 16 total\n"
	if out != want || code != 0 {
		t.Errorf("wc a b: got (%q, %d), want %q", out, code, want)
	}
}

func TestWcCharsAndMaxLine(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "u", "héllo\n") // 7 bytes, 6 chars

	out, _, _ := runTool(t, dir, "", "-m", "u")
	if out != "6 u\n" {
		t.Errorf("wc -m: got %q", out)
	}
	out, _, _ = runTool(t, dir, "", "-c", "u")
	if out != "7 u\n" {
		t.Errorf("wc -c: got %q", out)
	}

	// -L: tab advances to the next multiple of 8.
	out, _, _ = runTool(t, "", "ab\tc\nxy\n", "-L")
	if out != "9\n" {
		t.Errorf("wc -L: got %q", out)
	}

	// -L counts a final line without a newline.
	out, _, _ = runTool(t, "", "abcd", "-L")
	if out != "4\n" {
		t.Errorf("wc -L no newline: got %q", out)
	}
}

func TestWcWordRules(t *testing.T) {
	// Words are maximal non-whitespace runs (C locale whitespace).
	out, _, _ := runTool(t, "", "  a\t\tb  \n\nc", "-w")
	if out != "3\n" {
		t.Errorf("wc -w: got %q", out)
	}
}

func TestWcErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a", "x\n")

	out, errb, code := runTool(t, dir, "", "missing", "a")
	if code != 1 || !strings.Contains(errb, "wc: missing:") {
		t.Errorf("missing file: err=%q code=%d", errb, code)
	}
	if !strings.Contains(out, "a\n") || !strings.Contains(out, "total") {
		t.Errorf("surviving rows: out=%q", out)
	}

	_, errb, code = runTool(t, "", "", "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: err=%q code=%d", errb, code)
	}
}

func TestWcHelpVersion(t *testing.T) {
	out, _, code := runTool(t, "", "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: wc") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "", "", "--version")
	if code != 0 || !strings.Contains(out, "wc") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}
