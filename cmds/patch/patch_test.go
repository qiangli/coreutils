package patchcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runIn(t *testing.T, dir, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
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
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("ReadFile %s: %v", name, err)
	}
	return string(b)
}

func exists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

func TestApplyUnifiedInPlace(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "apple\nbanana\ncherry\n")
	diff := "--- f.txt\n+++ f.txt\n@@ -1,3 +1,3 @@\n apple\n-banana\n+berry\n cherry\n"
	_, stderr, code := runIn(t, dir, diff, "f.txt")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if got := readFile(t, dir, "f.txt"); got != "apple\nberry\ncherry\n" {
		t.Fatalf("got %q", got)
	}
}

func TestApplyFromPatchfileOperand(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "one\ntwo\n")
	writeFile(t, dir, "f.patch", "--- f.txt\n+++ f.txt\n@@ -1,2 +1,2 @@\n one\n-two\n+TWO\n")
	_, stderr, code := runIn(t, dir, "", "f.txt", "f.patch")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if got := readFile(t, dir, "f.txt"); got != "one\nTWO\n" {
		t.Fatalf("got %q", got)
	}
}

func TestApplyReverseFlag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "apple\nberry\ncherry\n")
	diff := "--- f.txt\n+++ f.txt\n@@ -1,3 +1,3 @@\n apple\n-banana\n+berry\n cherry\n"
	_, stderr, code := runIn(t, dir, diff, "-R", "f.txt")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if got := readFile(t, dir, "f.txt"); got != "apple\nbanana\ncherry\n" {
		t.Fatalf("got %q", got)
	}
}

func TestApplyStripComponents(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "one\ntwo\n")
	diff := "--- a/f.txt\n+++ b/f.txt\n@@ -1,2 +1,2 @@\n one\n-two\n+TWO\n"
	_, stderr, code := runIn(t, dir, diff, "-p1")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if got := readFile(t, dir, "f.txt"); got != "one\nTWO\n" {
		t.Fatalf("got %q", got)
	}
}

func TestApplyDefaultUsesHeaderBasename(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "one\ntwo\n")
	// POSIX selects the basename when -p is omitted.
	diff := "--- a/f.txt\n+++ b/f.txt\n@@ -1,2 +1,2 @@\n one\n-two\n+TWO\n"
	_, stderr, code := runIn(t, dir, diff)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if got := readFile(t, dir, "f.txt"); got != "one\nTWO\n" {
		t.Fatalf("got %q", got)
	}
}

func TestApplyCreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	diff := "--- /dev/null\n+++ new.txt\n@@ -0,0 +1,2 @@\n+hello\n+world\n"
	_, stderr, code := runIn(t, dir, diff)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if got := readFile(t, dir, "new.txt"); got != "hello\nworld\n" {
		t.Fatalf("got %q", got)
	}
}

func TestApplyDeletesFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "gone.txt", "bye\n")
	diff := "--- gone.txt\n+++ /dev/null\n@@ -1 +0,0 @@\n-bye\n"
	_, stderr, code := runIn(t, dir, diff)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if exists(dir, "gone.txt") {
		t.Fatalf("expected gone.txt to be removed")
	}
}

func TestFailedDeletionPreservesTarget(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "victim.txt", "actual\n")
	diff := "--- victim.txt\n+++ /dev/null\n@@ -1 +0,0 @@\n-expected\n"
	_, _, code := runIn(t, dir, diff)
	if code != 1 {
		t.Fatalf("exit=%d, want 1", code)
	}
	if got := readFile(t, dir, "victim.txt"); got != "actual\n" {
		t.Fatalf("failed deletion changed target: %q", got)
	}
}

func TestDeletionWithOutputLeavesOriginalAndWritesEmptyOutput(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "victim.txt", "bye\n")
	diff := "--- victim.txt\n+++ /dev/null\n@@ -1 +0,0 @@\n-bye\n"
	_, stderr, code := runIn(t, dir, diff, "-o", "out.txt")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if got := readFile(t, dir, "victim.txt"); got != "bye\n" {
		t.Fatalf("-o changed original: %q", got)
	}
	if got := readFile(t, dir, "out.txt"); got != "" {
		t.Fatalf("output=%q, want empty", got)
	}
}

