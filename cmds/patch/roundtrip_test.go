package patchcmd

// Round-trips this repo's own diff(1) output through patch(1): diff -u/-c
// generates the exact bytes real users pipe into patch, so parsing them
// correctly is the highest-value conformance check pkg/patch has.

import (
	"bytes"
	"context"
	"strings"
	"testing"

	_ "github.com/qiangli/coreutils/cmds/diff"
	"github.com/qiangli/coreutils/tool"
)

func generateDiff(t *testing.T, dir string, args ...string) (string, int) {
	t.Helper()
	dt := tool.Lookup("diff")
	if dt == nil {
		t.Fatal("diff tool not registered")
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{Out: &out, Err: &errb},
	}
	code := dt.Run(rc, args)
	if code == 2 {
		t.Fatalf("diff trouble: %s", errb.String())
	}
	return out.String(), code
}

func TestRoundTripUnifiedDiff(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "old.txt", "one\ntwo\nthree\nfour\nfive\n")
	writeFile(t, dir, "new.txt", "one\nTWO\nthree\nfour\nfive-changed\n")

	diffOut, code := generateDiff(t, dir, "-u", "old.txt", "new.txt")
	if code != 1 {
		t.Fatalf("expected diff to report a difference (exit 1), got %d: %s", code, diffOut)
	}

	target := t.TempDir()
	writeFile(t, target, "old.txt", "one\ntwo\nthree\nfour\nfive\n")
	_, stderr, pcode := runIn(t, target, diffOut, "old.txt")
	if pcode != 0 {
		t.Fatalf("patch failed: exit=%d stderr=%s\ndiff was:\n%s", pcode, stderr, diffOut)
	}
	if got := readFile(t, target, "old.txt"); got != "one\nTWO\nthree\nfour\nfive-changed\n" {
		t.Fatalf("got %q", got)
	}
}

func TestRoundTripContextDiff(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "old.txt", "alpha\nbeta\ngamma\n")
	writeFile(t, dir, "new.txt", "alpha\nBETA\ngamma\ndelta\n")

	diffOut, code := generateDiff(t, dir, "-c", "old.txt", "new.txt")
	if code != 1 {
		t.Fatalf("expected diff to report a difference (exit 1), got %d: %s", code, diffOut)
	}

	target := t.TempDir()
	writeFile(t, target, "old.txt", "alpha\nbeta\ngamma\n")
	_, stderr, pcode := runIn(t, target, diffOut, "old.txt")
	if pcode != 0 {
		t.Fatalf("patch failed: exit=%d stderr=%s\ndiff was:\n%s", pcode, stderr, diffOut)
	}
	if got := readFile(t, target, "old.txt"); got != "alpha\nBETA\ngamma\ndelta\n" {
		t.Fatalf("got %q", got)
	}
}

func TestRoundTripNormalDiff(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "old.txt", "1\n2\n3\n")
	writeFile(t, dir, "new.txt", "1\nTWO\n3\n4\n")

	diffOut, code := generateDiff(t, dir, "old.txt", "new.txt")
	if code != 1 {
		t.Fatalf("expected diff to report a difference (exit 1), got %d: %s", code, diffOut)
	}

	target := t.TempDir()
	writeFile(t, target, "old.txt", "1\n2\n3\n")
	_, stderr, pcode := runIn(t, target, diffOut, "old.txt")
	if pcode != 0 {
		t.Fatalf("patch failed: exit=%d stderr=%s\ndiff was:\n%s", pcode, stderr, diffOut)
	}
	if got := readFile(t, target, "old.txt"); got != "1\nTWO\n3\n4\n" {
		t.Fatalf("got %q", got)
	}
}

func TestRoundTripUnifiedNoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "old.txt", "one\ntwo")
	writeFile(t, dir, "new.txt", "one\nTWO")

	diffOut, code := generateDiff(t, dir, "-u", "old.txt", "new.txt")
	if code != 1 {
		t.Fatalf("expected a difference, got %d", code)
	}
	if !strings.Contains(diffOut, "No newline at end of file") {
		t.Fatalf("expected a no-newline marker in diff output:\n%s", diffOut)
	}

	target := t.TempDir()
	writeFile(t, target, "old.txt", "one\ntwo")
	_, stderr, pcode := runIn(t, target, diffOut, "old.txt")
	if pcode != 0 {
		t.Fatalf("patch failed: exit=%d stderr=%s", pcode, stderr)
	}
	if got := readFile(t, target, "old.txt"); got != "one\nTWO" {
		t.Fatalf("got %q, want no trailing newline preserved", got)
	}
}
