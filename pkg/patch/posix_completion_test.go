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
	want := []string{"#ifndef FEATURE", "gone", "#endif", "same", "#ifndef FEATURE", "old", "#else", "new", "added", "#endif"}
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

func TestRejectPreservesInputNotation(t *testing.T) {
	h := Hunk{OldStart: 0, OldCount: 1, NewStart: 0, NewCount: 1, Lines: []HunkLine{{Kind: LineDelete, Text: "old", NoEOL: true}, {Kind: LineAdd, Text: "new"}}}
	if got := string(WriteRejectFormat("a", "b", FormatContext, []Hunk{h})); !strings.HasPrefix(got, "*** a\n--- b\n***************\n") {
		t.Fatalf("context reject=%q", got)
	} else if !strings.Contains(got, "\\ No newline at end of file\n") {
		t.Fatalf("context reject lost no-newline marker: %q", got)
	}
	if got := string(WriteRejectFormat("a", "b", FormatNormal, []Hunk{h})); !strings.HasPrefix(got, "*** a\n--- b\n***************\n") {
		t.Fatalf("normal-as-context reject=%q", got)
	}
}
