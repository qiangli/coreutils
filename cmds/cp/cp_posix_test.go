package cpcmd

// POSIX.1-2016 (Issue 7) interface-evidence tests: description step
// ordering, operand rules, and per-source continuation that the
// original suite did not pin.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
