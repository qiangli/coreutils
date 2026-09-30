package m4cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runM4(t *testing.T, input string, args ...string) (string, string, int) {
	t.Helper()
	var out, err bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &err}, FS: tool.NewLocalFS()}
	code := run(rc, args)
	return out.String(), err.String(), code
}

func runM4Context(t *testing.T, dir string, env []string, input string, args ...string) (string, string, int) {
	t.Helper()
	var out, err bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: dir, Env: env,
		Stdio: tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &err}, FS: tool.NewLocalFS(),
	}
	code := run(rc, args)
	return out.String(), err.String(), code
}

type m4Case struct {
	name string
	in   string
	want string
}

func runTable(t *testing.T, cases []m4Case) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runM4(t, tc.in)
			if code != 0 {
				t.Fatalf("code=%d stderr=%q", code, errOut)
			}
			if out != tc.want {
				t.Fatalf("input %q\nstdout=%q\n  want=%q (stderr=%q)", tc.in, out, tc.want, errOut)
			}
		})
	}
}

func TestPlainTextAndQuotes(t *testing.T) {
	runTable(t, []m4Case{
		{"plain text is copied", "hello, world (1 + 2)\n", "hello, world (1 + 2)\n"},
		{"one quote level is stripped", "`hello' ``nested''\n", "hello `nested'\n"},
		{"quoted macro name is not expanded", "define(`x', `y')`x' x\n", "x y\n"},
		{"empty quotes split a name", "define(`x', `y')x`'x a`'x ax\n", "yy ay ax\n"},
		{"undefined name followed by paren", "foo(a, b)\n", "foo(a, b)\n"},
		{"quoted newline is kept", "`a\nb'\n", "a\nb\n"},
		{"a name starts at a letter or underscore", "define(`x', `y')1x x1 _x x\n", "1y x1 _x y\n"},
	})
}

func TestDefine(t *testing.T) {
	runTable(t, []m4Case{
		{"simple", "define(`foo', `bar')foo\n", "bar\n"},
		{"empty value", "define(`foo')[foo]\n", "[]\n"},
		{"redefine replaces", "define(`a', `1')define(`a', `2')a\n", "2\n"},
		{"positional parameters", "define(`f', `$2-$1')f(`a', `b')\n", "b-a\n"},
		{"missing parameters are empty", "define(`f', `[$1][$2][$9]')f(`a')\n", "[a][][]\n"},
		{"only one digit is a parameter", "define(`f', `$10')f(`a')\n", "a0\n"},
		{"argument count", "define(`n', `$#')n n() n(a) n(a,b,c)\n", "0 1 1 3\n"},
		{"dollar star", "define(`f', `$*')f(`a', `b', `c')\n", "a,b,c\n"},
		{"dollar at keeps quoting", "define(`x', `X')define(`f', `$@')define(`g', `$*')f(`x', `b') g(`x', `b')\n", "x,b X,b\n"},
		{"dollar zero is the name", "define(`f', ``$0'')f f(1)\n", "f f\n"},
		{"lone dollar is literal", "define(`f', `$ $x $')f\n", "$ $x $\n"},
		{"define without parens is text", "define\n", "define\n"},
		{"empty name is ignored", "define(`', `x')ok\n", "ok\n"},
		{"unquoted name is expanded first", "define(`a', `b')define(a, `c')a b\n", "c c\n"},
	})
}

