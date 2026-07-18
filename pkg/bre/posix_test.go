package bre

import (
	"strings"
	"testing"
)

// The behaviors pinned here are the POSIX-specified ones (XBD 9.3 Basic
// Regular Expressions, XCU sed), each of which this package previously got
// wrong. Every expectation is the byte-for-byte answer the spec requires and
// GNU grep/sed produce.

// POSIX XBD 9.1: "the longest of the leftmost matches" — the overall match is
// leftmost-longest, not the leftmost-first match a backtracking/RE2 engine
// finds by default. The difference is observable exactly when alternation can
// match at one offset with different lengths.
func TestPOSIXLeftmostLongest(t *testing.T) {
	cases := []struct {
		pattern      string
		in           string
		first        []int // leftmost-first (the default)
		posixLongest []int
	}{
		// The classic: the shorter alternative is written first.
		{`a\|ab`, "ab", []int{0, 1}, []int{0, 2}},
		{`ab\|a`, "ab", []int{0, 2}, []int{0, 2}}, // order must not matter
		{`foo\|foobar`, "foobarx", []int{0, 3}, []int{0, 6}},
		// Longest applies among matches at the *leftmost* offset only: the
		// match at 1 is longer, but the one at 0 starts earlier and wins.
		{`b\|aa`, "xaab", []int{1, 3}, []int{1, 3}},
		// Alternation inside a group, with a back-reference (the backtracking
		// engine, not RE2).
		{`\(a\|ab\)\1`, "abab", []int{0, 4}, []int{0, 4}},
		{`\(a\|ab\)c\1`, "abcab", []int{0, 5}, []int{0, 5}},
	}
	for _, c := range cases {
		re, err := Compile(c.pattern)
		if err != nil {
			t.Fatalf("Compile(%q): %v", c.pattern, err)
		}
		if got := re.FindStringIndex(c.in); !sameInts(got, c.first) {
			t.Errorf("default %q on %q = %v, want leftmost-first %v", c.pattern, c.in, got, c.first)
		}

		re, err = Compile(c.pattern)
		if err != nil {
			t.Fatalf("Compile(%q): %v", c.pattern, err)
		}
		re.Longest()
		if got := re.FindStringIndex(c.in); !sameInts(got, c.posixLongest) {
			t.Errorf("Longest %q on %q = %v, want POSIX leftmost-longest %v", c.pattern, c.in, got, c.posixLongest)
		}
	}
}

// Longest must not disturb which offsets match, only which extent is reported.
func TestPOSIXLongestPreservesMatchExistence(t *testing.T) {
	for _, c := range []struct {
		pattern string
		in      string
		want    bool
	}{
		{`a\|ab`, "zzz", false},
		{`a\|ab`, "zab", true},
		{`\(a*\)b\1`, "aaa", false},
		{`\(a*\)b\1`, "aaba", true},
	} {
		re, err := Compile(c.pattern)
		if err != nil {
			t.Fatalf("Compile(%q): %v", c.pattern, err)
		}
		re.Longest()
		if got := re.MatchString(c.in); got != c.want {
			t.Errorf("Longest %q.MatchString(%q) = %v, want %v", c.pattern, c.in, got, c.want)
		}
	}
}

