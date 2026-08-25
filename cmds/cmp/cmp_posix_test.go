package cmpcmd

import (
	"bytes"
	"context"
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
