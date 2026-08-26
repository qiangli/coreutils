package grepcmd

import "testing"

// The bounded de_DE.ISO-8859-1 certification locale is single-byte, so these
// fixtures build subject and pattern text from raw Latin-1 bytes:
//
//	0xC9 É   0xE9 é   0xE4 ä   0xC4 Ä   0xDC Ü
const (
	eAcuteUpper = "\xc9" // É
	eAcuteLower = "\xe9" // é
	aUmlaut     = "\xe4" // ä
	aUmlautUp   = "\xc4" // Ä
)

// TestGrepLocaleCTypeDiscriminatesClassesAndCase pins POSIX grep's LC_CTYPE and
// LC_COLLATE product for the non-C certification locale. Every row runs under
// de_DE.ISO-8859-1, feeds one accented Latin-1 line, and asserts the exact
// byte-preserved result, so a change that silently drops high-byte handling —
// or reintroduces the "invalid UTF-8" abort a raw high-byte pattern used to
// cause — fails loudly.
func TestGrepLocaleCTypeDiscriminatesClassesAndCase(t *testing.T) {
	de := []string{"LC_ALL=de_DE.iso88591"}
	cases := []struct {
		name  string
		args  []string
		input string
		want  string // "" means no match (exit 1)
	}{
		// Literal high-byte pattern: a bare É must denote the Latin-1
		// character and match, not abort as invalid UTF-8. Case is honored.
		{"literal-high-byte-matches", []string{eAcuteUpper}, eAcuteUpper + "\n", eAcuteUpper + "\n"},
		{"literal-high-byte-case-sensitive", []string{eAcuteUpper}, eAcuteLower + "\n", ""},
		// -i folds accented letters across ISO-8859-1 case pairs.
		{"ignore-case-folds-high-byte", []string{"-i", eAcuteUpper}, eAcuteLower + "\n", eAcuteLower + "\n"},
		{"ignore-case-folds-umlaut", []string{"-i", aUmlautUp}, aUmlaut + "\n", aUmlaut + "\n"},
		// [:upper:] / [:lower:] discriminate the high-byte case boundary.
		{"upper-class-matches-upper", []string{"[[:upper:]]"}, eAcuteUpper + "\n", eAcuteUpper + "\n"},
		{"upper-class-rejects-lower", []string{"[[:upper:]]"}, eAcuteLower + "\n", ""},
		{"lower-class-matches-lower", []string{"[[:lower:]]"}, eAcuteLower + "\n", eAcuteLower + "\n"},
		{"lower-class-rejects-upper", []string{"[[:lower:]]"}, eAcuteUpper + "\n", ""},
		// [:alpha:] admits both accented cases; [:digit:] admits neither.
		{"alpha-class-admits-upper", []string{"[[:alpha:]]"}, eAcuteUpper + "\n", eAcuteUpper + "\n"},
		{"alpha-class-admits-lower", []string{"[[:alpha:]]"}, eAcuteLower + "\n", eAcuteLower + "\n"},
		{"digit-class-rejects-letter", []string{"[[:digit:]]"}, aUmlaut + "\n", ""},
		// Equivalence class groups the umlaut with its base letter (LC_COLLATE).
		{"equivalence-class-matches-umlaut", []string{"[[=a=]]"}, aUmlaut + "\n", aUmlaut + "\n"},
		{"equivalence-class-rejects-other", []string{"[[=a=]]"}, eAcuteLower + "\n", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runGrepEnv(t, "", tc.input, de, tc.args...)
			wantCode := 1
			if tc.want != "" {
				wantCode = 0
			}
			if out != tc.want || errOut != "" || code != wantCode {
				t.Fatalf("grep %v = (%q, %q, %d), want (%q, '', %d)", tc.args, out, errOut, code, tc.want, wantCode)
			}
		})
	}
}

// TestGrepLocaleOnlyMatchingKeepsByteOffsets proves the ISO-8859-1 decode used
// for classification does not corrupt -o output: the reported extent is the
// original single-byte run, not its multi-byte UTF-8 expansion.
func TestGrepLocaleOnlyMatchingKeepsByteOffsets(t *testing.T) {
	de := []string{"LC_ALL=de_DE.iso88591"}
	input := "x" + aUmlaut + eAcuteLower + "y\n" // x ä é y
	out, errOut, code := runGrepEnv(t, "", input, de, "-o", "[[:alpha:]][[:alpha:]]*")
	want := "x" + aUmlaut + eAcuteLower + "y\n"
	if out != want || errOut != "" || code != 0 {
		t.Fatalf("grep -o = (%q, %q, %d), want (%q, '', 0)", out, errOut, code, want)
	}
}

// TestGrepLocaleFixedStringHighByteIsByteExact confirms -F (which is not
// decoded) still matches an accented literal byte-for-byte, and does not fold
// case the way the regex -i path does.
func TestGrepLocaleFixedStringHighByteIsByteExact(t *testing.T) {
	de := []string{"LC_ALL=de_DE.iso88591"}
	if out, errOut, code := runGrepEnv(t, "", eAcuteUpper+"\n", de, "-F", eAcuteUpper); out != eAcuteUpper+"\n" || errOut != "" || code != 0 {
		t.Fatalf("grep -F match = (%q, %q, %d)", out, errOut, code)
	}
	if out, errOut, code := runGrepEnv(t, "", eAcuteLower+"\n", de, "-F", eAcuteUpper); out != "" || errOut != "" || code != 1 {
		t.Fatalf("grep -F case-sensitive = (%q, %q, %d), want no match", out, errOut, code)
	}
}