func TestRescanAndArgumentCollection(t *testing.T) {
	runTable(t, []m4Case{
		{"expansion is rescanned", "define(`a', `b')define(`b', `c')a\n", "c\n"},
		{"quoted expansion stops rescan", "define(`b', `c')define(`a', ``b'')a\n", "b\n"},
		{"leading whitespace is skipped", "define(`g', `[$1][$2]')g(  a  ,  \n b )\n", "[a  ][b ]\n"},
		{"parens protect commas", "define(`g', `[$1][$2]')g((a,b),c)\n", "[(a,b)][c]\n"},
		{"expansion with parens in argument", "define(`q', `(1,2)')define(`g', `[$1][$2]')g(q,3)\n", "[(1,2)][3]\n"},
		{"expansion commas split arguments", "define(`q', `1,2')define(`g', `[$1][$2]')g(q)\n", "[1][2]\n"},
		{"quoted comma does not split", "define(`g', `[$1][$2]')g(`a,b', c)\n", "[a,b][c]\n"},
		{"nested calls in arguments", "define(`f', `<$1>')f(f(f(x)))\n", "<<<x>>>\n"},
		{"paren must follow immediately", "define(`f', `<$1>')f (x)\n", "<> (x)\n"},
		{"comment inside argument", "define(`g', `[$1][$2]')g(# c,\nd)\n", "[# c,\nd][]\n"},
		{"macro seen while collecting", "define(`x', `1')define(`y', `x`'2')y\n", "12\n"},
	})
}

func TestUndefinePushdefPopdef(t *testing.T) {
	runTable(t, []m4Case{
		{"undefine", "define(`a', `1')undefine(`a')a\n", "a\n"},
		{"undefine a builtin", "undefine(`len')len(abc)\n", "len(abc)\n"},
		{"pushdef and popdef", "define(`a', `1')pushdef(`a', `2')a popdef(`a')a popdef(`a')a\n", "2 1 a\n"},
		{"define replaces only the top", "pushdef(`a', `1')pushdef(`a', `2')define(`a', `3')a popdef(`a')a\n", "3 1\n"},
		{"undefine drops the whole stack", "pushdef(`a', `1')pushdef(`a', `2')undefine(`a')a\n", "a\n"},
		{"popdef of undefined name", "popdef(`zz')ok\n", "ok\n"},
		{"bare names are text", "undefine pushdef popdef\n", "undefine pushdef popdef\n"},
	})
}

func TestDefn(t *testing.T) {
	runTable(t, []m4Case{
		{"quoted definition", "define(`a', `b')define(`b', `c')define(`d', defn(`a'))d defn(`a')\n", "c b\n"},
		{"definition is not expanded in define", "define(`x', `X')define(`a', `x')define(`b', defn(`a'))undefine(`x')b\n", "x\n"},
		{"rename a builtin", "define(`mydef', defn(`define'))mydef(`b', `c')b\n", "c\n"},
		{"builtin definition is void as text", "[defn(`define')]\n", "[]\n"},
		{"undefined name", "[defn(`nope')]\n", "[]\n"},
	})
}

func TestIfdef(t *testing.T) {
	runTable(t, []m4Case{
		{"defined", "define(`a')ifdef(`a', `yes', `no')\n", "yes\n"},
		{"undefined", "ifdef(`a', `yes', `no')\n", "no\n"},
		{"builtin is defined", "ifdef(`define', `yes', `no')\n", "yes\n"},
		{"no else branch", "[ifdef(`nope', `yes')]\n", "[]\n"},
		{"branch is rescanned", "define(`a', `A')ifdef(`a', `a', `b')\n", "A\n"},
	})
}

func TestIfelse(t *testing.T) {
	runTable(t, []m4Case{
		{"equal", "ifelse(`a', `a', `yes', `no')\n", "yes\n"},
		{"not equal", "ifelse(`a', `b', `yes', `no')\n", "no\n"},
		{"three arguments", "[ifelse(`a', `b', `yes')]\n", "[]\n"},
		{"one argument is a comment", "[ifelse(`ignored')]\n", "[]\n"},
		{"multi branch", "ifelse(`a', `b', `1', `c', `c', `2', `3')\n", "2\n"},
		{"multi branch default", "ifelse(`a', `b', `1', `c', `d', `2', `3')\n", "3\n"},
		{"multi branch no default", "[ifelse(`a', `b', `1', `c', `d', `2')]\n", "[]\n"},
		{"arguments are expanded before comparing", "define(`x', `a')ifelse(x, `a', `yes', `no')\n", "yes\n"},
		{"bounded recursion", "define(`down', `$1`'ifelse($1, `0', `', ` down(decr($1))')')down(3)\n", "3 2 1 0\n"},
	})
}

