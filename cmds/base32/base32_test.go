package base32cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// runTool is the canonical test harness shape for cmds packages,
// extended with stdin content and an explicit working directory.
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

func TestEncode(t *testing.T) {
	cases := []struct {
		stdin string
		args  []string
		want  string
	}{
		// RFC 4648 test vectors; default wrap 76 adds the trailing newline.
		{"", nil, ""},
		{"f", nil, "MY======\n"},
		{"fo", nil, "MZXQ====\n"},
		{"foobar", nil, "MZXW6YTBOI======\n"},
		// -w 0: no wrapping AND no trailing newline (GNU).
		{"foobar", []string{"-w", "0"}, "MZXW6YTBOI======"},
		// wrapping, including an exact-multiple final line.
		{"foobar", []string{"-w", "8"}, "MZXW6YTB\nOI======\n"},
		{"foobar", []string{"--wrap=6"}, "MZXW6Y\nTBOI==\n====\n"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, "", c.stdin, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("base32 %v < %q = (%q, %q, %d), want (%q, 0)", c.args, c.stdin, out, errb, code, c.want)
		}
	}
}

func TestDecode(t *testing.T) {
	cases := []struct {
		stdin string
		args  []string
		want  string
	}{
		{"MZXW6YTBOI======\n", []string{"-d"}, "foobar"},
		// embedded newlines (wrapped input) are tolerated.
		{"MZXW6YTB\nOI======\n", []string{"--decode"}, "foobar"},
		// -i ignores non-alphabet garbage (lowercase is garbage in base32).
		{"mzMZXW6YTBOI;======\n", []string{"-d", "-i"}, "foobar"},
		{"", []string{"-d"}, ""},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, "", c.stdin, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("base32 %v < %q = (%q, %q, %d), want (%q, 0)", c.args, c.stdin, out, errb, code, c.want)
		}
	}
}

func TestDecodeInvalidInput(t *testing.T) {
	// lowercase / garbage without -i fails with the GNU diagnostic, exit 1.
	for _, stdin := range []string{"mzxw6ytboi======\n", "MZXW;6YTBOI======\n", "M\n"} {
		out, errb, code := runTool(t, "", stdin, "-d")
		if code != 1 || !strings.Contains(errb, "base32: invalid input") {
			t.Errorf("decode %q: out=%q err=%q code=%d, want invalid input + exit 1", stdin, out, errb, code)
		}
	}
}

func TestFileOperand(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in.txt"), []byte("foobar"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runTool(t, dir, "", "in.txt")
	if out != "MZXW6YTBOI======\n" || code != 0 {
		t.Errorf("file encode: got (%q, %d)", out, code)
	}
	_, errb, code := runTool(t, dir, "", "nope.txt")
	if code != 1 || !strings.Contains(errb, "base32: nope.txt: No such file or directory") {
		t.Errorf("missing file: err=%q code=%d", errb, code)
	}
}

func TestInvalidWrap(t *testing.T) {
	_, errb, code := runTool(t, "", "abc", "-w", "wide")
	if code != 1 || !strings.Contains(errb, "base32: invalid wrap size: 'wide'") {
		t.Errorf("-w wide: err=%q code=%d", errb, code)
	}
}

func TestOperandAndFlagErrors(t *testing.T) {
	_, errb, code := runTool(t, "", "", "a", "b")
	if code != 2 || !strings.Contains(errb, "extra operand") {
		t.Errorf("extra operand: err=%q code=%d", errb, code)
	}
	_, errb, code = runTool(t, "", "", "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: err=%q code=%d", errb, code)
	}
}

func TestHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, "", "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: base32") || !strings.Contains(out, "--ignore-garbage") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "", "", "--version")
	if code != 0 || !strings.Contains(out, "base32") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}
