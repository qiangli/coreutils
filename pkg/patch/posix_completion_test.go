package patch

import (
	"strings"
	"testing"
)

func TestApplyEdCommandsAndDotQuoting(t *testing.T) {
	got, err := ApplyEd([]byte("a\nb\nc"), []byte("3a\n..dot\n.\n2d\n1c\nA\n.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "A\nc\n..dot\n" {
		t.Fatalf("got %q", got)
	}
}

func TestApplyEdDiffDotProtectionAndEmptyScript(t *testing.T) {
	script := []byte("1a\n..\n.\ns/.//\na\n..\n.\ns/.//\n")
	got, err := ApplyEd([]byte("a\n"), script)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "a\n.\n.\n" {
		t.Fatalf("dot-protected result=%q", got)
	}
	got, err = ApplyEd([]byte("unchanged\n"), nil)
	if err != nil || string(got) != "unchanged\n" {
		t.Fatalf("empty script result=%q err=%v", got, err)
	}
}

func TestApplyEdAppendRangeUsesSecondAddress(t *testing.T) {
	got, err := ApplyEd([]byte("a\nb\nc\n"), []byte("1,2a\nX\n.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "a\nb\nX\nc\n" {
		t.Fatalf("range append=%q", got)
	}
}

func TestApplyEdInsertAndDetection(t *testing.T) {
	if !LooksLikeEdScript([]byte("2i\ninserted\n.\n")) || LooksLikeEdScript([]byte("2c2\n< a\n---\n> b\n")) {
		t.Fatal("ed script detection confused ed and normal forms")
	}
	got, err := ApplyEd([]byte("one\ntwo\n"), []byte("2i\ninserted\n.\n"))
	if err != nil || string(got) != "one\ninserted\ntwo\n" {
		t.Fatalf("insert=%q err=%v", got, err)
	}
	script, index, ok := ExtractEdScript([]byte("mail header\nIndex: tree/f\n2i\ninserted\n.\n"))
	if !ok || index != "tree/f" || string(script) != "2i\ninserted\n.\n" {
		t.Fatalf("extracted script=%q index=%q ok=%v", script, index, ok)
	}
}

func TestExtractEdScriptStripsCommonIndent(t *testing.T) {
	script, index, ok := ExtractEdScript([]byte("  Index: dir/f\n  1c\n  replacement\n  .\n"))
	if !ok || index != "dir/f" {
		t.Fatalf("ok=%v index=%q", ok, index)
	}
	if got, want := string(script), "1c\nreplacement\n.\n"; got != want {
		t.Fatalf("script=%q want=%q", got, want)
	}
}

func TestApplyEdIfdefAndDotProtection(t *testing.T) {
	got, err := ApplyEdIfdef([]byte("old\n"), []byte("1c\n..\n.\ns/.//\n"), "FEATURE")
	if err != nil {
		t.Fatal(err)
	}
	want := "#ifndef FEATURE\nold\n#else\n.\n#endif /* FEATURE */\n"
	if string(got) != want {
		t.Fatalf("ifdef ed=%q want=%q", got, want)
	}
}

func TestApplyEdRejectsMalformedOrOutOfRangeScript(t *testing.T) {
	for _, script := range []string{"bogus\n", "1c\nunterminated\n", "9d\n"} {
		if _, err := ApplyEd([]byte("a\n"), []byte(script)); err == nil {
			t.Fatalf("script %q unexpectedly succeeded", script)
		}
	}
}

func TestApplyIfdefInsertionDeletionAndReplacement(t *testing.T) {
	hunks := []Hunk{{
		OldStart: 0, OldCount: 3, NewStart: 0, NewCount: 3,
		Lines: []HunkLine{
			{Kind: LineDelete, Text: "gone"},
			{Kind: LineContext, Text: "same"},
			{Kind: LineDelete, Text: "old"}, {Kind: LineAdd, Text: "new"},
			{Kind: LineAdd, Text: "added"},
		},
	}}
	res := ApplyIfdef([]string{"gone", "same", "old"}, false, hunks, ApplyOptions{}, "FEATURE")
	if !res.AllApplied() {
		t.Fatalf("reports=%+v", res.Reports)
	}
	want := []string{"#ifndef FEATURE", "gone", "#endif /* FEATURE */", "same", "#ifndef FEATURE", "old", "#else", "new", "added", "#endif /* FEATURE */"}
	if strings.Join(res.Lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("lines=%q want=%q", res.Lines, want)
	}
}

func TestApplyIfdefPreservesUntouchedFinalNoNewline(t *testing.T) {
	hunk := Hunk{OldStart: 0, OldCount: 2, NewStart: 0, NewCount: 2, Lines: []HunkLine{
		{Kind: LineDelete, Text: "old"}, {Kind: LineAdd, Text: "new"},
		{Kind: LineContext, Text: "last", NoEOL: true},
	}}
	res := ApplyIfdef([]string{"old", "last"}, true, []Hunk{hunk}, ApplyOptions{}, "FEATURE")
	if !res.AllApplied() || !res.NoFinalNewline {
		t.Fatalf("reports=%+v no-final-newline=%v", res.Reports, res.NoFinalNewline)
	}
}

func TestParseIndexAndCommonIndent(t *testing.T) {
	p, err := Parse([]byte("\tIndex: f\n\t1c1\n\t< old\n\t---\n\t> new\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Files) != 1 || p.Files[0].IndexName != "f" || p.Files[0].Format != FormatNormal {
		t.Fatalf("parsed=%+v", p.Files)
	}
}

func TestRejectUsesCopiedContext(t *testing.T) {
	h := Hunk{OldStart: 0, OldCount: 1, NewStart: 0, NewCount: 1, Lines: []HunkLine{{Kind: LineDelete, Text: "old", NoEOL: true}, {Kind: LineAdd, Text: "new"}}}
	if got := string(WriteRejectFormat("a", "b", FormatContext, []Hunk{h})); !strings.HasPrefix(got, "*** a\n--- b\n***************\n") {
		t.Fatalf("context reject=%q", got)
	} else if !strings.Contains(got, "\\ No newline at end of file\n") {
		t.Fatalf("context reject lost no-newline marker: %q", got)
	}
	if got := string(WriteRejectFormat("a", "b", FormatNormal, []Hunk{h})); !strings.HasPrefix(got, "*** a\n--- b\n***************\n") {
		t.Fatalf("normal-as-context reject=%q", got)
	}
	if got := string(WriteRejectFormat("a", "b", FormatUnified, []Hunk{h})); !strings.HasPrefix(got, "*** a\n--- b\n***************\n") || strings.Contains(got, "@@") {
		t.Fatalf("unified reject was not converted to Issue 7 copied-context form: %q", got)
	}
}

func TestApplyNormalMultipleHunksUsesOriginalCoordinates(t *testing.T) {
	p, err := Parse([]byte("1c1\n< a\n---\n> A\n3d2\n< c\n4a4\n> e\n"))
	if err != nil {
		t.Fatal(err)
	}
	res := Apply([]string{"a", "b", "c", "d"}, false, p.Files[0].Hunks, ApplyOptions{})
	if !res.AllApplied() || strings.Join(res.Lines, "\n") != "A\nb\nd\ne" {
		t.Fatalf("reports=%+v lines=%q", res.Reports, res.Lines)
	}
}

func TestIgnoreWhitespaceMatchesPOSIXBlanksOnly(t *testing.T) {
	if !matchAt([]string{"a \t  b"}, []string{"a\tb"}, 0, true) {
		t.Fatal("space/tab runs did not match with -l")
	}
	if matchAt([]string{"a\vb"}, []string{"a b"}, 0, true) {
		t.Fatal("non-blank control character was treated as a POSIX blank")
	}
}

func TestTruncatedContextPatchReturnsError(t *testing.T) {
	if _, err := Parse([]byte("*** old\n--- new\n***************\n*** 1 ****\n")); err == nil {
		t.Fatal("truncated copied-context patch unexpectedly parsed")
	}
}

// diff -e always emits an address, so an address-less ed command is not
// evidence of an ed script; neither is anything that follows a line
// announcing a copied-context, unified, or normal difference listing.
func TestExtractEdScriptRequiresAddressAndYieldsToDiffListings(t *testing.T) {
	for _, in := range []string{
		"--- f.txt\n+++ f.txt\n@@ -1,2 +1,2 @@\n a\n-b\n+B\n",
		"*** f.txt\n--- f.txt\n***************\n*** 1,2 ****\n  a\n! b\n--- 1,2 ----\n  a\n! B\n",
		"Index: f.txt\n2c2\n< a\n---\n> A\n",
		"a\n",
		"i\n",
		"Index: f.txt\nc\nreplacement\n.\n",
	} {
		if script, _, ok := ExtractEdScript([]byte(in)); ok {
			t.Fatalf("input %q taken for an ed script (%q)", in, script)
		}
	}
	for _, in := range []string{"2i\ninserted\n.\n", "Index: f\n1,2c\nx\n.\n", "  3a\n  tail\n  .\n"} {
		if _, _, ok := ExtractEdScript([]byte(in)); !ok {
			t.Fatalf("addressed ed script %q not detected", in)
		}
	}
}