func TestShift(t *testing.T) {
	runTable(t, []m4Case{
		{"drops the first argument", "shift(`a', `b', `c')\n", "b,c\n"},
		{"single argument", "[shift(`a')]\n", "[]\n"},
		{"result is quoted", "define(`b', `B')define(`f', `$1')f(shift(`a', `b'))\n", "B\n"},
		{"quoted comma survives", "define(`n', `$#')n(shift(`a', `b,c', `d'))\n", "2\n"},
		{"reverse with shift", "define(`rev', `ifelse($#, `1', `$1', `rev(shift($@)),$1')')rev(`a', `b', `c')\n", "c,b,a\n"},
	})
}

func TestDnl(t *testing.T) {
	runTable(t, []m4Case{
		{"discards through newline", "a dnl comment\nb\n", "a b\n"},
		{"after a definition", "define(`a', `1')dnl\na\n", "1\n"},
		{"quoted dnl is text", "`dnl' x\n", "dnl x\n"},
		{"at end of input", "a dnl no newline", "a "},
		{"from an expansion", "define(`d', `dnl')a d junk\nb\n", "a b\n"},
	})
}

func TestLenIndexSubstr(t *testing.T) {
	runTable(t, []m4Case{
		{"len", "len(`') len(`abcdef') len(`a b')\n", "0 6 3\n"},
		{"len bare", "len\n", "len\n"},
		{"index", "index(`gnus, gnats', `nat') index(`abc', `x') index(`abc', `') index(`abc', `abc')\n", "7 -1 0 0\n"},
		{"substr", "substr(`abcdef', `2') substr(`abcdef', `2', `3') substr(`abcdef', `0', `1')\n", "cdef cde a\n"},
		{"substr out of range", "[substr(`abc', `5')][substr(`abc', `-1')][substr(`abc', `1', `-1')][substr(`abc', `3')]\n", "[][][][]\n"},
		{"substr length past end", "substr(`abc', `1', `99')\n", "bc\n"},
		{"substr result is rescanned", "define(`bc', `X')substr(`abc', `1')\n", "X\n"},
	})
}

func TestTranslit(t *testing.T) {
	runTable(t, []m4Case{
		{"replace", "translit(`hello', `el', `ip')\n", "hippo\n"},
		{"delete when no replacement", "translit(`hello', `l')\n", "heo\n"},
		{"shorter replacement deletes the rest", "translit(`abcdef', `abcd', `xy')\n", "xyef\n"},
		{"no source characters", "translit(`abc', `', `x')\n", "abc\n"},
		{"extra replacement characters are unused", "translit(`abc', `a', `xyz')\n", "xbc\n"},
		{"swap", "translit(`abba', `ab', `ba')\n", "baab\n"},
	})
}

func TestIncrDecr(t *testing.T) {
	runTable(t, []m4Case{
		{"incr", "incr(`4') incr(`-1') incr(`0')\n", "5 0 1\n"},
		{"decr", "decr(`4') decr(`0') decr(`-7')\n", "3 -1 -8\n"},
		{"nested", "incr(incr(decr(`10')))\n", "11\n"},
		{"bare names are text", "incr decr\n", "incr decr\n"},
	})
}

