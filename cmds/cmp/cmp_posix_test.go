package cmpcmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runToolEnv(t *testing.T, dir, stdin string, env []string, args ...string) (stdout, stderr string, code int) {
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

type cmpFailWriter struct{ err error }

func (w cmpFailWriter) Write([]byte) (int, error) { return 0, w.err }

type cmpShortWriter struct{}

func (cmpShortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

type cmpFailReader struct{ err error }

func (r cmpFailReader) Read([]byte) (int, error) { return 0, r.err }

func TestCmpPOSIXOperandGrammarAndOutputErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a", "a")
	writeFile(t, dir, "b", "b")

	for _, args := range [][]string{{"a"}, {"a", "b", "0"}} {
		_, errb, code := runToolEnv(t, dir, "", []string{"POSIXLY_CORRECT="}, args...)
		if code != 2 || !strings.Contains(errb, "operand") {
			t.Fatalf("POSIX cmp %v = (%q, %d), want operand diagnostic and 2", args, errb, code)
		}
	}

	var errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: cmpFailWriter{errors.New("broken pipe")}, Err: &errb},
	}
	if code := cmd.Run(rc, []string{"a", "b"}); code != 2 {
		t.Fatalf("write failure exit = %d, want 2", code)
	}
	if got := errb.String(); !strings.Contains(got, "cmp: write error: broken pipe") {
		t.Fatalf("write failure diagnostic = %q", got)
	}

	for _, args := range [][]string{{"a", "b"}, {"-l", "a", "b"}} {
		err := new(bytes.Buffer)
		rc := &tool.RunContext{
			Ctx: context.Background(), Dir: dir, Env: []string{"POSIXLY_CORRECT="},
			Stdio: tool.Stdio{In: strings.NewReader(""), Out: cmpShortWriter{}, Err: err},
		}
		if code := cmd.Run(rc, args); code != 2 {
			t.Errorf("short write for cmp %v: exit = %d, want 2", args, code)
		}
		if got := err.String(); !strings.Contains(got, "cmp: write error: short write") {
			t.Errorf("short write for cmp %v: diagnostic = %q", args, got)
		}
	}

	errb.Reset()
	rc.Stdio = tool.Stdio{In: cmpFailReader{errors.New("input failure")}, Out: io.Discard, Err: &errb}
	if code := cmd.Run(rc, []string{"-", "a"}); code != 2 {
		t.Fatalf("read failure exit = %d, want 2", code)
	}
	if got := errb.String(); !strings.Contains(got, "cmp: -: input failure") {
		t.Fatalf("read failure diagnostic = %q", got)
	}
}

// Issue 7 STDOUT fixes -l output as exactly "%d %o %o": no offset-column
// alignment and no octal padding. The GNU-diffutils aligned columns remain
// the default outside POSIX mode.
func TestCmpVerbosePOSIXModeFormat(t *testing.T) {
	dir := t.TempDir()
	// 12-byte files force a two-digit GNU offset column; the first
	// difference uses octal values below 0100 that GNU blank-pads.
	writeFile(t, dir, "a", "\x01aaaaaaaaaa!")
	writeFile(t, dir, "b", "\x02aaaaaaaaaa?")

	out, errb, code := runToolEnv(t, dir, "", []string{"POSIXLY_CORRECT="}, "-l", "a", "b")
	if want := "1 1 2\n12 41 77\n"; out != want || errb != "" || code != 1 {
		t.Fatalf("POSIX -l: out=%q err=%q code=%d, want out=%q err=\"\" code=1", out, errb, code, want)
	}

	out, errb, code = runToolEnv(t, dir, "", nil, "-l", "a", "b")
	if want := " 1   1   2\n12  41  77\n"; out != want || errb != "" || code != 1 {
		t.Fatalf("GNU -l: out=%q err=%q code=%d, want out=%q err=\"\" code=1", out, errb, code, want)
	}
}

// Issue 7 STDERR: with -l, files differing in length yield
// "cmp: EOF on %s%s" on standard error, alongside the listed
// differences and exit status 1.
func TestCmpVerboseEOFDiagnostic(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a", "aXc")
	writeFile(t, dir, "b", "aYcd")
	for _, env := range [][]string{nil, {"POSIXLY_CORRECT="}} {
		out, errb, code := runToolEnv(t, dir, "", env, "-l", "a", "b")
		if code != 1 {
			t.Fatalf("env=%v: code=%d, want 1", env, code)
		}
		if want := "2 130 131\n"; out != want {
			t.Errorf("env=%v: out=%q, want %q", env, out, want)
		}
		if want := "cmp: EOF on a after byte 3\n"; errb != want {
			t.Errorf("env=%v: err=%q, want %q", env, errb, want)
		}
	}
}
