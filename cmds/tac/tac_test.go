package taccmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, dir, stdin string, args ...string) (stdout, stderr string, code int) {
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

func TestTac(t *testing.T) {
	cases := []struct {
		name  string
		stdin string
		args  []string
		want  string
	}{
		{"basic", "a\nb\nc\n", nil, "c\nb\na\n"},
		{"no trailing newline", "a\nb\nc", nil, "cb\na\n"},
		{"single line", "only\n", nil, "only\n"},
		{"empty input", "", nil, ""},
		{"no separator at all", "abc", nil, "abc"},
		{"custom separator", "a:b:c:", []string{"-s", ":"}, "c:b:a:"},
		{"custom separator partial", "a:b:c", []string{"-s", ":"}, "cb:a:"},
		{"multi-byte separator", "a--b--", []string{"--separator", "--"}, "b--a--"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, "", c.stdin, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("%s: tac %v = (%q, %q, %d), want (%q, _, 0)", c.name, c.args, out, errb, code, c.want)
		}
	}
}

func TestTacFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("1\n2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runTool(t, dir, "x\n", "f", "-")
	if out != "2\n1\nx\n" || code != 0 {
		t.Errorf("tac f -: got (%q, %d)", out, code)
	}
}

func TestTacErrors(t *testing.T) {
	_, errb, code := runTool(t, "", "", "-s", "")
	if code != 1 || !strings.Contains(errb, "separator cannot be empty") {
		t.Errorf("empty separator: err=%q code=%d", errb, code)
	}

	_, errb, code = runTool(t, "", "", "missing")
	if code != 1 || !strings.Contains(errb, "failed to open 'missing' for reading") {
		t.Errorf("missing file: err=%q code=%d", errb, code)
	}

	_, errb, code = runTool(t, "", "", "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: err=%q code=%d", errb, code)
	}
}

func TestTacHelpVersion(t *testing.T) {
	out, _, code := runTool(t, "", "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: tac") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "", "", "--version")
	if code != 0 || !strings.Contains(out, "tac") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}

func TestTacBefore(t *testing.T) {
	out, _, code := runTool(t, "", "a\nb\nc\n", "-b")
	if code != 0 || out != "c\nb\na" {
		t.Errorf("tac -b: got=%q code=%d", out, code)
	}
}

func TestTacRegex(t *testing.T) {
	// Split on word boundaries
	out, _, code := runTool(t, "", "abc-def-ghi-", "-r", "-s", "-")
	if code != 0 || out != "ghi-def-abc-" {
		t.Errorf("tac -r -s '-': got=%q code=%d", out, code)
	}
}
