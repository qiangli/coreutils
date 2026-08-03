package findcmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// The regressions below pin the POSIX behaviours a conformance run
// found find getting wrong: the "--" end-of-options delimiter, -name
// matching the operand's own last component, C/POSIX-locale pattern
// matching that no LC_* setting can move, chmod's full symbolic
// grammar in -perm, and a failed write to standard output being an
// error rather than a silent success.

// "--" ends the leading -H/-L/-P options (POSIX Utility Syntax
// Guideline 10). It is consumed, not read as a predicate.
func TestFindEndOfLeadingOptions(t *testing.T) {
	dir := setupTree(t)

	out, errb, code := runFind(t, dir, "--", ".", "-name", "*.go")
	if out != "./b.go\n./sub/deep/d.go\n" || code != 0 {
		t.Errorf("find -- . -name '*.go' = (%q, %d, err=%q)", out, code, errb)
	}
	// It may follow a symlink option, and only ends the options.
	out, _, code = runFind(t, dir, "-L", "--", "sub", "-name", "*.go")
	if out != "sub/deep/d.go\n" || code != 0 {
		t.Errorf("find -L -- sub -name '*.go' = (%q, %d)", out, code)
	}
	// Inside the expression it is still an unknown predicate, exactly as
	// GNU treats any '-'-prefixed token there.
	_, errb, code = runFind(t, dir, ".", "--", "-name", "*.go")
	if code == 0 || !strings.Contains(errb, "--") {
		t.Errorf("'--' inside the expression: code=%d err=%q", code, errb)
	}
}

// -name matches the last component of the path as find names it — for a
// start point, the operand's own final component. Resolving the operand
// first made `find . -name .` miss and `find . -name <cwd>` hit.
func TestFindNameMatchesOperandBaseName(t *testing.T) {
	dir := setupTree(t)

	out, _, code := runFind(t, dir, ".", "-maxdepth", "0", "-name", ".")
	if out != ".\n" || code != 0 {
		t.Errorf("find . -name . = (%q, %d), want (\".\\n\", 0)", out, code)
	}
	// The working directory's real name is not what "." is called here.
	out, _, _ = runFind(t, dir, ".", "-maxdepth", "0", "-name", filepath.Base(dir))
	if out != "" {
		t.Errorf("find . -name <cwd basename> matched: %q", out)
	}
	// Trailing slashes are stripped before matching, as GNU and BSD do.
	out, _, _ = runFind(t, dir, "sub/", "-maxdepth", "0", "-name", "sub")
	if out != "sub/\n" {
		t.Errorf("find sub/ -name sub = %q, want \"sub/\\n\"", out)
	}
	out, _, _ = runFind(t, dir, "sub", "-maxdepth", "0", "-name", "*ub")
	if out != "sub\n" {
		t.Errorf("find sub -name '*ub' = %q", out)
	}
}

// Pattern matching is C/POSIX-locale: a character is a byte, so '?'
// matches one byte of a multi-byte sequence and the character classes
// are ASCII-only. No LC_* or LANG setting may change the answer.
func TestFindPatternsAreCLocaleAndLocaleInvariant(t *testing.T) {
	dir := t.TempDir()
	// "e" + COMBINING ACUTE ACCENT: one character, three bytes, and
	// decomposed so a normalizing filesystem stores the bytes as given.
	const eAcute = "e\u0301"
	writeFile(t, dir, eAcute, "x")
	writeFile(t, dir, "ab", "x")

	cases := []struct {
		pat  string
		want string
	}{
		{"e??", "./" + eAcute + "\n"},           // '?' is one byte ...
		{"e?", ""},                              // ... so one does not span the accent
		{"???", "./" + eAcute + "\n"},           //
		{"[[:alpha:]][[:alpha:]]", "./ab\n"},    // the classes are ASCII only
		{"[[:alpha:]]??", "./" + eAcute + "\n"}, // the leading 'e' is alphabetic
		{"?[[:alpha:]]?", ""},                   // the accent's bytes are not
	}

	// The same expectation must hold however the locale is named.
	envs := [][]string{
		nil,
		{"LC_ALL=C"},
		{"LC_ALL=en_US.UTF-8"},
		{"LC_CTYPE=en_US.UTF-8", "LANG=C"},
		{"LC_COLLATE=en_US.UTF-8"},
		{"LC_MESSAGES=fr_FR.UTF-8"},
		{"LANG=en_US.UTF-8"},
		// LC_ALL wins over every LC_* and LANG; here that is visible as
		// "still no change", which is the point.
		{"LC_ALL=C", "LC_CTYPE=en_US.UTF-8", "LC_COLLATE=en_US.UTF-8", "LANG=en_US.UTF-8"},
	}
	for _, env := range envs {
		for _, c := range cases {
			out, errb, code := runFindEnv(t, dir, env, ".", "-name", c.pat)
			if out != c.want || code != 0 {
				t.Errorf("env %v: find . -name %q = (%q, %d, err=%q), want %q",
					env, c.pat, out, code, errb, c.want)
			}
		}
	}
}

// -iname folds ASCII case only: in the C locale no other byte has a
// case pair, in a class or out of it.
func TestFindINameFoldsASCIIOnly(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Ab", "x")

	for _, c := range []struct {
		pat  string
		want string
	}{
		{"aB", "./Ab\n"},
		{"[[:lower:]][[:lower:]]", "./Ab\n"}, // fold: either case satisfies
		{"[a-b][a-b]", "./Ab\n"},
	} {
		out, _, code := runFind(t, dir, ".", "-iname", c.pat)
		if out != c.want || code != 0 {
			t.Errorf("find . -iname %q = (%q, %d), want %q", c.pat, out, code, c.want)
		}
	}
	// Without folding the class is exact.
	out, _, _ := runFind(t, dir, ".", "-name", "[[:lower:]][[:lower:]]")
	if out != "" {
		t.Errorf("-name '[[:lower:]][[:lower:]]' matched %q", out)
	}
}

