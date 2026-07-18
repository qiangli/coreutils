package basenamecmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// runTool is the canonical test harness shape for cmds packages.
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

func TestBasename(t *testing.T) {
	cases := []struct {
		args []string
		want string
		code int
	}{
		{[]string{"/usr/bin/sort"}, "sort\n", 0},
		{[]string{"include/stdio.h", ".h"}, "stdio\n", 0},
		{[]string{"-s", ".h", "include/stdio.h"}, "stdio\n", 0},
		{[]string{"-a", "any/str1", "any/str2"}, "str1\nstr2\n", 0},
		// An empty suffix is still an explicit -s, which implies -a.
		{[]string{"-s", "", "any/str1", "any/str2"}, "str1\nstr2\n", 0},
		{[]string{"--suffix", "", "any/str1", "any/str2"}, "str1\nstr2\n", 0},
		{[]string{"--suffix=", "any/str1", "any/str2"}, "str1\nstr2\n", 0},
		{[]string{"/usr/lib/"}, "lib\n", 0},
		{[]string{"/"}, "/\n", 0},
		{[]string{"-z", "a/b"}, "b\x00", 0},
		// suffix equal to the whole name is NOT removed (GNU rule)
		{[]string{".h", ".h"}, ".h\n", 0},
	}
	for _, c := range cases {
		out, _, code := runTool(t, c.args...)
		if out != c.want || code != c.code {
			t.Errorf("basename %v = (%q, %d), want (%q, %d)", c.args, out, code, c.want, c.code)
		}
	}
}

func TestBasenameErrors(t *testing.T) {
	_, errb, code := runTool(t)
	if code != 2 || !strings.Contains(errb, "missing operand") {
		t.Errorf("no args: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "a", "b", "c")
	if code != 2 || !strings.Contains(errb, "extra operand") {
		t.Errorf("3 args: code=%d err=%q", code, errb)
	}
	// Unknown flag: contract error, exit 2, names the flag.
	_, errb, code = runTool(t, "--frobnicate", "x")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, "--help")
	if code != 0 || !strings.Contains(out, "Usage: basename") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	for _, flag := range []string{"-h, --help", "-V, --version"} {
		if !strings.Contains(out, flag) {
			t.Errorf("--help output missing %q:\n%s", flag, out)
		}
	}
	out, _, code = runTool(t, "-h")
	if code != 0 || !strings.Contains(out, "Usage: basename") {
		t.Errorf("-h: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "--version")
	if code != 0 || !strings.Contains(out, "basename") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "-V")
	if code != 0 || !strings.Contains(out, "basename") {
		t.Errorf("-V: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "-azV", "a/b")
	if code != 0 || !strings.Contains(out, "basename") {
		t.Errorf("-azV: code=%d out=%q", code, out)
	}
}
