package testcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

// runIn is the canonical harness shape, parameterized by the invoked
// name: `test` and `[` are one implementation and every case has to be
// runnable through either spelling.
func runIn(t *testing.T, cmdt *tool.Tool, dir string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		FS:    tool.NewLocalFS(),
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code = cmdt.Run(rc, args)
	return out.String(), errb.String(), code
}

// run evaluates an expression through `test` in a scratch directory.
func runTest(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	return runIn(t, cmd, t.TempDir(), args...)
}

// runBracket evaluates through `[`, appending nothing: callers pass the
// closing bracket themselves, because its presence is what is under
// test in several cases.
func runBracket(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	return runIn(t, bracketCmd, t.TempDir(), args...)
}

// truth asserts an expression's exit status and that nothing was
// written to either stream — a true/false answer is carried by the exit
// status alone.
func truth(t *testing.T, want int, args ...string) {
	t.Helper()
	out, errb, code := runTest(t, args...)
	if code != want {
		t.Errorf("test %q = %d, want %d (stderr %q)", args, code, want, errb)
	}
	if out != "" || errb != "" {
		t.Errorf("test %q wrote stdout=%q stderr=%q, want silence", args, out, errb)
	}
}

func TestRegistration(t *testing.T) {
	for _, name := range []string{"test", "["} {
		got := tool.Lookup(name)
		if got == nil {
			t.Fatalf("%q is not registered", name)
		}
		if got.Name != name {
			t.Errorf("Lookup(%q).Name = %q", name, got.Name)
		}
		if got.Synopsis == "" || got.Usage == "" || got.Run == nil {
			t.Errorf("%q: incomplete tool entry", name)
		}
	}
	if tool.Lookup("test") == tool.Lookup("[") {
		t.Error("test and [ must be distinct entries (they differ on option handling)")
	}
}

// --- operand-count dispatch: the POSIX forms, 0 through 4 -----------------

func TestZeroOperands(t *testing.T) {
	// POSIX: an omitted expression is false.
	truth(t, statusFalse)

	// `[ ]` is the same expression with the bracket stripped.
	out, errb, code := runBracket(t, "]")
	if code != statusFalse || out != "" || errb != "" {
		t.Errorf("[ ] = (%q, %q, %d), want (\"\", \"\", 1)", out, errb, code)
	}
}

func TestOneOperand(t *testing.T) {
	// A lone operand is true exactly when it is not the empty string —
	// including operands that are operators everywhere else.
	truth(t, statusTrue, "x")
	truth(t, statusTrue, "0")
	truth(t, statusTrue, "false")
	truth(t, statusTrue, "!")
	truth(t, statusTrue, "(")
	truth(t, statusTrue, "-e")
	truth(t, statusTrue, "-a")
	truth(t, statusTrue, " ")
	truth(t, statusFalse, "")
}

func TestTwoOperands(t *testing.T) {
	truth(t, statusTrue, "-n", "x")
	truth(t, statusFalse, "-n", "")
	truth(t, statusTrue, "-z", "")
	truth(t, statusFalse, "-z", "x")

	// `!` negates the one-operand form.
	truth(t, statusFalse, "!", "x")
	truth(t, statusTrue, "!", "")
	// The operand of `!` is a string, not an operator: `-e` is non-empty.
	truth(t, statusFalse, "!", "-e")
}

func TestThreeOperands(t *testing.T) {
	// A binary primary in the middle wins over every other reading —
	// including when the first operand looks like a unary operator.
	truth(t, statusTrue, "-e", "=", "-e")
	truth(t, statusFalse, "-e", "=", "-f")

	// `!` applied to the two-operand form.
	truth(t, statusTrue, "!", "-n", "")
	truth(t, statusFalse, "!", "-z", "")

	// Parenthesized single operand.
	truth(t, statusTrue, "(", "x", ")")
	truth(t, statusFalse, "(", "", ")")

	// -a / -o joining two one-operand tests.
	truth(t, statusTrue, "x", "-a", "y")
	truth(t, statusFalse, "x", "-a", "")
	truth(t, statusTrue, "", "-o", "y")
	truth(t, statusFalse, "", "-o", "")
}

