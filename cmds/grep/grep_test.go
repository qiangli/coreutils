package grepcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// runGrep is the canonical test harness shape for cmds packages,
// extended with a working directory and stdin contents.
func runGrep(t *testing.T, dir, stdin string, args ...string) (stdout, stderr string, code int) {
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
	full := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setupTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "f1.txt", "hello world\nfoo bar\nHELLO again\n")
	writeFile(t, dir, "f2.txt", "nothing here\n")
	writeFile(t, dir, "sub/f3.go", "package main\nfoo bar baz\n")
	writeFile(t, dir, "bin.dat", "abc\x00def foo\n")
	return dir
}

func TestGrepStdin(t *testing.T) {
	out, _, code := runGrep(t, "", "a\nfoo\nb\n", "foo")
	if out != "foo\n" || code != 0 {
		t.Errorf("stdin match: out=%q code=%d", out, code)
	}
	out, _, code = runGrep(t, "", "a\nb\n", "zzz")
	if out != "" || code != 1 {
		t.Errorf("stdin no match: out=%q code=%d", out, code)
	}
	// "-" operand with forced name
	out, _, code = runGrep(t, "", "foo\n", "-H", "foo", "-")
	if out != "(standard input):foo\n" || code != 0 {
		t.Errorf("- operand: out=%q code=%d", out, code)
	}
}

func TestGrepFlagsOnFile(t *testing.T) {
	dir := setupTree(t)
	cases := []struct {
		args []string
		want string
		code int
	}{
		{[]string{"-i", "hello", "f1.txt"}, "hello world\nHELLO again\n", 0},
		{[]string{"-i", "-n", "hello", "f1.txt"}, "1:hello world\n3:HELLO again\n", 0},
		{[]string{"-v", "o", "f1.txt"}, "HELLO again\n", 0},
		{[]string{"-c", "o", "f1.txt"}, "2\n", 0},
		{[]string{"-c", "foo", "f1.txt", "f2.txt"}, "f1.txt:1\nf2.txt:0\n", 0},
		{[]string{"-l", "foo", "f1.txt", "f2.txt"}, "f1.txt\n", 0},
		{[]string{"-L", "foo", "f1.txt", "f2.txt"}, "f2.txt\n", 0},
		{[]string{"-m", "1", "-i", "hello", "f1.txt"}, "hello world\n", 0},
		{[]string{"-m", "0", "foo", "f1.txt"}, "", 1},
		{[]string{"-x", "foo bar", "f1.txt"}, "foo bar\n", 0},
		{[]string{"-x", "foo", "f1.txt"}, "", 1},
		// filename prefix defaults
		{[]string{"foo", "f1.txt", "f2.txt"}, "f1.txt:foo bar\n", 0},
		{[]string{"-h", "foo", "f1.txt", "f2.txt"}, "foo bar\n", 0},
		{[]string{"-H", "foo", "f1.txt"}, "f1.txt:foo bar\n", 0},
	}
	for _, c := range cases {
		out, _, code := runGrep(t, dir, "", c.args...)
		if out != c.want || code != c.code {
			t.Errorf("grep %v = (%q, %d), want (%q, %d)", c.args, out, code, c.want, c.code)
		}
	}
}

func TestGrepWordRegexp(t *testing.T) {
	out, _, code := runGrep(t, "", "foobar\nfoo bar\n", "-w", "foo")
	if out != "foo bar\n" || code != 0 {
		t.Errorf("-w: out=%q code=%d", out, code)
	}
	// match edge itself non-word: GNU selects "a-x" for -w -x pattern "-x"
	out, _, code = runGrep(t, "", "a-x\n", "-w", "-e", "-x")
	if out != "a-x\n" || code != 0 {
		t.Errorf("-w nonword edge: out=%q code=%d", out, code)
	}
	out, _, code = runGrep(t, "", "xfoox\n", "-w", "foo")
	if out != "" || code != 1 {
		t.Errorf("-w embedded: out=%q code=%d", out, code)
	}
}

func TestGrepQuiet(t *testing.T) {
	dir := setupTree(t)
	out, _, code := runGrep(t, dir, "", "-q", "foo", "f1.txt")
	if out != "" || code != 0 {
		t.Errorf("-q match: out=%q code=%d", out, code)
	}
	_, _, code = runGrep(t, dir, "", "-q", "zzz", "f1.txt")
	if code != 1 {
		t.Errorf("-q no match: code=%d", code)
	}
	// -q + match wins over an error (GNU exit rule)
	_, _, code = runGrep(t, dir, "", "-q", "foo", "missing.txt", "f1.txt")
	if code != 0 {
		t.Errorf("-q match despite error: code=%d", code)
	}
}

func TestGrepBRE(t *testing.T) {
	cases := []struct {
		pat, in, want string
		code          int
	}{
		{`fo\{2\}`, "fooo\n", "fooo\n", 0},          // interval
		{`^fo\|^ba`, "bar\nqux\n", "bar\n", 0},      // GNU \| alternation
		{`a(b)`, "a(b)\n", "a(b)\n", 0},             // unescaped parens literal
		{`*x`, "*x\n", "*x\n", 0},                   // leading * literal
		{`a\+`, "aaa\n", "aaa\n", 0},                // GNU \+
		{`x$y`, "x$y\n", "x$y\n", 0},                // mid-pattern $ literal
		{`a^b`, "a^b\n", "a^b\n", 0},                // mid-pattern ^ literal
		{`[]ab]`, "]\n", "]\n", 0},                  // ] first member
		{`[[:digit:]]\{2\}`, "ab12\n", "ab12\n", 0}, // class + interval
		{`\(a\)\1`, "aa\nab\n", "aa\n", 0},          // back-reference
	}
	for _, c := range cases {
		out, _, code := runGrep(t, "", c.in, c.pat)
		if out != c.want || code != c.code {
			t.Errorf("BRE %q = (%q, %d), want (%q, %d)", c.pat, out, code, c.want, c.code)
		}
	}
	// rejected constructs: clear error, exit 2
	for _, pat := range []string{`\<word`, `a\{2`, `[a`, `bad\m`} {
		_, errb, code := runGrep(t, "", "x\n", pat)
		if code != 2 || errb == "" {
			t.Errorf("BRE reject %q: code=%d err=%q", pat, code, errb)
		}
	}
}

