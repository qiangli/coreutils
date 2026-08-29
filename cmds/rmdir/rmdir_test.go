package rmdircmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// runTool is the canonical test harness shape for cmds packages:
// output is captured after Run returns.
func runTool(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestRmdirEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, "d")
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("rmdir d: code=%d out=%q err=%q", code, out, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "d")); !os.IsNotExist(err) {
		t.Error("directory still exists")
	}
}

func TestRmdirStopsOptionRecognitionAtFirstOperand(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"ordinary", "-p", "--"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	_, errb, code := runTool(t, dir, "ordinary", "-p", "--")
	if code != 0 || errb != "" {
		t.Fatalf("rmdir operand -p --: code=%d err=%q", code, errb)
	}
	for _, name := range []string{"ordinary", "-p", "--"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("operand %q was not removed: %v", name, err)
		}
	}
}

func TestRmdirNonEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "d", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "d")
	if code != 1 || !strings.Contains(errb, "failed to remove 'd'") ||
		!strings.Contains(strings.ToLower(errb), "not empty") {
		t.Errorf("code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "d", "sub")); err != nil {
		t.Error("non-empty directory contents were removed")
	}
}

func TestRmdirIgnoreFailOnNonEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "d", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "--ignore-fail-on-non-empty", "d")
	if code != 0 || errb != "" {
		t.Fatalf("ignore non-empty: code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "d", "sub")); err != nil {
		t.Error("non-empty directory contents were removed")
	}
}

func TestRmdirNotADirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "f")
	if code != 1 || !strings.Contains(errb, "failed to remove 'f': Not a directory") {
		t.Errorf("code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "f")); err != nil {
		t.Error("file was removed")
	}
}

func TestRmdirMissing(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "nope")
	if code != 1 || !strings.Contains(errb, "failed to remove 'nope'") {
		t.Errorf("code=%d err=%q", code, errb)
	}
}

func TestRmdirParents(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a", "b", "c"), 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join("a", "b", "c")
	out, errb, code := runTool(t, dir, "-pv", nested)
	if code != 0 {
		t.Fatalf("rmdir -pv: code=%d err=%q", code, errb)
	}
	want := "rmdir: removing directory, '" + nested + "'\n" +
		"rmdir: removing directory, '" + filepath.Join("a", "b") + "'\n" +
		"rmdir: removing directory, 'a'\n"
	if out != want {
		t.Errorf("out=%q want %q", out, want)
	}
	if _, err := os.Lstat(filepath.Join(dir, "a")); !os.IsNotExist(err) {
		t.Error("ancestors not removed")
	}
}

func TestRmdirParentsExplicitCurrentDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-p", "./a/b")
	if code != 1 || !strings.Contains(errb, "failed to remove '.'") {
		t.Errorf("code=%d err=%q", code, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "a")); !os.IsNotExist(err) {
		t.Error("explicit current-directory path did not remove its empty ancestors")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("working directory was removed: %v", err)
	}
}

func TestRmdirParentsStopsOnNonEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a", "keep"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-p", filepath.Join("a", "b"))
	if code != 1 || !strings.Contains(errb, "failed to remove 'a'") {
		t.Errorf("code=%d err=%q", code, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "a", "b")); !os.IsNotExist(err) {
		t.Error("operand itself not removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "a", "keep")); err != nil {
		t.Error("sibling file lost")
	}
}

func TestRmdirVerbose(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, _, code := runTool(t, dir, "-v", "d")
	if code != 0 || out != "rmdir: removing directory, 'd'\n" {
		t.Errorf("rmdir -v: code=%d out=%q", code, out)
	}
}

func TestRmdirContinuesPastErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "nope", "d")
	if code != 1 || !strings.Contains(errb, "failed to remove 'nope'") {
		t.Errorf("code=%d err=%q", code, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "d")); !os.IsNotExist(err) {
		t.Error("later operand not removed after earlier failure")
	}
}

func TestRmdirUsageErrors(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir)
	if code != 2 || !strings.Contains(errb, "missing operand") {
		t.Errorf("no args: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "--frobnicate", "d")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestRmdirHelpAndVersion(t *testing.T) {
	dir := t.TempDir()
	out, _, code := runTool(t, dir, "--help")
	if code != 0 || !strings.Contains(out, "Usage: rmdir") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, dir, "--version")
	if code != 0 || !strings.Contains(out, "rmdir") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}

// TestRmdirDotBare covers the POSIX requirement that rmdir reject a bare
// "." operand with EINVAL.
func TestRmdirDotBare(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, ".")
	if code != 1 || !strings.Contains(errb, "failed to remove '.'") ||
		!strings.Contains(errb, "Invalid argument") {
		t.Errorf("rmdir .: code=%d err=%q", code, errb)
	}
}