func TestFourOperands(t *testing.T) {
	// `!` applied to the three-operand form.
	truth(t, statusFalse, "!", "a", "=", "a")
	truth(t, statusTrue, "!", "a", "=", "b")
	truth(t, statusFalse, "!", "(", "x", ")")

	// `( EXPR )` around the two-operand form.
	truth(t, statusTrue, "(", "-n", "x", ")")
	truth(t, statusFalse, "(", "-z", "x", ")")
	truth(t, statusTrue, "(", "!", "", ")")

	// Four operands that are neither shape are unspecified by POSIX and
	// fall through to the documented grammar.
	truth(t, statusTrue, "-n", "x", "-a", "y")
	truth(t, statusFalse, "-z", "x", "-o", "")
}

// --- the grammar beyond four operands ------------------------------------

func TestOperatorPrecedence(t *testing.T) {
	// -a binds tighter than -o: ("" -a x) -o y is true.
	truth(t, statusTrue, "", "-a", "x", "-o", "y")
	// x -o ("" -a y) is true via the left branch.
	truth(t, statusTrue, "x", "-o", "", "-a", "y")
	// "" -o (x -a "") is false.
	truth(t, statusFalse, "", "-o", "x", "-a", "")
	// Chained -a is all-of; chained -o is any-of.
	truth(t, statusTrue, "a", "-a", "b", "-a", "c")
	truth(t, statusFalse, "a", "-a", "", "-a", "c")
	truth(t, statusFalse, "", "-o", "", "-o", "")
	truth(t, statusTrue, "", "-o", "", "-o", "c")
}

func TestNegationParity(t *testing.T) {
	truth(t, statusTrue, "!", "!", "x", "-a", "y")
	truth(t, statusFalse, "!", "!", "!", "x", "-a", "y")
	// `!` binds to the term, not to the whole -a chain.
	truth(t, statusFalse, "!", "x", "-a", "y")
	truth(t, statusTrue, "!", "", "-a", "y")

	// Inside the POSIX operand counts the negations are still just
	// operands once one of them is consumed as the operator: `! ! !`
	// negates `! !`, which negates the non-empty string "!".
	truth(t, statusFalse, "!", "!")
	truth(t, statusTrue, "!", "!", "!")
	truth(t, statusFalse, "!", "!", "!", "!")
}

func TestParenGrouping(t *testing.T) {
	// Without parens -a wins; with them -o is evaluated first.
	truth(t, statusFalse, "", "-a", "x", "-o", "")
	truth(t, statusTrue, "(", "", "-o", "x", ")", "-a", "y")
	truth(t, statusFalse, "(", "", "-o", "", ")", "-a", "y")
	// Nested groups.
	truth(t, statusTrue, "(", "(", "x", ")", ")")
	truth(t, statusTrue, "!", "(", "", "-o", "", ")")
}

func TestNoShortCircuit(t *testing.T) {
	// Both operators evaluate every branch, so a malformed branch is
	// still reported even when the answer was already settled.
	_, errb, code := runTest(t, "x", "-o", "-q", "y")
	if code != statusSyntax {
		t.Errorf("`x -o -q y` = %d, want 2", code)
	}
	if want := "test: '-q': unary operator expected\n"; errb != want {
		t.Errorf("stderr = %q, want %q", errb, want)
	}

	_, errb, code = runTest(t, "", "-a", "-q", "y")
	if code != statusSyntax || !strings.Contains(errb, "'-q'") {
		t.Errorf("`\"\" -a -q y` = %d, stderr %q", code, errb)
	}
}

// --- string and integer primaries ----------------------------------------

