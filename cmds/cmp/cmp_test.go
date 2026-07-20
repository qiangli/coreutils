package cmpcmd

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

func TestCmpIdentical(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a", "same\ncontent\n")
	writeFile(t, dir, "b", "same\ncontent\n")
	out, errb, code := runTool(t, dir, "", "a", "b")
	if out != "" || errb != "" || code != 0 {
		t.Errorf("identical: out=%q err=%q code=%d", out, errb, code)
	}
}

func TestCmpDiffer(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a", "abc\n")
	writeFile(t, dir, "b", "abd\n")
	out, _, code := runTool(t, dir, "", "a", "b")
	if out != "a b differ: byte 3, line 1\n" || code != 1 {
		t.Errorf("differ: out=%q code=%d", out, code)
	}

	writeFile(t, dir, "c", "one\ntwo\n")
	writeFile(t, dir, "d", "one\ntwX\n")
	out, _, code = runTool(t, dir, "", "c", "d")
	if out != "c d differ: byte 7, line 2\n" || code != 1 {
		t.Errorf("differ line 2: out=%q code=%d", out, code)
	}
}

func TestCmpVerbose(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a", "abc")
	writeFile(t, dir, "b", "aXd")
	out, _, code := runTool(t, dir, "", "-l", "a", "b")
	// Width 1 (min size 3); values in 3-wide octal.
	want := "2 142 130\n3 143 144\n"
	if out != want || code != 1 {
		t.Errorf("-l: out=%q code=%d, want %q", out, code, want)
	}
}

func TestCmpSilent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a", "x")
	writeFile(t, dir, "b", "y")
	out, errb, code := runTool(t, dir, "", "-s", "a", "b")
	if out != "" || errb != "" || code != 1 {
		t.Errorf("-s differ: out=%q err=%q code=%d", out, errb, code)
	}
	writeFile(t, dir, "c", "x")
	if _, _, code := runTool(t, dir, "", "--quiet", "a", "c"); code != 0 {
		t.Errorf("--quiet same: code=%d", code)
	}
}

func TestCmpEOF(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "short", "ab")
	writeFile(t, dir, "long", "abc")
	out, errb, code := runTool(t, dir, "", "short", "long")
	if code != 1 || out != "" || !strings.Contains(errb, "cmp: EOF on short after byte 2, in line 1") {
		t.Errorf("EOF: out=%q err=%q code=%d", out, errb, code)
	}

	writeFile(t, dir, "empty", "")
	_, errb, code = runTool(t, dir, "", "empty", "long")
	if code != 1 || !strings.Contains(errb, "cmp: EOF on empty which is empty") {
		t.Errorf("empty EOF: err=%q code=%d", errb, code)
	}

	// Prefix ending in a newline reports the completed line count.
	writeFile(t, dir, "p1", "a\nb\n")
	writeFile(t, dir, "p2", "a\nb\nc\n")
	_, errb, code = runTool(t, dir, "", "p1", "p2")
	if code != 1 || !strings.Contains(errb, "cmp: EOF on p1 after byte 4, in line 2") {
		t.Errorf("newline EOF: err=%q code=%d", errb, code)
	}
}

func TestCmpStdinAndSkips(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a", "hello\n")
	// FILE2 defaults to "-".
	if _, _, code := runTool(t, dir, "hello\n", "a"); code != 0 {
		t.Errorf("cmp a (stdin default): code=%d", code)
	}

	// Skips: compare a[1:] with b[2:].
	writeFile(t, dir, "s1", "Xabc")
	writeFile(t, dir, "s2", "YYabc")
	if _, _, code := runTool(t, dir, "", "s1", "s2", "1", "2"); code != 0 {
		t.Errorf("skips equal: code=%d", code)
	}
	// Hex skip.
	if _, _, code := runTool(t, dir, "", "s1", "s2", "0x1", "2"); code != 0 {
		t.Errorf("hex skip: code=%d", code)
	}
}

func TestCmpIgnoreInitial(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a", "XXpayload")
	writeFile(t, dir, "b", "YYpayload")
	if _, _, code := runTool(t, dir, "", "--ignore-initial=2:2", "a", "b"); code != 0 {
		t.Errorf("--ignore-initial equal: code=%d", code)
	}
	if _, _, code := runTool(t, dir, "", "-i", "1:2", "a", "b"); code != 1 {
		t.Errorf("-i mismatch: code=%d", code)
	}
	// A single SKIP value applies to both files (GNU form).
	if _, _, code := runTool(t, dir, "", "--ignore-initial=2", "a", "b"); code != 0 {
		t.Errorf("single-value -i equal: code=%d", code)
	}
	if _, _, code := runTool(t, dir, "", "-i", "1", "a", "b"); code != 1 {
		t.Errorf("single-value -i mismatch: code=%d", code)
	}
	_, errb, code := runTool(t, dir, "", "-i", "bad", "a", "b")
	if code != 2 || !strings.Contains(errb, "invalid --ignore-initial value") {
		t.Errorf("bad -i: err=%q code=%d", errb, code)
	}
	// Positional skip operands override a preceding -i.
	if _, _, code := runTool(t, dir, "", "-i", "0", "a", "b", "2", "2"); code != 0 {
		t.Errorf("positional overrides -i: code=%d", code)
	}
}