func TestEval(t *testing.T) {
	runTable(t, []m4Case{
		{"precedence", "eval(`1 + 2 * 3') eval(`(1 + 2) * 3') eval(`7 - 2 - 1')\n", "7 9 4\n"},
		{"division and modulo truncate", "eval(`7 / 2') eval(`-7 / 2') eval(`7 % 3') eval(`-7 % 3')\n", "3 -3 1 -1\n"},
		{"unary", "eval(`-3') eval(`+3') eval(`- -3') eval(`!0') eval(`!5') eval(`~0') eval(`-(2 + 3)')\n", "-3 3 3 1 0 -1 -5\n"},
		{"relational", "eval(`1 < 2') eval(`2 <= 1') eval(`3 > 2') eval(`2 >= 3') eval(`2 == 2') eval(`2 != 2')\n", "1 0 1 0 1 0\n"},
		{"bitwise", "eval(`6 & 3') eval(`6 | 3') eval(`6 ^ 3') eval(`1 << 4') eval(`-16 >> 2')\n", "2 7 5 16 -4\n"},
		{"logical", "eval(`2 && 3') eval(`2 && 0') eval(`0 || 3') eval(`0 || 0')\n", "1 0 1 0\n"},
		{"logical short circuit", "eval(`0 && 1 / 0') eval(`1 || 1 / 0')\n", "0 1\n"},
		{"mixed precedence", "eval(`1 | 2 ^ 3 & 4') eval(`1 + 2 << 3') eval(`1 < 2 == 1') eval(`1 || 0 && 0')\n", "3 24 1 1\n"},
		{"octal and hex constants", "eval(`010') eval(`0x1f') eval(`0X10 + 1')\n", "8 31 17\n"},
		{"radix", "eval(`255', `16') eval(`8', `8') eval(`5', `2') eval(`-5', `2')\n", "ff 10 101 -101\n"},
		{"minimum width", "eval(`5', `10', `3') eval(`-5', `2', `8') eval(`123', `10', `2')\n", "005 -00000101 123\n"},
		{"32-bit wraparound", "eval(`2147483647 + 1')\n", "-2147483648\n"},
		{"arguments are expanded", "define(`n', `6')eval(n * 7)\n", "42\n"},
		{"bare eval is text", "eval\n", "eval\n"},
	})
}

func TestEvalErrors(t *testing.T) {
	for _, in := range []string{"[eval(`1 / 0')]\n", "[eval(`1 % 0')]\n", "[eval(`1 +')]\n", "[eval(`2 ** 3')]\n", "[eval(`(1')]\n", "[eval(`1 2')]\n", "[eval(`1', `37')]\n", "[incr(`x')]\n"} {
		out, errOut, _ := runM4(t, in)
		if out != "[]\n" {
			t.Errorf("%q: stdout=%q, want empty expansion", in, out)
		}
		if !strings.HasPrefix(errOut, "m4:stdin:1: ") {
			t.Errorf("%q: stderr=%q, want a located diagnostic", in, errOut)
		}
	}
}

func TestChangequote(t *testing.T) {
	runTable(t, []m4Case{
		{"new delimiters", "changequote([, ])define([x], [y])[x] x `x'\n", "x y `y'\n"},
		{"multi-character delimiters", "changequote(`<<', `>>')define(<<x>>, <<y>>)<<x>> x <<a<<b>>c>>\n", "x y a<<b>>c\n"},
		{"no arguments restores the defaults", "changequote([, ])changequote`x' [x]\n", "x [x]\n"},
		{"missing end quote defaults", "changequote([)[foo' x\n", "foo x\n"},
		{"dollar at uses the current quotes", "define(`f', `$@')changequote([, ])define([x], [X])f([x])\n", "x\n"},
		{"same start and end do not nest", "changequote(`\"', `\"')\"a\" \"\"b\n", "a b\n"},
		{"empty start disables quoting", "changequote(,)`foo'\n", "`foo'\n"},
	})
}

func TestChangecom(t *testing.T) {
	runTable(t, []m4Case{
		{"default comment is copied unexpanded", "define(`x', `y')x # x `x'\nx\n", "y # x `x'\ny\n"},
		{"new delimiters", "define(`x', `y')changecom(`/*', `*/')/* x\n x */ x # x\n", "/* x\n x */ y # y\n"},
		{"no arguments disables comments", "define(`x', `y')changecom # x\n", " # y\n"},
		{"start only ends at newline", "define(`x', `y')changecom(`;')x ; x\nx # x\n", "y ; x\ny # y\n"},
		{"comment wins over a macro name", "define(`abc', `X')changecom(`a')abc zabc\nabc\n", "abc zabc\nabc\n"},
	})
}

func TestUnterminatedInput(t *testing.T) {
	for _, tc := range []struct{ in, wantOut, wantErr string }{
		{"a `abc", "a ", "end of file in string"},
		{"define(`foo', `x')foo(a", "", "end of file in argument list"},
	} {
		out, errOut, code := runM4(t, tc.in)
		if code != 1 || out != tc.wantOut || !strings.Contains(errOut, tc.wantErr) {
			t.Errorf("%q: code=%d stdout=%q stderr=%q", tc.in, code, out, errOut)
		}
	}
}