func TestStringComparison(t *testing.T) {
	truth(t, statusTrue, "abc", "=", "abc")
	truth(t, statusFalse, "abc", "=", "abd")
	truth(t, statusTrue, "abc", "==", "abc")
	truth(t, statusFalse, "abc", "==", "abd")
	truth(t, statusTrue, "abc", "!=", "abd")
	truth(t, statusFalse, "abc", "!=", "abc")

	// Byte order, i.e. LC_ALL=C: uppercase sorts before lowercase, and a
	// prefix sorts before its extension.
	truth(t, statusTrue, "Z", "<", "a")
	truth(t, statusFalse, "a", "<", "Z")
	truth(t, statusTrue, "a", ">", "Z")
	truth(t, statusTrue, "ab", "<", "abc")
	truth(t, statusFalse, "abc", "<", "abc")
	truth(t, statusFalse, "abc", ">", "abc")

	// Empty operands are ordinary strings.
	truth(t, statusTrue, "", "=", "")
	truth(t, statusTrue, "", "<", "a")
}

func TestIntegerComparison(t *testing.T) {
	cases := []struct {
		args []string
		want int
	}{
		{[]string{"1", "-eq", "1"}, statusTrue},
		{[]string{"1", "-eq", "2"}, statusFalse},
		{[]string{"1", "-ne", "2"}, statusTrue},
		{[]string{"2", "-gt", "1"}, statusTrue},
		{[]string{"1", "-gt", "1"}, statusFalse},
		{[]string{"1", "-ge", "1"}, statusTrue},
		{[]string{"1", "-lt", "2"}, statusTrue},
		{[]string{"2", "-lt", "2"}, statusFalse},
		{[]string{"2", "-le", "2"}, statusTrue},
		// Numeric, not lexical: "10" is greater than "9".
		{[]string{"10", "-gt", "9"}, statusTrue},
		// Signs and redundant zeros.
		{[]string{"-1", "-lt", "0"}, statusTrue},
		{[]string{"+1", "-eq", "1"}, statusTrue},
		{[]string{"007", "-eq", "7"}, statusTrue},
		{[]string{"-0", "-eq", "0"}, statusTrue},
		// Surrounding blanks are stripped before parsing.
		{[]string{" 3 ", "-eq", "3"}, statusTrue},
		{[]string{"\t3", "-eq", "3"}, statusTrue},
		// Arbitrary precision: these exceed int64 in both directions.
		{[]string{"99999999999999999999999", "-gt", "99999999999999999999998"}, statusTrue},
		{[]string{"-99999999999999999999999", "-lt", "0"}, statusTrue},
	}
	for _, c := range cases {
		truth(t, c.want, c.args...)
	}
}

func TestIntegerErrors(t *testing.T) {
	for _, args := range [][]string{
		{"x", "-eq", "1"},
		{"1", "-eq", "x"},
		{"", "-eq", "1"},
		{"1.5", "-eq", "1"},
		{"0x10", "-eq", "16"},
		{"1", "-eq", "1 2"},
		{"-", "-eq", "1"},
	} {
		_, errb, code := runTest(t, args...)
		if code != statusSyntax {
			t.Errorf("test %q = %d, want 2", args, code)
		}
		if !strings.HasPrefix(errb, "test: invalid integer ") {
			t.Errorf("test %q stderr = %q, want an invalid-integer diagnostic", args, errb)
		}
	}

	// The diagnostic quotes the operand exactly as given, blanks and all.
	_, errb, _ := runTest(t, "1", "-eq", "x y")
	if want := "test: invalid integer 'x y'\n"; errb != want {
		t.Errorf("stderr = %q, want %q", errb, want)
	}
}

// --- file primaries -------------------------------------------------------

