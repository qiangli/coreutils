package grepcmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// runGrep is the canonical test harness shape for cmds packages,
// extended with a working directory and stdin contents.
func runGrep(t *testing.T, dir, stdin string, args ...string) (stdout, stderr string, code int) {
	return runGrepEnv(t, dir, stdin, nil, args...)
}

func runGrepEnv(t *testing.T, dir, stdin string, env []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	if dir == "" {
		dir = t.TempDir()
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   env,
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

func TestGrepBREBackrefConformance(t *testing.T) {
	cases := []struct {
		name  string
		stdin string
		args  []string
		want  string
		code  int
	}{
		{
			name:  "literal caret in backref pattern",
			stdin: "a^bb\naXbb\n",
			args:  []string{`a^\(b\)\1`},
			want:  "a^bb\n",
			code:  0,
		},
		{
			name:  "gnu word class with backref",
			stdin: "__\n!!\nab\n",
			args:  []string{`\(\w\)\1`},
			want:  "__\n",
			code:  0,
		},
	}
	for _, c := range cases {
		out, _, code := runGrep(t, "", c.stdin, c.args...)
		if out != c.want || code != c.code {
			t.Errorf("%s: grep %v = (%q, %d), want (%q, %d)", c.name, c.args, out, code, c.want, c.code)
		}
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
		{`\<word\>`, "sword\nword\nwords\n", "word\n", 0},
		{`.*\<`, "!!!\nfoo\n", "foo\n", 0},
		{`\<\(.\)\1\>`, "-aa-\naaa\n", "-aa-\n", 0},
		{`\<[[:alnum:]_]\+\>`, "abc\n123\nx_y\n", "abc\n123\nx_y\n", 0},
		{`a\{0,2\}`, "bbb\naaa\n", "bbb\naaa\n", 0},
		{`^ba\{,2\}r$`, "br\nbar\nbaar\nbaaar\n", "br\nbar\nbaar\n", 0},
		{`^a\{2,\}$`, "a\naa\naaa\n", "aa\naaa\n", 0},
	}
	for _, c := range cases {
		out, _, code := runGrep(t, "", c.in, c.pat)
		if out != c.want || code != c.code {
			t.Errorf("BRE %q = (%q, %d), want (%q, %d)", c.pat, out, code, c.want, c.code)
		}
	}
	// rejected constructs: clear error, exit 2
	for _, pat := range []string{`a\{2`, `a\{,\}`, `a\{3,2\}`, `[a`, `bad\m`} {
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
	out, _, code = runGrep(t, "", "sword\nword\nwords\n", "-E", `\<word\>`)
	if out != "word\n" || code != 0 {
		t.Errorf("-E word edge: out=%q code=%d", out, code)
	}
	out, _, code = runGrep(t, "", "aa\nab\n", "-E", `(a)\1`)
	if out != "aa\n" || code != 0 {
		t.Errorf("-E backref: out=%q code=%d", out, code)
	}
	out, _, code = runGrep(t, "", "aba\nb\n", "-E", `(a*)b\1`)
	if out != "aba\nb\n" || code != 0 {
		t.Errorf("-E empty-capture backref: out=%q code=%d", out, code)
	}
	out, _, code = runGrep(t, "", ".\n\\\na\n", "-E", `[\.]`)
	if out != ".\n\\\n" || code != 0 {
		t.Errorf("-E POSIX bracket backslash literal: out=%q code=%d", out, code)
	}
	out, _, code = runGrep(t, "", "e\nx\n", "-E", `[[=e=]]`)
	if out != "e\n" || code != 0 {
		t.Errorf("-E equivalence class: out=%q code=%d", out, code)
	}
	var errb string
	out, _, code = runGrep(t, "", "a.b\naxb\n", "-F", "a.b")
	if out != "a.b\n" || code != 0 {
		t.Errorf("-F: out=%q code=%d", out, code)
	}
	_, errb, code = runGrep(t, "", "x\n", "-E", "-F", "x")
	if code != 2 || !strings.Contains(errb, "conflicting matchers") {
		t.Errorf("conflicting matchers: code=%d err=%q", code, errb)
	}
}

func TestGrepContext(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input", "zero\nbefore\nhit\nafter\ngap\nbefore2\nhit\nlast\ntail\n")
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "after separate argument",
			args: []string{"-n", "-A", "1", "hit", "input"},
			want: "3:hit\n4-after\n--\n7:hit\n8-last\n",
		},
		{
			name: "before attached argument",
			args: []string{"-n", "-B1", "hit", "input"},
			want: "2-before\n3:hit\n--\n6-before2\n7:hit\n",
		},
		{
			name: "both sides merge overlapping groups",
			args: []string{"-n", "-C2", "hit", "input"},
			want: "1-zero\n2-before\n3:hit\n4-after\n5-gap\n6-before2\n7:hit\n8-last\n9-tail\n",
		},
		{
			name: "later after overrides context trailing side",
			args: []string{"-n", "-C2", "-A1", "hit", "input"},
			want: "1-zero\n2-before\n3:hit\n4-after\n5-gap\n6-before2\n7:hit\n8-last\n",
		},
		{
			name: "later context overrides both sides",
			args: []string{"-n", "-A1", "-C2", "hit", "input"},
			want: "1-zero\n2-before\n3:hit\n4-after\n5-gap\n6-before2\n7:hit\n8-last\n9-tail\n",
		},
		{
			name: "max count retains trailing context",
			args: []string{"-n", "-m1", "-A2", "hit", "input"},
			want: "3:hit\n4-after\n5-gap\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runGrep(t, dir, "", tc.args...)
			if code != 0 || errOut != "" || out != tc.want {
				t.Errorf("grep %v = (%q, %q, %d), want (%q, empty, 0)",
					tc.args, out, errOut, code, tc.want)
			}
		})
	}
}

