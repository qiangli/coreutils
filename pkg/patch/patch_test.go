package patch

import (
	"reflect"
	"strings"
	"testing"
)

func lines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

func TestParseUnifiedBasic(t *testing.T) {
	data := "--- old.txt\n+++ new.txt\n@@ -1,3 +1,3 @@\n line1\n-line2\n+LINE2\n line3\n"
	p, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(p.Files) != 1 {
		t.Fatalf("got %d files, want 1", len(p.Files))
	}
	fp := p.Files[0]
	if fp.OldName != "old.txt" || fp.NewName != "new.txt" || fp.Format != FormatUnified {
		t.Fatalf("unexpected file patch: %+v", fp)
	}
	if len(fp.Hunks) != 1 {
		t.Fatalf("got %d hunks, want 1", len(fp.Hunks))
	}
	h := fp.Hunks[0]
	if h.OldStart != 0 || h.OldCount != 3 || h.NewStart != 0 || h.NewCount != 3 {
		t.Fatalf("unexpected hunk range: %+v", h)
	}
	want := []HunkLine{
		{Kind: LineContext, Text: "line1"},
		{Kind: LineDelete, Text: "line2"},
		{Kind: LineAdd, Text: "LINE2"},
		{Kind: LineContext, Text: "line3"},
	}
	if !reflect.DeepEqual(h.Lines, want) {
		t.Fatalf("hunk lines = %+v, want %+v", h.Lines, want)
	}
}

func TestParseUnifiedNoNewlineMarker(t *testing.T) {
	data := "--- old.txt\n+++ new.txt\n@@ -1,2 +1,2 @@\n a\n-b\n\\ No newline at end of file\n+B\n\\ No newline at end of file\n"
	p, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	h := p.Files[0].Hunks[0]
	if !h.Lines[1].NoEOL || h.Lines[1].Kind != LineDelete {
		t.Fatalf("delete line NoEOL not set: %+v", h.Lines[1])
	}
	if !h.Lines[2].NoEOL || h.Lines[2].Kind != LineAdd {
		t.Fatalf("add line NoEOL not set: %+v", h.Lines[2])
	}
}

func TestParseUnifiedCreateAndDelete(t *testing.T) {
	create := "--- /dev/null\n+++ new.txt\n@@ -0,0 +1,2 @@\n+a\n+b\n"
	p, err := Parse([]byte(create))
	if err != nil {
		t.Fatalf("Parse create: %v", err)
	}
	if !p.Files[0].IsCreate() {
		t.Fatalf("expected IsCreate, got %+v", p.Files[0])
	}

	del := "--- old.txt\n+++ /dev/null\n@@ -1,2 +0,0 @@\n-a\n-b\n"
	p2, err := Parse([]byte(del))
	if err != nil {
		t.Fatalf("Parse delete: %v", err)
	}
	if !p2.Files[0].IsDelete() {
		t.Fatalf("expected IsDelete, got %+v", p2.Files[0])
	}
}

func TestParseMultiFileUnified(t *testing.T) {
	data := "--- a.txt\n+++ a.txt\n@@ -1 +1 @@\n-x\n+X\n" +
		"--- b.txt\n+++ b.txt\n@@ -1 +1 @@\n-y\n+Y\n"
	p, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(p.Files) != 2 {
		t.Fatalf("got %d files, want 2", len(p.Files))
	}
	if p.Files[0].OldName != "a.txt" || p.Files[1].OldName != "b.txt" {
		t.Fatalf("unexpected file order: %+v", p.Files)
	}
}

func TestParseContextChange(t *testing.T) {
	data := "*** old.txt\n--- new.txt\n***************\n*** 1,3 ****\n  line1\n! line2\n  line3\n--- 1,3 ----\n  line1\n! LINE2\n  line3\n"
	p, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	fp := p.Files[0]
	if fp.Format != FormatContext {
		t.Fatalf("got format %v, want context", fp.Format)
	}
	h := fp.Hunks[0]
	if h.OldStart != 0 || h.OldCount != 3 || h.NewStart != 0 || h.NewCount != 3 {
		t.Fatalf("unexpected hunk range: %+v", h)
	}
	want := []HunkLine{
		{Kind: LineContext, Text: "line1"},
		{Kind: LineDelete, Text: "line2"},
		{Kind: LineAdd, Text: "LINE2"},
		{Kind: LineContext, Text: "line3"},
	}
	if !reflect.DeepEqual(h.Lines, want) {
		t.Fatalf("hunk lines = %+v, want %+v", h.Lines, want)
	}
}

