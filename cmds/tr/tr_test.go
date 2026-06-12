package trcmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestTrTranslate(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		stdin string
		want  string
	}{
		{"simple", []string{"abc", "xyz"}, "aabbcc\n", "xxyyzz\n"},
		{"range to range", []string{"a-z", "A-Z"}, "Hello, World!\n", "HELLO, WORLD!\n"},
		{"lower class to upper class", []string{"[:lower:]", "[:upper:]"}, "MiXeD 123\n", "MIXED 123\n"},
		{"upper class to lower class", []string{"[:upper:]", "[:lower:]"}, "MiXeD 123\n", "mixed 123\n"},
		{"set2 extended with last char", []string{"abc", "x"}, "cabbage\n", "xxxxxge\n"},
		{"set2 longer than set1 ignored", []string{"ab", "xyz"}, "ab\n", "xy\n"},
		{"escape newline", []string{"\\n", " "}, "a\nb\n", "a b "},
		{"escape tab", []string{"\\t", " "}, "a\tb\n", "a b\n"},
		{"escaped backslash", []string{"\\\\", "/"}, "a\\b\n", "a/b\n"},
		{"octal escape", []string{"\\101", "x"}, "ABC\n", "xBC\n"},
		{"repeat count in set2", []string{"abcd", "[x*2]yz"}, "abcd\n", "xxyz\n"},
		{"repeat fill in set2", []string{"abcde", "[x*]z"}, "abcde\n", "xxxxz\n"},
		{"octal repeat count", []string{"abcdefghi", "[x*010]z"}, "abcdefghi\n", "xxxxxxxxz\n"},
		{"last duplicate in set1 wins", []string{"aa", "xy"}, "a\n", "y\n"},
		{"equivalence class", []string{"[=a=]", "x"}, "aaa\n", "xxx\n"},
		// complement of {a} includes \n, so the trailing newline maps too
		{"complement translate", []string{"-c", "a", "x"}, "abca\n", "axxax"},
		{"capital C complement", []string{"-C", "a", "x"}, "abca\n", "axxax"},
		{"literal dash after --", []string{"-d", "--", "-x"}, "a-xb\n", "ab\n"},
		{"aligned case classes inside sets", []string{"1[:lower:]", "2[:upper:]"}, "1ab\n", "2AB\n"},
		// 'x' is also a member of [:lower:]; the later mapping wins (GNU order)
		{"class overlap, last wins", []string{"x[:lower:]", "y[:upper:]"}, "xab\n", "XAB\n"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, c.stdin, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("%s: tr %v = (%q, %q, %d), want (%q, _, 0)", c.name, c.args, out, errb, code, c.want)
		}
	}
}

func TestTrDeleteSqueeze(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		stdin string
		want  string
	}{
		{"delete", []string{"-d", "ab"}, "abcabc\n", "cc\n"},
		{"delete class", []string{"-d", "[:digit:]"}, "a1b2c3\n", "abc\n"},
		{"delete range", []string{"-d", "a-c"}, "abcdef\n", "def\n"},
		{"delete complement", []string{"-dc", "a\\n"}, "abca\n", "aa\n"},
		{"squeeze one set", []string{"-s", "a"}, "aaabaaa\n", "aba\n"},
		{"squeeze class", []string{"-s", "[:space:]"}, "a  b\t\tc\n", "a b\tc\n"},
		{"squeeze only listed", []string{"-s", "a"}, "bbaab\n", "bbab\n"},
		{"translate then squeeze", []string{"-s", "abc", "x"}, "aabbcc\n", "x\n"},
		{"delete and squeeze", []string{"-ds", "a", "b"}, "abbba\n", "b\n"},
		// complement of {a} includes \n: b's and \n all map to x and squeeze
		{"squeeze complement", []string{"-cs", "a", "x"}, "abbba\n", "axax"},
		{"classic words idiom", []string{"-cs", "[:alpha:]", "\\n"}, "one two\tthree\n", "one\ntwo\nthree\n"},
		{"squeeze across lines", []string{"-s", "\\n"}, "a\n\n\nb\n", "a\nb\n"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, c.stdin, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("%s: tr %v = (%q, %q, %d), want (%q, _, 0)", c.name, c.args, out, errb, code, c.want)
		}
	}
}

