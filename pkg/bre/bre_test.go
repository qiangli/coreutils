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

func TestCompileBackrefsNoMatch(t *testing.T) {
	re, err := Compile(`^\(.\)\1$`)
	if err != nil {
		t.Fatal(err)
	}
	if re.MatchString("aaa") {
		t.Fatal("anchored backref unexpectedly matched")
	}
	re, err = Compile(`\(a*\)b\1`)
	if err != nil {
		t.Fatal(err)
	}
	if re.MatchString("aaba") {
		t.Fatal("backref with leading repeated capture unexpectedly matched")
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