func TestParseContextPureInsertion(t *testing.T) {
	data := "*** old.txt\n--- new.txt\n***************\n*** 1,2 ****\n--- 1,3 ----\n  a\n+ b\n  c\n"
	p, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	h := p.Files[0].Hunks[0]
	// OldCount is 2, not 0: the header's "1,2" span covers both old-file
	// lines even though neither is printed under it (both are pure
	// context, reconstructed from the new-side block below).
	if h.OldStart != 0 || h.OldCount != 2 || h.NewStart != 0 || h.NewCount != 3 {
		t.Fatalf("unexpected hunk range: %+v", h)
	}
	want := []HunkLine{
		{Kind: LineContext, Text: "a"},
		{Kind: LineAdd, Text: "b"},
		{Kind: LineContext, Text: "c"},
	}
	if !reflect.DeepEqual(h.Lines, want) {
		t.Fatalf("hunk lines = %+v, want %+v", h.Lines, want)
	}
	res := Apply(lines("a\nc\n"), false, p.Files[0].Hunks, ApplyOptions{})
	if got := strings.Join(res.Lines, "\n"); got != "a\nb\nc" {
		t.Fatalf("apply = %q, want %q", got, "a\nb\nc")
	}
}

func TestParseContextPureDeletion(t *testing.T) {
	data := "*** old.txt\n--- new.txt\n***************\n*** 1,3 ****\n  a\n- b\n  c\n--- 1,2 ----\n"
	p, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	h := p.Files[0].Hunks[0]
	want := []HunkLine{
		{Kind: LineContext, Text: "a"},
		{Kind: LineDelete, Text: "b"},
		{Kind: LineContext, Text: "c"},
	}
	if !reflect.DeepEqual(h.Lines, want) {
		t.Fatalf("hunk lines = %+v, want %+v", h.Lines, want)
	}
	res := Apply(lines("a\nb\nc\n"), false, p.Files[0].Hunks, ApplyOptions{})
	if got := strings.Join(res.Lines, "\n"); got != "a\nc" {
		t.Fatalf("apply = %q, want %q", got, "a\nc")
	}
}

func TestParseNormalHunks(t *testing.T) {
	cases := []struct {
		name, data string
		old, want  string
	}{
		{"change", "2c2\n< banana\n---\n> berry\n", "apple\nbanana\ncherry\n", "apple\nberry\ncherry"},
		{"append", "2a3\n> 3\n", "1\n2\n", "1\n2\n3"},
		{"delete", "2d1\n< 2\n", "1\n2\n3\n", "1\n3"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := Parse([]byte(c.data))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if p.Files[0].Format != FormatNormal {
				t.Fatalf("got format %v, want normal", p.Files[0].Format)
			}
			res := Apply(lines(c.old), false, p.Files[0].Hunks, ApplyOptions{})
			if got := strings.Join(res.Lines, "\n"); got != c.want {
				t.Fatalf("apply = %q, want %q", got, c.want)
			}
		})
	}
}

func TestApplyOffsetDrift(t *testing.T) {
	// Two independent hunks; applying the first shifts line numbers for
	// the second, which Apply must account for via its running offset.
	data := "--- f\n+++ f\n@@ -1,2 +1,3 @@\n+inserted\n 1\n 2\n@@ -5,2 +6,2 @@\n 5\n-6\n+SIX\n"
	p, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	old := lines("1\n2\n3\n4\n5\n6\n")
	res := Apply(old, false, p.Files[0].Hunks, ApplyOptions{})
	if !res.AllApplied() {
		t.Fatalf("expected all hunks applied, got reports=%+v", res.Reports)
	}
	want := "inserted\n1\n2\n3\n4\n5\nSIX"
	if got := strings.Join(res.Lines, "\n"); got != want {
		t.Fatalf("apply = %q, want %q", got, want)
	}
}

func TestApplySearchesForDriftedContext(t *testing.T) {
	// Header claims the hunk starts at old line 1, but three lines were
	// inserted above it out of band; Apply must find it by content.
	data := "--- f\n+++ f\n@@ -1,2 +1,2 @@\n a\n-b\n+B\n"
	p, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	old := lines("x\ny\nz\na\nb\nc\n")
	res := Apply(old, false, p.Files[0].Hunks, ApplyOptions{})
	if !res.AllApplied() {
		t.Fatalf("expected all hunks applied, got reports=%+v", res.Reports)
	}
	want := "x\ny\nz\na\nB\nc"
	if got := strings.Join(res.Lines, "\n"); got != want {
		t.Fatalf("apply = %q, want %q", got, want)
	}
}

