package bre

import (
	"fmt"
	"strings"
	"testing"
)

// These tests exercise the custom ERE backend that AWK expression regexes
// (/pat/, ~, !~, match()) route through via awkERECompiler →
// CompileEREWithFlags. They are bounded regression/property tests for the
// interval-normalization, quantifier-composition, bracket-atom, UTF-8, and
// fail-closed behaviors documented by the backend. They do not exercise the
// standard Go-RE2 path that FS/RS/split/sub/gsub use (that routing is covered
// in cmds/awk/awk_routing_test.go).
//
// The flags prefix "(?s)" mirrors what awkERECompiler.Compile passes so the
// tests hit the same code path.

const awkFlagPrefix = "(?s)"

// compileERE is a shorthand that applies the AWK flag prefix and sets Longest,
// matching the production awkERECompiler.Compile / Regexp.Longest sequence.
func compileERE(t *testing.T, pattern string) *Regexp {
	t.Helper()
	re, err := CompileEREWithFlags(pattern, awkFlagPrefix)
	if err != nil {
		t.Fatalf("CompileEREWithFlags(%q, %q): %v", pattern, awkFlagPrefix, err)
	}
	re.Longest()
	return re
}

// matchLen returns the length of the first match of pattern against in, or -1
// if there is no match.
func matchLen(re *Regexp, in string) int {
	loc := re.FindStringIndex(in)
	if loc == nil {
		return -1
	}
	return loc[1] - loc[0]
}

// ---------------------------------------------------------------------------
// Leading-zero exact / range / unbounded counts through 255
// ---------------------------------------------------------------------------

// TestAwkERELeadingZeroExactThrough255 verifies that for every exact count
// 0..255, the zero-padded spelling a{0NNN} matches the same input lengths as
// the canonical a{N}. normalizeInterval strips the leading zeros; RE2 compiles
// the canonical decimal.
func TestAwkERELeadingZeroExactThrough255(t *testing.T) {
	for n := 0; n <= 255; n++ {
		padded := fmt.Sprintf(`a{%04d}`, n)
		canonical := fmt.Sprintf(`a{%d}`, n)
		paddedRe := compileERE(t, padded)
		canonRe := compileERE(t, canonical)
		// Probe a bounded set of lengths around the count.
		probes := []int{0, 1}
		if n > 0 {
			probes = append(probes, n-1)
		}
		probes = append(probes, n, n+1)
		if n > 0 && 2*n <= 510 {
			probes = append(probes, 2*n)
		}
		for _, len := range probes {
			if len < 0 {
				continue
			}
			in := strings.Repeat("a", len)
			if got, want := paddedRe.MatchString(in), canonRe.MatchString(in); got != want {
				t.Errorf("leading-zero exact n=%d len=%d: %q match=%v, canonical %q match=%v",
					n, len, padded, got, canonical, want)
			}
		}
	}
}

// TestAwkERELeadingZeroUnboundedThrough255 verifies the unbounded form
// a{0NNN,} has the same matching behavior as a{N,} for every N in [0,255].
func TestAwkERELeadingZeroUnboundedThrough255(t *testing.T) {
	for n := 0; n <= 255; n++ {
		padded := fmt.Sprintf(`a{%04d,}`, n)
		canonical := fmt.Sprintf(`a{%d,}`, n)
		paddedRe := compileERE(t, padded)
		canonRe := compileERE(t, canonical)
		for _, len := range []int{0, n, n + 1} {
			if len < 0 {
				continue
			}
			in := strings.Repeat("a", len)
			if got, want := paddedRe.MatchString(in), canonRe.MatchString(in); got != want {
				t.Errorf("leading-zero unbounded n=%d len=%d: %q match=%v, canonical %q match=%v",
					n, len, padded, got, canonical, want)
			}
		}
	}
}

