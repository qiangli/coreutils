package base64cmd

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
		{"f", nil, "Zg==\n"},
		{"fo", nil, "Zm8=\n"},
		{"foo", nil, "Zm9v\n"},
		{"foobar", nil, "Zm9vYmFy\n"},
		{"hello china\n", nil, "aGVsbG8gY2hpbmEK\n"},
		// -w 0: no wrapping AND no trailing newline (GNU).
		{"foobar", []string{"-w", "0"}, "Zm9vYmFy"},
		{"", []string{"-w", "0"}, ""},
		// small wrap; 36 encoded chars -> 10/10/10/6.
		{"abcdefghijklmnopqrstuvwxyz", []string{"-w", "10"},
			"YWJjZGVmZ2\nhpamtsbW5v\ncHFyc3R1dn\nd4eXo=\n"},
		// exact multiple of the wrap column still ends with one newline.
		{"abc", []string{"-w", "4"}, "YWJj\n"},
		{"abcd", []string{"-w", "4"}, "YWJj\nZA==\n"},
		// --wrap=COLS long form.
		{"foobar", []string{"--wrap=4"}, "Zm9v\nYmFy\n"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, "", c.stdin, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("base64 %v < %q = (%q, %q, %d), want (%q, 0)", c.args, c.stdin, out, errb, code, c.want)
		}
	}
}

func TestEncodeDefaultWrap76(t *testing.T) {
	in := strings.Repeat("a", 60) // 80 encoded chars -> 76 + 4
	enc := "YWFh"                 // "aaa"
	full := strings.Repeat(enc, 20)
	want := full[:76] + "\n" + full[76:] + "\n"
	out, _, code := runTool(t, "", in)
	if out != want || code != 0 {
		t.Errorf("default wrap: got (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestDecode(t *testing.T) {
	cases := []struct {
		stdin string
		args  []string
		want  string
	}{
		{"Zm9vYmFy\n", []string{"-d"}, "foobar"},
		{"Zm9vYmFy\n", []string{"-D"}, "foobar"},
		// embedded newlines (wrapped input) are tolerated.
		{"Zm9v\nYmFy\n", []string{"-d"}, "foobar"},
		{"aGVsbG8gY2hpbmEK\n", []string{"--decode"}, "hello china\n"},
		// -i ignores non-alphabet garbage.
		{"Zm9v;;Ym!Fy\n", []string{"-d", "-i"}, "foobar"},
		{"Zg==\n", []string{"-d"}, "f"},
		// GNU >= 9.5 auto-pads unpadded input at EOF.
		{"Zg\n", []string{"-d"}, "f"},
		{"QQ", []string{"-d"}, "A"},
		{"", []string{"-d"}, ""},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, "", c.stdin, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("base64 %v < %q = (%q, %q, %d), want (%q, 0)", c.args, c.stdin, out, errb, code, c.want)
		}
	}
}

func TestDecodeInvalidInput(t *testing.T) {
	// garbage without -i fails with the GNU diagnostic, exit 1; so do
	// an impossible length ("a") and non-zero padding bits ("QR==",
	// GNU >= 9.5).
	for _, stdin := range []string{"Zm9v;YmFy\n", "a\n", "QR==\n", "Zh==\n", "QR\n"} {
		out, errb, code := runTool(t, "", stdin, "-d")
		if code != 1 || !strings.Contains(errb, "base64: invalid input") {
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
	if out != "Zm9vYmFy\n" || code != 0 {
		t.Errorf("file encode: got (%q, %d)", out, code)
	}
	// missing file
	_, errb, code := runTool(t, dir, "", "nope.txt")
	if code != 1 || !strings.Contains(errb, "base64: nope.txt: No such file or directory") {
		t.Errorf("missing file: err=%q code=%d", errb, code)
	}
}

func TestInvalidWrap(t *testing.T) {
	for _, w := range []string{"x", "-1", ""} {
		_, errb, code := runTool(t, "", "abc", "-w", w)
		if code != 1 || !strings.Contains(errb, "base64: invalid wrap size: '"+w+"'") {
			t.Errorf("-w %q: err=%q code=%d, want invalid wrap size + exit 1", w, errb, code)
		}
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
	if code != 0 || !strings.Contains(out, "Usage: base64") ||
		!strings.Contains(out, "-D          same as --decode") || !strings.Contains(out, "--decode") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "", "", "-h")
	if code != 0 || !strings.Contains(out, "Usage: base64") || !strings.Contains(out, "-h, --help") {
		t.Errorf("-h: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "", "", "--version")
	if code != 0 || !strings.Contains(out, "base64") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "", "", "-V")
	if code != 0 || !strings.Contains(out, "base64") {
		t.Errorf("-V: code=%d out=%q", code, out)
	}
}