func TestGrepContextWithFilenamesAndOnlyMatching(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "one", "before\nhit\nafter\n")
	writeFile(t, dir, "two", "hit\n")

	out, errOut, code := runGrep(t, dir, "", "-n", "-A1", "hit", "one", "two")
	want := "one:2:hit\none-3-after\n--\ntwo:1:hit\n"
	if code != 0 || errOut != "" || out != want {
		t.Errorf("multi-file context = (%q, %q, %d), want (%q, empty, 0)", out, errOut, code, want)
	}

	out, errOut, code = runGrep(t, dir, "", "-o", "-A1", "hit", "one")
	if code != 0 || out != "hit\n" || !strings.Contains(errOut, "context options are ignored") {
		t.Errorf("-o -A1 = (%q, %q, %d), want match plus GNU warning", out, errOut, code)
	}
}

func TestGrepContextRejectsNegativeLength(t *testing.T) {
	_, errOut, code := runGrep(t, "", "hit\n", "--after-context=-1", "hit")
	if code != 2 || !strings.Contains(errOut, "context length") {
		t.Errorf("negative context = (%q, %d), want exit 2 naming context length", errOut, code)
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

func TestGrepPatternFileNamedDashAndCombinedLists(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "-", "aa\nbb\n")
	writeFile(t, dir, "patterns2", "aa\ncc\n")
	writeFile(t, dir, "input", "aaa111\nbbb222\nccc333\n")
	out, errOut, code := runGrep(t, dir, "", "-f", "-", "-f", "patterns2", "input")
	if code != 0 || errOut != "" || out != "aaa111\nbbb222\nccc333\n" {
		t.Errorf("combined -f with file named '-': (%q, %q, %d)", out, errOut, code)
	}
}

func TestGrepPOSIXOperandsStopOptionParsing(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "-i", "abc\n")
	writeFile(t, dir, "--", "def\n")
	writeFile(t, dir, "empty", "")
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"abc", "empty", "-i"}, "-i:abc\n"},
		{[]string{"def", "empty", "--"}, "--:def\n"},
	} {
		out, errOut, code := runGrepEnv(t, dir, "", []string{"POSIXLY_CORRECT=1"}, tc.args...)
		if code != 0 || errOut != "" || out != tc.want {
			t.Errorf("grep %q = (%q, %q, %d), want %q", tc.args, out, errOut, code, tc.want)
		}
	}
}

func TestGrepGNUDefaultPermutesOptionsAfterOperands(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input", "ABC\n")
	out, errOut, code := runGrep(t, dir, "", "abc", "input", "-i")
	if code != 0 || errOut != "" || out != "ABC\n" {
		t.Errorf("GNU option permutation = (%q, %q, %d), want case-folded match", out, errOut, code)
	}
}

