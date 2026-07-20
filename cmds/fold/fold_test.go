package foldcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runFold(t *testing.T, input string, args ...string) (string, string, int) {
	t.Helper()
	var out, err bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Stdio: tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &err}}
	code := run(rc, args)
	return out.String(), err.String(), code
}

func TestFoldWidthRunes(t *testing.T) {
	out, stderr, code := runFold(t, "abcdef\n", "-w", "3")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "abc\ndef\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFoldFirstWideUnitPreservesLeadingBreak(t *testing.T) {
	out, stderr, code := runFold(t, "\tCajkKP", "-w", "1")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "\n\t\nC\na\nj\nk\nK\nP"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFoldWideUnitAfterTextDoesNotAddEmptySegment(t *testing.T) {
	out, stderr, code := runFold(t, "a\t", "-w", "1")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "a\n\t"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFoldSpacesAndFile(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "in.txt")
	if err := os.WriteFile(name, []byte("alpha beta gamma\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &stderr}}
	code := run(rc, []string{"-s", "-w", "10", "in.txt"})
	if code != 0 || stderr.String() != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	// The blank after "beta" overflows column 10, so the break goes
	// after the last blank on the line — the one following "alpha" —
	// and that blank is kept (POSIX; verified against GNU/BSD fold).
	if want := "alpha \nbeta gamma\n"; out.String() != want {
		t.Fatalf("out=%q want %q", out.String(), want)
	}
}

func TestFoldSpacesKeepsTrailingBlank(t *testing.T) {
	// fold never deletes bytes: -s breaks after the blank, keeping it.
	out, stderr, code := runFold(t, "hello world\n", "-s", "-w", "8")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "hello \nworld\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFoldSpacesKeepsLeadingBlanks(t *testing.T) {
	// Continuation lines keep their leading blanks; nothing is trimmed.
	out, stderr, code := runFold(t, "aa   bb\n", "-s", "-w", "4")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "aa  \n bb\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFoldTabAdvancesColumn(t *testing.T) {
	// A tab advances to the next multiple of 8, so "a\tb" is 9 columns
	// and folds at width 8.
	out, stderr, code := runFold(t, "a\tb\n", "-w", "8")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "a\t\nb\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFoldBackspaceDecrementsColumn(t *testing.T) {
	out, stderr, code := runFold(t, "ab\bcd\n", "-w", "3")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	// Columns: a=1 b=2 \b=1 c=2 d=3 — all fit in one 3-column line.
	if want := "ab\bcd\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFoldBackspaceMovesOneColumnAfterWideRune(t *testing.T) {
	out, stderr, code := runFold(t, "界\bXX\n", "-w", "2")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "界\bX\nX\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFoldCarriageReturnResetsColumn(t *testing.T) {
	out, stderr, code := runFold(t, "abc\rdef\n", "-w", "4")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "abc\rdef\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFoldBytesCountsControlCharsAsOne(t *testing.T) {
	// With -b a tab is one byte like any other, so "a\tb" (3 bytes)
	// fits in width 3.
	out, stderr, code := runFold(t, "a\tb\n", "-b", "-w", "3")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "a\tb\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFoldBytesPreservesUtf8UnitsAtSmallWidth(t *testing.T) {
	out, stderr, code := runFold(t, "界界", "-w", "1", "-b")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "\n界\n界"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFoldBytesWideUnitAfterTextDoesNotAddEmptySegment(t *testing.T) {
	out, stderr, code := runFold(t, "a界", "-w", "1", "-b")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "a\n界"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFoldCharactersKeepsUTF8RunesWhole(t *testing.T) {
	out, stderr, code := runFold(t, "ééé\n", "-w", "2", "-c")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "éé\né\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFoldDefaultUsesDisplayColumns(t *testing.T) {
	out, stderr, code := runFold(t, "界界界\n", "-w", "3")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "界\n界\n界\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}

	out, stderr, code = runFold(t, "界界界\n", "-w", "3", "-c")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "界界界\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFoldCharactersTabStillAdvances(t *testing.T) {
	// -c counts characters, but per GNU a tab still advances to the
	// next multiple of 8 (only -b treats it as a single unit).
	out, stderr, code := runFold(t, "a\tb\n", "-c", "-w", "8")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "a\t\nb\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFoldBytesCountsUTF8EncodingWidth(t *testing.T) {
	out, stderr, code := runFold(t, "éé\n", "-w", "2", "-b")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "é\né\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFoldObsoleteWidthSyntax(t *testing.T) {
	out, stderr, code := runFold(t, "abcdef\n", "-3")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if want := "abc\ndef\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}
}

func TestFoldRejectsBadWidth(t *testing.T) {
	_, stderr, code := runFold(t, "", "-w", "0")
	if code != 2 || !strings.Contains(stderr, "invalid number of columns") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}
