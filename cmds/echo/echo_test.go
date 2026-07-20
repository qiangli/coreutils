package echocmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestEcho(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"hello", "world"}, "hello world\n"},
		{[]string{}, "\n"},
		{[]string{"-n", "hello"}, "hello"},
		{[]string{"-n"}, ""},
		// -E is the default: backslashes literal.
		{[]string{`a\tb`}, "a\\tb\n"},
		{[]string{"-E", `a\nb`}, "a\\nb\n"},
		// -e escape set.
		{[]string{"-e", `a\tb`}, "a\tb\n"},
		{[]string{"-e", `a\nb`}, "a\nb\n"},
		{[]string{"-e", `\a\b\f\r\v\\`}, "\a\b\f\r\v\\\n"},
		{[]string{"-e", `\e[m`}, "\x1b[m\n"},
		// \c stops all output, including the trailing newline.
		{[]string{"-e", `ab\cde`, "never"}, "ab"},
		{[]string{"-e", "ab", `\cde`}, "ab"},
		{[]string{"-e", "ab", `c\cde`}, "ab c"},
		{[]string{"-e", "ab", "", `\cde`}, "ab "},
		{[]string{"-e", "ab", "cd", `\cde`}, "ab cd"},
		// Octal: \0NNN with 0-3 digits.
		{[]string{"-e", `\0101`}, "A\n"},
		{[]string{"-e", `\07`}, "\a\n"},
		{[]string{"-e", `\0`}, "\x00\n"},
		{[]string{"-e", `\01018`}, "A8\n"},
		// Hex: \xHH with 1-2 digits; bare \x is literal.
		{[]string{"-e", `\x41`}, "A\n"},
		{[]string{"-e", `\x9`}, "\t\n"},
		{[]string{"-e", `\x`}, "\\x\n"},
		{[]string{"-e", `\xzz`}, "\\xzz\n"},
		{[]string{"-e", `\x418`}, "A8\n"},
		// Unknown escape passes through.
		{[]string{"-e", `\q`}, "\\q\n"},
		// Trailing backslash is literal.
		{[]string{"-e", `a\`}, "a\\\n"},
		// Combined and repeated short options; later -e/-E wins.
		{[]string{"-en", `a\tb`}, "a\tb"},
		{[]string{"-e", "-E", `a\tb`}, "a\\tb\n"},
		{[]string{"-E", "-e", `a\tb`}, "a\tb\n"},
		// Anything not exactly a run of [neE] is an operand and stops
		// option scanning.
		{[]string{"-na", "x"}, "-na x\n"},
		{[]string{"--", "x"}, "-- x\n"},
		{[]string{"x", "-n"}, "x -n\n"},
		{[]string{"-"}, "-\n"},
		// --help / --version are literal unless the sole argument.
		{[]string{"--help", "x"}, "--help x\n"},
		{[]string{"--version", "x"}, "--version x\n"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, c.args...)
		if out != c.want || code != 0 || errb != "" {
			t.Errorf("echo %q = (%q, %q, %d), want (%q, \"\", 0)", c.args, out, errb, code, c.want)
		}
	}
}

func TestEchoHelpVersion(t *testing.T) {
	out, _, code := runTool(t, "--help")
	if code != 0 || !strings.Contains(out, "Usage: echo") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	for _, want := range []string{"-n", "-e", "-E", "--help", "--version"} {
		if !strings.Contains(out, want) {
			t.Errorf("--help missing %q in %q", want, out)
		}
	}
	out, _, code = runTool(t, "--version")
	if code != 0 || !strings.Contains(out, "echo") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}

func TestEchoPOSIXLYCORRECT(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"--help"}, "--help\n"},
		{[]string{"-ne", "foo"}, "-ne foo\n"},
		{[]string{"foo\\n"}, "foo\n\n"},
		{[]string{"-n", "-E", "foo\\cbar"}, "foo"},
	}
	for _, c := range cases {
		var out, errb bytes.Buffer
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			Dir:   t.TempDir(),
			Env:   []string{"POSIXLY_CORRECT="},
			Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
		}
		code := cmd.Run(rc, c.args)
		if out.String() != c.want || errb.Len() != 0 || code != 0 {
			t.Errorf("POSIXLY_CORRECT echo %q = (%q, %q, %d), want (%q, \"\", 0)", c.args, out.String(), errb.String(), code, c.want)
		}
	}
}
