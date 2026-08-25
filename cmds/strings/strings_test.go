package stringscmd

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

func runToolEnv(t *testing.T, dir string, env []string, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	if dir == "" {
		dir = t.TempDir()
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestStrings(t *testing.T) {
	cases := []struct {
		name  string
		stdin string
		args  []string
		want  string
	}{
		{"default min 4", "\x00abc\x01hello\x02hi\x00", nil, "hello\n"},
		{"min 2", "\x00abc\x01hi\x00z\x00", []string{"-n", "2"}, "abc\nhi\n"},
		{"string at eof", "\x00world", nil, "world\n"},
		{"space is printable", "\x00ab cd\x00", []string{"-n", "5"}, "ab cd\n"},
		{"newline terminates", "abcd\nefgh\n", nil, "abcd\nefgh\n"},
		{"offsets decimal", "\x00\x00cool\x00", []string{"-t", "d"}, "      2 cool\n"},
		{"offsets hex", strings.Repeat("\x00", 16) + "cool", []string{"-t", "x"}, "     10 cool\n"},
		{"offsets octal", strings.Repeat("\x00", 8) + "cool", []string{"-t", "o"}, "     10 cool\n"},
		{"tab is not printable", "\x00ab\tcd\x00", []string{"-n", "5"}, ""},
		{"all flag", "\x00abc\x01hello\x02hi\x00", []string{"-a"}, "hello\n"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, "", c.stdin, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("%s: strings %v = (%q, %q, %d), want (%q, _, 0)", c.name, c.args, out, errb, code, c.want)
		}
	}
}

func TestStringsFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bin"), []byte("\x00first\x00\x7fsecond\xff"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runTool(t, dir, "", "bin")
	if out != "first\nsecond\n" || code != 0 {
		t.Errorf("file: got (%q, %d)", out, code)
	}

	// Offsets reset per file.
	out, _, _ = runTool(t, dir, "", "-t", "d", "bin", "bin")
	want := "      1 first\n      8 second\n      1 first\n      8 second\n"
	if out != want {
		t.Errorf("two files offsets: got %q, want %q", out, want)
	}
}

func TestStringsDashPathname(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "-"), []byte("\x00dashfile\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, "stdin-ignored", "-")
	if errb != "" || code != 0 || out != "dashfile\n" {
		t.Errorf("dash pathname: (%q, %q, %d), want (%q, _, 0)", out, errb, code, "dashfile\n")
	}
}

func TestStringsUTF8(t *testing.T) {
	stdin := "\x00\x00こんにちは\xffhi\x00"

	// Test UTF-8 mode
	out, errb, code := runToolEnv(t, "", []string{"LC_ALL=en_US.UTF-8"}, stdin, "-n", "2")
	if errb != "" || code != 0 {
		t.Errorf("utf8 error: %q %d", errb, code)
	}
	want := "こんにちは\nhi\n"
	if out != want {
		t.Errorf("utf8 out: got %q, want %q", out, want)
	}

	out, errb, code = runToolEnv(t, "", []string{"LC_ALL=en_US.UTF-8"}, stdin, "-n", "3")
	want = "こんにちは\n"
	if out != want {
		t.Errorf("utf8 min 3 out: got %q, want %q", out, want)
	}

	out, errb, code = runToolEnv(t, "", []string{"LC_ALL=C"}, stdin, "-n", "2")
	wantC := "hi\n"
	if out != wantC {
		t.Errorf("C locale out: got %q, want %q", out, wantC)
	}

	out, errb, code = runToolEnv(t, "", []string{"LC_ALL=invalid.locale"}, stdin, "-n", "2")
	if code == 0 || !strings.Contains(errb, "unsupported locale") {
		t.Errorf("unsupported locale: got (%q, %d)", errb, code)
	}
}

func TestStringsUTF8ReplacementCharacterPreservesBytesAndOffset(t *testing.T) {
	const utf8Locale = "en_US.UTF-8"

	// A canonical encoding of U+FFFD is a printable character, not the
	// one-byte RuneError sentinel returned for invalid UTF-8.
	stdin := "\x00\uFFFDAB\x00"
	out, errb, code := runToolEnv(t, "", []string{"LC_ALL=" + utf8Locale}, stdin, "-n", "3")
	if errb != "" || code != 0 || out != "\uFFFDAB\n" {
		t.Fatalf("canonical U+FFFD = (%q, %q, %d), want (%q, %q, 0)", out, errb, code, "\uFFFDAB\n", "")
	}
	if !bytes.Equal([]byte(out), []byte{0xef, 0xbf, 0xbd, 'A', 'B', '\n'}) {
		t.Fatalf("canonical U+FFFD bytes = % x, want original UTF-8 bytes", []byte(out))
	}

	// Offsets remain byte offsets: the three-byte Euro sign and one invalid
	// byte occupy four bytes before the printable U+FFFD run.
	stdin = "\u20AC\xff\uFFFDAB\x00"
	out, errb, code = runToolEnv(t, "", []string{"LC_ALL=" + utf8Locale}, stdin, "-n", "3", "-t", "d")
	if errb != "" || code != 0 || out != "      4 \uFFFDAB\n" {
		t.Fatalf("canonical U+FFFD offset = (%q, %q, %d), want (%q, %q, 0)", out, errb, code, "      4 \uFFFDAB\n", "")
	}
}

func TestStringsErrors(t *testing.T) {
	_, errb, code := runTool(t, "", "", "-n", "0")
	if code != 2 || !strings.Contains(errb, "invalid minimum string length") {
		t.Errorf("-n 0: err=%q code=%d", errb, code)
	}

	_, errb, code = runTool(t, "", "", "-t", "e")
	if code != 2 || !strings.Contains(errb, "invalid radix") {
		t.Errorf("-t e: err=%q code=%d", errb, code)
	}

	_, errb, code = runTool(t, "", "", "missing")
	if code != 1 || !strings.Contains(errb, "strings: missing:") {
		t.Errorf("missing file: err=%q code=%d", errb, code)
	}

	_, errb, code = runTool(t, "", "", "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: err=%q code=%d", errb, code)
	}
}

func TestStringsHelpVersion(t *testing.T) {
	out, _, code := runTool(t, "", "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: strings") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "", "", "--version")
	if code != 0 || !strings.Contains(out, "strings") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}
