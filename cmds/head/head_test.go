package headcmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

type headFailWriter struct{ err error }

func (w headFailWriter) Write([]byte) (int, error) { return 0, w.err }

type headFailReader struct{ err error }

func (r headFailReader) Read([]byte) (int, error) { return 0, r.err }

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

func TestHead(t *testing.T) {
	cases := []struct {
		name  string
		stdin string
		args  []string
		want  string
	}{
		{"default ten lines", twelveLines(), nil,
			"line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n"},
		{"n two", "a\nb\nc\n", []string{"-n", "2"}, "a\nb\n"},
		{"obsolete shorthand", "a\nb\nc\n", []string{"-2"}, "a\nb\n"},
		{"obsolete bytes", "abcdef", []string{"-2c"}, "ab"},
		{"obsolete byte blocks", strings.Repeat("x", 600), []string{"-1b"}, strings.Repeat("x", 512)},
		{"obsolete kibibytes", strings.Repeat("x", 1200), []string{"-1k"}, strings.Repeat("x", 1024)},
		{"n zero", "a\nb\n", []string{"-n", "0"}, ""},
		{"all but last two", "a\nb\nc\nd\n", []string{"-n", "-2"}, "a\nb\n"},
		{"all but last zero", "a\nb\n", []string{"-n", "-0"}, "a\nb\n"},
		{"bytes", "abcdef", []string{"-c", "4"}, "abcd"},
		{"bytes beyond eof", "ab", []string{"-c", "5"}, "ab"},
		{"all but last bytes", "abcdef", []string{"-c", "-2"}, "abcd"},
		{"bytes with suffix", "abc", []string{"-c", "1K"}, "abc"},
		{"final partial line counts", "a\nb", []string{"-n", "2"}, "a\nb"},
		{"c overrides n", "abc\ndef\n", []string{"-n", "1", "-c", "2"}, "ab"},
		{"n after c wins", "abc\ndef\n", []string{"-c", "2", "-n", "1"}, "abc\n"},
		{"abbreviated lines after bytes wins", "abc\ndef\n", []string{"--bytes=2", "--l=1"}, "abc\n"},
		{"abbreviated bytes after lines wins", "abc\ndef\n", []string{"--lines=1", "--b=2"}, "ab"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, "", c.stdin, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("%s: head %v = (%q, %q, %d), want (%q, _, 0)", c.name, c.args, out, errb, code, c.want)
		}
	}
}

func TestHeadAbbreviatedHeaderOptionsRespectOrder(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a", "1\n")
	writeFile(t, dir, "b", "2\n")

	out, _, code := runTool(t, dir, "", "--verb", "--q", "a", "b")
	if code != 0 || out != "1\n2\n" {
		t.Errorf("--verb --q: out=%q code=%d", out, code)
	}

	out, _, code = runTool(t, dir, "", "--q", "--verb", "a", "b")
	want := "==> a <==\n1\n\n==> b <==\n2\n"
	if code != 0 || out != want {
		t.Errorf("--q --verb: out=%q code=%d want=%q", out, code, want)
	}
}

func TestHeadObsoleteHeaderOptionsRespectOrder(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a", "1\n2\n")
	writeFile(t, dir, "b", "3\n4\n")

	out, _, code := runTool(t, dir, "", "-1q", "a", "b")
	if code != 0 || out != "1\n3\n" {
		t.Errorf("-1q: out=%q code=%d", out, code)
	}

	out, _, code = runTool(t, dir, "", "-1v", "a")
	want := "==> a <==\n1\n"
	if code != 0 || out != want {
		t.Errorf("-1v: out=%q code=%d want=%q", out, code, want)
	}

	out, _, code = runTool(t, dir, "", "-1qv", "a", "b")
	want = "==> a <==\n1\n\n==> b <==\n3\n"
	if code != 0 || out != want {
		t.Errorf("-1qv: out=%q code=%d want=%q", out, code, want)
	}
}

func TestHeadHeaders(t *testing.T) {
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

	out, _, _ = runTool(t, dir, "", "-v", "a")
	if out != "==> a <==\n1\n2\n" {
		t.Errorf("-v single file: got %q", out)
	}

	out, _, _ = runTool(t, dir, "in\n", "a", "-")
	want = "==> a <==\n1\n2\n\n==> standard input <==\nin\n"
	if out != want {
		t.Errorf("stdin header: got %q, want %q", out, want)
	}
}

func TestHeadZeroTerminated(t *testing.T) {
	out, _, code := runTool(t, "", "a\x00b\x00c\x00", "-z", "-n", "2")
	if code != 0 || out != "a\x00b\x00" {
		t.Errorf("head -z -n 2: out=%q code=%d", out, code)
	}
	out, _, code = runTool(t, "", "a\x00b\x00c\x00", "-z", "-n", "-1")
	if code != 0 || out != "a\x00b\x00" {
		t.Errorf("head -z -n -1: out=%q code=%d", out, code)
	}
}

func TestHeadErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a", "1\n")

	out, errb, code := runTool(t, dir, "", "missing", "a")
	if code != 1 || !strings.Contains(errb, "cannot open 'missing' for reading") {
		t.Errorf("missing file: err=%q code=%d", errb, code)
	}
	if !strings.Contains(out, "==> a <==") {
		t.Errorf("remaining file should still print with header: out=%q", out)
	}

	_, errb, code = runTool(t, "", "", "-n", "x")
	if code != 2 || !strings.Contains(errb, "invalid number of lines") {
		t.Errorf("bad -n: err=%q code=%d", errb, code)
	}

	_, errb, code = runTool(t, "", "", "-c", "5q")
	if code != 2 || !strings.Contains(errb, "invalid number of bytes") {
		t.Errorf("bad -c: err=%q code=%d", errb, code)
	}

	_, errb, code = runTool(t, "", "", "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: err=%q code=%d", errb, code)
	}
}

func TestHeadWriteError(t *testing.T) {
	var errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(),
		Stdio: tool.Stdio{
			In:  strings.NewReader("line\n"),
			Out: headFailWriter{err: io.ErrClosedPipe},
			Err: &errb,
		},
	}
	if code := cmd.Run(rc, nil); code != 1 {
		t.Fatalf("write failure exit = %d, want 1", code)
	}
	if got := errb.String(); !strings.Contains(got, "head: write error:") ||
		!strings.Contains(got, "closed pipe") {
		t.Fatalf("write failure diagnostic = %q", got)
	}
}

func TestHeadMidStreamWriteErrorIsNotReportedAsReadError(t *testing.T) {
	var errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(),
		Stdio: tool.Stdio{
			In:  strings.NewReader(strings.Repeat("line\n", 2000)),
			Out: headFailWriter{err: io.ErrClosedPipe},
			Err: &errb,
		},
	}
	if code := cmd.Run(rc, []string{"-n", "2000"}); code != 1 {
		t.Fatalf("mid-stream write failure exit = %d, want 1", code)
	}
	if got := errb.String(); !strings.Contains(got, "head: write error:") || strings.Contains(got, "error reading") {
		t.Fatalf("mid-stream write diagnostic = %q", got)
	}
}

func TestHeadStandardInputOperandAndReadError(t *testing.T) {
	out, errb, code := runTool(t, "", "line\n", "-")
	if out != "line\n" || errb != "" || code != 0 {
		t.Fatalf("dash stdin = (%q, %q, %d)", out, errb, code)
	}

	var errbBuf bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(),
		Stdio: tool.Stdio{In: headFailReader{fmt.Errorf("input failure")}, Out: io.Discard, Err: &errbBuf},
	}
	if code := cmd.Run(rc, nil); code != 1 {
		t.Fatalf("read failure exit = %d, want 1", code)
	}
	if got := errbBuf.String(); !strings.Contains(got, "head: error reading '-': Input failure") {
		t.Fatalf("read failure diagnostic = %q", got)
	}
}

func TestHeadPreservesSeekableInputOffset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "input")
	if err := os.WriteFile(path, []byte("a\nb\nc\nd\ne\nf\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{In: f, Out: &out, Err: &errb}}
	if code := cmd.Run(rc, []string{"-n", "3"}); code != 0 || errb.Len() != 0 || out.String() != "a\nb\nc\n" {
		t.Fatalf("head seekable input = (%q, %q, %d)", out.String(), errb.String(), code)
	}
	rest, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(rest) != "d\ne\nf\n" {
		t.Fatalf("seekable input remainder = %q", rest)
	}
}

func TestHeadStopsOptionParsingAtFirstOperand(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "first", "one\n")
	writeFile(t, dir, "-n", "two\n")
	writeFile(t, dir, "--lin", "three\n")
	out, errb, code := runTool(t, dir, "", "first", "-n", "--lin")
	if code != 0 || errb != "" || !strings.Contains(out, "one\n") || !strings.Contains(out, "two\n") || !strings.Contains(out, "three\n") {
		t.Fatalf("head option-looking operand = (%q, %q, %d)", out, errb, code)
	}
}

func TestHeadHelpVersion(t *testing.T) {
	out, _, code := runTool(t, "", "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: head") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "", "", "--version")
	if code != 0 || !strings.Contains(out, "head") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}

func TestParseCount(t *testing.T) {
	cases := []struct {
		in   string
		val  int64
		neg  bool
		plus bool
		ok   bool
	}{
		{"10", 10, false, false, true},
		{"-3", 3, true, false, true},
		{"+7", 7, false, true, true},
		{"1b", 512, false, false, true},
		{"2K", 2048, false, false, true},
		{"1kB", 1000, false, false, true},
		{"1KiB", 1024, false, false, true},
		{"1M", 1 << 20, false, false, true},
		{"1MB", 1000000, false, false, true},
		{"", 0, false, false, false},
		{"x", 0, false, false, false},
		{"5x", 0, false, false, false},
	}
	for _, c := range cases {
		v, neg, plus, err := parseCount(c.in)
		if c.ok != (err == nil) {
			t.Errorf("parseCount(%q) err=%v, want ok=%v", c.in, err, c.ok)
			continue
		}
		if c.ok && (v != c.val || neg != c.neg || plus != c.plus) {
			t.Errorf("parseCount(%q) = (%d,%v,%v), want (%d,%v,%v)", c.in, v, neg, plus, c.val, c.neg, c.plus)
		}
	}
}