// POSIX XBD 9.3.6: a back-reference "\n" matches "the same string as was
// matched by" the n-th subexpression. When that subexpression matched the empty
// string, the string it matched is "" — so the back-reference matches the empty
// string, and \(a*\)b\1 matches "b". This engine used to reject an empty
// capture outright, so every such pattern failed to match.
func TestPOSIXBackrefEmptyGroup(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		in      string
		want    []int
	}{
		{"empty capture, backref matches empty", `\(a*\)b\1`, "b", []int{0, 1}},
		{"empty capture mid-subject", `\(x*\)y\1z`, "yz", []int{0, 2}},
		{"non-empty capture still required to repeat", `\(a*\)x\1`, "aaxaa", []int{0, 5}},
		// The leftmost match is "aba" at offset 1 (group 1 = "a"), NOT the
		// empty-capture match "b" at offset 2. Both are matches; POSIX takes the
		// leftmost. An unsound "skip past the leading repeat" optimization used
		// to skip offset 1 entirely and report [2,3].
		{"leftmost wins over a later empty-capture match", `\(a*\)b\1`, "aaba", []int{1, 4}},
		// A group that never participated is a different state from one that
		// participated and matched empty. POSIX leaves the former undefined; we
		// fail the back-reference. Pinned so the fix above cannot drift into
		// treating "never ran" as "matched empty".
		{"group that never participated", `\(a\)\{0\}b\1`, "b", nil},
	}
	for _, c := range cases {
		re, err := Compile(c.pattern)
		if err != nil {
			t.Fatalf("%s: Compile(%q): %v", c.name, c.pattern, err)
		}
		if got := re.FindStringIndex(c.in); !sameInts(got, c.want) {
			t.Errorf("%s: %q on %q = %v, want %v", c.name, c.pattern, c.in, got, c.want)
		}
	}
}

// POSIX requires interval bounds through RE_DUP_MAX (255). GNU grep documents
// 255 as the portable limit (while extending its own implementation to 32767),
// and GNU bash 5.3 rejects 256 as an invalid regular expression. Keep the
// package at the portable ceiling. Leading zeroes are valid decimal spelling;
// {,m} remains the documented GNU extension supported by this package.
func TestPOSIXIntervalBounds(t *testing.T) {
	valid := []struct {
		pattern string
		in      string
	}{
		{`^a\{255\}$`, strings.Repeat("a", 255)},
		{`^a\{0002\}$`, "aa"},
		{`^a\{0002,0003\}$`, "aaa"},
		{`^a\{,3\}$`, "aaa"},
		{`^a\{2,\}$`, "aa"},
	}
	for _, c := range valid {
		re, err := Compile(c.pattern)
		if err != nil {
			t.Errorf("Compile(%q): %v", c.pattern, err)
			continue
		}
		if !re.MatchString(c.in) {
			t.Errorf("%q did not match %q", c.pattern, c.in)
		}
	}

	for _, pattern := range []string{
		`a\{256\}`,
		`a\{1,256\}`,
		`a\{999999999999999999999999999999\}`,
		`a\{3,2\}`,
	} {
		if _, err := Compile(pattern); err == nil {
			t.Errorf("Compile(%q) succeeded, want invalid interval error", pattern)
		}
	}
}

func TestPOSIXBracketExpressions(t *testing.T) {
	classes := map[string]string{
		"alnum": "a", "alpha": "a", "blank": " ", "cntrl": "\t",
		"digit": "7", "graph": "!", "lower": "a", "print": " ",
		"punct": "!", "space": "\n", "upper": "A", "xdigit": "f",
	}
	for class, in := range classes {
		pattern := "[[:" + class + ":]]"
		re, err := Compile(pattern)
		if err != nil {
			t.Errorf("Compile(%q): %v", pattern, err)
			continue
		}
		if !re.MatchString(in) {
			t.Errorf("%q did not match %q", pattern, in)
		}
	}

	for _, translate := range []struct {
		name string
		fn   func(string) (string, error)
	}{
		{"BRE", ToGo},
		{"ERE", ToGoERE},
	} {
		for _, pattern := range []string{`[[:bogus:]]`, `[z-a]`} {
			if _, err := translate.fn(pattern); err == nil {
				t.Errorf("%s translation of %q succeeded, want bracket expression error", translate.name, pattern)
			}
		}
	}

	positions := []struct {
		pattern, matches, notMatches string
	}{
		{`[]a]`, "]", "b"},  // ] is literal when it is the first member.
		{`[^]a]`, "b", "]"}, // The same rule applies after leading ^.
		{`[-a]`, "-", "b"},  // - is literal as the first member.
		{`[a-]`, "-", "b"},  // - is literal as the last member.
		{`[a^]`, "^", "b"},  // ^ is literal away from the first position.
		{`[^^]`, "a", "^"},  // ^ is negation only in the first position.
	}
	for _, c := range positions {
		re, err := Compile(c.pattern)
		if err != nil {
			t.Errorf("Compile(%q): %v", c.pattern, err)
			continue
		}
		if !re.MatchString(c.matches) {
			t.Errorf("%q did not match %q", c.pattern, c.matches)
		}
		if re.MatchString(c.notMatches) {
			t.Errorf("%q unexpectedly matched %q", c.pattern, c.notMatches)
		}
	}
}