func TestGrepEREAndFixed(t *testing.T) {
	out, _, code := runGrep(t, "", "fooo\nbar\n", "-E", "fo+|bar")
	if out != "fooo\nbar\n" || code != 0 {
		t.Errorf("-E: out=%q code=%d", out, code)
	}
	_, errb, code := runGrep(t, "", "x\n", "-E", `(a)\1`)
	if code != 2 || !strings.Contains(errb, "back-reference") {
		t.Errorf("-E backref: code=%d err=%q", code, errb)
	}
	out, _, code = runGrep(t, "", "a.b\naxb\n", "-F", "a.b")
	if out != "a.b\n" || code != 0 {
		t.Errorf("-F: out=%q code=%d", out, code)
	}
	_, errb, code = runGrep(t, "", "x\n", "-E", "-F", "x")
	if code != 2 || !strings.Contains(errb, "conflicting matchers") {
		t.Errorf("conflicting matchers: code=%d err=%q", code, errb)
	}
}

func TestGrepMultiplePatterns(t *testing.T) {
	out, _, code := runGrep(t, "", "aaa\nbbb\nccc\n", "-e", "aaa", "-e", "ccc")
	if out != "aaa\nccc\n" || code != 0 {
		t.Errorf("-e -e: out=%q code=%d", out, code)
	}
	// newline-separated pattern list in one argument
	out, _, code = runGrep(t, "", "aaa\nbbb\nccc\n", "aaa\nccc")
	if out != "aaa\nccc\n" || code != 0 {
		t.Errorf("newline patterns: out=%q code=%d", out, code)
	}
}

func TestGrepRecursive(t *testing.T) {
	dir := setupTree(t)
	out, _, code := runGrep(t, dir, "", "-r", "foo", ".")
	if code != 0 {
		t.Fatalf("-r: code=%d", code)
	}
	for _, want := range []string{"./f1.txt:foo bar\n", "./sub/f3.go:foo bar baz\n", "Binary file ./bin.dat matches\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("-r missing %q in %q", want, out)
		}
	}
	out, _, _ = runGrep(t, dir, "", "-r", "--include=*.go", "foo", ".")
	if out != "./sub/f3.go:foo bar baz\n" {
		t.Errorf("--include: out=%q", out)
	}
	out, _, _ = runGrep(t, dir, "", "-r", "--exclude=*.go", "--exclude=*.dat", "foo", ".")
	if out != "./f1.txt:foo bar\n" {
		t.Errorf("--exclude: out=%q", out)
	}
	out, _, _ = runGrep(t, dir, "", "-r", "--exclude-dir=sub", "--exclude=*.dat", "foo", ".")
	if out != "./f1.txt:foo bar\n" {
		t.Errorf("--exclude-dir: out=%q", out)
	}
	// -r with no FILE operand searches "."
	out, _, code = runGrep(t, dir, "", "-r", "--include=*.go", "foo")
	if out != "./sub/f3.go:foo bar baz\n" || code != 0 {
		t.Errorf("-r default .: out=%q code=%d", out, code)
	}
}

func TestGrepBinary(t *testing.T) {
	dir := setupTree(t)
	out, _, code := runGrep(t, dir, "", "foo", "bin.dat")
	if out != "Binary file bin.dat matches\n" || code != 0 {
		t.Errorf("binary: out=%q code=%d", out, code)
	}
	// -c is exempt from the binary summary
	out, _, _ = runGrep(t, dir, "", "-c", "foo", "bin.dat")
	if out != "1\n" {
		t.Errorf("binary -c: out=%q", out)
	}
}

func TestGrepErrors(t *testing.T) {
	dir := setupTree(t)
	_, errb, code := runGrep(t, dir, "", "foo", "missing.txt")
	if code != 2 || !strings.Contains(errb, "missing.txt") {
		t.Errorf("missing file: code=%d err=%q", code, errb)
	}
	_, errb, code = runGrep(t, dir, "", "foo", "sub")
	if code != 2 || !strings.Contains(errb, "Is a directory") {
		t.Errorf("directory operand: code=%d err=%q", code, errb)
	}
	_, errb, code = runGrep(t, "", "")
	if code != 2 || !strings.Contains(errb, "missing pattern") {
		t.Errorf("missing operand: code=%d err=%q", code, errb)
	}
	_, errb, code = runGrep(t, "", "", "--frobnicate", "x")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestGrepCRPreserved(t *testing.T) {
	// GNU grep treats \r as line data: "foo\r" does not match foo$.
	_, _, code := runGrep(t, "", "foo\r\n", "-x", "foo")
	if code != 1 {
		t.Errorf("CR stripped: -x foo matched a CRLF line")
	}
}

func TestGrepHelpAndVersion(t *testing.T) {
	out, _, code := runGrep(t, "", "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: grep") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runGrep(t, "", "", "--version")
	if code != 0 || !strings.Contains(out, "grep") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}
