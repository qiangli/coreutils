package dirnamecmd

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

func TestDirname(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"/usr/bin/sort"}, "/usr/bin\n"},
		{[]string{"stdio.h"}, ".\n"},
		{[]string{"/"}, "/\n"},
		{[]string{"//"}, "/\n"},
		{[]string{"/usr/"}, "/\n"},
		{[]string{"a/b/"}, "a\n"},
		{[]string{"a//b"}, "a\n"},
		{[]string{"dir/file"}, "dir\n"},
		// GNU does not clean: the "." component survives.
		{[]string{"a/./b"}, "a/.\n"},
		{[]string{""}, ".\n"},
		// multiple operands, one line each
		{[]string{"a/b", "/c/d", "e"}, "a\n/c\n.\n"},
		// -z: NUL terminators
		{[]string{"-z", "a/b", "c/d"}, "a\x00c\x00"},
		{[]string{"--zero", "a/b"}, "a\x00"},
	}
	for _, c := range cases {
		out, _, code := runTool(t, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("dirname %v = (%q, %d), want (%q, 0)", c.args, out, code, c.want)
		}
	}
}

func TestDirnameErrors(t *testing.T) {
	_, errb, code := runTool(t)
	if code != 2 || !strings.Contains(errb, "missing operand") {
		t.Errorf("no args: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "--frobnicate", "x")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestDirnameHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, "--help")
	if code != 0 || !strings.Contains(out, "Usage: dirname") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "--version")
	if code != 0 || !strings.Contains(out, "dirname") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}