// TestRmdirDotDotBareFailsNaturally covers the POSIX distinction between a
// final component of "." (mandatory EINVAL) and ".." (the operation must
// fail, but POSIX leaves the errno to the host).
func TestRmdirDotDotBareFailsNaturally(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "..")
	if code != 1 || !strings.Contains(errb, "failed to remove '..'") {
		t.Errorf("rmdir ..: code=%d err=%q, want the host's native failure", code, errb)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("rmdir .. removed the working directory's parent chain: %v", err)
	}
}

// TestRmdirRealDirectoryDotDotMayBeIgnored proves that a valid child/..
// pathname reaches the host removal call. Hosts which report ENOTEMPTY or
// EEXIST are covered by --ignore-fail-on-non-empty.
func TestRmdirRealDirectoryDotDotMayBeIgnored(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	nativeErr := os.Remove(rawOperandPath(&tool.RunContext{Dir: dir}, "a/.."))
	if nativeErr == nil {
		t.Fatal("host unexpectedly removed a directory through final dot-dot")
	}
	_, errb, code := runTool(t, dir, "--ignore-fail-on-non-empty", "a/..")
	if isNonEmpty(nativeErr) {
		if code != 0 || errb != "" {
			t.Errorf("native non-empty a/.. was not ignored: code=%d err=%q", code, errb)
		}
	} else if code != 1 || errb == "" {
		t.Errorf("native %v a/.. was wrongly ignored: code=%d err=%q", nativeErr, code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "a", "b")); err != nil {
		t.Errorf("ancestors removed despite ignore flag: %v", err)
	}
}

// TestRmdirTrailingDotComponent covers the POSIX EINVAL guarantee for a
// path whose final component is "." — including non-bare forms like
// "a/." and "a/./" where filepath.Clean would otherwise hide the dot.
func TestRmdirTrailingDotComponent(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, op := range []string{"a/.", "a/./"} {
		_, errb, code := runTool(t, dir, op)
		if code != 1 || !strings.Contains(errb, "failed to remove '"+op+"'") ||
			!strings.Contains(errb, "Invalid argument") {
			t.Errorf("rmdir %s: code=%d err=%q", op, code, errb)
		}
	}
	// Directory must not have been removed.
	if _, err := os.Stat(filepath.Join(dir, "a")); err != nil {
		t.Errorf("rmdir a/. removed the directory: %v", err)
	}
}

// TestRmdirTrailingDotDotComponent covers non-bare forms of a final ".."
// component. POSIX requires failure but leaves the errno unspecified, so the
// test checks the outcome and preservation rather than pinning an errno.
func TestRmdirTrailingDotDotComponent(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, op := range []string{"a/..", "a/b/.."} {
		_, errb, code := runTool(t, dir, op)
		if code != 1 || !strings.Contains(errb, "failed to remove '"+op+"'") {
			t.Errorf("rmdir %s: code=%d err=%q, want the host's native failure", op, code, errb)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "a")); err != nil {
		t.Errorf("rmdir a/.. removed the directory: %v", err)
	}
}

// TestRmdirTrailingSlash covers rmdir of a path with a trailing slash —
// a valid empty directory must be removable, and the diagnostic (if any)
// reports the operand as given.
func TestRmdirTrailingSlash(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, _, code := runTool(t, dir, "-v", "d/")
	if code != 0 || !strings.Contains(out, "removing directory, 'd/'") {
		t.Errorf("rmdir -v d/: code=%d out=%q", code, out)
	}
	if _, err := os.Lstat(filepath.Join(dir, "d")); !os.IsNotExist(err) {
		t.Error("trailing-slash directory not removed")
	}
}