func TestPOSIXCollatingSymbolsCLocale(t *testing.T) {
	cases := []struct {
		pattern, matches, notMatches string
	}{
		{`[[.a.]]`, "a", "b"},
		{`[[=a=]]`, "a", "b"},
		{`[^[=a=]]`, "b", "a"},
		{`[[.-.]]`, "-", "a"},
		{`[[.].]]`, "]", "a"},
		{`[[.^.]]`, "^", "a"},
		{`[[.a.]-c]`, "b", "z"},
		// POSIX's example: ] is the first literal member and the collating
		// symbol '-' is the starting point of the range through '0'.
		{`[][.-.]-0]`, "/", "1"},
	}
	for _, c := range cases {
		re, err := Compile(c.pattern)
		if err != nil {
			t.Errorf("Compile(%q): %v", c.pattern, err)
			continue
		}
		if !re.MatchString(c.matches) {
			t.Errorf("%q did not match %q", c.pattern, c.matches)
		}
		if re.MatchString(c.notMatches) {
			t.Errorf("%q unexpectedly matched %q", c.pattern, c.notMatches)
		}
	}
}

func TestPOSIXInvalidCollatingSymbolsCLocale(t *testing.T) {
	patterns := []string{
		`[[.ab.]]`, // no multi-character collating elements in the C locale
		`[[=ab=]]`,
		`[[..]]`,
		`[[==]]`,
		`[[.a]`,
		`[[=a]`,
	}
	for _, translate := range []struct {
		name string
		fn   func(string) (string, error)
	}{
		{"BRE", ToGo},
		{"ERE", ToGoERE},
	} {
		for _, pattern := range patterns {
			if _, err := translate.fn(pattern); err == nil {
				t.Errorf("%s translation of %q succeeded, want invalid collating element error", translate.name, pattern)
			}
		}
	}
}