func TestGrepEREAnchorPositions(t *testing.T) {
	for _, source := range []string{"argument", "-e", "-f"} {
		t.Run(source, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "patterns", "^^+\n")
			args := []string{"-E"}
			switch source {
			case "argument":
				args = append(args, "^^+")
			case "-e":
				args = append(args, "-e", "^^+")
			case "-f":
				args = append(args, "-f", "patterns")
			}
			out, errOut, code := runGrep(t, dir, ".^^^^^+++++\n^^^^^+++++\n", args...)
			if code != 0 || errOut != "" || out != "^^^^^+++++\n" {
				t.Errorf("ERE anchor via %s = (%q, %q, %d)", source, out, errOut, code)
			}
		})
	}
}

func TestGrepStandalonePreservesLexicalOperands(t *testing.T) {
	rc := &tool.RunContext{Dir: "/work", DirIsProcessCwd: true}
	for _, operand := range []string{"../input", "dir/../dir/input", "dir//input"} {
		if got := operandPath(rc, operand); got != operand {
			t.Errorf("operandPath(%q) = %q, want lexical spelling preserved", operand, got)
		}
	}
}

type grepFailWriter struct{ err error }

func (w grepFailWriter) Write([]byte) (int, error) { return 0, w.err }

func TestGrepOutputErrorIsDiagnosticStatusTwo(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"literal-buffer-flush", []string{"match"}},
		{"regexp-line", []string{"m.tch"}},
		{"files-with-match", []string{"-l", "match"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var errOut bytes.Buffer
			rc := &tool.RunContext{
				Ctx: context.Background(),
				Stdio: tool.Stdio{
					In:  strings.NewReader("match\n"),
					Out: grepFailWriter{err: errors.New("broken pipe")},
					Err: &errOut,
				},
			}
			if code := cmd.Run(rc, tc.args); code != 2 {
				t.Errorf("write error status = %d, want 2", code)
			}
			got := strings.ToLower(errOut.String())
			if !strings.Contains(got, "grep: write error: broken pipe") {
				t.Errorf("write error diagnostic = %q", got)
			}
			if n := strings.Count(got, "write error"); n != 1 {
				t.Errorf("write error diagnostic count = %d, want 1: %q", n, got)
			}
		})
	}
}

func TestGrepVSCLocalePrecedence(t *testing.T) {
	latin1Aumlaut := string([]byte{0xe4}) + "\n"
	cases := []struct {
		name string
		env  []string
		pat  string
		want bool
	}{
		{"posix-ctype", []string{"LANG=POSIX"}, "[[:alpha:]]", false},
		{"lang-ctype", []string{"LANG=de_DE.iso88591"}, "[[:alpha:]]", true},
		{"lc-ctype", []string{"LANG=POSIX", "LC_CTYPE=de_DE.iso88591"}, "[[:alpha:]]", true},
		{"lc-all-ctype", []string{"LANG=POSIX", "LC_CTYPE=POSIX", "LC_ALL=de_DE.iso88591"}, "[[:alpha:]]", true},
		{"lc-all-overrides", []string{"LANG=de_DE.iso88591", "LC_CTYPE=de_DE.iso88591", "LC_ALL=POSIX"}, "[[:alpha:]]", false},
		{"posix-collate", []string{"LANG=POSIX"}, "[[=a=]]", false},
		{"lang-collate", []string{"LANG=de_DE.iso88591"}, "[[=a=]]", true},
		{"lc-collate", []string{"LANG=POSIX", "LC_CTYPE=de_DE.iso88591", "LC_COLLATE=de_DE.iso88591"}, "[[=a=]]", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runGrepEnv(t, "", latin1Aumlaut, tc.env, tc.pat)
			wantCode := 1
			wantOut := ""
			if tc.want {
				wantCode, wantOut = 0, latin1Aumlaut
			}
			if code != wantCode || errOut != "" || out != wantOut {
				t.Errorf("locale grep = (%q, %q, %d), want (%q, '', %d)", out, errOut, code, wantOut, wantCode)
			}
		})
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
	out, _, _ = runGrep(t, dir, "", "-r", "--exclude=*.go", "--include=f3.go", "foo", ".")
	if out != "Binary file ./bin.dat matches\n./f1.txt:foo bar\n./sub/f3.go:foo bar baz\n" {
		t.Errorf("last matching include wins: out=%q", out)
	}
	out, _, _ = runGrep(t, dir, "", "-r", "--include=*.go", "--exclude=f3.go", "foo", ".")
	if out != "" {
		t.Errorf("last matching exclude wins: out=%q", out)
	}
	// -r with no FILE operand searches "."
	out, _, code = runGrep(t, dir, "", "-r", "--include=*.go", "foo")
	if out != "./sub/f3.go:foo bar baz\n" || code != 0 {
		t.Errorf("-r default .: out=%q code=%d", out, code)
	}
}

