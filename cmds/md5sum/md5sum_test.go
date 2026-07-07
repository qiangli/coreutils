package md5sumcmd

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

const (
	abcMD5   = "900150983cd24fb0d6963f7d28e17f72" // md5("abc")
	emptyMD5 = "d41d8cd98f00b204e9800998ecf8427e" // md5("")
)

// runTool is the canonical test harness shape for cmds packages,
// extended with stdin content and an explicit working directory.
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

func TestComputeStdin(t *testing.T) {
	out, _, code := runTool(t, "", "abc")
	if out != abcMD5+"  -\n" || code != 0 {
		t.Errorf("stdin: got (%q, %d)", out, code)
	}
	out, _, code = runTool(t, "", "abc", "-")
	if out != abcMD5+"  -\n" || code != 0 {
		t.Errorf("explicit -: got (%q, %d)", out, code)
	}
}

func TestComputeFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "abc")
	writeFile(t, dir, "b.txt", "")

	out, _, code := runTool(t, dir, "", "a.txt", "b.txt")
	want := abcMD5 + "  a.txt\n" + emptyMD5 + "  b.txt\n"
	if out != want || code != 0 {
		t.Errorf("two files: got (%q, %d), want (%q, 0)", out, code, want)
	}

	// -b switches the separator to " *"; digest bytes are identical.
	out, _, code = runTool(t, dir, "", "-b", "a.txt")
	if out != abcMD5+" *a.txt\n" || code != 0 {
		t.Errorf("-b: got (%q, %d)", out, code)
	}

	// --tag emits BSD-style output.
	out, _, code = runTool(t, dir, "", "--tag", "a.txt")
	if out != "MD5 (a.txt) = "+abcMD5+"\n" || code != 0 {
		t.Errorf("--tag: got (%q, %d)", out, code)
	}
}

func TestComputeMissingFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "abc")
	out, errb, code := runTool(t, dir, "", "nope.txt", "a.txt")
	if code != 1 {
		t.Errorf("missing file: code=%d, want 1", code)
	}
	if !strings.Contains(errb, "md5sum: nope.txt: No such file or directory") {
		t.Errorf("missing file: err=%q", errb)
	}
	// the good operand is still processed
	if !strings.Contains(out, abcMD5+"  a.txt\n") {
		t.Errorf("missing file: out=%q", out)
	}
}

func TestCheckOKAndFailures(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "good", "abc")
	writeFile(t, dir, "bad1", "not abc")
	writeFile(t, dir, "bad2", "also not abc")
	sums := abcMD5 + "  good\n" +
		abcMD5 + "  bad1\n" +
		abcMD5 + "  bad2\n" +
		emptyMD5 + "  missing\n"
	writeFile(t, dir, "sums.txt", sums)

	out, errb, code := runTool(t, dir, "", "-c", "sums.txt")
	wantOut := "good: OK\nbad1: FAILED\nbad2: FAILED\nmissing: FAILED open or read\n"
	if out != wantOut {
		t.Errorf("-c out = %q, want %q", out, wantOut)
	}
	if code != 1 {
		t.Errorf("-c code = %d, want 1", code)
	}
	if !strings.Contains(errb, "md5sum: WARNING: 2 computed checksums did NOT match") {
		t.Errorf("-c missing plural mismatch warning: %q", errb)
	}
	if !strings.Contains(errb, "md5sum: WARNING: 1 listed file could not be read") {
		t.Errorf("-c missing read warning: %q", errb)
	}
	if !strings.Contains(errb, "md5sum: missing: No such file or directory") {
		t.Errorf("-c missing open diagnostic: %q", errb)
	}
}

func TestCheckSingularWarning(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bad", "nope")
	writeFile(t, dir, "sums.txt", abcMD5+"  bad\n")
	_, errb, code := runTool(t, dir, "", "-c", "sums.txt")
	if code != 1 || !strings.Contains(errb, "md5sum: WARNING: 1 computed checksum did NOT match") {
		t.Errorf("singular warning: err=%q code=%d", errb, code)
	}
}

func TestCheckAllOK(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "good", "abc")
	// All accepted line shapes: two-space, binary asterisk, single
	// space, BSD tagged, uppercase hex, plus comments/blanks skipped.
	sums := "# a comment\n" +
		"\n" +
		abcMD5 + "  good\n" +
		abcMD5 + " *good\n" +
		abcMD5 + " good\n" +
		"MD5 (good) = " + abcMD5 + "\n" +
		strings.ToUpper(abcMD5) + "  good\n"
	writeFile(t, dir, "sums.txt", sums)

	out, errb, code := runTool(t, dir, "", "--check", "sums.txt")
	if code != 0 || errb != "" {
		t.Errorf("all-OK: code=%d err=%q", code, errb)
	}
	if out != strings.Repeat("good: OK\n", 5) {
		t.Errorf("all-OK out = %q", out)
	}
}

