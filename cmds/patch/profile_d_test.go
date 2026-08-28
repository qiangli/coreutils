package patchcmd

import (
	"strings"
	"testing"
)

// A copied-context, unified, or normal listing announces its own format. A
// line of context that happens to read like an address-less ed command must
// not turn the whole patch into an ed script, which previously left the file
// unpatched and exited 2 after prompting for a filename.
func TestContextLineLikeEdCommandStaysAUnifiedDiff(t *testing.T) {
	for _, line := range []string{"a", "c", "d", "i"} {
		dir := t.TempDir()
		writeFile(t, dir, "f.txt", line+"\nbeta\ngamma\n")
		diff := "--- f.txt\n+++ f.txt\n@@ -1,3 +1,3 @@\n " + line + "\n-beta\n+BETA\n gamma\n"
		stdout, stderr, code := runIn(t, dir, diff)
		if code != 0 {
			t.Fatalf("%q context: code=%d stdout=%q stderr=%q", line, code, stdout, stderr)
		}
		if got, want := readFile(t, dir, "f.txt"), line+"\nBETA\ngamma\n"; got != want {
			t.Fatalf("%q context: result=%q want=%q", line, got, want)
		}
	}
}

// The same guard for a normal difference, whose "<"/">" data lines sit under
// an ordinary "2c2" command.
func TestNormalDiffWithEdLikeDataStaysANormalDiff(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "a\nbeta\n")
	_, stderr, code := runIn(t, dir, "Index: f.txt\n2c2\n< beta\n---\n> BETA\n")
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if got, want := readFile(t, dir, "f.txt"), "a\nBETA\n"; got != want {
		t.Fatalf("result=%q want=%q", got, want)
	}
}

// A real diff -e script still auto-detects, including behind header material.
func TestAddressedEdScriptStillAutoDetects(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "one\ntwo\n")
	_, stderr, code := runIn(t, dir, "Index: f.txt\n2c\nTWO\n.\n")
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if got, want := readFile(t, dir, "f.txt"), "one\nTWO\n"; got != want {
		t.Fatalf("result=%q want=%q", got, want)
	}
}

// POSIX: "If the pathname in the patch file is absolute, any leading <slash>
// characters shall be considered the first component (that is, -p 1 shall
// remove the leading <slash> characters)."
func TestStripTreatsLeadingSlashRunAsOneComponent(t *testing.T) {
	for _, prefix := range []string{"/", "//", "///"} {
		dir := t.TempDir()
		writeFile(t, dir, "curds/whey/f.txt", "one\ntwo\n")
		diff := "--- " + prefix + "curds/whey/f.txt\n+++ " + prefix + "curds/whey/f.txt\n@@ -1,2 +1,2 @@\n one\n-two\n+TWO\n"
		_, stderr, code := runIn(t, dir, diff, "-p1")
		if code != 0 {
			t.Fatalf("prefix %q: code=%d stderr=%q", prefix, code, stderr)
		}
		if got, want := readFile(t, dir, "curds/whey/f.txt"), "one\nTWO\n"; got != want {
			t.Fatalf("prefix %q: result=%q want=%q", prefix, got, want)
		}
	}
}

// An interior run of <slash> characters separates two components rather than
// standing for empty components of its own, so neither -p nor the default
// basename selection is thrown off by it.
func TestStripSkipsAdjacentInteriorSlashes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "whey/f.txt", "one\ntwo\n")
	diff := "--- curds//whey/f.txt\n+++ curds//whey/f.txt\n@@ -1,2 +1,2 @@\n one\n-two\n+TWO\n"
	if _, stderr, code := runIn(t, dir, diff, "-p1"); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if got, want := readFile(t, dir, "whey/f.txt"), "one\nTWO\n"; got != want {
		t.Fatalf("-p1 result=%q want=%q", got, want)
	}

	dir = t.TempDir()
	writeFile(t, dir, "f.txt", "one\ntwo\n")
	if _, stderr, code := runIn(t, dir, diff); code != 0 {
		t.Fatalf("default: code=%d stderr=%q", code, stderr)
	}
	if got, want := readFile(t, dir, "f.txt"), "one\nTWO\n"; got != want {
		t.Fatalf("default result=%q want=%q", got, want)
	}
}

// The informational message has to name the file patch actually operated on
// -- the stripped name, not the raw header text -- and carry no stray
// trailing <blank>.
func TestProgressNamesTheStrippedTargetExactly(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "one\ntwo\n")
	diff := "--- a/deep/f.txt\n+++ b/deep/f.txt\n@@ -1,2 +1,2 @@\n one\n-two\n+TWO\n"
	stdout, stderr, code := runIn(t, dir, diff)
	if code != 0 || stdout != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if stderr != "patching file f.txt\n" {
		t.Fatalf("stderr=%q", stderr)
	}
	fresh := t.TempDir()
	writeFile(t, fresh, "f.txt", "one\ntwo\n")
	if _, stderr, code = runIn(t, fresh, diff, "--dry-run"); code != 0 || stderr != "patching file f.txt (dry run)\n" {
		t.Fatalf("dry run: code=%d stderr=%q", code, stderr)
	}
	if strings.Contains(stderr, "  ") {
		t.Fatalf("dry-run message has doubled blanks: %q", stderr)
	}
}

func TestStripComponentsPOSIXExamples(t *testing.T) {
	const name = "/curds/whey/src/blurfl/blurfl.c"
	cases := []struct {
		strip int
		want  string
	}{
		{autoStripSentinel, "blurfl.c"},
		{0, name},
		{1, "curds/whey/src/blurfl/blurfl.c"},
		{4, "blurfl/blurfl.c"},
		{6, ""},
	}
	for _, c := range cases {
		if got := targetName(name, c.strip); got != c.want {
			t.Fatalf("targetName(-p %d)=%q want %q", c.strip, got, c.want)
		}
	}
}