func TestPOSIXBREEREOperatorDistinction(t *testing.T) {
	translations := []struct {
		name, pattern, want string
		fn                  func(string) (string, error)
	}{
		{"BRE literals", `a+?{2}(b)|c`, `a\+\?\{2\}\(b\)\|c`, ToGo},
		{"BRE operators", `a\+\?\{2\}\(b\|c\)`, `a+?{2}(b|c)`, ToGo},
		{"ERE operators", `a+?{2}(b)|c`, `a+?{2}(b)|c`, ToGoERE},
		{"ERE escaped parens", `\(a\)`, `\(a\)`, ToGoERE},
	}
	for _, c := range translations {
		got, err := c.fn(c.pattern)
		if err != nil {
			t.Errorf("%s: translate %q: %v", c.name, c.pattern, err)
		} else if got != c.want {
			t.Errorf("%s: translate %q = %q, want %q", c.name, c.pattern, got, c.want)
		}
	}

	matchCases := []struct {
		name, pattern, in string
		extended          bool
	}{
		{"BRE plus literal", `^a+$`, "a+", false},
		{"BRE question literal", `^a?$`, "a?", false},
		{"BRE braces literal", `^a{2}$`, "a{2}", false},
		{"BRE parens and bar literal", `^(a|b)$`, "(a|b)", false},
		{"BRE escaped operators", `^\(a\|b\)\+$`, "aba", false},
		{"ERE plus", `^a+$`, "aaa", true},
		{"ERE question", `^a?$`, "", true},
		{"ERE interval", `^a{2}$`, "aa", true},
		{"ERE grouping and alternation", `^(a|b)$`, "b", true},
		{"ERE escaped paren literal", `^\($`, "(", true},
	}
	for _, c := range matchCases {
		var re *Regexp
		var err error
		if c.extended {
			re, err = CompileEREWithFlags(c.pattern, "")
		} else {
			re, err = Compile(c.pattern)
		}
		if err != nil {
			t.Errorf("%s: compile %q: %v", c.name, c.pattern, err)
		} else if !re.MatchString(c.in) {
			t.Errorf("%s: %q did not match %q", c.name, c.pattern, c.in)
		}
	}

	// Force each syntax through the bounded backtracker as well.
	for _, c := range []struct {
		pattern, in string
		extended    bool
	}{
		{`^\(a\+\)\1$`, "aaaa", false},
		{`^(a+)\1$`, "aaaa", true},
		{`^(a?)\1$`, "aa", true},
		{`^(a{2})\1$`, "aaaa", true},
		{`^(a|b)\1$`, "bb", true},
		{`^\((a)\1\)$`, "(aa)", true},
	} {
		var re *Regexp
		var err error
		if c.extended {
			re, err = CompileEREWithFlags(c.pattern, "")
		} else {
			re, err = Compile(c.pattern)
		}
		if err != nil {
			t.Errorf("backtracking compile %q: %v", c.pattern, err)
		} else if !re.MatchString(c.in) {
			t.Errorf("backtracking %q did not match %q", c.pattern, c.in)
		}
	}
}

func TestPOSIXAnchorTranslation(t *testing.T) {
	breCases := []struct{ pattern, want string }{
		{`^a$`, `^a$`},
		{`a^b`, `a\^b`},
		{`a$b`, `a\$b`},
		{`\(^a\)`, `(^a)`},
		{`\(a$\)`, `(a$)`},
		{`a\|^b`, `a|^b`},
		{`a$\|b`, `a$|b`},
	}
	for _, c := range breCases {
		got, err := ToGo(c.pattern)
		if err != nil {
			t.Errorf("ToGo(%q): %v", c.pattern, err)
		} else if got != c.want {
			t.Errorf("ToGo(%q) = %q, want %q", c.pattern, got, c.want)
		}
	}

	// Unlike BRE, ^ and $ are anchors wherever they occur in an ERE. GNU
	// bash 5.3 therefore agrees with Go regexp that these spellings cannot
	// match literal ^ or $ characters.
	for _, pattern := range []string{
		`^a$`, `a^b`, `a$b`, `(^a)`, `(a$)`, `a|^b`, `a$|b`,
	} {
		got, err := ToGoERE(pattern)
		if err != nil {
			t.Errorf("ToGoERE(%q): %v", pattern, err)
		} else if got != pattern {
			t.Errorf("ToGoERE(%q) = %q, want anchors unchanged", pattern, got)
		}
	}
}

