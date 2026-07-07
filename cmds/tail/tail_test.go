package tailcmd

import (
	"bytes"
	"context"
	"fmt"
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

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func twelveLines() string {
	var b strings.Builder
	for i := 1; i <= 12; i++ {
		fmt.Fprintf(&b, "line%d\n", i)
	}
	return b.String()
}

func TestTail(t *testing.T) {
	cases := []struct {
		name  string
		stdin string
		args  []string
		want  string
	}{
		{"default ten lines", twelveLines(), nil,
			"line3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\nline11\nline12\n"},
		{"n two", "a\nb\nc\n", []string{"-n", "2"}, "b\nc\n"},
		{"n minus two same", "a\nb\nc\n", []string{"-n", "-2"}, "b\nc\n"},
		{"obsolete shorthand", "a\nb\nc\n", []string{"-2"}, "b\nc\n"},
		{"n zero", "a\nb\n", []string{"-n", "0"}, ""},
		{"from line two", "a\nb\nc\n", []string{"-n", "+2"}, "b\nc\n"},
		{"plus one whole file", "a\nb\n", []string{"-n", "+1"}, "a\nb\n"},
		{"plus zero whole file", "a\nb\n", []string{"-n", "+0"}, "a\nb\n"},
		{"plus beyond eof", "a\nb\n", []string{"-n", "+9"}, ""},
		{"bytes", "abcdef", []string{"-c", "4"}, "cdef"},
		{"bytes beyond size", "ab", []string{"-c", "9"}, "ab"},
		{"bytes from start", "abcdef", []string{"-c", "+3"}, "cdef"},
		{"bytes plus one whole", "ab", []string{"-c", "+1"}, "ab"},
		{"final partial line counts", "a\nb\nc", []string{"-n", "2"}, "b\nc"},
		{"c after n wins", "abc\ndef\n", []string{"-n", "1", "-c", "2"}, "f\n"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, "", c.stdin, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("%s: tail %v = (%q, %q, %d), want (%q, _, 0)", c.name, c.args, out, errb, code, c.want)
		}
	}
}

func TestTailHeaders(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a", "1\n2\n")
	writeFile(t, dir, "b", "3\n")

	out, _, code := runTool(t, dir, "", "a", "b")
	want := "==> a <==\n1\n2\n\n==> b <==\n3\n"
	if out != want || code != 0 {
		t.Errorf("two files: got (%q, %d), want %q", out, code, want)
	}

	out, _, _ = runTool(t, dir, "", "-q", "a", "b")
	if out != "1\n2\n3\n" {
		t.Errorf("-q: got %q", out)
	}

	out, _, _ = runTool(t, dir, "in\n", "-v", "-")
	if out != "==> standard input <==\nin\n" {
		t.Errorf("-v stdin: got %q", out)
	}
}

func TestTailZeroTerminated(t *testing.T) {
	out, _, code := runTool(t, "", "a\x00b\x00c\x00", "-z", "-n", "2")
	if code != 0 || out != "b\x00c\x00" {
		t.Errorf("tail -z -n 2: out=%q code=%d", out, code)
	}
	out, _, code = runTool(t, "", "a\x00b\x00c\x00", "-z", "-n", "+2")
	if code != 0 || out != "b\x00c\x00" {
		t.Errorf("tail -z -n +2: out=%q code=%d", out, code)
	}
}

func TestTailFollowNotSupported(t *testing.T) {
	for _, args := range [][]string{{"-f"}, {"--follow"}, {"--follow=name"}} {
		_, errb, code := runTool(t, "", "x\n", args...)
		if code != 2 || !strings.Contains(errb, "follow") || !strings.Contains(errb, "not supported") {
			t.Errorf("tail %v: err=%q code=%d, want not-supported exit 2", args, errb, code)
		}
	}
}

func TestTailErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a", "1\n")

	_, errb, code := runTool(t, dir, "", "missing", "a")
	if code != 1 || !strings.Contains(errb, "cannot open 'missing' for reading") {
		t.Errorf("missing file: err=%q code=%d", errb, code)
	}

	_, errb, code = runTool(t, "", "", "-n", "x")
	if code != 2 || !strings.Contains(errb, "invalid number of lines") {
		t.Errorf("bad -n: err=%q code=%d", errb, code)
	}

	_, errb, code = runTool(t, "", "", "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: err=%q code=%d", errb, code)
	}
}

func TestTailHelpVersion(t *testing.T) {
	out, _, code := runTool(t, "", "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: tail") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "", "", "--version")
	if code != 0 || !strings.Contains(out, "tail") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}
