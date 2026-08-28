package findcmd

// Locale-category regressions for the Profile D find residuals.
//
// find resolves LC_CTYPE, LC_COLLATE and LC_MESSAGES itself, and POSIX.1
// Issue 7 XBD 7.3 gives each a disjoint job. For -name/-iname/-path two of
// them matter:
//
//	LC_CTYPE   the character model — how the pattern and the filename
//	           divide into characters, what [:class:] denotes, what
//	           -iname folds together.
//	LC_COLLATE the collation — what [=equivalence=] and
//	           [.collating-symbol.] denote, and how a range orders.
//
// The residual was that find had no character model at all: under the
// certification shell's LC_ALL=C.UTF-8 it matched '?' against one BYTE of
// a multi-byte character, where GNU find matches one character.
//
// The hazard in repairing that — and the reason every case below names
// BOTH categories — is deriving one decision from both variables. Reading
// "either category has a UTF-8 codeset" as permission to decode characters
// is wrong in exactly the two environments a single-locale test never
// reaches, and the expectations here were taken from GNU find run in those
// environments:
//
//	LC_CTYPE=C.UTF-8 LC_COLLATE=C        '?' matches one character
//	LC_CTYPE=C       LC_COLLATE=C.UTF-8  '?' matches one byte

import (
	"strings"
	"testing"
)

// utf8Tree stages one file per character class the cases below discriminate:
// a one-byte name, a two-byte name and a three-byte name.
func utf8Tree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"e", "é", "世", "ab"} {
		writeFile(t, dir, name, "")
	}
	return dir
}

// TestFindCTypeSelectsTheCharacterModel is the Profile D residual itself,
// in the exact environment a certification shell stages.
func TestFindCTypeSelectsTheCharacterModel(t *testing.T) {
	dir := utf8Tree(t)
	cases := []struct {
		args []string
		want string
	}{
		// '?' is one character, so it spans a whole UTF-8 sequence.
		{[]string{"-name", "?"}, "./e\n./é\n./世\n"},
		{[]string{"-name", "??"}, "./ab\n"},
		{[]string{"-name", "e?"}, ""},
		// The classes are the Unicode sets, not the C locale's ASCII sets.
		{[]string{"-name", "[[:alpha:]]"}, "./e\n./é\n./世\n"},
		{[]string{"-name", "[[:digit:]]"}, ""},
		// A range still orders by character value, so it stays ASCII here.
		{[]string{"-name", "[a-z]"}, "./e\n"},
		// -iname folds with the full Unicode case mapping.
		{[]string{"-iname", "É"}, "./é\n"},
		// Inside a class, -iname matches when EITHER case satisfies it —
		// find's existing rule (TestFindINameFoldsASCIIOnly), carried over
		// to the Unicode case pairs. A caseless character such as 世 has
		// no case that is upper, so it does not match.
		{[]string{"-iname", "[[:upper:]]"}, "./e\n./é\n"},
		// '*' advances by characters, and a trailing one must still land
		// on a character boundary.
		{[]string{"-name", "*é"}, "./é\n"},
		{[]string{"-name", "?世"}, ""},
	}
	for _, env := range [][]string{
		{"LC_ALL=C.UTF-8"},
		{"LANG=C.UTF-8"},
		{"LC_CTYPE=POSIX.utf8"},
		{"LC_ALL=C.UTF-8@euro"},
	} {
		for _, c := range cases {
			args := append([]string{".", "-maxdepth", "1", "-type", "f"}, c.args...)
			out, errb, code := runFindEnv(t, dir, env, args...)
			if out != c.want || errb != "" || code != 0 {
				t.Errorf("env %v: find %v = (%q, %q, %d), want %q",
					env, c.args, out, errb, code, c.want)
			}
		}
	}
}

