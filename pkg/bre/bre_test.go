package bre

import (
	"regexp"
	"testing"
)

// TestCollatingEquivalenceCLocale verifies [.x.] and [=x=] reduce to their literal
// member under C-locale semantics (the coreutils locale), matching GNU grep/sed.
func TestCollatingEquivalenceCLocale(t *testing.T) {
	cases := []struct {
		bre, matches, notMatches string
	}{
		{`[[.a.]]`, "a", "b"},
		{`[[=e=]]`, "e", "x"},
		{`x[[.-.]]y`, "x-y", "xzy"}, // '-' collating element inside a class
		{`[[.].]]`, "]", "a"},       // ']' collating element
		{`[^[=a=]]`, "b", "a"},      // negated equivalence class
		{`[[.a.]-c]`, "b", "z"},     // collating element as a range endpoint
	}
	for _, c := range cases {
		goRE, err := ToGo(c.bre)
		if err != nil {
			t.Errorf("ToGo(%q) errored (should translate under C locale): %v", c.bre, err)
			continue
		}
		re, err := regexp.Compile(goRE)
		if err != nil {
			t.Errorf("ToGo(%q)=%q not a valid RE2: %v", c.bre, goRE, err)
			continue
		}
		if !re.MatchString(c.matches) {
			t.Errorf("%q -> %q should match %q", c.bre, goRE, c.matches)
		}
		if re.MatchString(c.notMatches) {
			t.Errorf("%q -> %q should NOT match %q", c.bre, goRE, c.notMatches)
		}
	}
}

func TestCompileBackrefs(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		in      string
		want    []int
	}{
		{"literal", `\(a\)\1`, "aa", []int{0, 2}},
		{"dot", `\(.\)\1`, "abbb", []int{1, 3}},
		{"multi", `\(ab\)\1`, "xabab", []int{1, 5}},
		{"anchored", `^\(.\)\1$`, "aa", []int{0, 2}},
		{"repeated capture", `\(a*\)b\1`, "aabaa", []int{0, 5}},
	}
	for _, c := range cases {
		re, err := Compile(c.pattern)
		if err != nil {
			t.Fatalf("%s: Compile(%q): %v", c.name, c.pattern, err)
		}
		got := re.FindStringIndex(c.in)
		if !sameInts(got, c.want) {
			t.Errorf("%s: FindStringIndex(%q)=%v, want %v", c.name, c.in, got, c.want)
		}
	}
}

func TestBackrefMatcherPOSIXAnchorPositions(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		in      string
		want    []int
	}{
		{"mid caret literal", `a^\(b\)\1`, "a^bb", []int{0, 4}},
		{"mid dollar literal", `a$\(b\)\1`, "a$bb", []int{0, 4}},
		{"group caret anchor", `\(^a\)\1`, "aa", []int{0, 2}},
		{"dollar before group end", `\(a$\)\1`, "aa", nil},
	}
	for _, c := range cases {
		re, err := Compile(c.pattern)
		if err != nil {
			t.Fatalf("%s: Compile(%q): %v", c.name, c.pattern, err)
		}
		got := re.FindStringIndex(c.in)
		if !sameInts(got, c.want) {
			t.Errorf("%s: FindStringIndex(%q)=%v, want %v", c.name, c.in, got, c.want)
		}
	}
}

func TestBackrefMatcherGNUEscapes(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		in      string
		want    []int
	}{
		{"word class", `\(\w\)\1`, "--__--", []int{2, 4}},
		{"nonword class", `\(\W\)\1`, "aa!!", []int{2, 4}},
		{"space class", `\(\s\)\1`, "a \t\tb", []int{2, 4}},
		{"word boundary", `\b\(a\)\1\b`, "-aa-", []int{1, 3}},
	}
	for _, c := range cases {
		re, err := Compile(c.pattern)
		if err != nil {
			t.Fatalf("%s: Compile(%q): %v", c.name, c.pattern, err)
		}
		got := re.FindStringIndex(c.in)
		if !sameInts(got, c.want) {
			t.Errorf("%s: FindStringIndex(%q)=%v, want %v", c.name, c.in, got, c.want)
		}
	}
}

func TestCompileWordEdgeAnchors(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		in      string
		want    []int
	}{
		{"word", `\<word\>`, "a word.", []int{2, 6}},
		{"start", `\<[[:alpha:]]\+`, "..abc", []int{2, 5}},
		{"end", `[[:alpha:]]\+\>`, "abc..", []int{0, 3}},
		{"underscore word", `\<foo_bar\>`, "foo_bar", []int{0, 7}},
		{"repeat backtracks to word start", `.*\<`, "foo", []int{0, 0}},
		{"word edge with backref", `\<\(.\)\1\>`, "-aa-", []int{1, 3}},
	}
	for _, c := range cases {
		re, err := Compile(c.pattern)
		if err != nil {
			t.Fatalf("%s: Compile(%q): %v", c.name, c.pattern, err)
		}
		got := re.FindStringIndex(c.in)
		if !sameInts(got, c.want) {
			t.Errorf("%s: FindStringIndex(%q)=%v, want %v", c.name, c.in, got, c.want)
		}
	}
}