func TestPOSIXAnchorBackrefParity(t *testing.T) {
	breCases := []struct {
		pattern, in string
		want        []int
	}{
		{`^\(a\)\1$`, "aa", []int{0, 2}},
		{`a^\(b\)\1`, "a^bb", []int{0, 4}}, // mid-expression ^ is literal
		{`a$\(b\)\1`, "a$bb", []int{0, 4}}, // mid-expression $ is literal
		{`\(^a\)\1`, "aa", []int{0, 2}},
		{`\(a$\)\1`, "aa", nil},
		{`\(^a\|b\)\1`, "aa", []int{0, 2}},
		{`\(a$\|b\)\1`, "bb", []int{0, 2}},
	}
	for _, c := range breCases {
		re, err := Compile(c.pattern)
		if err != nil {
			t.Errorf("Compile(%q): %v", c.pattern, err)
			continue
		}
		if got := re.FindStringIndex(c.in); !sameInts(got, c.want) {
			t.Errorf("BRE %q on %q = %v, want %v", c.pattern, c.in, got, c.want)
		}
	}

	ereCases := []struct {
		pattern, in string
		want        []int
	}{
		{`^(a)\1$`, "aa", []int{0, 2}},
		{`(a)\1^`, "aa^", nil},
		{`(a)$\1`, "a$a", nil},
		{`(a^)\1`, "a^a^", nil},
		{`($a)\1`, "$a$a", nil},
		{`(^a)\1`, "aa", []int{0, 2}},
		{`(a$)\1`, "aa", nil},
		{`(a^|b)\1`, "a^a^", nil},
		{`($a|b)\1`, "$a$a", nil},
	}
	for _, c := range ereCases {
		re, err := CompileEREWithFlags(c.pattern, "")
		if err != nil {
			t.Errorf("CompileEREWithFlags(%q): %v", c.pattern, err)
			continue
		}
		if got := re.FindStringIndex(c.in); !sameInts(got, c.want) {
			t.Errorf("ERE %q on %q = %v, want %v", c.pattern, c.in, got, c.want)
		}
	}
}

func TestPOSIXBackrefParticipationAndNumbering(t *testing.T) {
	cases := []struct {
		name, pattern, in string
		want              []int
	}{
		{"optional group absent", `^\(a\)\{0,1\}\1$`, "", nil},
		{"optional group participates", `^\(a\)\{0,1\}\1$`, "aa", []int{0, 2}},
		{"optional empty group participates", `^\(a*\)\{0,1\}\1$`, "", []int{0, 0}},
		{"nested group two", `\(\(a\)\2\)`, "aa", []int{0, 2}},
		{"nested numbering", `^\(\(a\)b\)\1\2$`, "ababa", []int{0, 5}},
		{"ninth group", `^\(a\)\(b\)\(c\)\(d\)\(e\)\(f\)\(g\)\(h\)\(i\)\9$`, "abcdefghii", []int{0, 10}},
		{"interval repeats group then last capture", `^\(ab\)\{2\}\1$`, "ababab", []int{0, 6}},
		{"interval repeats backref", `^\(a\)\1\{2\}$`, "aaa", []int{0, 3}},
	}
	for _, c := range cases {
		re, err := Compile(c.pattern)
		if err != nil {
			t.Errorf("%s: Compile(%q): %v", c.name, c.pattern, err)
			continue
		}
		if got := re.FindStringIndex(c.in); !sameInts(got, c.want) {
			t.Errorf("%s: %q on %q = %v, want %v", c.name, c.pattern, c.in, got, c.want)
		}
	}
}

func TestPOSIXBackrefRequiresClosedGroup(t *testing.T) {
	invalidBRE := []string{
		`\1\(a\)`,     // forward reference
		`\(a\1\)`,     // self-reference
		`\(\(a\1\)\)`, // reference to the still-open outer group
	}
	for _, pattern := range invalidBRE {
		if _, err := Compile(pattern); err == nil {
			t.Errorf("Compile(%q) succeeded, want invalid back-reference error", pattern)
		}
	}

	// ERE back-references are a GNU extension, not POSIX ERE syntax, but the
	// package supports them and applies the same preceding-closed-group rule.
	invalidERE := []string{`\1(a)`, `(a\1)`, `((a\1))`}
	for _, pattern := range invalidERE {
		if _, err := CompileEREWithFlags(pattern, ""); err == nil {
			t.Errorf("CompileEREWithFlags(%q) succeeded, want invalid back-reference error", pattern)
		}
	}

	for _, c := range []struct{ pattern, in string }{
		{`((a)\2)`, "aa"},
		{`((a)b)\1\2`, "ababa"},
	} {
		re, err := CompileEREWithFlags(c.pattern, "")
		if err != nil {
			t.Errorf("CompileEREWithFlags(%q): %v", c.pattern, err)
		} else if got := re.FindStringIndex(c.in); !sameInts(got, []int{0, len(c.in)}) {
			t.Errorf("ERE %q on %q = %v, want full match", c.pattern, c.in, got)
		}
	}
}

