package catcmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func TestCat(t *testing.T) {
	cases := []struct {
		name  string
		stdin string
		args  []string
		want  string
	}{
		{"stdin default", "hello\n", nil, "hello\n"},
		{"stdin dash", "hi\n", []string{"-"}, "hi\n"},
		{"number", "a\nb\n", []string{"-n"}, "     1\ta\n     2\tb\n"},
		{"number nonblank", "a\n\nb\n", []string{"-b"}, "     1\ta\n\n     2\tb\n"},
		{"b overrides n", "a\n\n", []string{"-n", "-b"}, "     1\ta\n\n"},
		{"squeeze", "a\n\n\n\nb\n", []string{"-s"}, "a\n\nb\n"},
		{"show ends", "a\n", []string{"-E"}, "a$\n"},
		{"show ends crlf", "a\r\n", []string{"-E"}, "a^M$\n"},
		{"show tabs", "a\tb\n", []string{"-T"}, "a^Ib\n"},
		{"nonprinting", "\x01\x7f\x80\xa1\xff\n", []string{"-v"}, "^A^?M-^@M-!M-^?\n"},
		{"v leaves tab and nl", "a\tb\n", []string{"-v"}, "a\tb\n"},
		{"show all", "a\t\x01\n", []string{"-A"}, "a^I^A$\n"},
		{"short only e", "a\x01\n", []string{"-e"}, "a^A$\n"},
		{"short only t", "a\t\n", []string{"-t"}, "a^I\n"},
		{"short only u ignored", "a\n", []string{"-u"}, "a\n"},
		{"combined cluster", "a\t\n", []string{"-vt"}, "a^I\n"},
		{"no final newline", "ab", []string{"-E"}, "ab"},
		{"number no final newline", "ab", []string{"-n"}, "     1\tab"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, "", c.stdin, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("%s: cat %v = (%q, %q, %d), want (%q, _, 0)", c.name, c.args, out, errb, code, c.want)
		}
	}
}

func TestCatFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "one\n")
	writeFile(t, dir, "b.txt", "two\n")

	out, _, code := runTool(t, dir, "", "a.txt", "b.txt")
	if out != "one\ntwo\n" || code != 0 {
		t.Errorf("cat two files = (%q, %d)", out, code)
	}

	// Numbering continues across files (GNU behavior).
	out, _, code = runTool(t, dir, "", "-n", "a.txt", "b.txt")
	if out != "     1\tone\n     2\ttwo\n" || code != 0 {
		t.Errorf("cat -n across files = (%q, %d)", out, code)
	}

	// File and stdin interleaved via "-".
	out, _, code = runTool(t, dir, "in\n", "a.txt", "-")
	if out != "one\nin\n" || code != 0 {
		t.Errorf("cat file - = (%q, %d)", out, code)
	}
}

func TestCatLineStateAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "no-nl", "left")
	writeFile(t, dir, "right", "right\n")
	writeFile(t, dir, "blank-prefix", "\n\nnext\n")

	out, _, code := runTool(t, dir, "", "-n", "no-nl", "right")
	if out != "     1\tleftright\n" || code != 0 {
		t.Errorf("cat -n files without boundary newline = (%q, %d)", out, code)
	}

	out, _, code = runTool(t, dir, "", "-b", "no-nl", "right")
	if out != "     1\tleftright\n" || code != 0 {
		t.Errorf("cat -b files without boundary newline = (%q, %d)", out, code)
	}

	out, _, code = runTool(t, dir, "", "-s", "no-nl", "blank-prefix")
	if out != "left\n\nnext\n" || code != 0 {
		t.Errorf("cat -s files without boundary newline = (%q, %d)", out, code)
	}
}

func TestCatUnbufferedFlushesBeforeEOF(t *testing.T) {
	pr, pw := io.Pipe()
	defer pr.Close()
	defer pw.Close()

	var out lockedBuffer
	var errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: pr, Out: &out, Err: &errb},
	}
	done := make(chan int, 1)
	go func() {
		done <- cmd.Run(rc, []string{"-u"})
	}()

	if _, err := pw.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(2 * time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for out.String() != "x" {
		select {
		case <-deadline:
			t.Fatalf("cat -u did not flush before EOF; out=%q err=%q", out.String(), errb.String())
		case <-tick.C:
		}
	}

	if err := pw.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-done:
		if code != 0 || errb.String() != "" {
			t.Fatalf("cat -u after close: code=%d err=%q", code, errb.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cat -u did not finish after stdin close")
	}
}

func TestCatErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "one\n")

	out, errb, code := runTool(t, dir, "", "missing", "a.txt")
	if code != 1 || !strings.Contains(errb, "cat: missing:") || out != "one\n" {
		t.Errorf("missing file: out=%q err=%q code=%d", out, errb, code)
	}

	_, errb, code = runTool(t, "", "", "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: err=%q code=%d", errb, code)
	}

	_, errb, code = runTool(t, "", "", "-x")
	if code != 2 || !strings.Contains(errb, "x") {
		t.Errorf("unknown short flag: err=%q code=%d", errb, code)
	}

	// -e, -t, and -u are GNU short-only options. Do not invent long
	// spellings for them: an unknown option must remain an error.
	for _, arg := range []string{"--show-ends-nonprinting", "--show-tabs-nonprinting", "--unbuffered"} {
		_, errb, code = runTool(t, "", "", arg)
		if code != 2 || !strings.Contains(errb, arg[2:]) {
			t.Errorf("unknown short-only long spelling %q: err=%q code=%d", arg, errb, code)
		}
	}
}

func TestCatHelpVersion(t *testing.T) {
	out, _, code := runTool(t, "", "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: cat") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "", "", "-h")
	if code != 0 || !strings.Contains(out, "Usage: cat") {
		t.Errorf("-h: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "", "", "--version")
	if code != 0 || !strings.Contains(out, "cat") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "", "", "-V")
	if code != 0 || !strings.Contains(out, "cat") {
		t.Errorf("-V: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "", "", "-etV")
	if code != 0 || !strings.Contains(out, "cat") {
		t.Errorf("-etV: code=%d out=%q", code, out)
	}
}
