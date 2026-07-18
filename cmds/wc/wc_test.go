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

func TestWcTotalModes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a", "x\n")
	writeFile(t, dir, "b", "yy\n")

	out, _, code := runTool(t, dir, "", "--total=never", "a", "b")
	if code != 0 || out != "1 1 2 a\n1 1 3 b\n" {
		t.Errorf("--total=never: out=%q code=%d", out, code)
	}
	out, _, code = runTool(t, dir, "", "-l", "--total=only", "a", "b")
	if code != 0 || out != "2 total\n" {
		t.Errorf("--total=only: out=%q code=%d", out, code)
	}
	out, _, code = runTool(t, dir, "", "-l", "--total=always", "a")
	if code != 0 || out != "1 a\n1 total\n" {
		t.Errorf("--total=always: out=%q code=%d", out, code)
	}
}

func TestWcFiles0From(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a", "x\n")
	writeFile(t, dir, "b", "y z\n")
	writeFile(t, dir, "list", "a\x00b\x00")

	out, _, code := runTool(t, dir, "", "-l", "--files0-from=list")
	if code != 0 || out != "1 a\n1 b\n2 total\n" {
		t.Errorf("--files0-from file: out=%q code=%d", out, code)
	}
	out, _, code = runTool(t, dir, "a\x00b\x00", "-w", "--files0-from=-", "--total=only")
	if code != 0 || out != "3 total\n" {
		t.Errorf("--files0-from stdin: out=%q code=%d", out, code)
	}
	_, errb, code := runTool(t, dir, "", "--files0-from=list", "a")
	if code != 2 || !strings.Contains(errb, "cannot be combined") {
		t.Errorf("--files0-from operands: err=%q code=%d", errb, code)
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

// --total applies to standard input too: "always" adds a total line for a
// single input, "only" suppresses the per-input row.
func TestWcTotalModesStdin(t *testing.T) {
	out, _, code := runTool(t, "", "a b\nc\n", "-l", "--total=only")
	if code != 0 || out != "2 total\n" {
		t.Errorf("stdin --total=only: out=%q code=%d", out, code)
	}
	out, _, code = runTool(t, "", "a b\nc\n", "-l", "--total=always")
	if code != 0 || out != "2\n2 total\n" {
		t.Errorf("stdin --total=always: out=%q code=%d", out, code)
	}
	out, _, code = runTool(t, "", "hi\n", "-l", "--total=never")
	if code != 0 || out != "1\n" {
		t.Errorf("stdin --total=never: out=%q code=%d", out, code)
	}
	// auto stays a bare row: one input, no total.
	out, _, code = runTool(t, "", "hi there\n")
	if code != 0 || out != "      1       2       9\n" {
		t.Errorf("stdin --total=auto: out=%q code=%d", out, code)
	}
}

func TestWcFiles0FromUnreadable(t *testing.T) {
	dir := t.TempDir()

	// The errno text after the colon is the platform's; the GNU-shaped
	// "cannot open X for reading" prefix is what this asserts.
	_, errb, code := runTool(t, dir, "", "--files0-from=nosuch")
	if code != 1 || !strings.Contains(errb, "wc: cannot open 'nosuch' for reading: ") {
		t.Errorf("missing name list: err=%q code=%d", errb, code)
	}

	// An empty argument names no file; it must not resolve to the cwd
	// (which would misreport the failure as "Is a directory").
	_, errb, code = runTool(t, dir, "", "--files0-from=")
	if code != 1 || !strings.Contains(errb, "wc: cannot open '' for reading: ") {
		t.Errorf("empty name list: err=%q code=%d", errb, code)
	}
	if strings.Contains(errb, "Is a directory") {
		t.Errorf("empty name list resolved to the cwd: err=%q", errb)
	}
}

func TestWcFiles0FromBadNames(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a", "x\n")
	writeFile(t, dir, "b", "y\n")

	// A zero-length record is diagnosed by record number; the remaining
	// names are still counted and the exit status is 1.
	writeFile(t, dir, "zl", "a\x00\x00b\x00")
	out, errb, code := runTool(t, dir, "", "-l", "--files0-from=zl")
	if code != 1 || !strings.Contains(errb, "zl:2: invalid zero-length file name") {
		t.Errorf("zero-length name: err=%q code=%d", errb, code)
	}
	if out != "1 a\n1 b\n2 total\n" {
		t.Errorf("zero-length name survivors: out=%q", out)
	}

	// "-" is rejected only when the list itself came from standard input.
	out, errb, code = runTool(t, dir, "a\x00-\x00b\x00", "-l", "--files0-from=-")
	if code != 1 || !strings.Contains(errb, "when reading file names from stdin, no file name of '-' allowed") {
		t.Errorf("dash from stdin list: err=%q code=%d", errb, code)
	}
	if out != "1 a\n1 b\n2 total\n" {
		t.Errorf("dash from stdin survivors: out=%q", out)
	}

	// From a file list, "-" is an ordinary name meaning standard input.
	writeFile(t, dir, "dashlist", "a\x00-\x00")
	out, _, code = runTool(t, dir, "zz\n", "-l", "--files0-from=dashlist")
	if code != 0 || out != "      1 a\n      1 -\n      2 total\n" {
		t.Errorf("dash from file list: out=%q code=%d", out, code)
	}
}

func TestWcFiles0FromEmptyList(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "empty", "")

	out, errb, code := runTool(t, dir, "", "--files0-from=empty")
	if code != 0 || out != "" || errb != "" {
		t.Errorf("empty list: out=%q err=%q code=%d", out, errb, code)
	}
}

func TestWcDirectoryOperand(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "a", "x\n")

	// The errno text is the platform's (GNU says "Is a directory"; Windows
	// denies the read), so only the diagnostic shape is asserted here.
	out, errb, code := runTool(t, dir, "", "d", "a")
	if code != 1 || !strings.Contains(errb, "wc: d: ") {
		t.Errorf("directory operand: err=%q code=%d", errb, code)
	}
	// GNU still emits a zero row for the directory, and the non-regular
	// operand widens every column to 7.
	want := "      0       0       0 d\n      1       1       2 a\n      1       1       2 total\n"
	if out != want {
		t.Errorf("directory operand: out=%q want %q", out, want)
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
