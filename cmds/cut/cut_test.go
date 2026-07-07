package cutcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
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

func runTool(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	return runToolDir(t, t.TempDir(), stdin, args...)
}

func TestCutFields(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		stdin string
		want  string
	}{
		{"single field", []string{"-f", "2", "-d", ":"}, "a:b:c\n", "b\n"},
		{"comma list", []string{"-f", "1,3", "-d", ":"}, "a:b:c\n", "a:c\n"},
		{"range", []string{"-f", "2-3", "-d", ":"}, "a:b:c:d\n", "b:c\n"},
		{"open range", []string{"-f", "2-", "-d", ":"}, "a:b:c:d\n", "b:c:d\n"},
		{"-M range", []string{"-f", "-2", "-d", ":"}, "a:b:c:d\n", "a:b\n"},
		{"default tab delim", []string{"-f", "2"}, "a\tb\tc\n", "b\n"},
		{"attached value", []string{"-f2", "-d:"}, "a:b:c\n", "b\n"},
		{"no delimiter passthrough", []string{"-f", "2", "-d", ":"}, "noseparator\n", "noseparator\n"},
		{"only-delimited", []string{"-s", "-f", "2", "-d", ":"}, "a:b\nplain\nc:d\n", "b\nd\n"},
		{"complement", []string{"--complement", "-f", "2", "-d", ":"}, "a:b:c\n", "a:c\n"},
		{"complement no-delim passthrough", []string{"--complement", "-f", "2", "-d", ":"}, "plain\n", "plain\n"},
		{"field beyond line", []string{"-f", "5", "-d", ":"}, "a:b\n", "\n"},
		{"multiple lines", []string{"-f", "1", "-d", ","}, "a,b\nc,d\n", "a\nc\n"},
		{"no trailing newline preserved", []string{"-f", "1", "-d", ":"}, "a:b", "a"},
		{"whitespace separated list", []string{"-f", "1 3", "-d", ":"}, "a:b:c\n", "a:c\n"},
		{"overlapping ranges", []string{"-f", "1-3,2-4", "-d", ":"}, "a:b:c:d:e\n", "a:b:c:d\n"},
		{"NUL delimiter via -d ''", []string{"-f", "2", "-d", ""}, "a\x00b\n", "b\n"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, c.stdin, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("%s: cut %v = (%q, %q, %d), want (%q, _, 0)", c.name, c.args, out, errb, code, c.want)
		}
	}
}

func TestCutBytesAndChars(t *testing.T) {
	cases := []struct {
		args  []string
		stdin string
		want  string
	}{
		{[]string{"-b", "1-3"}, "abcdef\n", "abc\n"},
		{[]string{"-b", "2"}, "abcdef\n", "b\n"},
		{[]string{"-b", "4-"}, "abcdef\n", "def\n"},
		{[]string{"-b", "-2"}, "abcdef\n", "ab\n"},
		{[]string{"-b", "1,3,5"}, "abcdef\n", "ace\n"},
		{[]string{"-c", "1-3"}, "abcdef\n", "abc\n"},
		{[]string{"-b", "10-"}, "abc\n", "\n"},
		{[]string{"--complement", "-b", "2-4"}, "abcdef\n", "aef\n"},
		{[]string{"-b", "1-2"}, "abc\ndefgh\n", "ab\nde\n"},
		{[]string{"-b", "1"}, "", ""},
	}
	for _, c := range cases {
		out, _, code := runTool(t, c.stdin, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("cut %v = (%q, %d), want (%q, 0)", c.args, out, code, c.want)
		}
	}
}

func TestCutFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f1"), []byte("a:b\nc:d\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f2"), []byte("e:f\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// relative operands resolve against rc.Dir
	out, _, code := runToolDir(t, dir, "", "-d:", "-f2", "f1", "f2")
	if out != "b\nd\nf\n" || code != 0 {
		t.Errorf("two files: out=%q code=%d", out, code)
	}
	// "-" mixes stdin between files
	out, _, code = runToolDir(t, dir, "x:y\n", "-d:", "-f1", "f1", "-")
	if out != "a\nc\nx\n" || code != 0 {
		t.Errorf("file + stdin: out=%q code=%d", out, code)
	}
	// missing file: diagnostic, status 1, other files still processed
	out, errb, code := runToolDir(t, dir, "", "-d:", "-f1", "nosuch", "f2")
	if code != 1 || out != "e\n" || !strings.Contains(errb, "cut: nosuch:") {
		t.Errorf("missing file: out=%q err=%q code=%d", out, errb, code)
	}
}

