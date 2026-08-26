package grepcmd

import (
	"strings"
	"testing"
)

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

// TestGrepLocaleFixedStringHighByteIsByteExact confirms -F preserves the
// original matching byte extent and stays case-sensitive without -i.
func TestGrepLocaleFixedStringHighByteIsByteExact(t *testing.T) {
	de := []string{"LC_ALL=de_DE.iso88591"}
	if out, errOut, code := runGrepEnv(t, "", eAcuteUpper+"\n", de, "-F", eAcuteUpper); out != eAcuteUpper+"\n" || errOut != "" || code != 0 {
		t.Fatalf("grep -F match = (%q, %q, %d)", out, errOut, code)
	}
	if out, errOut, code := runGrepEnv(t, "", eAcuteLower+"\n", de, "-F", eAcuteUpper); out != "" || errOut != "" || code != 1 {
		t.Fatalf("grep -F case-sensitive = (%q, %q, %d), want no match", out, errOut, code)
	}
}

func TestGrepLocaleCompleteLatin1ClassesAndGermanRange(t *testing.T) {
	de := []string{"LC_ALL=de_DE.iso88591"}
	for _, tc := range []struct {
		name    string
		pattern string
		input   byte
	}{
		{"feminine-ordinal-alpha", "[[:alpha:]]", 0xaa},
		{"micro-sign-lower", "[[:lower:]]", 0xb5},
		{"inverted-exclamation-punct", "[[:punct:]]", 0xa1},
		{"multiplication-sign-punct", "[[:punct:]]", 0xd7},
		{"c1-control", "[[:cntrl:]]", 0x85},
		{"nbsp-print", "[[:print:]]", 0xa0},
		{"german-a-through-b-range", "[a-b]", 0xe4},
		{"german-collating-range", "[[.a.]-[.b.]]", 0xe4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := string([]byte{tc.input, '\n'})
			out, errOut, code := runGrepEnv(t, "", input, de, "-o", tc.pattern)
			want := string([]byte{tc.input, '\n'})
			if code != 0 || errOut != "" || out != want {
				t.Fatalf("grep -o %q byte %#x = (%x, %q, %d), want (%x, empty, 0)", tc.pattern, tc.input, out, errOut, code, want)
			}
		})
	}
	// NBSP is printable but not graphical in ISO-8859-1.
	if out, errOut, code := runGrepEnv(t, "", "\xa0\n", de, "[[:graph:]]"); code != 1 || out != "" || errOut != "" {
		t.Fatalf("NBSP graph = (%x, %q, %d), want no match", out, errOut, code)
	}
	// LC_COLLATE remains effective when LC_CTYPE independently selects C.
	out, errOut, code := runGrepEnv(t, "", "\xe4\n",
		[]string{"LANG=POSIX", "LC_CTYPE=C", "LC_COLLATE=de_DE.iso88591"}, "-o", "[a-b]")
	if code != 0 || errOut != "" || out != "\xe4\n" {
		t.Fatalf("independent LC_COLLATE range = (%x, %q, %d), want e4 newline", out, errOut, code)
	}
}

func TestGrepLocaleAllLatin1ClassMembership(t *testing.T) {
	isUpper := func(b byte) bool { return b >= 'A' && b <= 'Z' || b >= 0xc0 && b <= 0xd6 || b >= 0xd8 && b <= 0xde }
	isLower := func(b byte) bool {
		return b >= 'a' && b <= 'z' || b == 0xaa || b == 0xb5 || b == 0xba || b >= 0xdf && b <= 0xf6 || b >= 0xf8
	}
	isAlpha := func(b byte) bool { return isUpper(b) || isLower(b) }
	isDigit := func(b byte) bool { return b >= '0' && b <= '9' }
	isAlnum := func(b byte) bool { return isAlpha(b) || isDigit(b) }
	isBlank := func(b byte) bool { return b == ' ' || b == '\t' }
	isCntrl := func(b byte) bool { return b < 0x20 || b >= 0x7f && b <= 0x9f }
	isGraph := func(b byte) bool { return b >= 0x21 && b <= 0x7e || b >= 0xa1 }
	isPrint := func(b byte) bool { return b >= 0x20 && b <= 0x7e || b >= 0xa0 }
	isSpace := func(b byte) bool { return b == ' ' || b >= '\t' && b <= '\r' }
	isXDigit := func(b byte) bool { return isDigit(b) || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F' }
	classes := []struct {
		name string
		want func(byte) bool
	}{
		{"alpha", isAlpha}, {"alnum", isAlnum}, {"blank", isBlank},
		{"cntrl", isCntrl}, {"digit", isDigit}, {"graph", isGraph},
		{"lower", isLower}, {"print", isPrint},
		{"punct", func(b byte) bool { return isGraph(b) && !isAlnum(b) }},
		{"space", isSpace}, {"upper", isUpper}, {"xdigit", isXDigit},
	}
	locale := grepLocale{ctypeGerman: true}
	for _, class := range classes {
		t.Run(class.name, func(t *testing.T) {
			pattern := locale.rewritePattern("[[:" + class.name + ":]]")
			re, err := compilePattern([]string{pattern}, false, false, false, false, false)
			if err != nil {
				t.Fatal(err)
			}
			matcher := localeMatcher{inner: re}
			for value := 0; value < 256; value++ {
				got := matcher.MatchString(string([]byte{byte(value)}))
				if got != class.want(byte(value)) {
					t.Errorf("byte %#02x match=%v, want %v", value, got, class.want(byte(value)))
				}
			}
		})
	}
}

func TestGrepLocaleRejectsUnreviewedCodesetsBeforeInput(t *testing.T) {
	for _, localeName := range []string{"de_DE.UTF-8", "de_DE.ISO-8859-15", "de_DE"} {
		t.Run(localeName, func(t *testing.T) {
			out, errOut, code := runGrepEnv(t, "", "a\n", []string{"LC_ALL=" + localeName}, "a")
			if code != 2 || out != "" || !strings.Contains(errOut, "unsupported locale") {
				t.Fatalf("LC_ALL=%s = (%x, %q, %d), want fail-closed exit 2", localeName, out, errOut, code)
			}
		})
	}
}