// TestFindLocaleCategoriesDoNotSubstituteForEachOther is the mixed-locale
// regression: a UTF-8 codeset in one category must not decide the other
// category's question. Both expectations were confirmed against GNU find.
func TestFindLocaleCategoriesDoNotSubstituteForEachOther(t *testing.T) {
	dir := utf8Tree(t)
	const chars = "./e\n./é\n./世\n"
	const bytes = "./e\n"
	tests := []struct {
		name string
		env  []string
		want string
	}{
		{"utf-8 ctype, C collate", []string{"LC_CTYPE=C.UTF-8", "LC_COLLATE=C"}, chars},
		// The case the rejected repair got wrong: a UTF-8 LC_COLLATE
		// decides collation, and collation is not a character model.
		{"C ctype, utf-8 collate", []string{"LC_CTYPE=C", "LC_COLLATE=C.UTF-8"}, bytes},
		// The same, reached through the other precedence levels.
		{"C ctype outranks a utf-8 LANG", []string{"LC_CTYPE=C", "LANG=C.UTF-8"}, bytes},
		{"utf-8 collate and messages only", []string{"LC_COLLATE=C.UTF-8", "LC_MESSAGES=C.UTF-8"}, bytes},
		// LC_ALL outranks every category, in both directions.
		{"C LC_ALL over a utf-8 ctype", []string{"LC_ALL=C", "LC_CTYPE=C.UTF-8"}, bytes},
		{"utf-8 LC_ALL over a C ctype", []string{"LC_ALL=C.UTF-8", "LC_CTYPE=C"}, chars},
		// An empty value does not shadow the next level down.
		{"empty ctype falls through to LANG", []string{"LC_CTYPE=", "LANG=C.UTF-8"}, chars},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, errb, code := runFindEnv(t, dir, tc.env,
				".", "-maxdepth", "1", "-type", "f", "-name", "?")
			if out != tc.want || errb != "" || code != 0 {
				t.Errorf("env %v: find . -name ? = (%q, %q, %d), want %q",
					tc.env, out, errb, code, tc.want)
			}
		})
	}
}

// TestFindCollateCategoryOwnsBracketCollation is the other direction: with
// the character model held fixed at UTF-8, whether [[=a=]] has members is
// decided by LC_COLLATE alone. The carried de_DE collation groups the
// umlauts with their base letters; the C collation gives no character an
// equivalent (POSIX XBD 7.3.2).
func TestFindCollateCategoryOwnsBracketCollation(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "ä", "")
	writeFile(t, dir, "a", "")

	tests := []struct {
		name string
		env  []string
		want string
	}{
		{"C collation has no equivalents", []string{"LC_CTYPE=C.UTF-8", "LC_COLLATE=C"}, "./a\n"},
		{"C.UTF-8 collation has none either", []string{"LC_CTYPE=C.UTF-8", "LC_COLLATE=C.UTF-8"}, "./a\n"},
		{"carried de_DE collation supplies them", []string{"LC_CTYPE=C.UTF-8", "LC_COLLATE=de_DE.iso88591"}, "./a\n./ä\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, errb, code := runFindEnv(t, dir, tc.env,
				".", "-maxdepth", "1", "-type", "f", "-name", "[[=a=]]")
			if out != tc.want || errb != "" || code != 0 {
				t.Errorf("env %v: find . -name [[=a=]] = (%q, %q, %d), want %q",
					tc.env, out, errb, code, tc.want)
			}
		})
	}
}

// TestFindUncarriedLocalesKeepTheByteModel pins the boundary of the carried
// inventory. A UTF-8 codeset alone is not a character model: only the
// C/POSIX aliases, whose semantics this repository actually carries, select
// one. Everything else keeps the byte model it already had rather than
// being approximated by a nearby locale.
func TestFindUncarriedLocalesKeepTheByteModel(t *testing.T) {
	dir := utf8Tree(t)
	for _, env := range [][]string{
		{"LC_ALL=en_US.UTF-8"},
		{"LC_CTYPE=fr_FR.UTF-8"},
		{"LC_CTYPE=de_DE.UTF-8"},
		{"LC_CTYPE=C.ISO-8859-1"},
		{"LC_CTYPE=CC.UTF-8"},
		{"LC_CTYPE=c.utf-8"}, // the locale name is not case-folded
	} {
		out, errb, code := runFindEnv(t, dir, env,
			".", "-maxdepth", "1", "-type", "f", "-name", "?")
		if out != "./e\n" || errb != "" || code != 0 {
			t.Errorf("env %v: find . -name ? = (%q, %q, %d), want the byte model",
				env, out, errb, code)
		}
	}
}