func TestApplyConflictWritesRejectAndExitsOne(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "one\ncompletely-different\n")
	diff := "--- f.txt\n+++ f.txt\n@@ -1,2 +1,2 @@\n one\n-two\n+TWO\n"
	_, stderr, code := runIn(t, dir, diff, "f.txt")
	if code != 1 {
		t.Fatalf("exit=%d, want 1; stderr=%s", code, stderr)
	}
	if !exists(dir, "f.txt.rej") {
		t.Fatalf("expected f.txt.rej to be written")
	}
	if got := readFile(t, dir, "f.txt"); got != "one\ncompletely-different\n" {
		t.Fatalf("file should be left untouched on reject, got %q", got)
	}
	rej := readFile(t, dir, "f.txt.rej")
	if !strings.Contains(rej, "-two") || !strings.Contains(rej, "+TWO") {
		t.Fatalf("reject content missing hunk body: %s", rej)
	}
}

func TestAlreadyAppliedRequiresForwardFlag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "one\nTWO\n")
	diff := "--- f.txt\n+++ f.txt\n@@ -1,2 +1,2 @@\n one\n-two\n+TWO\n"
	_, _, code := runIn(t, dir, diff, "f.txt")
	if code != 1 {
		t.Fatalf("default exit=%d, want 1", code)
	}
	_, stderr, code := runIn(t, dir, diff, "-N", "f.txt")
	if code != 0 {
		t.Fatalf("-N exit=%d stderr=%s", code, stderr)
	}
	if got := readFile(t, dir, "f.txt"); got != "one\nTWO\n" {
		t.Fatalf("already-applied hunk must not change content, got %q", got)
	}
}

func TestApplyOutputFlag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "one\ntwo\n")
	diff := "--- f.txt\n+++ f.txt\n@@ -1,2 +1,2 @@\n one\n-two\n+TWO\n"
	_, stderr, code := runIn(t, dir, diff, "-o", "out.txt", "f.txt")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if got := readFile(t, dir, "f.txt"); got != "one\ntwo\n" {
		t.Fatalf("original should be untouched with -o, got %q", got)
	}
	if got := readFile(t, dir, "out.txt"); got != "one\nTWO\n" {
		t.Fatalf("got %q", got)
	}
}

func TestHeaderSelectionTriesOldNameBeforeNew(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "old.txt", "before\n")
	diff := "--- old.txt\n+++ new.txt\n@@ -1 +1 @@\n-before\n+after\n"
	_, stderr, code := runIn(t, dir, diff)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if got := readFile(t, dir, "old.txt"); got != "after\n" {
		t.Fatalf("old header target=%q", got)
	}
}

func TestFormatHintRejectsDifferentInputFormat(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "one\n")
	normal := "1c1\n< one\n---\n> two\n"
	_, stderr, code := runIn(t, dir, normal, "-u", "f.txt")
	if code != 2 || !strings.Contains(stderr, "not requested unified") {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
}

func TestDirectoryAppliesToInputOption(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "sub/f.txt", "one\n")
	writeFile(t, dir, "sub/change.patch", "--- f.txt\n+++ f.txt\n@@ -1 +1 @@\n-one\n+two\n")
	_, stderr, code := runIn(t, dir, "", "-d", "sub", "-i", "change.patch")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if got := readFile(t, dir, "sub/f.txt"); got != "two\n" {
		t.Fatalf("target=%q", got)
	}
}

func TestProgressUsesStandardError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "one\n")
	diff := "--- f.txt\n+++ f.txt\n@@ -1 +1 @@\n-one\n+two\n"
	stdout, stderr, code := runIn(t, dir, diff)
	if code != 0 || stdout != "" || !strings.Contains(stderr, "patching file") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestApplyDryRunChangesNothing(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "one\ntwo\n")
	diff := "--- f.txt\n+++ f.txt\n@@ -1,2 +1,2 @@\n one\n-two\n+TWO\n"
	_, stderr, code := runIn(t, dir, diff, "--dry-run", "f.txt")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if got := readFile(t, dir, "f.txt"); got != "one\ntwo\n" {
		t.Fatalf("dry-run must not modify the file, got %q", got)
	}
}

