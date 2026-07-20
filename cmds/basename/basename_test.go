package basenamecmd

import (
	"bytes"
	"context"
	"errors"
	"io"
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

type failingWriter struct {
	n   int
	err error
}

func (w failingWriter) Write(p []byte) (int, error) {
	if w.n > len(p) {
		return len(p), w.err
	}
	return w.n, w.err
}

type failAfterWriter struct {
	okWrites int
	err      error
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	if w.okWrites > 0 {
		w.okWrites--
		return len(p), nil
	}
	return 0, w.err
}

func TestBasenameWriteErrors(t *testing.T) {
	tests := []struct {
		name string
		out  io.Writer
	}{
		{"error", failingWriter{err: errors.New("output unavailable")}},
		{"short write", failingWriter{n: 1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var errb bytes.Buffer
			rc := &tool.RunContext{
				Ctx:   context.Background(),
				Dir:   t.TempDir(),
				Stdio: tool.Stdio{In: strings.NewReader(""), Out: tc.out, Err: &errb},
			}
			if code := cmd.Run(rc, []string{"a/b"}); code != 1 {
				t.Errorf("write failure: code=%d, want 1", code)
			}
			if !strings.Contains(errb.String(), "basename: write error:") {
				t.Errorf("write failure: stderr=%q, want diagnostic", errb.String())
			}
		})
	}
}

func TestBasenameWriteErrorAfterMultipleOutput(t *testing.T) {
	var errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(),
		Dir: t.TempDir(),
		Stdio: tool.Stdio{
			In:  strings.NewReader(""),
			Out: &failAfterWriter{okWrites: 1, err: errors.New("output unavailable")},
			Err: &errb,
		},
	}
	if code := cmd.Run(rc, []string{"-a", "a/b", "c/d"}); code != 1 {
		t.Errorf("write failure after partial output: code=%d, want 1", code)
	}
	if !strings.Contains(errb.String(), "basename: write error:") {
		t.Errorf("write failure after partial output: stderr=%q, want diagnostic", errb.String())
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