// TestAwkERELeadingZeroRangeParity verifies the ranged form a{0LLL,0HHH}
// matches identically to a{L,H} across a bounded grid of bounds. The grid
// samples low and high values spanning the full [0,255] range.
func TestAwkERELeadingZeroRangeParity(t *testing.T) {
	bounds := []int{0, 1, 2, 7, 15, 63, 127, 200, 255}
	for _, lo := range bounds {
		for _, hi := range bounds {
			if hi < lo {
				continue
			}
			padded := fmt.Sprintf(`a{%04d,%04d}`, lo, hi)
			canonical := fmt.Sprintf(`a{%d,%d}`, lo, hi)
			paddedRe := compileERE(t, padded)
			canonRe := compileERE(t, canonical)
			probes := []int{0, lo, hi, lo + (hi-lo)/2, hi + 1}
			for _, len := range probes {
				if len < 0 || len > 510 {
					continue
				}
				in := strings.Repeat("a", len)
				if got, want := paddedRe.MatchString(in), canonRe.MatchString(in); got != want {
					t.Errorf("leading-zero range [%d,%d] len=%d: %q match=%v, canonical %q match=%v",
						lo, hi, len, padded, got, canonical, want)
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Adjacent quantifiers whose effective repetition exceeds 255
// ---------------------------------------------------------------------------

// TestAwkEREAdjacentQuantifiersRejected verifies that the ERE backend rejects
// adjacent quantifier compositions (a{2}{3}, a*{2}, a{2}*, etc.). POSIX ERE
// forbids applying a repetition operator to an already-quantified atom; the
// bre backend enforces this by passing the constructs to Go RE2, which rejects
// nested repetition operators. This is distinct from the standard path
// (split/sub/gsub), which uses regex.Normalize to wrap them in non-capturing
// groups — see cmds/awk/awk_routing_test.go.
//
// Each individual bound can be ≤ 255 while the effective repetition exceeds
// 255 (e.g. a{200}{200} = 40 000), but the rejection happens regardless of
// magnitude because POSIX ERE syntax is violated, not because of the cap.
func TestAwkEREAdjacentQuantifiersRejected(t *testing.T) {
	patterns := []string{
		// effective repetition ≤ 255 but still rejected (syntax violation)
		`a{2}{3}`,
		`a{1}{5}`,
		`a*{2}`,
		`a+{2}`,
		`a?{2}`,
		// effective repetition exceeds 255
		`a{200}{200}`,
		`a{20}{20}`,
		// interval after a quantifier
		`a{2}*`,
		`a{2}+`,
	}
	for _, p := range patterns {
		if _, err := CompileEREWithFlags(p, awkFlagPrefix); err == nil {
			t.Errorf("CompileEREWithFlags(%q) succeeded; want nested-repetition error", p)
		}
	}
}

// ---------------------------------------------------------------------------
// Lazy suffix cardinality
// ---------------------------------------------------------------------------

// TestAwkERELazySuffixCardinality verifies that lazy quantifier suffixes are
// syntactically accepted by the ERE backend, and that under Longest() — the
// AWK-mandated leftmost-longest mode — lazy and greedy forms produce the same
// match length. AWK follows POSIX leftmost-longest semantics; the lazy
// modifier (a Go RE2 extension) must not shrink the overall match extent.
func TestAwkERELazySuffixCardinality(t *testing.T) {
	pairs := []struct {
		greedy, lazy string
		in           string
		wantLen      int
	}{
		{`a{2}`, `a{2}?`, "aaaa", 2},
		{`a{2,4}`, `a{2,4}?`, "aaaaa", 4},
		{`a+`, `a+?`, "aaa", 3},
		{`a?`, `a??`, "aaa", 1},
		{`a{0,3}`, `a{0,3}?`, "aaaa", 3},
	}
	for _, c := range pairs {
		greedyRe := compileERE(t, c.greedy)
		lazyRe := compileERE(t, c.lazy)
		if got := matchLen(greedyRe, c.in); got != c.wantLen {
			t.Errorf("greedy %q on %q: match length %d, want %d", c.greedy, c.in, got, c.wantLen)
		}
		if got := matchLen(lazyRe, c.in); got != c.wantLen {
			t.Errorf("lazy %q on %q: match length %d, want %d (Longest overrides lazy)", c.lazy, c.in, got, c.wantLen)
		}
	}
}

// ---------------------------------------------------------------------------
// POSIX bracket atoms / escaped brackets / unclosed class fallback
// ---------------------------------------------------------------------------

// TestAwkEREBracketAtoms verifies that POSIX character-class atoms inside
// bracket expressions compile and match correctly through the ERE backend.
func TestAwkEREBracketAtoms(t *testing.T) {
	cases := []struct {
		pattern string
		in      string
		want    bool
	}{
		{`[[:alpha:]]`, "a", true},
		{`[[:alpha:]]`, "9", false},
		{`[[:digit:]]`, "7", true},
		{`[[:digit:]]`, "x", false},
		{`[[:upper:]]`, "A", true},
		{`[[:lower:]]`, "a", true},
		{`[[:alnum:]]+`, "abc123", true},
		{`[[:space:]]`, " ", true},
		{`[[:punct:]]`, "!", true},
		{`[a-z[:digit:]]`, "5", true},
		{`[a-z[:digit:]]`, "_", false},
	}
	for _, c := range cases {
		re := compileERE(t, c.pattern)
		if got := re.MatchString(c.in); got != c.want {
			t.Errorf("%q.MatchString(%q) = %v, want %v", c.pattern, c.in, got, c.want)
		}
	}
}

// TestAwkEREEscapedBrackets verifies POSIX bracket-expression semantics for
// backslash and ] as first member. In POSIX bracket expressions, backslash is
// a literal character (not an escape); the bre backend honors this, while the
// standard Go-RE2 path treats backslash as an escape — the routing test
// documents that divergence.
func TestAwkEREEscapedBrackets(t *testing.T) {
	// ] as the first member is a literal.
	re := compileERE(t, `[]a]`)
	if !re.MatchString("]") {
		t.Errorf(`[]a] should match ]`)
	}
	if re.MatchString("b") {
		t.Errorf(`[]a] should not match b`)
	}
	// ] as the first member after ^ is also literal.
	re2 := compileERE(t, `[^]a]`)
	if !re2.MatchString("b") {
		t.Errorf(`[^]a] should match b`)
	}
	if re2.MatchString("]") {
		t.Errorf(`[^]a] should not match ]`)
	}
	// Backslash is a literal inside [...] per POSIX. [a\] closes at the next ],
	// so the class is {a, \}; the remaining x] is literal text outside the
	// class. This is the POSIX reading; the standard Go-RE2 path instead treats
	// \] as an escaped member, which is a Go extension (documented in
	// awk_routing_test.go).
	re3 := compileERE(t, `[a\]x]`)
	if !re3.MatchString("ax]") {
		t.Errorf(`[a\]x] should match "ax]" (class {a,\} then literal "x]")`)
	}
	if re3.MatchString("ax") {
		t.Errorf(`[a\]x] should not match "ax" (missing literal "]")`)
	}
}

// TestAwkEREUnclosedClass verifies that unclosed bracket expressions are
// rejected with a clear error (fail-closed), not silently accepted.
func TestAwkEREUnclosedClass(t *testing.T) {
	patterns := []string{
		`[a-`,
		`[`,
		`[abc`,
		`[[:alpha:`,
		`[^abc`,
	}
	for _, p := range patterns {
		if _, err := CompileEREWithFlags(p, awkFlagPrefix); err == nil {
			t.Errorf("CompileEREWithFlags(%q) succeeded; want unmatched-bracket error", p)
		}
	}
}

// ---------------------------------------------------------------------------
// UTF-8 atoms
// ---------------------------------------------------------------------------

// TestAwkEREUTF8Atoms verifies that multi-byte UTF-8 atoms participate
// correctly in intervals and matching through the ERE backend.
func TestAwkEREUTF8Atoms(t *testing.T) {
	cases := []struct {
		pattern string
		in      string
		want    bool
	}{
		{`^é{2}$`, "éé", true},
		{`^é{2}$`, "é", false},
		{`^é{2,3}$`, "ééé", true},
		{`^π{3}$`, "πππ", true},
		{`^π{3}$`, "ππ", false},
		{`^(日本){2}$`, "日本日本", true},
		{`^日本{2}$`, "日本本", true},
		{`^café+$`, "caféé", true},
	}
	for _, c := range cases {
		re := compileERE(t, c.pattern)
		if got := re.MatchString(c.in); got != c.want {
			t.Errorf("%q.MatchString(%q) = %v, want %v", c.pattern, c.in, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Malformed / repeated quantifier fail-closed behavior
// ---------------------------------------------------------------------------

// TestAwkEREMalformedQuantifiersFailClosed verifies that syntactically invalid
// quantifier usage is rejected, not silently accepted with different
// semantics. Lazy single-suffix modifiers (a??, a*?, a+?, a{2}?) are the
// only repeated-quantifier spellings that are valid (they are RE2 lazy
// modifiers); everything else fails closed.
func TestAwkEREMalformedQuantifiersFailClosed(t *testing.T) {
	reject := []string{
		// nested repetition (not a lazy suffix)
		`a**`, `a++`, `a{2}??`, `a{2}*`, `a{2}+`, `a{2}{3}`,
		// dangling quantifier (nothing to repeat)
		`*a`, `+a`, `?a`, `{2}`, `*`, `**`,
		// malformed interval syntax
		`a{`, `a{}`, `a{,}`, `a{x}`, `a{2,3,4}`,
		// inverted bounds
		`a{3,2}`,
	}
	for _, p := range reject {
		if _, err := CompileEREWithFlags(p, awkFlagPrefix); err == nil {
			t.Errorf("CompileEREWithFlags(%q) succeeded; want error", p)
		}
	}

	// Valid lazy-suffix spellings must be accepted.
	accept := []string{`a??`, `a*?`, `a+?`, `a{2}?`, `a{2,4}?`}
	for _, p := range accept {
		if _, err := CompileEREWithFlags(p, awkFlagPrefix); err != nil {
			t.Errorf("CompileEREWithFlags(%q) failed: %v; want accepted lazy suffix", p, err)
		}
	}
}