// -perm takes chmod's symbolic grammar in full: who-lists, all three
// operators applied in order, permcopy, and 'X'.
func TestParsePermBitsSymbolic(t *testing.T) {
	cases := []struct {
		mode string
		want uint32
	}{
		{"644", 0o644},
		{"u=rwx", 0o700},
		{"ug=rw", 0o660},
		{"a=r", 0o444},
		{"u=rwx,g=u", 0o770},         // permcopy from the mode so far
		{"u=rwx,g=u,o=g", 0o777},     //
		{"g=u", 0},                   // copied from the zero template
		{"u=rwx,u=r", 0o400},         // '=' replaces, it does not accumulate
		{"u=rwx,u-w", 0o500},         // '-' clears
		{"a=rwx,go-w", 0o755},        //
		{"u=rx,go+X", 0o511},         // 'X': an execute bit is already set
		{"a=r,u+X", 0o444},           // ... and here none is
		{"u+s,g+s", 0o6000},          //
		{"+t", 0o1000},               //
		{"u=rw,u+g", 0o600},          // permcopy of an empty class adds nothing
		{"u=rwx,u=", 0},              // an empty list clears the class
		{"o=w", 0o002},               //
		{"u+w,g+w,o+w", 0o222},       //
		{"a+rw", 0o666},              //
		{"ugo=rwx", 0o777},           //
		{"u=rwxs", 0o4700},           //
		{"u=rwx,g+u,o+u", 0o777},     //
		{"a=rwx,u=r,g=w,o=x", 0o421}, //
	}
	for _, c := range cases {
		got, err := parsePermBits(c.mode)
		if err != nil || got != c.want {
			t.Errorf("parsePermBits(%q) = (%#o, %v), want (%#o, nil)", c.mode, got, err, c.want)
		}
	}
	for _, bad := range []string{"", "u=q", "u", "888", "u@rw", "rw"} {
		if got, err := parsePermBits(bad); err == nil {
			t.Errorf("parsePermBits(%q) = %#o, want an error", bad, got)
		}
	}
}

// The permcopy form reaches -perm end to end, and its diagnostics stay
// usage errors rather than being rejected as a bad mode.
func TestFindPermSymbolic(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not settable on windows")
	}
	dir := t.TempDir()
	writeFile(t, dir, "rw.txt", "x")
	if err := os.Chmod(filepath.Join(dir, "rw.txt"), 0o644); err != nil {
		t.Fatal(err)
	}

	// -g=u copies an empty class off the zero template: no bits are
	// required, so every file qualifies (as GNU and BSD have it).
	out, _, code := runFind(t, dir, ".", "-type", "f", "-perm", "-g=u")
	if out != "./rw.txt\n" || code != 0 {
		t.Errorf("-perm -g=u = (%q, %d)", out, code)
	}
	out, _, code = runFind(t, dir, ".", "-type", "f", "-perm", "u=rw,go=r")
	if out != "./rw.txt\n" || code != 0 {
		t.Errorf("-perm u=rw,go=r = (%q, %d)", out, code)
	}
	out, _, _ = runFind(t, dir, ".", "-type", "f", "-perm", "-u+X")
	if out != "./rw.txt\n" {
		t.Errorf("-perm -u+X = %q, want every file (X adds nothing here)", out)
	}
	_, errb, code := runFind(t, dir, ".", "-perm", "u=q")
	if code != 2 || !strings.Contains(errb, "invalid mode") {
		t.Errorf("-perm u=q: code=%d err=%q", code, errb)
	}
}

// errWriter fails every write, standing in for a full filesystem or a
// closed descriptor on standard output.
type errWriter struct{ err error }

func (w *errWriter) Write([]byte) (int, error) { return 0, w.err }

// A write to standard output that fails is diagnosed on standard error
// and exits non-zero; `find > /dev/full` must not look like success.
func TestFindWriteErrorIsDiagnosed(t *testing.T) {
	dir := setupTree(t)
	var errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(),
		Dir: dir,
		Stdio: tool.Stdio{
			In:  strings.NewReader(""),
			Out: &errWriter{errors.New("no space left on device")},
			Err: &errb,
		},
	}
	code := cmd.Run(rc, []string{".", "-name", "*.txt"})
	if code != 1 {
		t.Errorf("write error: code=%d, want 1", code)
	}
	if !strings.Contains(errb.String(), "find: write error:") {
		t.Errorf("write error: stderr=%q, want a 'find: write error:' diagnostic", errb.String())
	}
	// One diagnostic, not one per path.
	if n := strings.Count(errb.String(), "write error"); n != 1 {
		t.Errorf("write error reported %d times, want 1: %q", n, errb.String())
	}
	// -print0 takes the same path.
	errb.Reset()
	rc.Out = &errWriter{errors.New("no space left on device")}
	if code := cmd.Run(rc, []string{".", "-print0"}); code != 1 ||
		!strings.Contains(errb.String(), "find: write error:") {
		t.Errorf("-print0 write error: code=%d stderr=%q", code, errb.String())
	}
}

// runFindEnv is runFind with an explicit environment.
func runFindEnv(t *testing.T, dir string, env []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}