func TestCheckOutputControls(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "good", "abc")
	writeFile(t, dir, "missing.sums", abcMD5+"  missing\n")

	out, errb, code := runTool(t, dir, abcMD5+"  good\n", "-c", "--quiet")
	if out != "" || errb != "" || code != 0 {
		t.Errorf("--quiet ok: out=%q err=%q code=%d", out, errb, code)
	}
	out, errb, code = runTool(t, dir, strings.Repeat("0", len(abcMD5))+"  good\n", "-c", "--status")
	if out != "" || errb != "" || code != 1 {
		t.Errorf("--status mismatch: out=%q err=%q code=%d", out, errb, code)
	}
	out, errb, code = runTool(t, dir, "", "-c", "--ignore-missing", "missing.sums")
	if out != "" || errb != "" || code != 0 {
		t.Errorf("--ignore-missing: out=%q err=%q code=%d", out, errb, code)
	}
}

func TestCheckFromStdin(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "good", "abc")
	out, _, code := runTool(t, dir, abcMD5+"  good\n", "-c")
	if out != "good: OK\n" || code != 0 {
		t.Errorf("-c from stdin: got (%q, %d)", out, code)
	}
}

func TestCheckImproperlyFormatted(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "good", "abc")
	// bad lines alongside one good line: warning, but exit 0.
	sums := "garbage\n" +
		"deadbeef  short-digest\n" +
		"SHA256 (good) = " + abcMD5 + "\n" + // wrong algo tag for md5sum
		abcMD5 + "  good\n"
	writeFile(t, dir, "sums.txt", sums)
	out, errb, code := runTool(t, dir, "", "-c", "sums.txt")
	if code != 0 {
		t.Errorf("bad-format lines should not fail without --strict: code=%d err=%q", code, errb)
	}
	if out != "good: OK\n" {
		t.Errorf("out = %q", out)
	}
	if !strings.Contains(errb, "md5sum: WARNING: 3 lines are improperly formatted") {
		t.Errorf("err = %q", errb)
	}
}

func TestCheckNoValidLines(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "sums.txt", "garbage\nmore garbage\n")
	_, errb, code := runTool(t, dir, "", "-c", "sums.txt")
	if code != 1 || !strings.Contains(errb, "md5sum: sums.txt: no properly formatted checksum lines found") {
		t.Errorf("no valid lines: err=%q code=%d", errb, code)
	}
	// empty list file behaves the same
	writeFile(t, dir, "empty.txt", "")
	_, errb, code = runTool(t, dir, "", "-c", "empty.txt")
	if code != 1 || !strings.Contains(errb, "no properly formatted checksum lines found") {
		t.Errorf("empty list: err=%q code=%d", errb, code)
	}
}

func TestCheckMissingSumsFile(t *testing.T) {
	_, errb, code := runTool(t, "", "", "-c", "nope.sums")
	if code != 1 || !strings.Contains(errb, "md5sum: nope.sums: No such file or directory") {
		t.Errorf("missing sums file: err=%q code=%d", errb, code)
	}
}

func TestTagCheckConflict(t *testing.T) {
	_, errb, code := runTool(t, "", "", "--tag", "-c", "x")
	if code != 2 || !strings.Contains(errb, "the --tag option is meaningless when verifying checksums") {
		t.Errorf("--tag -c: err=%q code=%d", errb, code)
	}
}

func TestEscapedFilename(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("backslash is a path separator on windows")
	}
	dir := t.TempDir()
	name := `a\b`
	writeFile(t, dir, name, "abc")
	out, _, code := runTool(t, dir, "", name)
	want := `\` + abcMD5 + `  a\\b` + "\n"
	if out != want || code != 0 {
		t.Errorf("escaped output: got (%q, %d), want (%q, 0)", out, code, want)
	}
	// the escaped line round-trips through -c
	writeFile(t, dir, "sums.txt", out)
	out, errb, code := runTool(t, dir, "", "-c", "sums.txt")
	if code != 0 || !strings.Contains(out, `a\\b: OK`) {
		t.Errorf("escaped check: out=%q err=%q code=%d", out, errb, code)
	}
}

func TestUnknownFlag(t *testing.T) {
	_, errb, code := runTool(t, "", "", "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: err=%q code=%d", errb, code)
	}
}

func TestHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, "", "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: md5sum") || !strings.Contains(out, "--check") ||
		!strings.Contains(out, "-h, --help") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "", "", "-h")
	if code != 0 || !strings.Contains(out, "Usage: md5sum") {
		t.Errorf("-h: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "", "", "--version")
	if code != 0 || !strings.Contains(out, "md5sum") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "", "", "-V")
	if code != 0 || !strings.Contains(out, "md5sum") {
		t.Errorf("-V: code=%d out=%q", code, out)
	}
}