func TestGrepIncludeMatchesCommandLineNameSuffix(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "sub/input.go", "match\n")
	out, errOut, code := runGrep(t, dir, "", "--include=sub/*.go", "match", "sub/input.go")
	if code != 0 || errOut != "" || out != "match\n" {
		t.Errorf("command-line suffix include = (%q, %q, %d), want match", out, errOut, code)
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

func TestGrepPOSIXDiagnosticsAndPatternFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "patterns", "foo\nbar\n")
	writeFile(t, dir, "input", "foo\nbar\nbaz\n")

	out, errb, code := runGrep(t, dir, "", "-f", "patterns", "input")
	if out != "foo\nbar\n" || errb != "" || code != 0 {
		t.Errorf("-f: out=%q err=%q code=%d", out, errb, code)
	}

	out, _, code = runGrep(t, dir, "", "-e", "foo", "-f", "patterns", "input")
	if out != "foo\nbar\n" || code != 0 {
		t.Errorf("-e with -f: out=%q code=%d", out, code)
	}

	writeFile(t, dir, "empty-line-patterns", "\nfoo\n")
	out, _, code = runGrep(t, dir, "", "-f", "empty-line-patterns", "input")
	if out != "foo\nbar\nbaz\n" || code != 0 {
		t.Errorf("empty pattern line: out=%q code=%d", out, code)
	}

	_, errb, code = runGrep(t, dir, "", "-f", "missing-patterns", "input")
	if code != 2 || !strings.Contains(errb, "missing-patterns") {
		t.Errorf("missing pattern file: err=%q code=%d", errb, code)
	}
	_, errb, code = runGrep(t, dir, "", "-s", "foo", "missing-input")
	if code != 2 || errb != "" {
		t.Errorf("-s missing input: err=%q code=%d", errb, code)
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
	if code != 0 || !strings.Contains(out, "Usage: grep") ||
		!strings.Contains(out, "--after-context") || !strings.Contains(out, "--include") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runGrep(t, "", "", "--version")
	if code != 0 || !strings.Contains(out, "grep") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}

// POSIX behaviors that grep's regex layer previously got wrong.
func TestGrepPOSIXRegexConformance(t *testing.T) {
	cases := []struct {
		name  string
		stdin string
		args  []string
		want  string
		code  int
	}{
		// POSIX XBD 9.3.6: a back-reference to a group that matched the empty
		// string matches the empty string, so \(a*\) matching empty before "b"
		// lets \(a*\)b\1 match "b". This used to reject the line.
		{"backref to a group that matched empty", "b\n", []string{`\(a*\)b\1`}, "b\n", 0},
		{"...and still rejects a line with no b", "aaa\n", []string{`\(a*\)b\1`}, "", 1},
		{"non-empty backref still has to repeat", "aaxaa\naaxa\n", []string{`^\(a*\)x\1$`}, "aaxaa\n", 0},

		// POSIX XBD 9.1 leftmost-longest, observable through -w: -w asks whether
		// some match is bounded by non-word characters. Under leftmost-first,
		// `a\|ab` reports the "a" alternative, whose right edge sits against the
		// word character "b", so the line was wrongly rejected. POSIX reports the
		// longest match at that offset — "ab" — which is a whole word.
		{"-w with a shorter alternative first", "ab\n", []string{"-w", `a\|ab`}, "ab\n", 0},
		{"-w with a longer alternative later", "foobar\n", []string{"-w", `foo\|foobar`}, "foobar\n", 0},
		{"-w still rejects a genuine non-word match", "abc\n", []string{"-w", `a\|ab`}, "", 1},
		// -w over the back-reference engine.
		{"-w with a backref", "aa\n", []string{"-w", `\(a\)\1`}, "aa\n", 0},
		{"-w with a backref inside a longer word", "aab\n", []string{"-w", `\(a\)\1`}, "", 1},

		// Leftmost-longest must not change whether a line matches at all, only
		// which extent is reported: plain grep is unaffected.
		{"plain match existence is unchanged", "zab\n", []string{`a\|ab`}, "zab\n", 0},
		{"plain non-match is unchanged", "zzz\n", []string{`a\|ab`}, "", 1},
	}
	for _, c := range cases {
		out, errOut, code := runGrep(t, "", c.stdin, c.args...)
		if out != c.want || code != c.code {
			t.Errorf("%s: grep %v on %q = (%q, %d, err %q), want (%q, %d)",
				c.name, c.args, c.stdin, out, code, errOut, c.want, c.code)
		}
	}
}