func TestCmpPrintBytes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a", "abc\n")
	writeFile(t, dir, "b", "abd\n")
	out, _, code := runTool(t, dir, "", "-b", "a", "b")
	want := "a b differ: byte 3, line 1 is 143 c 144 d\n"
	if out != want || code != 1 {
		t.Errorf("-b: out=%q code=%d, want %q", out, code, want)
	}

	// -b in verbose mode adds the character column per line.
	writeFile(t, dir, "v1", "abc")
	writeFile(t, dir, "v2", "aXd")
	out, _, code = runTool(t, dir, "", "-lb", "v1", "v2")
	want = "2 142 b 130 X\n3 143 c 144 d\n"
	if out != want || code != 1 {
		t.Errorf("-lb: out=%q code=%d, want %q", out, code, want)
	}

	// Control and high-bit bytes use caret / M- notation.
	writeFile(t, dir, "c1", "\x00")
	writeFile(t, dir, "c2", "\xff")
	out, _, code = runTool(t, dir, "", "-b", "c1", "c2")
	want = "c1 c2 differ: byte 1, line 1 is   0 ^@ 377 M-^?\n"
	if out != want || code != 1 {
		t.Errorf("-b control: out=%q code=%d, want %q", out, code, want)
	}
}

func TestCmpBytesLimit(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a", "same-prefix-a")
	writeFile(t, dir, "b", "same-prefix-b")
	if _, _, code := runTool(t, dir, "", "--bytes=11", "a", "b"); code != 0 {
		t.Errorf("limited equal: code=%d", code)
	}
	out, errb, code := runTool(t, dir, "", "-n", "13", "a", "b")
	if code != 1 || out == "" || errb != "" {
		t.Errorf("limited difference: out=%q err=%q code=%d", out, errb, code)
	}
	writeFile(t, dir, "short", "same")
	writeFile(t, dir, "long", "same-long")
	if _, errb, code := runTool(t, dir, "", "-n", "4", "short", "long"); code != 0 || errb != "" {
		t.Errorf("limit suppresses EOF: err=%q code=%d", errb, code)
	}
	// A multiplier suffix is accepted, as on the SKIP operands.
	if _, _, code := runTool(t, dir, "", "--bytes=1K", "a", "b"); code != 1 {
		t.Errorf("suffixed limit: code=%d", code)
	}
	if _, errb, code := runTool(t, dir, "", "--bytes=bad", "a", "b"); code != 2 || !strings.Contains(errb, "invalid --bytes value") {
		t.Errorf("bad limit: err=%q code=%d", errb, code)
	}
}

func TestCmpRejectsRepeatedStandardInput(t *testing.T) {
	out, errb, code := runTool(t, "", "abc", "-", "-")
	if code != 2 || out != "" || !strings.Contains(errb, "standard input") {
		t.Errorf("repeated stdin: out=%q err=%q code=%d", out, errb, code)
	}
}

func TestCmpErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a", "x")

	_, errb, code := runTool(t, dir, "")
	if code != 2 || !strings.Contains(errb, "missing operand") {
		t.Errorf("no operands: err=%q code=%d", errb, code)
	}

	_, errb, code = runTool(t, dir, "", "-l", "-s", "a", "a")
	if code != 2 || !strings.Contains(errb, "options -l and -s are incompatible") {
		t.Errorf("-l -s: err=%q code=%d", errb, code)
	}

	_, errb, code = runTool(t, dir, "", "a", "missing")
	if code != 2 || !strings.Contains(errb, "cmp: missing:") {
		t.Errorf("missing file: err=%q code=%d", errb, code)
	}

	_, errb, code = runTool(t, dir, "", "a", "a", "zz")
	if code != 2 || !strings.Contains(errb, "invalid --ignore-initial value") {
		t.Errorf("bad skip: err=%q code=%d", errb, code)
	}

	_, errb, code = runTool(t, dir, "", "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: err=%q code=%d", errb, code)
	}
}

func TestCmpHelpVersion(t *testing.T) {
	out, _, code := runTool(t, "", "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: cmp") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "", "", "--version")
	if code != 0 || !strings.Contains(out, "cmp") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}