func TestCompileWordEdgeNoMatch(t *testing.T) {
	re, err := Compile(`\<word\>`)
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range []string{"sword", "words", "swordfish"} {
		if re.MatchString(in) {
			t.Fatalf("%q unexpectedly matched word-edge pattern", in)
		}
	}
}

func TestCompileIntervalEdges(t *testing.T) {
	cases := []struct {
		pattern string
		in      string
		want    bool
	}{
		{`^a\{0,2\}$`, "", true},
		{`^a\{0,2\}$`, "aa", true},
		{`^a\{0,2\}$`, "aaa", false},
		{`^ba\{,2\}r$`, "br", true},
		{`^ba\{,2\}r$`, "baar", true},
		{`^ba\{,2\}r$`, "baaar", false},
		{`^a\{2,\}$`, "aa", true},
		{`^a\{2,\}$`, "a", false},
	}
	for _, c := range cases {
		re, err := Compile(c.pattern)
		if err != nil {
			t.Fatalf("Compile(%q): %v", c.pattern, err)
		}
		if got := re.MatchString(c.in); got != c.want {
			t.Errorf("%q.MatchString(%q)=%v, want %v", c.pattern, c.in, got, c.want)
		}
	}
}

func TestCompileInvalidIntervals(t *testing.T) {
	for _, pattern := range []string{
		`a\{`,
		`a\{\}`,
		`a\{,\}`,
		`a\{3,2\}`,
		`a\{x\}`,
		`a\{1,2,3\}`,
		`\(a\)\1\{3,2\}`,
		`\{\(a\)\1`,
		`\(a\)\1\}`,
	} {
		if _, err := Compile(pattern); err == nil {
			t.Errorf("Compile(%q) succeeded, want invalid interval error", pattern)
		}
	}
}

func TestToGoEREWordEdgeAnchors(t *testing.T) {
	got, err := ToGoERE(`\<word\>`)
	if err != nil {
		t.Fatal(err)
	}
	if got != `\bword\b` {
		t.Fatalf(`ToGoERE(\<word\>)=%q, want \bword\b`, got)
	}
	if _, err := ToGoERE(`(a)\1`); err == nil {
		t.Fatal("ToGoERE accepted an ERE back-reference")
	}
}

func TestToGoEREBracketExpressions(t *testing.T) {
	cases := []struct {
		ere, matches, notMatches string
	}{
		{`[[.a.]]`, "a", "b"},
		{`[[=e=]]`, "e", "x"},
		{`[\.]`, ".", "a"},
		{`[\.]`, `\`, "a"},
		{`[[.].]]`, "]", "x"},
	}
	for _, c := range cases {
		goRE, err := ToGoERE(c.ere)
		if err != nil {
			t.Errorf("ToGoERE(%q) errored: %v", c.ere, err)
			continue
		}
		re, err := regexp.Compile(goRE)
		if err != nil {
			t.Errorf("ToGoERE(%q)=%q not a valid RE2: %v", c.ere, goRE, err)
			continue
		}
		if !re.MatchString(c.matches) {
			t.Errorf("%q -> %q should match %q", c.ere, goRE, c.matches)
		}
		if re.MatchString(c.notMatches) {
			t.Errorf("%q -> %q should NOT match %q", c.ere, goRE, c.notMatches)
		}
	}
}

func TestCompileEREBackrefs(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		in      string
		want    []int
	}{
		{"simple group", `(a)\1`, "xaa", []int{1, 3}},
		{"class group", `([[:alpha:]])\1`, "11bb", []int{2, 4}},
		{"alternation group", `(a|ab)c\1`, "abcab", []int{0, 5}},
		{"interval group", `(a{2})\1`, "xaaaa", []int{1, 5}},
		{"empty capture", `(a*)b\1`, "b", []int{0, 1}},
		{"word edge", `\<([[:alpha:]])\1\>`, "-aa-", []int{1, 3}},
		{"literal escaped parens", `\((a)\1\)`, "(aa)", []int{0, 4}},
	}
	for _, c := range cases {
		re, err := CompileEREWithFlags(c.pattern, "")
		if err != nil {
			t.Fatalf("%s: CompileEREWithFlags(%q): %v", c.name, c.pattern, err)
		}
		if got := re.FindStringIndex(c.in); !sameInts(got, c.want) {
			t.Errorf("%s: %q on %q = %v, want %v", c.name, c.pattern, c.in, got, c.want)
		}
	}
}

func TestCompileBackrefsNoMatch(t *testing.T) {
	re, err := Compile(`^\(.\)\1$`)
	if err != nil {
		t.Fatal(err)
	}
	if re.MatchString("aaa") {
		t.Fatal("anchored backref unexpectedly matched")
	}
	// \(a*\) can match the empty string, and a back-reference to a group that
	// matched the empty string matches the empty string (POSIX XBD 9.3.6), so
	// \(a*\)b\1 matches every subject containing a 'b'. A subject with no 'b' is
	// the real negative. (This case previously asserted that "aaba" does NOT
	// match, which POSIX contradicts — see TestPOSIXBackrefEmptyGroup.)
	re, err = Compile(`\(a*\)b\1`)
	if err != nil {
		t.Fatal(err)
	}
	if re.MatchString("aaa") {
		t.Fatal(`\(a*\)b\1 unexpectedly matched a subject with no "b"`)
	}
}

func sameInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
