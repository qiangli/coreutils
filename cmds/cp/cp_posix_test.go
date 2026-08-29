package cpcmd

// POSIX.1-2016 (Issue 7) interface-evidence tests: description step
// ordering, operand rules, and per-source continuation that the
// original suite did not pin.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestCpInteractiveSameFileDiagnosesWithoutPrompt pins Issue 7 step 1
// ordering: when source and destination are the same file, the
// diagnostic is written and the file skipped BEFORE step 3's -i
// prompt, so no prompt appears and the run fails even though the
// response stream is empty (an EOF that would otherwise decline).
func TestCpInteractiveSameFileDiagnosesWithoutPrompt(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "x")
	out, errb, code := runToolWithInput(t, dir, "", "-i", "a", "a")
	if code != 1 || !strings.Contains(errb, "'a' and 'a' are the same file") {
		t.Fatalf("cp -i a a: code=%d err=%q, want same-file diagnostic and status 1", code, errb)
	}
	if strings.Contains(errb, "overwrite") {
		t.Errorf("-i prompt issued before the step-1 same-file check: %q", errb)
	}
	if out != "" {
		t.Errorf("stdout not empty: %q", out)
	}
}

// The GNU -u extension must not bypass POSIX step 1. Even when -u would
// otherwise skip an equal-or-newer destination, identical source and target
// are diagnosed before update and interactive handling.
func TestCpUpdateDoesNotBypassSameFileDiagnostic(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "x")
	_, errb, code := runToolWithInput(t, dir, "n\n", "-iu", "a", "a")
	if code != 1 || !strings.Contains(errb, "'a' and 'a' are the same file") {
		t.Fatalf("cp -iu a a: code=%d err=%q", code, errb)
	}
	if strings.Contains(errb, "overwrite") {
		t.Fatalf("cp -iu prompted before same-file diagnostic: %q", errb)
	}
}

// TestCpInteractiveDirDestDiagnosesWithoutPrompt: overwriting a
// directory with a non-directory is diagnosed without consuming an -i
// response (the open of step 3 can never succeed, and GNU diagnoses
// the same way).
func TestCpInteractiveDirDestDiagnosesWithoutPrompt(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "x")
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runToolWithInput(t, dir, "", "-iT", "a", "d")
	if code != 1 || !strings.Contains(errb, "cannot overwrite directory 'd' with non-directory") {
		t.Fatalf("cp -iT a d: code=%d err=%q", code, errb)
	}
	if strings.Contains(errb, "overwrite 'd'?") {
		t.Errorf("-i prompt issued for an un-overwritable directory: %q", errb)
	}
}

// TestCpRecursiveDirOverExistingNonDirContinues pins Issue 7 step 2:
// with -R, a source directory whose destination exists and is not a
// directory gets a diagnostic and its hierarchy is skipped, while the
// remaining source operands are still processed (exit status >0).
func TestCpRecursiveDirOverExistingNonDirContinues(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "tree", "f"), "payload")
	write(t, filepath.Join(dir, "ok"), "second")
	if err := os.Mkdir(filepath.Join(dir, "target"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "target", "tree"), "keep") // non-directory in the way
	_, errb, code := runTool(t, dir, "-R", "tree", "ok", "target")
	if code != 1 {
		t.Fatalf("code=%d err=%q, want 1", code, errb)
	}
	if !strings.Contains(errb, "cannot overwrite non-directory") {
		t.Errorf("missing step-2 diagnostic: %q", errb)
	}
	if got := read(t, filepath.Join(dir, "target", "tree")); got != "keep" {
		t.Errorf("existing non-directory destination modified: %q", got)
	}
	if got := read(t, filepath.Join(dir, "target", "ok")); got != "second" {
		t.Errorf("remaining source not processed after skipped hierarchy: %q", got)
	}
}

// TestCpDashOperandsAreOrdinaryPathnames: per Issue 7 OPERANDS, a
// source_file or target_file of "-" refers to a file named -, never to
// standard input or standard output.
func TestCpDashOperandsAreOrdinaryPathnames(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "-"), "dash-file")
	_, errb, code := runToolWithInput(t, dir, "not-the-source\n", "-", "out")
	if code != 0 || errb != "" {
		t.Fatalf("cp - out: code=%d err=%q", code, errb)
	}
	if got := read(t, filepath.Join(dir, "out")); got != "dash-file" {
		t.Errorf("source '-' read stdin instead of the file named -: %q", got)
	}
	write(t, filepath.Join(dir, "src2"), "second-payload")
	out, errb, code := runTool(t, dir, "src2", "-")
	if code != 0 || errb != "" || out != "" {
		t.Fatalf("cp src2 -: code=%d out=%q err=%q", code, out, errb)
	}
	if got := read(t, filepath.Join(dir, "-")); got != "second-payload" {
		t.Errorf("target '-' did not name the file -: %q", got)
	}
}

// POSIX obtains the destination descriptor with open(O_CREAT); it does not
// create missing path components. Parent synthesis belongs only to GNU
// --parents and must not turn this failure into a successful copy.
func TestCpDoesNotCreateMissingDestinationParents(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "source"), "payload")
	_, errb, code := runTool(t, dir, "source", filepath.Join("missing", "target"))
	if code != 1 || !strings.Contains(errb, "cannot create regular file") {
		t.Fatalf("cp source missing/target: code=%d err=%q, want destination-open failure", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "missing")); !os.IsNotExist(err) {
		t.Fatalf("cp invented a missing destination hierarchy: %v", err)
	}
}

