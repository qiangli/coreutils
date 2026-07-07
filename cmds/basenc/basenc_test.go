package basenccmd

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

func TestBasencEncodings(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"--base64"}, "Zm9vYmFy\n"},
		{[]string{"--base64url"}, "Zm9vYmFy\n"},
		{[]string{"--base32"}, "MZXW6YTBOI======\n"},
		{[]string{"--base32hex"}, "CPNMUOJ1E8======\n"},
		{[]string{"--base16"}, "666F6F626172\n"},
		{[]string{"--base2lsbf"}, "011001101111011011110110010001101000011001001110\n"},
		{[]string{"--base2msbf"}, "011001100110111101101111011000100110000101110010\n"},
		{[]string{"--base58"}, "t1Zv2yaZ\n"},
		{[]string{"--base64", "-w", "4"}, "Zm9v\nYmFy\n"},
		{[]string{"--base16", "-w", "0"}, "666F6F626172"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, "", "foobar", c.args...)
		if out != c.want || code != 0 {
			t.Fatalf("basenc %v = (%q, %q, %d), want %q", c.args, out, errb, code, c.want)
		}
	}
}

func TestBasencZ85(t *testing.T) {
	out, errb, code := runTool(t, "", "1234", "--z85")
	if out != "f!$Kw\n" || errb != "" || code != 0 {
		t.Fatalf("z85 encode = (%q, %q, %d)", out, errb, code)
	}
	out, errb, code = runTool(t, "", "f!$Kw\n", "--z85", "-d")
	if out != "1234" || errb != "" || code != 0 {
		t.Fatalf("z85 decode = (%q, %q, %d)", out, errb, code)
	}
	_, errb, code = runTool(t, "", "123", "--z85")
	if code != 1 || !strings.Contains(errb, "basenc: invalid input (length must be multiple of 4 characters)") {
		t.Fatalf("z85 invalid length = (%q, %d)", errb, code)
	}
}

func TestBasencDecode(t *testing.T) {
	cases := []struct {
		stdin string
		args  []string
		want  string
	}{
		{"Zm9vYmFy\n", []string{"--base64", "-d"}, "foobar"},
		{"Zm9v;;Ym!Fy\n", []string{"--base64", "-d", "-i"}, "foobar"},
		{"MZXW6YTBOI======\n", []string{"--base32", "--decode"}, "foobar"},
		{"CPNMUOJ1E8======\n", []string{"--base32hex", "-d"}, "foobar"},
		{"666F6F626172\n", []string{"--base16", "-d"}, "foobar"},
		{"011001101111011011110110010001101000011001001110\n", []string{"--base2lsbf", "-d"}, "foobar"},
		{"011001100110111101101111011000100110000101110010\n", []string{"--base2msbf", "-d"}, "foobar"},
		{"t1Zv2yaZ\n", []string{"--base58", "-d"}, "foobar"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, "", c.stdin, c.args...)
		if out != c.want || code != 0 {
			t.Fatalf("basenc %v decode = (%q, %q, %d), want %q", c.args, out, errb, code, c.want)
		}
	}
}

func TestBasencFileAndErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in.txt"), []byte("foobar"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runTool(t, dir, "", "--base64", "in.txt")
	if out != "Zm9vYmFy\n" || code != 0 {
		t.Fatalf("file = (%q, %d)", out, code)
	}
	tests := []struct {
		args []string
		want string
		code int
	}{
		{nil, "missing encoding type", 2},
		{[]string{"--base64", "-w", "-1"}, "invalid wrap size", 1},
		{[]string{"--base64", "-d"}, "invalid input", 1},
		{[]string{"--base64", "missing"}, "No such file or directory", 1},
		{[]string{"--base64", "-D"}, "unknown shorthand flag", 2},
	}
	for _, tt := range tests {
		_, errb, code := runTool(t, dir, "%%%%", tt.args...)
		if code != tt.code || !strings.Contains(errb, tt.want) {
			t.Fatalf("args %v err = (%q, %d), want %q code %d", tt.args, errb, code, tt.want, tt.code)
		}
	}
}

// GNU takes the last encoding flag given, not an error.
func TestBasencLastEncodingWins(t *testing.T) {
	out, errb, code := runTool(t, "", "foobar", "--base64", "--base16")
	if out != "666F6F626172\n" || errb != "" || code != 0 {
		t.Fatalf("--base64 --base16 = (%q, %q, %d), want base16 output", out, errb, code)
	}
	out, errb, code = runTool(t, "", "foobar", "--base16", "--base64")
	if out != "Zm9vYmFy\n" || errb != "" || code != 0 {
		t.Fatalf("--base16 --base64 = (%q, %q, %d), want base64 output", out, errb, code)
	}
}

// GNU >= 9.5 decode semantics: auto-pad unpadded input at EOF, reject
// non-zero padding bits, and treat '=' as garbage in encodings where
// padding is invalid.
func TestBasencDecodePadding(t *testing.T) {
	okCases := []struct {
		stdin string
		args  []string
		want  string
	}{
		{"QQ", []string{"--base64", "-d"}, "A"},
		{"QQ", []string{"--base64url", "-d"}, "A"},
		{"ME", []string{"--base32", "-d"}, "a"},
		{"C4", []string{"--base32hex", "-d"}, "a"},
		{"MZXW6", []string{"--base32", "-d"}, "foo"},
		// -d -i strips '=' where padding is not part of the encoding.
		{"4142=", []string{"--base16", "-d", "-i"}, "AB"},
		{"01000001=", []string{"--base2msbf", "-d", "-i"}, "A"},
	}
	for _, c := range okCases {
		out, errb, code := runTool(t, "", c.stdin, c.args...)
		if out != c.want || errb != "" || code != 0 {
			t.Errorf("basenc %v < %q = (%q, %q, %d), want %q", c.args, c.stdin, out, errb, code, c.want)
		}
	}
	badCases := []struct {
		stdin string
		args  []string
	}{
		{"QR==", []string{"--base64", "-d"}}, // non-zero padding bits
		{"QR", []string{"--base64", "-d"}},   // ditto, auto-padded
		{"MF======", []string{"--base32", "-d"}},
		{"Q", []string{"--base64", "-d"}}, // impossible length
		{"4142=", []string{"--base16", "-d"}},
		{"414", []string{"--base16", "-d"}}, // dangling nibble
	}
	for _, c := range badCases {
		out, errb, code := runTool(t, "", c.stdin, c.args...)
		if code != 1 || !strings.Contains(errb, "basenc: invalid input") {
			t.Errorf("basenc %v < %q = (%q, %q, %d), want invalid input + exit 1", c.args, c.stdin, out, errb, code)
		}
	}
}

func TestBasencHelp(t *testing.T) {
	out, _, code := runTool(t, "", "", "--help")
	if code != 0 || !strings.Contains(out, "--base64url") || !strings.Contains(out, "--base16") {
		t.Fatalf("help = (%q, %d)", out, code)
	}
}