func TestFilePrimaries(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "file"), "hello")
	mustWrite(t, filepath.Join(dir, "empty"), "")
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		args []string
		want int
	}{
		{[]string{"-e", "file"}, statusTrue},
		{[]string{"-e", "sub"}, statusTrue},
		{[]string{"-e", "missing"}, statusFalse},
		// -a is the deprecated spelling of -e in the unary position.
		{[]string{"-a", "file"}, statusTrue},
		{[]string{"-a", "missing"}, statusFalse},
		{[]string{"-f", "file"}, statusTrue},
		{[]string{"-f", "sub"}, statusFalse},
		{[]string{"-f", "missing"}, statusFalse},
		{[]string{"-d", "sub"}, statusTrue},
		{[]string{"-d", "file"}, statusFalse},
		{[]string{"-d", "missing"}, statusFalse},
		{[]string{"-s", "file"}, statusTrue},
		{[]string{"-s", "empty"}, statusFalse},
		{[]string{"-s", "missing"}, statusFalse},
		// Types this file is not.
		{[]string{"-b", "file"}, statusFalse},
		{[]string{"-c", "file"}, statusFalse},
		{[]string{"-p", "file"}, statusFalse},
		{[]string{"-S", "file"}, statusFalse},
		{[]string{"-u", "file"}, statusFalse},
		{[]string{"-g", "file"}, statusFalse},
		{[]string{"-k", "file"}, statusFalse},
		{[]string{"-h", "file"}, statusFalse},
		{[]string{"-L", "file"}, statusFalse},
		{[]string{"-h", "missing"}, statusFalse},
	}
	for _, c := range cases {
		out, errb, code := runIn(t, cmd, dir, c.args...)
		if code != c.want || out != "" || errb != "" {
			t.Errorf("test %q = (%q, %q, %d), want code %d", c.args, out, errb, code, c.want)
		}
	}
}

func TestFileComparisonPrimaries(t *testing.T) {
	dir := t.TempDir()
	older := filepath.Join(dir, "older")
	newer := filepath.Join(dir, "newer")
	mustWrite(t, older, "a")
	mustWrite(t, newer, "b")

	base := time.Now().Add(-time.Hour)
	chtimes(t, older, base, base)
	chtimes(t, newer, base.Add(time.Minute), base.Add(time.Minute))

	cases := []struct {
		args []string
		want int
	}{
		// -ef: same device and inode. A path is always the same file as
		// itself; two separate files never are.
		{[]string{"older", "-ef", "older"}, statusTrue},
		{[]string{"older", "-ef", "newer"}, statusFalse},
		{[]string{"older", "-ef", "missing"}, statusFalse},
		{[]string{"missing", "-ef", "missing"}, statusFalse},

		{[]string{"newer", "-nt", "older"}, statusTrue},
		{[]string{"older", "-nt", "newer"}, statusFalse},
		{[]string{"older", "-nt", "older"}, statusFalse},
		// FILE2 missing makes an existing FILE1 newer; a missing FILE1
		// is never newer than anything.
		{[]string{"older", "-nt", "missing"}, statusTrue},
		{[]string{"missing", "-nt", "older"}, statusFalse},
		{[]string{"missing", "-nt", "missing"}, statusFalse},

		{[]string{"older", "-ot", "newer"}, statusTrue},
		{[]string{"newer", "-ot", "older"}, statusFalse},
		{[]string{"older", "-ot", "older"}, statusFalse},
		{[]string{"missing", "-ot", "older"}, statusTrue},
		{[]string{"older", "-ot", "missing"}, statusFalse},
		{[]string{"missing", "-ot", "missing"}, statusFalse},
	}
	for _, c := range cases {
		out, errb, code := runIn(t, cmd, dir, c.args...)
		if code != c.want || out != "" || errb != "" {
			t.Errorf("test %q = (%q, %q, %d), want code %d", c.args, out, errb, code, c.want)
		}
	}
}

func TestFileModifiedSinceRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	mustWrite(t, path, "x")

	base := time.Now().Add(-time.Hour)
	// Modified after it was last read.
	chtimes(t, path, base, base.Add(time.Minute))
	if _, _, code := runIn(t, cmd, dir, "-N", "f"); code != statusTrue {
		t.Errorf("-N with mtime > atime = %d, want 0", code)
	}
	// Read after it was last modified.
	chtimes(t, path, base.Add(time.Minute), base)
	if _, _, code := runIn(t, cmd, dir, "-N", "f"); code != statusFalse {
		t.Errorf("-N with atime > mtime = %d, want 1", code)
	}
	// A file that does not exist has not been modified.
	if _, errb, code := runIn(t, cmd, dir, "-N", "missing"); code != statusFalse || errb != "" {
		t.Errorf("-N missing = (%q, %d), want (\"\", 1)", errb, code)
	}
}

func TestSymlinkPrimaries(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	mustWrite(t, target, "x")
	if err := os.Symlink(target, filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(filepath.Join(dir, "nothing"), filepath.Join(dir, "dangling")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	cases := []struct {
		args []string
		want int
	}{
		// -h/-L do not follow the link; every other primary does.
		{[]string{"-h", "link"}, statusTrue},
		{[]string{"-L", "link"}, statusTrue},
		{[]string{"-h", "target"}, statusFalse},
		{[]string{"-h", "dangling"}, statusTrue},
		{[]string{"-L", "dangling"}, statusTrue},
		{[]string{"-e", "link"}, statusTrue},
		{[]string{"-f", "link"}, statusTrue},
		// A dangling link resolves to nothing, so -e is false even
		// though the link itself exists.
		{[]string{"-e", "dangling"}, statusFalse},
		{[]string{"-f", "dangling"}, statusFalse},
		// -ef follows both operands, so a link is the file it names.
		{[]string{"link", "-ef", "target"}, statusTrue},
	}
	for _, c := range cases {
		out, errb, code := runIn(t, cmd, dir, c.args...)
		if code != c.want || out != "" || errb != "" {
			t.Errorf("test %q = (%q, %q, %d), want code %d", c.args, out, errb, code, c.want)
		}
	}
}

// TestOperandsResolveAgainstRunContext pins the framework rule: every
// file operand goes through rc.Path, so a relative operand names a file
// under the invocation's directory and never under the process cwd.
func TestOperandsResolveAgainstRunContext(t *testing.T) {
	here, elsewhere := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(here, "marker"), "x")

	if _, _, code := runIn(t, cmd, here, "-f", "marker"); code != statusTrue {
		t.Errorf("-f marker in its own dir = %d, want 0", code)
	}
	if _, _, code := runIn(t, cmd, elsewhere, "-f", "marker"); code != statusFalse {
		t.Errorf("-f marker in another dir = %d, want 1", code)
	}
	// An absolute operand is independent of the directory.
	abs := filepath.Join(here, "marker")
	if _, _, code := runIn(t, cmd, elsewhere, "-f", abs); code != statusTrue {
		t.Errorf("-f %s from another dir = %d, want 0", abs, code)
	}
	// The process cwd must not leak in: this test's own working
	// directory is the package dir, which contains test.go.
	if _, _, code := runIn(t, cmd, elsewhere, "-f", "test.go"); code != statusFalse {
		t.Error("-f test.go resolved against the process cwd, not rc.Dir")
	}
}

// --- -t FD ---------------------------------------------------------------

func TestTerminalPrimary(t *testing.T) {
	// Streams that are not *os.File cannot be terminals.
	truth(t, statusFalse, "-t", "0")
	truth(t, statusFalse, "-t", "1")
	truth(t, statusFalse, "-t", "2")
	// Out-of-range and negative descriptors are not terminals.
	truth(t, statusFalse, "-t", "-1")
	truth(t, statusFalse, "-t", "99999999999999999999999")

	// A real file is a *os.File but not a terminal.
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		FS:    tool.NewLocalFS(),
		Stdio: tool.Stdio{In: f, Out: &out, Err: &errb},
	}
	if code := cmd.Run(rc, []string{"-t", "0"}); code != statusFalse {
		t.Errorf("-t 0 on %s = %d, want 1", os.DevNull, code)
	}
	if code := cmd.Run(rc, []string{"-t", strconv.FormatUint(uint64(f.Fd()), 10)}); code != statusFalse {
		t.Errorf("-t %d on %s = %d, want 1", f.Fd(), os.DevNull, code)
	}

	// A non-numeric descriptor is a syntax error, not a false answer.
	_, errb2, code := runTest(t, "-t", "tty")
	if code != statusSyntax || errb2 != "test: invalid integer 'tty'\n" {
		t.Errorf("-t tty = (%q, %d), want an invalid-integer error", errb2, code)
	}
}