// A source that becomes unreadable after its initial stat must not cause cp to
// create or truncate its destination. POSIX requires processing to continue
// with later source operands, but the failed source is not a successful copy.
func TestCpUnreadableSourceLeavesDestinationAbsentAndContinues(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "blocked"), "secret")
	write(t, filepath.Join(dir, "readable"), "payload")
	if err := os.Mkdir(filepath.Join(dir, "out"), 0o755); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(dir, "blocked")
	if err := os.Chmod(blocked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o600) })
	if f, err := os.Open(blocked); err == nil {
		_ = f.Close()
		t.Skip("current user can read a mode-000 file")
	}

	_, errb, code := runTool(t, dir, "blocked", "readable", "out")
	if code != 1 || !strings.Contains(errb, "cannot open 'blocked' for reading") {
		t.Fatalf("cp blocked readable out: code=%d err=%q", code, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "out", "blocked")); !os.IsNotExist(err) {
		t.Fatalf("destination for unreadable source exists: %v", err)
	}
	if got := read(t, filepath.Join(dir, "out", "readable")); got != "payload" {
		t.Fatalf("later source was not copied: %q", got)
	}
}

func TestCpPreserveFailsLoudlyWhenAccessTimeIsUnavailable(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "source"), "payload")
	old := fileAtime
	fileAtime = func(os.FileInfo) (time.Time, bool) { return time.Time{}, false }
	t.Cleanup(func() { fileAtime = old })

	_, errb, code := runTool(t, dir, "-p", "source", "target")
	if code != 1 || !strings.Contains(errb, "access time unsupported") {
		t.Fatalf("cp -p without atime support: code=%d err=%q, want explicit preservation failure", code, errb)
	}
	if got := read(t, filepath.Join(dir, "target")); got != "payload" {
		t.Fatalf("file data was not copied before metadata failure: %q", got)
	}
}

// POSIX step 1 classifies identical files before step 3's overwrite controls.
// GNU -n is an extension, but it cannot turn `cp -n a a` into a silent
// successful skip: the same-file diagnostic still wins.
func TestCpNoClobberDoesNotHideSameFile(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "payload")
	_, errb, code := runTool(t, dir, "-n", "a", "a")
	if code != 1 || !strings.Contains(errb, "'a' and 'a' are the same file") {
		t.Fatalf("cp -n a a: code=%d err=%q, want same-file diagnostic", code, errb)
	}
}

// With -P, source_file is the symlink itself. Copying it to the same pathname
// must stop at step 1 without unlinking and recreating it (which changes the
// inode and can lose the link entirely if recreation fails).
func TestCpPhysicalSymlinkSameFileIsNotReplaced(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation normally requires elevated privilege on Windows")
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, "target"), "payload")
	link := filepath.Join(dir, "link")
	if err := os.Symlink("target", link); err != nil {
		t.Fatal(err)
	}
	before, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}

	_, errb, code := runTool(t, dir, "-P", "link", "link")
	if code != 1 || !strings.Contains(errb, "'link' and 'link' are the same file") {
		t.Fatalf("cp -P link link: code=%d err=%q, want same-file diagnostic", code, errb)
	}
	after, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("source symlink disappeared: %v", err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("same-file copy replaced the source symlink instead of leaving it unchanged")
	}
}

func TestCpPhysicalSymlinkDoesNotReplaceDestinationDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation normally requires elevated privilege on Windows")
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, "target"), "payload")
	if err := os.Symlink("target", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "out", "link"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, errb, code := runTool(t, dir, "-P", "link", "out")
	if code != 1 || !strings.Contains(errb, "cannot overwrite directory 'out/link' with non-directory") {
		t.Fatalf("cp -P link out: code=%d err=%q, want destination-directory diagnostic", code, errb)
	}
	if fi, err := os.Lstat(filepath.Join(dir, "out", "link")); err != nil || !fi.IsDir() {
		t.Fatalf("destination directory was replaced: mode=%v err=%v", fiMode(fi), err)
	}
}

// A destination pathname can be inside source without having a lexical source
// prefix. Stat each existing destination ancestor so a directory symlink back
// to source is rejected before the destination is created and enters traversal.
func TestCpRecursiveRejectsSymlinkAliasedDestinationInsideSource(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory symlink creation normally requires elevated privilege on Windows")
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src", "file"), "payload")
	if err := os.Symlink("src", filepath.Join(dir, "alias")); err != nil {
		t.Fatal(err)
	}

	_, errb, code := runTool(t, dir, "-R", "src", "alias")
	if code != 1 || !strings.Contains(errb, "into itself") {
		t.Fatalf("cp -R src alias: code=%d err=%q, want into-itself diagnostic", code, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "src", "src")); !os.IsNotExist(err) {
		t.Fatalf("destination was created inside source before rejection: %v", err)
	}
}

func TestCpRecursiveRejectsDestinationAliasedToSourceSubdirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory symlink creation normally requires elevated privilege on Windows")
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src", "sub", "file"), "payload")
	if err := os.Symlink(filepath.Join("src", "sub"), filepath.Join(dir, "alias")); err != nil {
		t.Fatal(err)
	}

	_, errb, code := runTool(t, dir, "-R", "src", filepath.Join("alias", "new"))
	if code != 1 || !strings.Contains(errb, "into itself") {
		t.Fatalf("cp -R src alias/new: code=%d err=%q, want into-itself diagnostic", code, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "src", "sub", "new")); !os.IsNotExist(err) {
		t.Fatalf("destination was created inside a source subdirectory: %v", err)
	}
}