func TestApplyFuzzPeelsMismatchedContext(t *testing.T) {
	data := "--- f\n+++ f\n@@ -1,3 +1,3 @@\n before\n-mid\n+MID\n after\n"
	p, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// The trailing context line ("after" -> "after-changed") no longer
	// matches, so an exact match fails everywhere; fuzz=1 must still
	// place the hunk using just the leading context plus the changed line.
	old := lines("before\nmid\nafter-changed\n")
	res := Apply(old, false, p.Files[0].Hunks, ApplyOptions{Fuzz: 1})
	if res.Reports[0].Outcome != HunkAppliedFuzzy {
		t.Fatalf("outcome = %v, want fuzzy apply; reports=%+v", res.Reports[0].Outcome, res.Reports)
	}
	want := "before\nMID\nafter-changed"
	if got := strings.Join(res.Lines, "\n"); got != want {
		t.Fatalf("apply = %q, want %q", got, want)
	}

	res0 := Apply(old, false, p.Files[0].Hunks, ApplyOptions{Fuzz: 0})
	if res0.AllApplied() {
		t.Fatalf("expected fuzz=0 to fail, got reports=%+v", res0.Reports)
	}
}

func TestApplyAlreadyApplied(t *testing.T) {
	data := "--- f\n+++ f\n@@ -1,2 +1,2 @@\n a\n-b\n+B\n"
	p, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	old := lines("a\nB\n") // the edit is already present
	res := Apply(old, false, p.Files[0].Hunks, ApplyOptions{})
	if res.Reports[0].Outcome != HunkAlreadyApplied {
		t.Fatalf("outcome = %v, want already applied; reports=%+v", res.Reports[0].Outcome, res.Reports)
	}
	if got := strings.Join(res.Lines, "\n"); got != "a\nB" {
		t.Fatalf("apply changed content: %q", got)
	}
}

func TestApplyAlreadyAppliedSuppressedByForce(t *testing.T) {
	data := "--- f\n+++ f\n@@ -1,2 +1,2 @@\n a\n-b\n+B\n"
	p, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	old := lines("a\nB\n")
	res := Apply(old, false, p.Files[0].Hunks, ApplyOptions{Force: true})
	if res.Reports[0].Outcome != HunkFailed {
		t.Fatalf("outcome = %v, want failed under -f; reports=%+v", res.Reports[0].Outcome, res.Reports)
	}
	if len(res.Rejects) != 1 {
		t.Fatalf("expected one reject, got %d", len(res.Rejects))
	}
}

func TestApplyRejectOnConflict(t *testing.T) {
	data := "--- f\n+++ f\n@@ -1,2 +1,2 @@\n a\n-b\n+B\n"
	p, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	old := lines("a\ncompletely-different\n")
	res := Apply(old, false, p.Files[0].Hunks, ApplyOptions{})
	if res.AllApplied() {
		t.Fatalf("expected a failure, got reports=%+v", res.Reports)
	}
	if got := strings.Join(res.Lines, "\n"); got != "a\ncompletely-different" {
		t.Fatalf("rejected hunk must leave content untouched, got %q", got)
	}
	rej := WriteReject("f", "f", res.Rejects)
	if !strings.Contains(string(rej), "! b\n") || !strings.Contains(string(rej), "! B\n") {
		t.Fatalf("reject content missing expected hunk body: %s", rej)
	}
}

func TestApplyDeletionMismatchIsRejected(t *testing.T) {
	data := "--- f\n+++ /dev/null\n@@ -1 +0,0 @@\n-expected\n"
	p, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	res := Apply(lines("actual\n"), false, p.Files[0].Hunks, ApplyOptions{})
	if res.Reports[0].Outcome != HunkFailed {
		t.Fatalf("outcome = %v, want failed; reports=%+v", res.Reports[0].Outcome, res.Reports)
	}
	if got := strings.Join(res.Lines, "\n"); got != "actual" {
		t.Fatalf("rejected deletion changed content: %q", got)
	}
	if len(res.Rejects) != 1 {
		t.Fatalf("expected one reject, got %d", len(res.Rejects))
	}
}