// The backtracking engine must report the leftmost match, which means trying
// every start offset. These all take the backref path.
func TestPOSIXBacktrackLeftmost(t *testing.T) {
	cases := []struct {
		pattern string
		in      string
		want    []int
	}{
		{`\(a\)\1`, "xaay", []int{1, 3}},
		{`\(ab\)\1`, "zzababzz", []int{2, 6}},
		{`\(a*\)b\1`, "aaba", []int{1, 4}},
		{`\(.\)\1`, "abcdd", []int{3, 5}},
	}
	for _, c := range cases {
		re, err := Compile(c.pattern)
		if err != nil {
			t.Fatalf("Compile(%q): %v", c.pattern, err)
		}
		if got := re.FindStringIndex(c.in); !sameInts(got, c.want) {
			t.Errorf("%q on %q = %v, want leftmost %v", c.pattern, c.in, got, c.want)
		}
	}
}

// POSIX XCU sed: "the escape sequence '\n' shall match a <newline> embedded in
// the pattern space". GNU sed adds \t \r \f \v \a. Everything else — including
// \1..\9 and the BRE metacharacter escapes — must pass through untouched, and
// an escaped backslash must not let the next byte be read as an escape.
func TestSedEscapes(t *testing.T) {
	cases := []struct{ in, want string }{
		{`a\nb`, "a\nb"},
		{`a\tb`, "a\tb"},
		{`\r\f\v\a`, "\r\f\v\a"},
		{`[\n]`, "[\n]"},
		// Untouched: BRE syntax.
		{`\(a\)\1`, `\(a\)\1`},
		{`a\{2,3\}`, `a\{2,3\}`},
		{`a\|b`, `a\|b`},
		{`\<w\>`, `\<w\>`},
		{`\w\s\b`, `\w\s\b`},
		{`\.`, `\.`},
		// An escaped backslash is consumed as a unit: \\n is "literal
		// backslash, then n", not "literal backslash, then newline".
		{`a\\nb`, `a\\nb`},
		{`\\`, `\\`},
		// No backslash at all, and a trailing lone backslash.
		{`abc`, `abc`},
		{`abc\`, `abc\`},
	}
	for _, c := range cases {
		if got := SedEscapes(c.in); got != c.want {
			t.Errorf("SedEscapes(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// POSIX: a period matches any character. sed's pattern space can hold embedded
// newlines (after N/G), and the "s" flag compiles '.' to match them — on both
// the RE2 path and the backtracking (back-reference) path.
func TestDotAllFlag(t *testing.T) {
	cases := []struct {
		pattern string
		flags   string
		in      string
		want    []int
	}{
		{`a.b`, "(?s)", "a\nb", []int{0, 3}},
		{`a.b`, "", "a\nb", nil},
		// Backtracking engine: the pattern has a back-reference.
		{`\(a\).\1`, "(?s)", "a\na", []int{0, 3}},
		{`\(a\).\1`, "", "a\na", nil},
		// The i and s flags compose.
		{`\(a\).\1`, "(?is)", "A\na", []int{0, 3}},
	}
	for _, c := range cases {
		re, err := CompileWithFlags(c.pattern, c.flags)
		if err != nil {
			t.Fatalf("CompileWithFlags(%q, %q): %v", c.pattern, c.flags, err)
		}
		if got := re.FindStringIndex(c.in); !sameInts(got, c.want) {
			t.Errorf("CompileWithFlags(%q, %q) on %q = %v, want %v", c.pattern, c.flags, c.in, got, c.want)
		}
	}
}