func TestGrepOnlyMatching(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f1.txt", "abc 123 def 456\nno digits here\n")
	writeFile(t, dir, "f2.txt", "789 xyz\n")

	cases := []struct {
		name  string
		stdin string
		dir   string
		args  []string
		want  string
		code  int
	}{
		{
			name:  "basic match",
			stdin: "foo\nbar\n",
			args:  []string{"-o", "o"},
			want:  "o\no\n",
			code:  0,
		},
		{
			name:  "match with line numbers",
			stdin: "foo\nbar\n",
			args:  []string{"-o", "-n", "o"},
			want:  "1:o\n1:o\n",
			code:  0,
		},
		{
			name: "regex match with multiple files",
			dir:  dir,
			args: []string{"-o", "-E", "[0-9]+", "f1.txt", "f2.txt"},
			want: "f1.txt:123\nf1.txt:456\nf2.txt:789\n",
			code: 0,
		},
		{
			name:  "match with -w",
			stdin: "foobar foo\n",
			args:  []string{"-o", "-w", "foo"},
			want:  "foo\n",
			code:  0,
		},
		{
			name:  "match with -v",
			stdin: "foo\nbar\n",
			args:  []string{"-o", "-v", "foo"},
			want:  "bar\n",
			code:  0,
		},
		{
			name:  "empty match prevention",
			stdin: "foo\n",
			args:  []string{"-o", "o*"},
			want:  "oo\n",
			code:  0,
		},
		{
			name:  "quiet overrides only-matching",
			stdin: "foo\nbar\n",
			args:  []string{"-o", "-q", "o"},
			want:  "",
			code:  0,
		},
		{
			name:  "count overrides only-matching",
			stdin: "foo\nbar\n",
			args:  []string{"-o", "-c", "o"},
			want:  "1\n",
			code:  0,
		},
		{
			name: "files-with-matches overrides only-matching",
			dir:  dir,
			args: []string{"-o", "-l", "123", "f1.txt", "f2.txt"},
			want: "f1.txt\n",
			code: 0,
		},
	}
	for _, c := range cases {
		out, errOut, code := runGrep(t, c.dir, c.stdin, c.args...)
		if out != c.want || code != c.code {
			t.Errorf("%s: grep %v = (%q, %d, err %q), want (%q, %d)",
				c.name, c.args, out, code, errOut, c.want, c.code)
		}
	}
}

func TestGrepOnlyMatchingUsesLeftmostLongest(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "basic alternation", args: []string{"-o", `a\|ab`}},
		{name: "extended alternation", args: []string{"-o", "-E", `a|ab`}},
		{name: "separate expressions", args: []string{"-o", "-e", "a", "-e", "ab"}},
		{name: "fixed expressions", args: []string{"-o", "-F", "-e", "a", "-e", "ab"}},
	}
	for _, c := range cases {
		out, errOut, code := runGrep(t, "", "ab ab\n", c.args...)
		if out != "ab\nab\n" || errOut != "" || code != 0 {
			t.Errorf("%s: grep %v = (%q, %q, %d), want (%q, %q, 0)",
				c.name, c.args, out, errOut, code, "ab\nab\n", "")
		}
	}
}
