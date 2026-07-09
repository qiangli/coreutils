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
		{`x[[.-.]]y`, "x-y", "xzy"},        // '-' collating element inside a class
		{`[[.].]]`, "]", "a"},              // ']' collating element
		{`[^[=a=]]`, "b", "a"},             // negated equivalence class
		{`[[.a.]-c]`, "b", "z"},            // collating element as a range endpoint
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