// TestRmdirIgnoreNonEmptyWithParents verifies that --ignore-fail-on-non-empty
// suppresses the error for a non-empty ancestor during a -p walk without
// affecting directories that cannot be removed for other reasons.
func TestRmdirIgnoreNonEmptyWithParents(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a", "b", "c"), 0o755); err != nil {
		t.Fatal(err)
	}
	// a is non-empty even after removing b/c chain is attempted: put a
	// sibling file in a so that the -p walk hits a non-empty ancestor.
	if err := os.WriteFile(filepath.Join(dir, "a", "keep"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-p", "--ignore-fail-on-non-empty", filepath.Join("a", "b", "c"))
	// c and b are removed; a is non-empty → ignored (no error, exit 0).
	if code != 0 || errb != "" {
		t.Errorf("ignore-non-empty -p: code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "a", "b")); !os.IsNotExist(err) {
		t.Error("b not removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "a", "keep")); err != nil {
		t.Error("sibling file lost")
	}
}

// TestRmdirIgnoreNonEmptyDoesNotIgnoreOtherErrors verifies that
// --ignore-fail-on-non-empty only suppresses ENOTEMPTY/EEXIST, not
// missing-directory or not-a-directory failures.
func TestRmdirIgnoreNonEmptyDoesNotIgnoreOtherErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Missing directory → still an error even with the flag.
	_, errb, code := runTool(t, dir, "--ignore-fail-on-non-empty", "nope")
	if code != 1 || !strings.Contains(errb, "failed to remove 'nope'") {
		t.Errorf("missing dir not reported: code=%d err=%q", code, errb)
	}
	// Regular file → still an error even with the flag.
	_, errb, code = runTool(t, dir, "--ignore-fail-on-non-empty", "f")
	if code != 1 || !strings.Contains(errb, "Not a directory") {
		t.Errorf("not-a-directory not reported: code=%d err=%q", code, errb)
	}
}

// TestRmdirMissingPrefixDotDotIsNotCleaned guards against resolving a
// relative operand with filepath.Join. The lexical clean would collapse
// missing/.. to the invocation directory and, when that directory is empty,
// remove it. Native pathname resolution must instead fail at missing.
func TestRmdirMissingPrefixDotDotIsNotCleaned(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "work")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "--ignore-fail-on-non-empty", "missing/..")
	if code != 1 || !strings.Contains(errb, "failed to remove 'missing/..'") ||
		!strings.Contains(strings.ToLower(errb), "no such file or directory") {
		t.Errorf("missing/..: code=%d err=%q, want ENOENT-like failure", code, errb)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("missing/.. removed the invocation directory: %v", err)
	}
}

// TestRmdirNonDirectoryPrefixDotDotIsNotIgnored ensures an ENOTDIR-like
// failure from an intermediate component is not reclassified as a removable
// non-empty directory merely because filepath.Clean would erase that prefix.
func TestRmdirNonDirectoryPrefixDotDotIsNotIgnored(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "--ignore-fail-on-non-empty", "f/..")
	if code != 1 || !strings.Contains(errb, "failed to remove 'f/..'") ||
		!strings.Contains(strings.ToLower(errb), "not a directory") {
		t.Errorf("f/..: code=%d err=%q, want ENOTDIR-like failure", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "f")); err != nil {
		t.Fatalf("f/.. changed its non-directory prefix: %v", err)
	}
}

// TestRmdirOperandOrderMatters covers the POSIX requirement that dir
// operands are processed "in the order specified": given a directory and
// its own child as two separate operands, each operand is attempted exactly
// once at the point it is reached, so the two orderings leave the tree in
// different states. Parent-then-child: the parent fails (still non-empty at
// that point) but is never retried once the child empties it afterward, so
// the parent survives, now empty, and only the child is removed.
// Child-then-parent: the child empties the parent in time for the parent's
// own turn, so both are removed.
func TestRmdirOperandOrderMatters(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "a", filepath.Join("a", "b"))
	if code != 1 || !strings.Contains(errb, "failed to remove 'a'") {
		t.Errorf("parent-then-child: code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "a")); err != nil {
		t.Errorf("parent-then-child: parent operand not left in place after its own failure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a", "b")); !os.IsNotExist(err) {
		t.Errorf("parent-then-child: child operand not removed on its own turn: %v", err)
	}

	dir2 := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir2, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code = runTool(t, dir2, filepath.Join("a", "b"), "a")
	if code != 0 || errb != "" {
		t.Errorf("child-then-parent: code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir2, "a")); !os.IsNotExist(err) {
		t.Errorf("child-then-parent: parent not removed: %v", err)
	}
}

// TestRmdirParentsWithTrailingSlash covers the interaction between -p and a
// trailing slash on the operand: the ancestor walk must still compute the
// same ancestor chain as the non-trailing-slash form.
func TestRmdirParentsWithTrailingSlash(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-p", filepath.Join("a", "b")+"/")
	if code != 0 || errb != "" {
		t.Errorf("rmdir -p a/b/: code=%d err=%q", code, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "a")); !os.IsNotExist(err) {
		t.Error("ancestor not removed when leaf operand had a trailing slash")
	}
}

// TestRmdirDoubleDashOperand verifies that -- terminates option parsing
// so operands beginning with - are treated as directory names.
func TestRmdirDoubleDashOperand(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"-p", "-v"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
		_, _, code := runTool(t, dir, "--", name)
		if code != 0 {
			t.Errorf("rmdir -- %s: code=%d", name, code)
		}
		if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("rmdir -- %s did not remove the directory", name)
		}
	}
}

// TestRmdirDashOperand verifies that a lone "-" is treated as a filename,
// not as an option introducer (matching POSIX utility syntax).
func TestRmdirDashOperand(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "-"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, _, code := runTool(t, dir, "-")
	if code != 0 {
		t.Errorf("rmdir -: code=%d", code)
	}
	if _, err := os.Lstat(filepath.Join(dir, "-")); !os.IsNotExist(err) {
		t.Error("rmdir - did not remove the directory")
	}
}

