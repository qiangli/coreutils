package bre

import "testing"

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