// --- syntax and usage errors ---------------------------------------------

func TestSyntaxErrors(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		// An operand that is neither a unary primary nor the start of a
		// two-operand form. At two operands the diagnostic names the
		// operand in the operator position, which is the first one.
		{[]string{"-q", "x"}, "test: '-q': unary operator expected\n"},
		{[]string{"x", "y"}, "test: 'x': unary operator expected\n"},
		{[]string{"x", "-a"}, "test: 'x': unary operator expected\n"},
		{[]string{"--help", "x"}, "test: '--help': unary operator expected\n"},
		// At three operands the middle one has to be a binary primary,
		// -a, or -o.
		{[]string{"x", "y", "z"}, "test: 'y': binary operator expected\n"},
		{[]string{"-n", "x", "-a"}, "test: 'x': binary operator expected\n"},
		// Everything after a complete expression is extra.
		{[]string{"x", "y", "z", "w", "v"}, "test: extra argument 'y'\n"},
		// A group that is never closed.
		{[]string{"(", "-n", "x", "-a", "y"}, "test: ')' expected\n"},
		{[]string{"(", "x", "-o", "y"}, "test: ')' expected\n"},
		// An operator left dangling at the end of the expression. The
		// diagnostic names the last operand, which is the dangling one.
		{[]string{"a", "-a", "b", "-a", "c", "-a"}, "test: missing argument after '-a'\n"},
		{[]string{"a", "-o", "b", "-o", "c", "-o"}, "test: missing argument after '-o'\n"},
		// A unary primary with no operand left to apply to.
		{[]string{"x", "-a", "y", "-a", "-n"}, "test: missing argument after '-n'\n"},
		// -o is the OR operator only; it is never a unary primary.
		{[]string{"-o", "name"}, "test: '-o': unary operator expected\n"},
		// -l (a string-length modifier some shells accept in front of a
		// numeric comparison) is not implemented, and says so.
		{[]string{"-l", "abc", "-eq", "3"}, "test: '-l': unary operator expected\n"},
	}
	for _, c := range cases {
		out, errb, code := runTest(t, c.args...)
		if code != statusSyntax {
			t.Errorf("test %q = %d, want 2", c.args, code)
		}
		if errb != c.want {
			t.Errorf("test %q stderr = %q, want %q", c.args, errb, c.want)
		}
		if out != "" {
			t.Errorf("test %q wrote %q to stdout", c.args, out)
		}
	}
}

// TestDiagnosticsAreDeterministic pins the contract's determinism bar:
// the same operands produce byte-identical diagnostics every time, and
// the invoked name is the one that appears in them.
func TestDiagnosticsAreDeterministic(t *testing.T) {
	args := []string{"x", "y", "z"}
	_, first, _ := runTest(t, args...)
	for range 3 {
		if _, again, _ := runTest(t, args...); again != first {
			t.Fatalf("diagnostic varied between runs: %q vs %q", first, again)
		}
	}
	// The bracket spelling names itself, not "test".
	_, errb, code := runBracket(t, "x", "y", "z", "]")
	if code != statusSyntax || errb != "[: 'y': binary operator expected\n" {
		t.Errorf("[ x y z ] = (%q, %d)", errb, code)
	}
}

// --- the `[` spelling -----------------------------------------------------