func TestOptionsDefineUndefine(t *testing.T) {
	for _, tc := range []struct {
		args []string
		in   string
		want string
	}{
		{[]string{"-D", "foo=bar"}, "foo\n", "bar\n"},
		{[]string{"-Dfoo"}, "[foo]\n", "[]\n"},
		{[]string{"-Dfoo=a=b"}, "foo\n", "a=b\n"},
		{[]string{"-Dfoo=1", "-Ufoo"}, "foo\n", "foo\n"},
		{[]string{"-Ufoo", "-Dfoo=1"}, "foo\n", "1\n"},
		{[]string{"-U", "len"}, "len(abc)\n", "len(abc)\n"},
		{[]string{"-Dx=y", "-"}, "x\n", "y\n"},
	} {
		out, errOut, code := runM4(t, tc.in, tc.args...)
		if code != 0 || out != tc.want {
			t.Errorf("%v: code=%d stdout=%q want %q stderr=%q", tc.args, code, out, tc.want, errOut)
		}
	}
}

func TestFileOperands(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.m4")
	b := filepath.Join(dir, "b.m4")
	if err := os.WriteFile(a, []byte("define(`x', `from a')dnl\nA x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("B x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := runM4(t, "S x\n", a, "-", b)
	if code != 0 || out != "A from a\nS from a\nB from a\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}

	out, errOut, code = runM4(t, "", filepath.Join(dir, "missing.m4"), b)
	if code != 1 || out != "B x\n" || !strings.Contains(errOut, "missing.m4") {
		t.Fatalf("missing operand: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestSyncLines(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "s.m4")
	if err := os.WriteFile(f, []byte("a\ndefine(`x',`1\n2')dnl\nx\n\nb x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := runM4(t, "foo\n", "-s", "-", f)
	want := "#line 1 \"stdin\"\nfoo\n#line 1 \"" + f + "\"\na\n#line 4\n1\n#line 4\n2\n\nb 1\n#line 6\n2\n"
	if code != 0 || out != want {
		t.Fatalf("code=%d stderr=%q\nstdout=%q\n  want=%q", code, errOut, out, want)
	}
}

func TestUsageErrors(t *testing.T) {
	if _, errOut, code := runM4(t, "", "--no-such-option"); code != 2 || errOut == "" {
		t.Fatalf("unknown option: code=%d stderr=%q", code, errOut)
	}
	if out, _, code := runM4(t, "", "--help"); code != 0 || !strings.Contains(out, "m4 [-s]") {
		t.Fatalf("--help: code=%d stdout=%q", code, out)
	}
}

func TestDiversions(t *testing.T) {
	runTable(t, []m4Case{
		{"ordered at eof", "a divert(2)two\ndivert(1)one\ndivert(0)b\n", "a b\none\ntwo\n"},
		{"undivert", "divert(1)x\ndivert(0)[undivert(1)]\n", "[x\n]\n"},
		{"undivert into current", "divert(1)ONE divert(2)TWO undivert(1) END divert(0)", "TWO ONE  END "},
		{"undivert self", "divert(1)SELF undivert(1) END divert(0)", "SELF  END "},
		{"undivert into discard consumes source", "divert(1)ONE divert(-1)undivert(1)divert(0) END\n", " END\n"},
		{"negative discards", "a divert(-1)no divert(0)b divnum()\n", "a b 0\n"},
	})
}

func TestIncludeAndSinclude(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "in.m4")
	if err := os.WriteFile(f, []byte("define(`x', `OK')x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := runM4(t, "include(`"+f+"')sinclude(`"+filepath.Join(dir, "missing")+"')")
	if code != 0 || out != "OK\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestIncludeTracksDiagnosticAndSynclineIdentity(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "inc.m4"), []byte("inside\n[eval(`bad')]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := runM4Context(t, dir, nil, "before\ninclude(`inc.m4')after\n", "-s")
	if code != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	for _, want := range []string{`#line 1 "inc.m4"`, `#line 2 "stdin"`} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout=%q missing %q", out, want)
		}
	}
	if !strings.Contains(errOut, "m4:inc.m4:2:") {
		t.Fatalf("stderr=%q does not identify included file and line", errOut)
	}
}

func TestSyscmdSysvalErrprintDumpdefTrace(t *testing.T) {
	out, errOut, code := runM4(t, "syscmd(`printf hi') sysval() errprint(`ERR') define(`x', `y')dumpdef(`x') traceon(`eval')eval(`1') traceoff(`eval')eval(`1')\n")
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	if out != "hi 0   1 1\n" {
		t.Fatalf("stdout=%q stderr=%q", out, errOut)
	}
	for _, want := range []string{"ERR", "x:\ty", "m4trace:"} {
		if !strings.Contains(errOut, want) {
			t.Fatalf("stderr=%q missing %q", errOut, want)
		}
	}
}

func TestSyscmdUsesRunContextAndPreservesOutputOrder(t *testing.T) {
	dir := t.TempDir()
	out, errOut, code := runM4Context(t, dir, []string{"M4_MARK=carried", "PATH="},
		"before-syscmd(`printf %s \"$M4_MARK\"; printf :; pwd')after")
	want := "before-carried:" + dir + "\nafter"
	if code != 0 || out != want || errOut != "" {
		t.Fatalf("code=%d stdout=%q want=%q stderr=%q", code, out, want, errOut)
	}
}

func TestMaketempMkstempWrapExit(t *testing.T) {
	dir := t.TempDir()
	tmpl := filepath.Join(dir, "aXXXXXX")
	out, errOut, code := runM4(t, "m4wrap(`wrapped')maketemp(`"+tmpl+"')\n")
	wantName := strings.TrimSuffix(tmpl, "XXXXXX") + strconv.Itoa(os.Getpid())
	if code != 0 || out != wantName+"\nwrapped" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	if _, err := os.Stat(wantName); !os.IsNotExist(err) {
		t.Fatalf("maketemp created file %q err=%v", wantName, err)
	}
	out, errOut, code = runM4(t, "mkstemp(`"+filepath.Join(dir, "bXXXXXX")+"')")
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("mkstemp did not create %q: %v", out, err)
	}
	out, errOut, code = runM4Context(t, dir, nil, "mkstemp(`relativeXXXXXX')")
	if code != 0 || filepath.IsAbs(out) {
		t.Fatalf("relative mkstemp: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	if _, err := os.Stat(filepath.Join(dir, out)); err != nil {
		t.Fatalf("relative mkstemp did not create %q: %v", out, err)
	}
	out, errOut, code = runM4Context(t, dir, nil, "[mkstemp(`bad-template')]")
	if code != 1 || out != "[]" || !strings.Contains(errOut, "template must end in XXXXXX") {
		t.Fatalf("invalid mkstemp: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	_, _, code = runM4(t, "before m4exit(`7') after")
	if code != 7 {
		t.Fatalf("m4exit code=%d", code)
	}
}

func TestWrapIsFIFOAndExitSkipsWrapsAndDiversions(t *testing.T) {
	out, errOut, code := runM4(t, "A m4wrap(`W1')m4wrap(`W2') Z\n")
	if code != 0 || out != "A  Z\nW1W2" || errOut != "" {
		t.Fatalf("wrap: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	out, errOut, code = runM4(t, "before divert(1)DIV m4wrap(`WRAP')m4exit(`7') after")
	if code != 7 || out != "before " || errOut != "" {
		t.Fatalf("exit: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestRegistered(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{In: strings.NewReader("define(`a', `ok')a\n"), Out: &out, Err: &errb}, FS: tool.NewLocalFS()}
	registered := tool.Lookup("m4")
	if registered == nil {
		t.Fatal("m4 is not registered")
	}
	if code := registered.Run(rc, nil); code != 0 || out.String() != "ok\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), errb.String())
	}
}
