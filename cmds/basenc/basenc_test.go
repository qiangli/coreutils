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
		{[]string{"--base64", "--base16"}, "multiple encoding types", 2},
		{[]string{"--base64", "-w", "-1"}, "invalid wrap size", 1},
		{[]string{"--base64", "-d"}, "invalid input", 1},
		{[]string{"--base64", "missing"}, "No such file or directory", 1},
	}
	for _, tt := range tests {
		_, errb, code := runTool(t, dir, "%%%%", tt.args...)
		if code != tt.code || !strings.Contains(errb, tt.want) {
			t.Fatalf("args %v err = (%q, %d), want %q code %d", tt.args, errb, code, tt.want, tt.code)
		}
	}
}

func TestBasencHelp(t *testing.T) {
	out, _, code := runTool(t, "", "", "--help")
	if code != 0 || !strings.Contains(out, "--base64url") || !strings.Contains(out, "--base16") {
		t.Fatalf("help = (%q, %d)", out, code)
	}
}
