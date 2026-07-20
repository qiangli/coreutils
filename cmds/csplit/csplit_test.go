package csplitcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, dir string, stdin string, args ...string) (stdout, stderr string, code int) {
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

func TestCsplitLineNumber(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, "", "-s", "in", "2")
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errb)
	}
	assertFile(t, dir, "xx00", "a\n")
	assertFile(t, dir, "xx01", "b\nc\n")
}

func TestCsplitRegexAndPrefix(t *testing.T) {
	dir := t.TempDir()
	stdout, errb, code := runTool(t, dir, "one\ntwo\nthree\n", "-f", "part", "-n", "1", "-", "/two/")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if stdout != "4\n10\n" {
		t.Fatalf("stdout=%q", stdout)
	}
	assertFile(t, dir, "part0", "one\n")
	assertFile(t, dir, "part1", "two\nthree\n")
}

func TestCsplitRepeatedRegexAdvances(t *testing.T) {
	dir := t.TempDir()
	stdout, errb, code := runTool(t, dir, "a\nb\nb\nc\n", "-f", "p", "-n", "1", "-", "/b/", "/b/")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if stdout != "2\n2\n4\n" {
		t.Fatalf("stdout=%q", stdout)
	}
	assertFile(t, dir, "p0", "a\n")
	assertFile(t, dir, "p1", "b\n")
	assertFile(t, dir, "p2", "b\nc\n")
}

func TestCsplitRepeatToEOF(t *testing.T) {
	dir := t.TempDir()
	stdout, errb, code := runTool(t, dir, "a\ncut\nb\ncut\nc\n", "-s", "-f", "p", "-n", "1", "-", "/cut/", "{*}")
	if code != 0 || stdout != "" || errb != "" {
		t.Fatalf("code=%d out=%q err=%q", code, stdout, errb)
	}
	assertFile(t, dir, "p0", "a\n")
	assertFile(t, dir, "p1", "cut\nb\n")
	assertFile(t, dir, "p2", "cut\nc\n")
}

func TestCsplitRegexOffsets(t *testing.T) {
	dir := t.TempDir()
	stdout, errb, code := runTool(t, dir, "a\nmatch\nb\nc\n", "-s", "-f", "p", "-n", "1", "-", "/match/+1")
	if code != 0 || stdout != "" || errb != "" {
		t.Fatalf("code=%d out=%q err=%q", code, stdout, errb)
	}
	assertFile(t, dir, "p0", "a\nmatch\n")
	assertFile(t, dir, "p1", "b\nc\n")

	dir = t.TempDir()
	stdout, errb, code = runTool(t, dir, "a\nskip\nb\nc\n", "-s", "-f", "q", "-n", "1", "-", "%skip%+1")
	if code != 0 || stdout != "" || errb != "" {
		t.Fatalf("code=%d out=%q err=%q", code, stdout, errb)
	}
	assertFile(t, dir, "q0", "b\nc\n")
}

func TestCsplitSuffixSuppressRepeatAndElideEmpty(t *testing.T) {
	dir := t.TempDir()
	stdout, errb, code := runTool(t, dir, "a\nmatch\nb\nmatch\nc\n", "-q", "-z", "--suppress-matched", "-b", "%03d.out", "-", "/match/", "{1}")
	if code != 0 || stdout != "" || errb != "" {
		t.Fatalf("code=%d out=%q err=%q", code, stdout, errb)
	}
	assertFile(t, dir, "xx000.out", "a\n")
	assertFile(t, dir, "xx001.out", "b\n")
	assertFile(t, dir, "xx002.out", "c\n")
}

func TestCsplitSuffixIntegerAliases(t *testing.T) {
	dir := t.TempDir()
	stdout, errb, code := runTool(t, dir, "a\nb\n", "-s", "-b", "%03i", "-", "2")
	if code != 0 || stdout != "" || errb != "" {
		t.Fatalf("code=%d out=%q err=%q", code, stdout, errb)
	}
	assertFile(t, dir, "xx000", "a\n")
	assertFile(t, dir, "xx001", "b\n")
}