func TestTrOperandErrors(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{}, "missing operand"},
		{[]string{"a"}, "missing operand after 'a'"},
		{[]string{"a"}, "Two strings must be given when translating."},
		{[]string{"a", "b", "c"}, "extra operand 'c'"},
		{[]string{"-d", "a", "b"}, "extra operand 'b'"},
		{[]string{"-d", "a", "b"}, "Only one string may be given when deleting without squeezing repeats."},
		{[]string{"-ds", "a"}, "Two strings must be given when both deleting and squeezing repeats."},
	}
	for _, c := range cases {
		_, errb, code := runTool(t, "", c.args...)
		if code != 2 || !strings.Contains(errb, c.want) {
			t.Errorf("tr %v: code=%d err=%q, want code=2 containing %q", c.args, code, errb, c.want)
		}
	}
}

func TestTrSetErrors(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"z-a", "x"}, "range-endpoints of 'z-a' are in reverse collating sequence order"},
		{[]string{"[:bogus:]", "x"}, "invalid character class 'bogus'"},
		{[]string{"a-z", "[:upper:]"}, "misaligned [:upper:] and/or [:lower:] construct"},
		{[]string{"[x*2]", "y"}, "the [c*] repeat construct may not appear in string1"},
		{[]string{"ab", "[x*][y*]"}, "only one [c*] repeat construct may appear in string2"},
		{[]string{"abc", "[:digit:]"}, "the only character classes that may appear in string2 are 'upper' and 'lower'"},
		{[]string{"abc", ""}, "when not truncating set1, string2 must be non-empty"},
		{[]string{"[:upper:][:lower:]", "[:lower:]"}, "the latter string must not end with a character class"},
		{[]string{"abcd", "w[:lower:]"}, "misaligned [:upper:] and/or [:lower:] construct"},
		{[]string{"-c", "a", "[:upper:]"}, "when translating with complemented character classes"},
		{[]string{"ab", "[x*08]"}, "invalid repeat count '08' in [c*n] construct"},
	}
	for _, c := range cases {
		_, errb, code := runTool(t, "", c.args...)
		if code != 1 || !strings.Contains(errb, c.want) {
			t.Errorf("tr %v: code=%d err=%q, want code=1 containing %q", c.args, code, errb, c.want)
		}
	}
}

func TestTrClasses(t *testing.T) {
	// every documented class is accepted in -d mode
	for _, cls := range []string{"alpha", "digit", "space", "upper", "lower",
		"alnum", "punct", "cntrl", "graph", "print", "xdigit", "blank"} {
		_, errb, code := runTool(t, "probe", "-d", "[:"+cls+":]")
		if code != 0 {
			t.Errorf("tr -d [:%s:]: code=%d err=%q", cls, code, errb)
		}
	}
	out, _, code := runTool(t, "a1 B\tc!", "-d", "[:punct:]")
	if out != "a1 B\tc" || code != 0 {
		t.Errorf("punct delete: out=%q code=%d", out, code)
	}
	out, _, code = runTool(t, "a1 B\tc", "-d", "[:blank:]")
	if out != "a1Bc" || code != 0 {
		t.Errorf("blank delete: out=%q code=%d", out, code)
	}
	out, _, code = runTool(t, "0aF9zG", "-d", "[:xdigit:]")
	if out != "zG" || code != 0 {
		t.Errorf("xdigit delete: out=%q code=%d", out, code)
	}
}

func TestTrUnknownFlag(t *testing.T) {
	_, errb, code := runTool(t, "", "--frobnicate", "a", "b")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
	// -t (truncate-set1) is deliberately not implemented
	_, errb, code = runTool(t, "", "-t", "a", "b")
	if code != 2 || !strings.Contains(errb, "t") {
		t.Errorf("-t: code=%d err=%q", code, errb)
	}
}

func TestTrHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: tr") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "", "--version")
	if code != 0 || !strings.Contains(out, "tr") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}