func TestBracketRequiresClosingBracket(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"x"},
		{"-n", "x"},
		{"x", "]", "y"},
		{"]", "x"},
	} {
		out, errb, code := runBracket(t, args...)
		if code != statusSyntax {
			t.Errorf("[ %q = %d, want 2", args, code)
		}
		if want := "[: missing ']'\n"; errb != want {
			t.Errorf("[ %q stderr = %q, want %q", args, errb, want)
		}
		if out != "" {
			t.Errorf("[ %q wrote %q to stdout", args, out)
		}
	}
}

func TestBracketEvaluatesLikeTest(t *testing.T) {
	// Every form is the same expression once the bracket is stripped.
	cases := []struct {
		args []string
		want int
	}{
		{[]string{"]"}, statusFalse},
		{[]string{"x", "]"}, statusTrue},
		{[]string{"", "]"}, statusFalse},
		{[]string{"-n", "x", "]"}, statusTrue},
		{[]string{"a", "=", "a", "]"}, statusTrue},
		{[]string{"!", "a", "=", "a", "]"}, statusFalse},
		{[]string{"1", "-lt", "2", "-a", "3", "-lt", "4", "]"}, statusTrue},
		// A `]` in the middle is an ordinary string operand.
		{[]string{"]", "=", "]", "]"}, statusTrue},
	}
	for _, c := range cases {
		out, errb, code := runBracket(t, c.args...)
		if code != c.want || out != "" || errb != "" {
			t.Errorf("[ %q = (%q, %q, %d), want code %d", c.args, out, errb, code, c.want)
		}
	}
}

// TestOptionHandling pins the one place the two names differ, which is
// upstream's rule: only `[` recognizes the long options, only as its
// single argument, and only before the closing bracket is considered.
func TestOptionHandling(t *testing.T) {
	// `test` has no options at all: these are non-empty strings.
	for _, arg := range []string{"--help", "--version", "--he"} {
		out, errb, code := runTest(t, arg)
		if code != statusTrue || out != "" || errb != "" {
			t.Errorf("test %s = (%q, %q, %d), want a true string test", arg, out, errb, code)
		}
	}

	// `[ --help` / `[ --version` are the documented spellings.
	out, errb, code := runBracket(t, "--help")
	if code != statusTrue || errb != "" {
		t.Errorf("[ --help = (%q, %d), want help on stdout", errb, code)
	}
	for _, want := range []string{"Usage: [ EXPRESSION ]", "-z STRING", "INTEGER1 -eq INTEGER2", "-ef FILE2"} {
		if !strings.Contains(out, want) {
			t.Errorf("[ --help output is missing %q", want)
		}
	}

	out, errb, code = runBracket(t, "--version")
	if code != statusTrue || errb != "" {
		t.Errorf("[ --version = (%q, %d)", errb, code)
	}
	if !strings.HasPrefix(out, "[ (qiangli/coreutils) ") {
		t.Errorf("[ --version = %q", out)
	}

	// A well-formed bracket expression is never an option: these are
	// one-operand string tests that are true and silent.
	for _, arg := range []string{"--help", "--version"} {
		out, errb, code := runBracket(t, arg, "]")
		if code != statusTrue || out != "" || errb != "" {
			t.Errorf("[ %s ] = (%q, %q, %d), want a true string test", arg, out, errb, code)
		}
	}

	// No abbreviation expansion: `[ --he` is a one-operand string test,
	// so it is a missing-bracket error rather than help.
	out, errb, code = runBracket(t, "--he")
	if code != statusSyntax || out != "" || errb != "[: missing ']'\n" {
		t.Errorf("[ --he = (%q, %q, %d), want a missing-']' error", out, errb, code)
	}

	// The options are recognized only as the whole argument list.
	_, errb, code = runBracket(t, "--help", "x")
	if code != statusSyntax || errb != "[: missing ']'\n" {
		t.Errorf("[ --help x = (%q, %d), want a missing-']' error", errb, code)
	}
}

func mustWrite(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func chtimes(t *testing.T, path string, atime, mtime time.Time) {
	t.Helper()
	if err := os.Chtimes(path, atime, mtime); err != nil {
		t.Fatal(err)
	}
}