// rmdirPanicReader proves a code path never reads standard input: POSIX
// documents rmdir's STDIN as "Not used", and rmdir has no interactive
// prompt of its own to justify ever touching it.
type rmdirPanicReader struct{}

func (rmdirPanicReader) Read([]byte) (int, error) {
	panic("rmdir read standard input")
}

// TestRmdirDoesNotConsumeStdin covers the POSIX STDIN requirement across a
// representative mix of successful, failing, and -p invocations.
func TestRmdirDoesNotConsumeStdin(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "nonempty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nonempty", "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) (stdout, stderr string, code int) {
		t.Helper()
		var out, errb bytes.Buffer
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			Dir:   dir,
			Stdio: tool.Stdio{In: rmdirPanicReader{}, Out: &out, Err: &errb},
		}
		code = cmd.Run(rc, args)
		return out.String(), errb.String(), code
	}
	if _, _, code := run("-p", filepath.Join("a", "b")); code != 0 {
		t.Errorf("rmdir -p a/b: code=%d", code)
	}
	if _, errb, code := run("nonempty"); code != 1 || !strings.Contains(errb, "not empty") {
		t.Errorf("rmdir nonempty: code=%d err=%q", code, errb)
	}
	if _, errb, code := run("does-not-exist"); code != 1 || errb == "" {
		t.Errorf("rmdir does-not-exist: code=%d err=%q", code, errb)
	}
}

// rmdirErrWriter simulates a broken standard error stream.
type rmdirErrWriter struct{}

func (rmdirErrWriter) Write([]byte) (int, error) {
	return 0, os.ErrClosed
}

// TestRmdirDiagnosticWriteFailureStillFails verifies that a broken standard
// error stream does not mask an operand failure: exit status must still
// reflect the failed removal even though the diagnostic itself could not be
// written.
func TestRmdirDiagnosticWriteFailureStillFails(t *testing.T) {
	dir := t.TempDir()
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &bytes.Buffer{}, Err: rmdirErrWriter{}},
	}
	code := cmd.Run(rc, []string{"does-not-exist"})
	if code != 1 {
		t.Errorf("rmdir with broken stderr: code=%d, want 1", code)
	}
}

// TestRmdirPOSIXEmptyStringOperand covers the POSIX requirement that rmdir()
// with an empty pathname fails. An empty string is not a valid directory.
func TestRmdirPOSIXEmptyStringOperand(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "")
	if code != 1 || !strings.Contains(errb, "failed to remove ''") {
		t.Errorf("rmdir '': code=%d err=%q", code, errb)
	}
}

// TestRmdirPOSIXParentsSingleComponent covers the POSIX -p spec: "If the dir
// operand includes more than one pathname component, effects equivalent to
// rmdir -p $(dirname dir) shall occur." A single component (no directory
// separator) removes only itself — no ancestor walk.
func TestRmdirPOSIXParentsSingleComponent(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "only"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-p", "only")
	if code != 0 || errb != "" {
		t.Errorf("rmdir -p only: code=%d err=%q", code, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "only")); !os.IsNotExist(err) {
		t.Error("single component not removed")
	}
}

// TestRmdirPOSIXStdoutNotUsed verifies that without the GNU -v extension,
// rmdir produces no standard output, matching the POSIX Issue 7 STDOUT
// requirement ("Not used").
func TestRmdirPOSIXStdoutNotUsed(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Successful removal — no stdout.
	out, errb, code := runTool(t, dir, "-p", filepath.Join("a", "b"))
	if code != 0 || out != "" || errb != "" {
		t.Errorf("rmdir -p a/b: out=%q err=%q code=%d", out, errb, code)
	}
	// Failed removal (missing) — diagnostics go to stderr, not stdout.
	out, errb, code = runTool(t, dir, "nope")
	if code != 1 || out != "" || errb == "" {
		t.Errorf("rmdir nope: out=%q err=%q code=%d", out, errb, code)
	}
}

// TestRmdirPOSIXSymlinkToDirRejected verifies that rmdir rejects a symlink
// that points to a directory. POSIX rmdir() shall fail with ENOTDIR if the
// path names a symbolic link.
func TestRmdirPOSIXSymlinkToDirRejected(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlinks not available: %v", err)
	}
	_, errb, code := runTool(t, dir, "link")
	if code != 1 || !strings.Contains(errb, "failed to remove 'link'") ||
		!strings.Contains(errb, "Not a directory") {
		t.Errorf("rmdir link: code=%d err=%q", code, errb)
	}
	// real directory must not have been removed.
	if _, err := os.Stat(filepath.Join(dir, "real")); err != nil {
		t.Errorf("real directory was affected: %v", err)
	}
}
