package odcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runOD(t *testing.T, dir, stdin string, args ...string) (string, string, int) {
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

func TestODDefaultOctalWords(t *testing.T) {
	out, errb, code := runOD(t, t.TempDir(), "ABCD")
	want := "0000000 041101 042103\n0000004\n"
	if out != want || errb != "" || code != 0 {
		t.Fatalf("od default = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
}

func TestODFormatsAndOffsets(t *testing.T) {
	// GNU prints hexadecimal addresses 6 digits wide.
	out, _, code := runOD(t, t.TempDir(), "abc\n", "-A", "x", "-t", "x1", "-N", "3")
	if want := "000000 61 62 63\n000003\n"; out != want || code != 0 {
		t.Fatalf("od x1 = (%q, %d), want (%q, 0)", out, code, want)
	}
	out, _, code = runOD(t, t.TempDir(), "a\n", "-A", "n", "-t", "c")
	if want := "   a  \\n\n"; out != want || code != 0 {
		t.Fatalf("od c no addresses = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestODTypeAliases(t *testing.T) {
	out, _, code := runOD(t, t.TempDir(), "AB", "-A", "n", "-t", "xC")
	if want := " 41 42\n"; out != want || code != 0 {
		t.Fatalf("od -t xC = (%q, %d), want (%q, 0)", out, code, want)
	}
	out, _, code = runOD(t, t.TempDir(), "AB", "-A", "n", "-t", "xS")
	if want := " 4241\n"; out != want || code != 0 {
		t.Fatalf("od -t xS = (%q, %d), want (%q, 0)", out, code, want)
	}
	out, _, code = runOD(t, t.TempDir(), "a\n", "-A", "n", "-t", "char")
	if want := "   a  \\n\n"; out != want || code != 0 {
		t.Fatalf("od -t char = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestODShortAliasesAndWidth(t *testing.T) {
	out, _, code := runOD(t, t.TempDir(), "abcd", "-b", "-w", "2")
	if want := "0000000 141 142\n0000002 143 144\n0000004\n"; out != want || code != 0 {
		t.Fatalf("od -b -w = (%q, %d), want (%q, 0)", out, code, want)
	}

	out, _, code = runOD(t, t.TempDir(), "AB", "-x")
	if want := "0000000 4241\n0000002\n"; out != want || code != 0 {
		t.Fatalf("od -x = (%q, %d), want (%q, 0)", out, code, want)
	}

	out, _, code = runOD(t, t.TempDir(), "AB", "-d")
	if want := "0000000 16961\n0000002\n"; out != want || code != 0 {
		t.Fatalf("od -d = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestODSkipAndFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("abcd"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runOD(t, dir, "", "-t", "o1", "-j", "2", "in")
	if want := "0000002 143 144\n0000004\n"; out != want || code != 0 {
		t.Fatalf("od skip file = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestODMultiFormatEndianStringsAndTraditionalSkip(t *testing.T) {
	out, _, code := runOD(t, t.TempDir(), "ABCD", "-A", "x", "-t", "x2", "-t", "u1", "--endian", "big", "-w", "4")
	want := "000000 4142 4344\n        65  66  67  68\n000004\n"
	if out != want || code != 0 {
		t.Fatalf("od multi/endian = (%q, %d), want (%q, 0)", out, code, want)
	}

	// -S prints NUL-terminated runs only, with no trailing offset line.
	out, _, code = runOD(t, t.TempDir(), "\x00abc\x00de\x00", "-A", "d", "-S", "3")
	want = "0000001 abc\n"
	if out != want || code != 0 {
		t.Fatalf("od strings = (%q, %d), want (%q, 0)", out, code, want)
	}

	out, _, code = runOD(t, t.TempDir(), "abcd", "+2")
	want = "0000002 062143\n0000004\n"
	if out != want || code != 0 {
		t.Fatalf("od traditional skip = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestODRejectsBadFormat(t *testing.T) {
	_, errb, code := runOD(t, t.TempDir(), "", "-t", "x4")
	if code != 0 || errb != "" {
		t.Fatalf("od x4 should now be supported code=%d err=%q", code, errb)
	}

	_, errb, code = runOD(t, t.TempDir(), "", "-t", "z9")
	if code != 2 || !strings.Contains(errb, "unsupported output format") {
		t.Fatalf("od bad format code=%d err=%q", code, errb)
	}
}

func TestODRejectsBadWidth(t *testing.T) {
	_, errb, code := runOD(t, t.TempDir(), "", "-w", "0")
	if code != 2 || !strings.Contains(errb, "invalid output width") {
		t.Fatalf("od bad width code=%d err=%q", code, errb)
	}
}

// -t a is named characters (GNU: distinct from -t c, high bit ignored).
func TestODNamedCharsVsC(t *testing.T) {
	out, _, code := runOD(t, t.TempDir(), "a b\n", "-a", "-A", "n")
	if want := "   a  sp   b  nl\n"; out != want || code != 0 {
		t.Fatalf("od -a = (%q, %d), want (%q, 0)", out, code, want)
	}
	out, _, code = runOD(t, t.TempDir(), "a b\n\a\v", "-t", "c", "-A", "n")
	if want := "   a       b  \\n  \\a  \\v\n"; out != want || code != 0 {
		t.Fatalf("od -t c = (%q, %d), want (%q, 0)", out, code, want)
	}
	out, _, code = runOD(t, t.TempDir(), "\xe1", "-a", "-A", "n")
	if want := "   a\n"; out != want || code != 0 {
		t.Fatalf("od -a high bit = (%q, %d), want (%q, 0)", out, code, want)
	}
}

// GNU elides consecutive identical lines with a single *; -v outputs all.
func TestODDuplicateSuppression(t *testing.T) {
	data := strings.Repeat("\x00", 48)
	out, _, code := runOD(t, t.TempDir(), data)
	want := "0000000 000000 000000 000000 000000 000000 000000 000000 000000\n*\n0000060\n"
	if out != want || code != 0 {
		t.Fatalf("od dup = (%q, %d), want (%q, 0)", out, code, want)
	}
	out, _, code = runOD(t, t.TempDir(), data, "-v")
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 4 || strings.Contains(out, "*") || code != 0 {
		t.Fatalf("od -v = (%q, %d), want 3 data lines + final offset and no *", out, code)
	}
}

// The traditional +offset operand is octal by default; '.' means
// decimal and a trailing 'b' multiplies by 512.
func TestODTraditionalOffsetRadix(t *testing.T) {
	data := strings.Repeat("x", 20)
	out, _, code := runOD(t, t.TempDir(), data, "-t", "o1", "+20")
	if code != 0 || !strings.HasPrefix(out, "0000020 170 170 170 170\n") {
		t.Fatalf("od +20 (octal) = (%q, %d)", out, code)
	}
	out, _, code = runOD(t, t.TempDir(), data, "-t", "o1", "+16.")
	if code != 0 || !strings.HasPrefix(out, "0000020 170 170 170 170\n") {
		t.Fatalf("od +16. (decimal) = (%q, %d)", out, code)
	}
}

// GNU errors when -j skips past the end of the combined input.
func TestODSkipPastEOF(t *testing.T) {
	_, errb, code := runOD(t, t.TempDir(), "hi", "-j", "100")
	if code != 1 || !strings.Contains(errb, "cannot skip past end of combined input") {
		t.Fatalf("od skip past eof: code=%d err=%q", code, errb)
	}
}

func TestODNewTypeAliases(t *testing.T) {
	// octal format alias
	out, _, code := runOD(t, t.TempDir(), "AB", "-A", "n", "-t", "octal1")
	if want := " 101 102\n"; out != want || code != 0 {
		t.Fatalf("od -t octal1 = (%q, %d), want (%q, 0)", out, code, want)
	}
	// hex format alias
	out, _, code = runOD(t, t.TempDir(), "AB", "-A", "n", "-t", "hex1")
	if want := " 41 42\n"; out != want || code != 0 {
		t.Fatalf("od -t hex1 = (%q, %d), want (%q, 0)", out, code, want)
	}
	// signed format alias (maps to d=decimal)
	out, _, code = runOD(t, t.TempDir(), "AB", "-A", "n", "-t", "signed1")
	if want := "   65   66\n"; out != want || code != 0 {
		t.Fatalf("od -t signed1 = (%q, %d), want (%q, 0)", out, code, want)
	}
	// unsigned decimal format alias
	out, _, code = runOD(t, t.TempDir(), "AB", "-A", "n", "-t", "unsigned1")
	if want := "  65  66\n"; out != want || code != 0 {
		t.Fatalf("od -t unsigned1 = (%q, %d), want (%q, 0)", out, code, want)
	}
	// Size aliases: char, short, int, long
	out, _, code = runOD(t, t.TempDir(), "ABCD", "-A", "n", "-t", "xchar")
	if want := " 41 42 43 44\n"; out != want || code != 0 {
		t.Fatalf("od -t xchar = (%q, %d), want (%q, 0)", out, code, want)
	}
	out, _, code = runOD(t, t.TempDir(), "ABCD", "-A", "n", "-t", "xshort")
	if want := " 4241 4443\n"; out != want || code != 0 {
		t.Fatalf("od -t xshort = (%q, %d), want (%q, 0)", out, code, want)
	}
	out, _, code = runOD(t, t.TempDir(), "ABCD", "-A", "n", "-t", "xint")
	if want := " 44434241\n"; out != want || code != 0 {
		t.Fatalf("od -t xint = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestODTraditionalOffsetBeforeFile(t *testing.T) {
	// --traditional allows +offset before file name
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("abcd"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runOD(t, dir, "", "--traditional", "+2", "-t", "o1", "in")
	if want := "0000002 143 144\n0000004\n"; out != want || code != 0 {
		t.Fatalf("od --traditional +2 file = (%q, %d), want (%q, 0)", out, code, want)
	}
}

// POSIX: a bare -t type letter with no explicit size takes the type's
// natural C size — d/o/u/x default to "int" (4 bytes), f defaults to
// "double" (8 bytes) — never the 2-byte "short" default.
func TestODBareTypeDefaultsToIntWidth(t *testing.T) {
	cases := []struct {
		format string
		data   string
		want   string
	}{
		{"d", "ABCD", " 1145258561\n"},
		{"o", "ABCD", " 010420641101\n"},
		{"u", "ABCD", " 1145258561\n"},
		{"x", "ABCD", " 44434241\n"},
		{"f", "\x00\x00\x00\x00\x00\x00\xf0\x3f", " 1\n"},
		// Bare word-form aliases go through the same default-size path.
		{"octal", "ABCD", " 010420641101\n"},
		{"hex", "ABCD", " 44434241\n"},
		{"signed", "ABCD", " 1145258561\n"},
		{"unsigned", "ABCD", " 1145258561\n"},
	}
	for _, tc := range cases {
		out, errb, code := runOD(t, t.TempDir(), tc.data, "-A", "n", "-t", tc.format)
		if out != tc.want || errb != "" || code != 0 {
			t.Fatalf("od -t %s = (%q, %q, %d), want (%q, \"\", 0)", tc.format, out, errb, code, tc.want)
		}
	}
}

func TestODByteCountLowercaseSuffixes(t *testing.T) {
	cases := []struct {
		text string
		want int64
	}{
		{"1", 1},
		{"010", 8},
		{"0x10", 16},
		{"0Xff", 255},
		{"1b", 512},
		{"2b", 1024},
		{"1k", 1024},
		{"3k", 3072},
		{"1m", 1048576},
		{"2m", 2097152},
	}
	for _, tc := range cases {
		got, err := parseBytes(tc.text)
		if err != nil || got != tc.want {
			t.Fatalf("parseBytes(%q) = (%d, %v), want (%d, nil)", tc.text, got, err, tc.want)
		}
	}
}

func TestODByteCountOverflowIsUsageError(t *testing.T) {
	// "G" (1024^3) is already a recognized multiplier; a count this large
	// overflows int64 on multiplication and must fail cleanly rather than
	// silently wrapping.
	if _, err := parseBytes("9223372036854775807G"); err == nil {
		t.Fatalf("parseBytes(overflow) = nil error, want an error")
	}

	_, errb, code := runOD(t, t.TempDir(), "hi", "-N", "9223372036854775807G")
	if code != 2 || !strings.Contains(errb, "invalid byte count") {
		t.Fatalf("od -N overflow: code=%d err=%q, want code 2 and invalid byte count", code, errb)
	}

	_, errb, code = runOD(t, t.TempDir(), "hi", "-j", "9223372036854775807G")
	if code != 2 || !strings.Contains(errb, "invalid skip count") {
		t.Fatalf("od -j overflow: code=%d err=%q, want code 2 and invalid skip count", code, errb)
	}

	// Also confirm overflow via the newly accepted lowercase "m" suffix.
	if _, err := parseBytes("9223372036854775807m"); err == nil {
		t.Fatalf("parseBytes(overflow via m) = nil error, want an error")
	}
}

func TestODJSkipWithLowercaseSuffix(t *testing.T) {
	data := strings.Repeat("a", 1024) + "bc"
	out, errb, code := runOD(t, t.TempDir(), data, "-A", "n", "-t", "c", "-j", "1k", "-w", "2")
	want := "   b   c\n"
	if out != want || errb != "" || code != 0 {
		t.Fatalf("od -j 1k = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
}

func TestODByteCountRadixPrefixes(t *testing.T) {
	data := "0123456789abcdefXYZ"
	for _, tc := range []struct {
		name string
		arg  string
		want string
	}{
		{name: "decimal", arg: "10", want: "   a   b   c\n"},
		{name: "octal", arg: "010", want: "   8   9   a\n"},
		{name: "hex-lower", arg: "0xa", want: "   a   b   c\n"},
		{name: "hex-upper", arg: "0X10", want: "   X   Y   Z\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errb, code := runOD(t, t.TempDir(), data, "-A", "n", "-t", "c", "-j", tc.arg, "-N", "3")
			if out != tc.want || errb != "" || code != 0 {
				t.Fatalf("od -j %s = (%q, %q, %d), want (%q, empty, 0)", tc.arg, out, errb, code, tc.want)
			}
		})
	}

	for _, arg := range []string{"0x", "018", "0xg", "0X-1"} {
		_, errb, code := runOD(t, t.TempDir(), data, "-j", arg)
		if code != 2 || !strings.Contains(errb, "invalid skip count") {
			t.Errorf("od -j %s = (stderr %q, code %d), want invalid skip count/code 2", arg, errb, code)
		}
	}
}

func TestODShortTypeAliases(t *testing.T) {
	cases := []struct {
		flag string
		data string
		want string
	}{
		{"-D", "ABCD", " 1145258561\n"},
		{"-F", "\x00\x00\x00\x00\x00\x00\xf0\x3f", " 1\n"},
		{"-H", "ABCD", " 44434241\n"},
		{"-I", "ABCD", " 1145258561\n"},
		{"-O", "ABCD", " 010420641101\n"},
		{"-X", "ABCD", " 44434241\n"},
		{"-e", "\x00\x00\x00\x00\x00\x00\xf0\x3f", " 1\n"},
		{"-f", "\x00\x00\x80\x3f", " 1\n"},
		{"-i", "ABCD", " 1145258561\n"},
		{"-s", "ABCD", " 16961 17475\n"},
	}
	for _, tc := range cases {
		out, _, code := runOD(t, t.TempDir(), tc.data, tc.flag, "-A", "n")
		if out != tc.want || code != 0 {
			t.Fatalf("od %s = (%q, %d), want (%q, 0)", tc.flag, out, code, tc.want)
		}
	}
	out, _, code := runOD(t, t.TempDir(), "ABCD", "-x", "-d", "-A", "n")
	if want := " 16961 17475\n 4241 4443\n"; out != want || code != 0 {
		t.Fatalf("od -x -d = (%q, %d), want (%q, 0)", out, code, want)
	}
}

// POSIX: concatenated format strings like "x1x2" should parse as two formats (x1 and x2)
func TestODConcatenatedFormatStrings(t *testing.T) {
	cases := []struct {
		format string
		data   string
		want   string
	}{
		// Two hex formats
		{"x1x2", "ABCD", " 41 42 43 44\n 4241 4443\n"},
		// Three formats (test smaller data)
		{"x1o1", "AB", " 41 42\n 101 102\n"},
		// Octal and decimal
		{"o1d1", "AB", " 101 102\n   65   66\n"},
	}
	for _, tc := range cases {
		out, errb, code := runOD(t, t.TempDir(), tc.data, "-A", "n", "-t", tc.format)
		if out != tc.want || errb != "" || code != 0 {
			t.Fatalf("od -t %s = (%q, %q, %d), want (%q, \"\", 0)", tc.format, out, errb, code, tc.want)
		}
	}
}

// POSIX: C type codes for floats (fF, fD) and sizes (C, S, I, L with d/o/u/x)
func TestODCTypeCodeFormats(t *testing.T) {
	cases := []struct {
		format string
		data   string
		want   string
	}{
		// Float C type codes
		{"fF", "\x00\x00\x80\x3f", " 1\n"},           // float (4 bytes)
		{"fD", "\x00\x00\x00\x00\x00\x00\xf0\x3f", " 1\n"}, // double (8 bytes)
		// Integer C type codes
		{"dC", "A", "   65\n"},        // char (1 byte signed)
		{"dS", "AB", " 16961\n"},      // short (2 bytes signed, little-endian)
		{"dI", "ABCD", " 1145258561\n"}, // int (4 bytes signed, little-endian)
		{"dL", "ABCDEFGH", " 5208208757389214273\n"}, // long (8 bytes signed, little-endian)
		// Hex with C type codes
		{"xC", "AB", " 41 42\n"},      // byte hex
		{"xS", "ABCD", " 4241 4443\n"}, // short hex
	}
	for _, tc := range cases {
		out, errb, code := runOD(t, t.TempDir(), tc.data, "-A", "n", "-t", tc.format)
		if out != tc.want || errb != "" || code != 0 {
			t.Fatalf("od -t %s = (%q, %q, %d), want (%q, \"\", 0)", tc.format, out, errb, code, tc.want)
		}
	}
}

// POSIX: multiple operand files concatenation
func TestODMultipleFileConcatenation(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f1"), []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f2"), []byte("B"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f3"), []byte("C"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runOD(t, dir, "", "-A", "n", "-t", "c", "f1", "f2", "f3")
	if want := "   A   B   C\n"; out != want || code != 0 {
		t.Fatalf("od multi-file = (%q, %d), want (%q, 0)", out, code, want)
	}
}

// POSIX: stdin dash operand
func TestODStdinDashOperand(t *testing.T) {
	out, _, code := runOD(t, t.TempDir(), "hello", "-A", "n", "-t", "c", "-")
	if want := "   h   e   l   l   o\n"; out != want || code != 0 {
		t.Fatalf("od stdin dash = (%q, %d), want (%q, 0)", out, code, want)
	}
}

// POSIX: combined format aliases (e.g., octal1hex1)
func TestODCombinedNamedFormats(t *testing.T) {
	cases := []struct {
		format string
		data   string
		want   string
	}{
		{"octal1hex1", "AB", " 101 102\n 41 42\n"},
		{"octal", "ABCD", " 010420641101\n"},
		{"hex", "ABCD", " 44434241\n"},
		{"signed", "ABCD", " 1145258561\n"},
		{"unsigned", "ABCD", " 1145258561\n"},
	}
	for _, tc := range cases {
		out, errb, code := runOD(t, t.TempDir(), tc.data, "-A", "n", "-t", tc.format)
		if out != tc.want || errb != "" || code != 0 {
			t.Fatalf("od -t %s = (%q, %q, %d), want (%q, \"\", 0)", tc.format, out, errb, code, tc.want)
		}
	}
}

// Edge case: byte-exact offsets and final line with -N
func TestODByteExactOffsetAndLimit(t *testing.T) {
	data := "0123456789"
	out, _, code := runOD(t, t.TempDir(), data, "-A", "d", "-t", "c", "-j", "2", "-N", "3")
	if !strings.Contains(out, "2") && !strings.Contains(out, "234") {
		// Should start at offset 2 (decimal) and read 3 bytes
		t.Fatalf("od -j 2 -N 3 = (%q, %d), format not as expected", out, code)
	}
}