func TestCutUsageErrors(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{}, "you must specify a list of bytes, characters, or fields"},
		{[]string{"-b", "1", "-f", "2"}, "only one type of list may be specified"},
		{[]string{"-b", "1", "-c", "2"}, "only one type of list may be specified"},
		{[]string{"-d", ":", "-b", "1"}, "an input delimiter may be specified only when operating on fields"},
		{[]string{"-d", "ab", "-f", "1"}, "the delimiter must be a single character"},
		{[]string{"-s", "-b", "1"}, "suppressing non-delimited lines makes sense"},
		{[]string{"-f", "0"}, "fields and positions are numbered from 1"},
		{[]string{"-f", ""}, "fields and positions are numbered from 1"},
		{[]string{"-f", "1,"}, "fields and positions are numbered from 1"},
		{[]string{"-f", "0-2"}, "fields are numbered from 1"},
		{[]string{"-b", "0-2"}, "byte/character positions are numbered from 1"},
		{[]string{"-f", "-"}, "invalid range with no endpoint: -"},
		{[]string{"-f", "3-2"}, "invalid decreasing range"},
		{[]string{"-f", "x"}, "invalid field range"},
		{[]string{"-b", "x"}, "invalid byte or character range"},
		{[]string{"-f", "1-2-3"}, "invalid field range"},
		{[]string{"-f", "99999999999999999999"}, "field number '99999999999999999999' is too large"},
	}
	for _, c := range cases {
		_, errb, code := runTool(t, "", c.args...)
		if code != 2 || !strings.Contains(errb, c.want) {
			t.Errorf("cut %v: code=%d err=%q, want code=2 containing %q", c.args, code, errb, c.want)
		}
		if !strings.Contains(errb, "Try 'cut --help'") {
			t.Errorf("cut %v: missing try-help in %q", c.args, errb)
		}
	}
}

func TestCutUnknownFlag(t *testing.T) {
	_, errb, code := runTool(t, "", "--frobnicate", "-f1")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestCutHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: cut") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "", "--version")
	if code != 0 || !strings.Contains(out, "cut") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}

func TestCutNewOptions(t *testing.T) {
	// 1. Output Delimiter in Field Mode
	out, _, code := runTool(t, "a:b:c\n", "-f", "1,3", "-d", ":", "--output-delimiter=XYZ")
	if code != 0 || out != "aXYZc\n" {
		t.Errorf("output delimiter field mode: out=%q code=%d", out, code)
	}

	// 2. Output Delimiter in Byte Mode
	out, _, code = runTool(t, "abcdef\n", "-b", "1-2,4-5", "--output-delimiter=XYZ")
	if code != 0 || out != "abXYZde\n" {
		t.Errorf("output delimiter byte mode: out=%q code=%d", out, code)
	}

	// 3. Zero Terminated
	out, _, code = runTool(t, "a:b:c\x00d:e:f\x00", "-f", "2", "-d", ":", "-z")
	if code != 0 || out != "b\x00e\x00" {
		t.Errorf("zero terminated: out=%q code=%d", out, code)
	}

	// 4. Ignored -n option
	out, _, code = runTool(t, "abcdef\n", "-b", "1-3", "-n")
	if code != 0 || out != "abc\n" {
		t.Errorf("ignored -n: out=%q code=%d", out, code)
	}

	// 5. Whitespace Delimited (-w)
	out, _, code = runTool(t, "  a   b\tc  \n", "-f", "1,3", "-w")
	if code != 0 || out != "a c\n" {
		t.Errorf("whitespace delimited default output: out=%q code=%d", out, code)
	}

	// 6. Whitespace Delimited (-w) with output-delimiter
	out, _, code = runTool(t, "  a   b\tc  \n", "-f", "1,3", "-w", "--output-delimiter=,")
	if code != 0 || out != "a,c\n" {
		t.Errorf("whitespace delimited with output-delimiter: out=%q code=%d", out, code)
	}

	// 7. Whitespace Delimited (-w) with -s (only-delimited)
	out, _, code = runTool(t, "a\n  b  c\n", "-f", "2", "-w", "-s")
	if code != 0 || out != "c\n" {
		t.Errorf("whitespace delimited with only-delimited: out=%q code=%d", out, code)
	}
	out, _, code = runTool(t, "a:b:c\n", "-f", "1,3", "-d", ":", "-O", "XYZ")
	if code != 0 || out != "aXYZc\n" {
		t.Errorf("-O output delimiter alias: out=%q code=%d", out, code)
	}
}
