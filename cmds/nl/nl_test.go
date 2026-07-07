package nlcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runNL(t *testing.T, dir, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code := cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestNLDefaultsNumberNonEmptyLines(t *testing.T) {
	out, errb, code := runNL(t, t.TempDir(), "a\n\nb\n")
	// Unnumbered lines get width+len(separator) spaces (GNU padding).
	want := "     1\ta\n       \n     2\tb\n"
	if out != want || errb != "" || code != 0 {
		t.Fatalf("nl default = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
}

func TestNLStylesAndFormatting(t *testing.T) {
	out, _, code := runNL(t, t.TempDir(), "a\n\n", "-b", "a", "-n", "rz", "-s", ":", "-w", "3")
	if want := "001:a\n002:\n"; out != want || code != 0 {
		t.Fatalf("nl formatted = (%q, %d), want (%q, 0)", out, code, want)
	}
	// Style n still pads with width+len(separator) spaces.
	out, _, code = runNL(t, t.TempDir(), "a\n", "-b", "n")
	if want := "       a\n"; out != want || code != 0 {
		t.Fatalf("nl -bn = (%q, %d), want (%q, 0)", out, code, want)
	}
	out, _, code = runNL(t, t.TempDir(), "a\n", "-b", "n", "-w", "2", "-s", "::")
	if want := "    a\n"; out != want || code != 0 {
		t.Fatalf("nl -bn -w2 -s:: = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestNLStartIncrementAndNoRenumber(t *testing.T) {
	out, _, code := runNL(t, t.TempDir(), "a\nb\n", "-b", "a", "-v", "10", "-i", "5", "-w", "2", "-s", ":")
	if want := "10:a\n15:b\n"; out != want || code != 0 {
		t.Fatalf("nl start/increment = (%q, %d), want (%q, 0)", out, code, want)
	}

	// Section delimiter lines are replaced with an empty line on output.
	input := "a\n\\:\\:\nb\n"
	out, _, code = runNL(t, t.TempDir(), input, "-b", "a", "-v", "3", "-w", "1", "-s", ":")
	if want := "3:a\n\n3:b\n"; out != want || code != 0 {
		t.Fatalf("nl renumber delimiter = (%q, %d), want (%q, 0)", out, code, want)
	}

	out, _, code = runNL(t, t.TempDir(), input, "-b", "a", "-v", "3", "-w", "1", "-s", ":", "-p")
	if want := "3:a\n\n4:b\n"; out != want || code != 0 {
		t.Fatalf("nl no-renumber delimiter = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestNLNegativeIncrement(t *testing.T) {
	out, _, code := runNL(t, t.TempDir(), "a\nb\nc\n", "-b", "a", "-v", "10", "-i", "-2", "-w", "2", "-s", ":")
	if want := "10:a\n 8:b\n 6:c\n"; out != want || code != 0 {
		t.Fatalf("nl -i -2 = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestNLHeaderFooterStyles(t *testing.T) {
	input := "\\:\\:\\:\nh\n\\:\\:\nb\n\\:\nf\n"
	out, _, code := runNL(t, t.TempDir(), input, "-h", "a", "-b", "n", "-f", "a", "-w", "1", "-s", ":")
	// Delimiter lines become empty lines; style n pads with 2 spaces (w1+sep1).
	want := "\n1:h\n\n  b\n\n1:f\n"
	if out != want || code != 0 {
		t.Fatalf("nl section styles = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestNLSingleCharDelimiterKeepsColon(t *testing.T) {
	// -d + means delimiters are "+:" (second char stays ':').
	input := "+:+:+:\nHEAD\n+:+:\nkeep\nskip\n+:\nfoot\n"
	out, _, code := runNL(t, t.TempDir(), input, "-d", "+", "-h", "pHEAD", "-b", "p^keep$", "-f", "a", "-w", "1", "-s", ":")
	want := "\n1:HEAD\n\n1:keep\n  skip\n\n1:foot\n"
	if out != want || code != 0 {
		t.Fatalf("nl -d+ = (%q, %d), want (%q, 0)", out, code, want)
	}
	// A bare "+++" line is NOT a delimiter under -d +.
	out, _, code = runNL(t, t.TempDir(), "+++\na\n", "-d", "+", "-b", "a", "-w", "1", "-s", ":")
	if want := "1:+++\n2:a\n"; out != want || code != 0 {
		t.Fatalf("nl -d+ non-delimiter = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestNLEmptyDelimiterDisablesSections(t *testing.T) {
	input := "a\n\\:\\:\nb\n"
	out, _, code := runNL(t, t.TempDir(), input, "-d", "", "-b", "a", "-w", "1", "-s", ":")
	want := "1:a\n2:\\:\\:\n3:b\n"
	if out != want || code != 0 {
		t.Fatalf("nl -d '' = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestNLBlankJoin(t *testing.T) {
	out, _, code := runNL(t, t.TempDir(), "a\n\n\n\nb\n", "-b", "a", "-l", "2", "-w", "1", "-s", ":")
	// Skipped blanks in a join group still get the aligned space prefix.
	want := "1:a\n  \n2:\n  \n3:b\n"
	if out != want || code != 0 {
		t.Fatalf("nl blank join = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestNLBasicRegularExpressionStyle(t *testing.T) {
	// BRE: \(x\)\{2\} matches "xx"; the same pattern is not a valid Go regexp.
	out, _, code := runNL(t, t.TempDir(), "xx\ny\n", "-b", "p\\(x\\)\\{2\\}", "-w", "1", "-s", ":")
	if want := "1:xx\n  y\n"; out != want || code != 0 {
		t.Fatalf("nl BRE style = (%q, %d), want (%q, 0)", out, code, want)
	}
	// BRE treats + as a literal.
	out, _, code = runNL(t, t.TempDir(), "a+\nab\n", "-b", "pa+", "-w", "1", "-s", ":")
	if want := "1:a+\n  ab\n"; out != want || code != 0 {
		t.Fatalf("nl BRE literal + = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestNLFilesFormOneDocument(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b"), []byte("y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runNL(t, dir, "", "-w", "1", "a", "b")
	if want := "1\tx\n2\ty\n"; out != want || code != 0 {
		t.Fatalf("nl files = (%q, %d), want (%q, 0)", out, code, want)
	}

	// Section state also carries across files: file a ends in a footer
	// section, so file b's lines are still footer lines.
	if err := os.WriteFile(filepath.Join(dir, "c"), []byte("x\n\\:\nf\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "d"), []byte("g\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code = runNL(t, dir, "", "-f", "a", "-w", "1", "-s", ":", "c", "d")
	if want := "1:x\n\n1:f\n2:g\n"; out != want || code != 0 {
		t.Fatalf("nl cross-file section = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestNLRejectsBadStyle(t *testing.T) {
	_, errb, code := runNL(t, t.TempDir(), "", "-b", "x")
	if code != 2 || !strings.Contains(errb, "invalid body numbering style") {
		t.Fatalf("nl bad style code=%d err=%q", code, errb)
	}
}

func TestNLRejectsZeroWidth(t *testing.T) {
	_, errb, code := runNL(t, t.TempDir(), "", "-w", "0")
	if code != 2 || !strings.Contains(errb, "invalid line number field width") {
		t.Fatalf("nl -w0 code=%d err=%q", code, errb)
	}
}