// TestFnmatchUTF8HandlesInvalidSequences covers what no directory walk can
// portably stage: a filename that is not valid UTF-8 even though LC_CTYPE
// says the codeset is (filesystems that reject such names — Darwin/APFS —
// make this unreachable end to end). Each undecodable byte is its own
// character, so it advances the scan by one byte and matches only the same
// raw byte — never a class, a range or a case fold.
func TestFnmatchUTF8HandlesInvalidSequences(t *testing.T) {
	utf8Locale := findLocale{ctypeUTF8: true}
	const raw = "\xe9"  // a lone Latin-1 e-acute: not a UTF-8 sequence
	const encoded = "é" // U+00E9 encoded as UTF-8

	tests := []struct {
		pattern, name string
		fold          bool
		want          bool
	}{
		{"?", raw, false, true},
		{raw, raw, false, true},
		{encoded, raw, false, false},
		{raw, encoded, false, false},
		{"[[:alpha:]]", raw, false, false},
		{"[a-\xff]", raw, false, false},
		{"[[:print:]]", raw, false, false},
		{raw, raw, true, true},
		{"\xe9\xe9", raw + raw, false, true},
		{"?", raw + raw, false, false},
		{"*" + raw, encoded + raw, false, true},
	}
	for _, tc := range tests {
		if got := fnmatchLocale(tc.pattern, tc.name, tc.fold, utf8Locale); got != tc.want {
			t.Errorf("fnmatchLocale(%q, %q, fold=%v, C.UTF-8) = %v, want %v",
				tc.pattern, tc.name, tc.fold, got, tc.want)
		}
	}
}

// TestFindLocaleFromEnvKeepsCategoriesSeparate asserts the resolution
// itself, so a future edit that folds two categories into one flag fails
// here — where the reason is legible — and not only in a walk.
func TestFindLocaleFromEnvKeepsCategoriesSeparate(t *testing.T) {
	tests := []struct {
		name string
		env  []string
		want findLocale
	}{
		{"unset", nil, findLocale{}},
		{"utf-8 ctype only", []string{"LC_CTYPE=C.UTF-8"}, findLocale{ctypeUTF8: true}},
		{"utf-8 collate only", []string{"LC_COLLATE=C.UTF-8"}, findLocale{}},
		{"german collate only", []string{"LC_COLLATE=de_DE.iso88591"}, findLocale{collateGerman: true}},
		{"german ctype only", []string{"LC_CTYPE=de_DE.iso88591"}, findLocale{ctypeGerman: true}},
		{"german messages only", []string{"LC_MESSAGES=de_DE.iso88591"}, findLocale{messagesGerman: true}},
		{
			"one variable per category",
			[]string{"LC_CTYPE=C.UTF-8", "LC_COLLATE=de_DE.iso88591", "LC_MESSAGES=C"},
			findLocale{ctypeUTF8: true, collateGerman: true},
		},
		{
			"LC_ALL outranks all three",
			[]string{"LC_ALL=C.UTF-8", "LC_CTYPE=de_DE.iso88591", "LC_COLLATE=de_DE.iso88591", "LC_MESSAGES=de_DE.iso88591"},
			findLocale{ctypeUTF8: true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := findLocaleFromEnv(tc.env); got != tc.want {
				t.Errorf("findLocaleFromEnv(%v) = %+v, want %+v", tc.env, got, tc.want)
			}
		})
	}
}

// TestFindUTF8CTypeKeepsPOSIXDigitAndXdigitASCII pins the two classes POSIX
// fixes by enumeration rather than by category: XBD 7.3.1 defines digit as
// "the digits 0 through 9" in every locale, and xdigit on top of it. A
// Unicode-category mapping would quietly widen both.
func TestFindUTF8CTypeKeepsPOSIXDigitAndXdigitASCII(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"5", "٣", "e"} {
		writeFile(t, dir, name, "")
	}
	env := []string{"LC_ALL=C.UTF-8"}
	for _, tc := range []struct{ pat, want string }{
		{"[[:digit:]]", "./5\n"},
		{"[[:xdigit:]]", "./5\n./e\n"},
		{"[[:alpha:]]", "./e\n"},
	} {
		out, errb, code := runFindEnv(t, dir, env,
			".", "-maxdepth", "1", "-type", "f", "-name", tc.pat)
		if out != tc.want || errb != "" || code != 0 {
			t.Errorf("find . -name %s = (%q, %q, %d), want %q", tc.pat, out, errb, code, tc.want)
		}
	}
}

// TestFindUTF8CTypeDoesNotChangeOtherPredicates keeps the character model
// scoped to pattern matching: it is the model for -name/-iname/-path, not a
// licence to reinterpret unrelated operands.
func TestFindUTF8CTypeDoesNotChangeOtherPredicates(t *testing.T) {
	dir := utf8Tree(t)
	env := []string{"LC_ALL=C.UTF-8"}
	out, errb, code := runFindEnv(t, dir, env, ".", "-maxdepth", "1", "-type", "f", "-size", "0")
	if code != 0 || errb != "" {
		t.Fatalf("-size 0 = (%q, %q, %d), want success", out, errb, code)
	}
	if n := strings.Count(out, "\n"); n != 4 {
		t.Errorf("-size 0 printed %d paths, want 4: %q", n, out)
	}
}