func TestApplyFuzzRequiresFullOldSideToFit(t *testing.T) {
	data := "--- f\n+++ f\n@@ -1,2 +1,2 @@\n-x\n+y\n tail\n"
	p, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// The changed line matches, but the peeled trailing context is absent.
	// The hunk must reject rather than indexing beyond oldLines.
	res := Apply(lines("x\n"), false, p.Files[0].Hunks, ApplyOptions{Fuzz: 1})
	if res.Reports[0].Outcome != HunkFailed {
		t.Fatalf("outcome = %v, want failed; reports=%+v", res.Reports[0].Outcome, res.Reports)
	}
	if got := strings.Join(res.Lines, "\n"); got != "x" {
		t.Fatalf("rejected fuzzy hunk changed content: %q", got)
	}
}

func TestApplyRejectedOutOfOrderHunkDoesNotMoveCursorBackward(t *testing.T) {
	data := "--- f\n+++ f\n" +
		"@@ -1,3 +1,3 @@\n A\n-B\n+BB\n C\n" +
		"@@ -1 +1 @@\n-WRONG\n+NOPE\n"
	p, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	res := Apply(lines("A\nB\nC\nD\n"), false, p.Files[0].Hunks, ApplyOptions{})
	if len(res.Reports) != 2 || res.Reports[0].Outcome != HunkApplied || res.Reports[1].Outcome != HunkFailed {
		t.Fatalf("unexpected reports: %+v", res.Reports)
	}
	if got := strings.Join(res.Lines, "\n"); got != "A\nBB\nC\nD" {
		t.Fatalf("rejected out-of-order hunk corrupted content: %q", got)
	}
}

func TestApplyReverse(t *testing.T) {
	data := "--- f\n+++ f\n@@ -1,2 +1,2 @@\n a\n-b\n+B\n"
	p, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// Applying -R to the already-forward-patched content should recover
	// the original.
	res := Apply(lines("a\nB\n"), false, p.Files[0].Hunks, ApplyOptions{Reverse: true})
	if !res.AllApplied() {
		t.Fatalf("expected reverse apply to succeed, got reports=%+v", res.Reports)
	}
	if got := strings.Join(res.Lines, "\n"); got != "a\nb" {
		t.Fatalf("reverse apply = %q, want %q", got, "a\nb")
	}
}

func TestApplyNoFinalNewlinePreserved(t *testing.T) {
	data := "--- f\n+++ f\n@@ -1,2 +1,2 @@\n a\n-b\n+B\n\\ No newline at end of file\n"
	p, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	res := Apply(lines("a\nb\n"), false, p.Files[0].Hunks, ApplyOptions{})
	if !res.NoFinalNewline {
		t.Fatalf("expected NoFinalNewline true")
	}
}

func TestParseGitDiffHeaderSkipped(t *testing.T) {
	data := "diff --git a/f.txt b/f.txt\n" +
		"index 1234567..89abcde 100644\n" +
		"--- a/f.txt\n+++ b/f.txt\n@@ -1 +1 @@\n-old\n+new\n"
	p, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(p.Files) != 1 || p.Files[0].OldName != "a/f.txt" {
		t.Fatalf("unexpected result: %+v", p.Files)
	}
}

func TestParseGitBinaryPatchIsUnsupported(t *testing.T) {
	data := "diff --git a/img.png b/img.png\n" +
		"index 1234567..89abcde 100644\n" +
		"GIT binary patch\nliteral 4\nXcmZ?wb\n" +
		"diff --git a/f.txt b/f.txt\nindex 1..2 100644\n--- a/f.txt\n+++ b/f.txt\n@@ -1 +1 @@\n-old\n+new\n"
	p, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(p.Files) != 2 {
		t.Fatalf("got %d files, want 2", len(p.Files))
	}
	if p.Files[0].Unsupported == "" {
		t.Fatalf("expected binary section to be marked unsupported: %+v", p.Files[0])
	}
	if p.Files[1].Unsupported != "" || len(p.Files[1].Hunks) != 1 {
		t.Fatalf("second file should parse normally: %+v", p.Files[1])
	}
}

func TestParseSkipsLeadingGarbage(t *testing.T) {
	data := "Index: f.txt\n===================================================================\n" +
		"--- f.txt\n+++ f.txt\n@@ -1 +1 @@\n-x\n+X\n"
	p, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(p.Files) != 1 || p.Files[0].OldName != "f.txt" {
		t.Fatalf("unexpected: %+v", p.Files)
	}
}

func TestParseNoRecognizablePatch(t *testing.T) {
	_, err := Parse([]byte("this is not a patch\njust some text\n"))
	if err == nil {
		t.Fatalf("expected an error for unparseable input")
	}
}