func TestApplyBackupFlag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "one\ntwo\n")
	diff := "--- f.txt\n+++ f.txt\n@@ -1,2 +1,2 @@\n one\n-two\n+TWO\n"
	_, stderr, code := runIn(t, dir, diff, "-b", "f.txt")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if got := readFile(t, dir, "f.txt.orig"); got != "one\ntwo\n" {
		t.Fatalf("backup content = %q", got)
	}
	if got := readFile(t, dir, "f.txt"); got != "one\nTWO\n" {
		t.Fatalf("got %q", got)
	}
}

func TestApplyRemoveEmptyFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "only\n")
	diff := "--- f.txt\n+++ f.txt\n@@ -1 +0,0 @@\n-only\n"
	_, stderr, code := runIn(t, dir, diff, "-E", "f.txt")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if exists(dir, "f.txt") {
		t.Fatalf("expected f.txt removed by -E")
	}
}

func TestApplyMultiFilePatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "x\n")
	writeFile(t, dir, "b.txt", "y\n")
	diff := "--- a.txt\n+++ a.txt\n@@ -1 +1 @@\n-x\n+X\n" +
		"--- b.txt\n+++ b.txt\n@@ -1 +1 @@\n-y\n+Y\n"
	_, stderr, code := runIn(t, dir, diff)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if got := readFile(t, dir, "a.txt"); got != "X\n" {
		t.Fatalf("a.txt = %q", got)
	}
	if got := readFile(t, dir, "b.txt"); got != "Y\n" {
		t.Fatalf("b.txt = %q", got)
	}
}

func TestApplyContextFormat(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "line1\nline2\nline3\n")
	diff := "*** f.txt\n--- f.txt\n***************\n*** 1,3 ****\n  line1\n! line2\n  line3\n--- 1,3 ----\n  line1\n! LINE2\n  line3\n"
	_, stderr, code := runIn(t, dir, diff, "f.txt")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if got := readFile(t, dir, "f.txt"); got != "line1\nLINE2\nline3\n" {
		t.Fatalf("got %q", got)
	}
}

func TestApplyNormalFormatRequiresFileOperand(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "apple\nbanana\ncherry\n")
	diff := "2c2\n< banana\n---\n> berry\n"
	_, stderr, code := runIn(t, dir, diff)
	if code == 0 {
		t.Fatalf("expected failure without a FILE operand, stderr=%s", stderr)
	}

	_, stderr, code = runIn(t, dir, diff, "f.txt")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if got := readFile(t, dir, "f.txt"); got != "apple\nberry\ncherry\n" {
		t.Fatalf("got %q", got)
	}
}

func TestEdFlagIsNotSupported(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := runIn(t, dir, "", "--ed")
	if code != 2 || !strings.Contains(stderr, "not supported") {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
}

func TestExtraOperandIsUsageError(t *testing.T) {
	dir := t.TempDir()
	_, _, code := runIn(t, dir, "", "a", "b", "c")
	if code != 2 {
		t.Fatalf("exit=%d, want 2", code)
	}
}

func TestHelp(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := runIn(t, dir, "", "--help")
	if code != 0 || !strings.Contains(stdout, "Usage: patch") {
		t.Fatalf("code=%d stdout=%s", code, stdout)
	}
}

func TestGitBinaryPatchSectionIsReportedAndFails(t *testing.T) {
	dir := t.TempDir()
	diff := "diff --git a/img.png b/img.png\nindex 111..222 100644\nGIT binary patch\nliteral 4\nXcmZ?wb\n"
	_, stderr, code := runIn(t, dir, diff)
	if code != 1 || !strings.Contains(stderr, "binary") {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
}