func TestCsplitZeroRepeatIsNoop(t *testing.T) {
	dir := t.TempDir()
	stdout, errb, code := runTool(t, dir, "a\nb\nc\n", "-s", "-", "2", "{0}")
	if code != 0 || stdout != "" || errb != "" {
		t.Fatalf("code=%d out=%q err=%q", code, stdout, errb)
	}
	assertFile(t, dir, "xx00", "a\n")
	assertFile(t, dir, "xx01", "b\nc\n")
}

func TestCsplitElidesEmptyInitialPiece(t *testing.T) {
	dir := t.TempDir()
	stdout, errb, code := runTool(t, dir, "match\nbody\n", "-s", "-z", "-", "/match/")
	if code != 0 || stdout != "" || errb != "" {
		t.Fatalf("code=%d out=%q err=%q", code, stdout, errb)
	}
	assertFile(t, dir, "xx00", "match\nbody\n")
	if _, err := os.Stat(filepath.Join(dir, "xx01")); !os.IsNotExist(err) {
		t.Fatalf("xx01 exists or stat failed unexpectedly: %v", err)
	}
}

func TestCsplitErrors(t *testing.T) {
	_, errb, code := runTool(t, t.TempDir(), "", "missing")
	if code != 2 || !strings.Contains(errb, "missing operand") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, t.TempDir(), "a\n", "-", "{*}")
	if code != 2 || !strings.Contains(errb, "missing pattern before repeat count") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, t.TempDir(), "a\n", "-", "/a/+2")
	if code != 2 || !strings.Contains(errb, "line number out of range") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, t.TempDir(), "a\n", "-b", "%s", "-", "/a/")
	if code != 2 || !strings.Contains(errb, "requires one integer conversion") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, t.TempDir(), "a\n", "-b", "%d-%d", "-", "/a/")
	if code != 2 || !strings.Contains(errb, "requires one integer conversion") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
}

func assertFile(t *testing.T, dir, name, want string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s=%q want %q", name, got, want)
	}
}

// POSIX: a repeated line-number pattern advances by N lines each round.
func TestCsplitLineNumberRepeatAdvances(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "", "in", "3", "{1}")
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	assertFile(t, dir, "xx00", "l1\nl2\n")
	assertFile(t, dir, "xx01", "l3\nl4\nl5\n")
	assertFile(t, dir, "xx02", "l6\nl7\nl8\n")
}

// A line-number pattern with {*} repeats until the input is exhausted
// (this used to loop forever).
func TestCsplitLineNumberRepeatToEOF(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("l1\nl2\nl3\nl4\nl5\nl6\nl7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "", "in", "2", "{*}")
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	assertFile(t, dir, "xx00", "l1\n")
	assertFile(t, dir, "xx01", "l2\nl3\n")
	assertFile(t, dir, "xx02", "l4\nl5\n")
	assertFile(t, dir, "xx03", "l6\nl7\n")
}

// An explicit repeat count that runs past EOF is an error.
func TestCsplitLineNumberRepeatOutOfRange(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("l1\nl2\nl3\nl4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "", "in", "3", "{5}")
	if code != 2 || !strings.Contains(errb, "line number out of range") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
}

// csplit patterns are BREs: \(...\) groups, \{n\} intervals.
func TestCsplitPatternsAreBRE(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("aa\nxbbz\ncc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "", "in", `/xb\{2\}z/`)
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	assertFile(t, dir, "xx00", "aa\n")
	assertFile(t, dir, "xx01", "xbbz\ncc\n")
	// In a BRE, ( is a literal — this must not be a regex syntax error.
	dir2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir2, "in"), []byte("aa\nx(y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code = runTool(t, dir2, "", "in", "/x(y/")
	if code != 0 {
		t.Fatalf("literal paren: code=%d err=%q", code, errb)
	}
	assertFile(t, dir2, "xx01", "x(y\n")
}

// --suppress-matched also suppresses line-number split lines.
func TestCsplitSuppressMatchedLineNumber(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("l1\nl2\nl3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "", "--suppress-matched", "in", "2")
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	assertFile(t, dir, "xx00", "l1\n")
	assertFile(t, dir, "xx01", "l3\n")
}
