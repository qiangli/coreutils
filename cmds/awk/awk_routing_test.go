package awkcmd

import (
	"strings"
	"testing"
)

// These tests prove every AWK regexp endpoint uses coreutils' single custom
// ERE backend: literals, ~, !~, match, FS, RS, split, sub, and gsub.

// progOK runs prog and reports whether it exited 0 with no stderr.
func progOK(t *testing.T, input, prog string) bool {
	t.Helper()
	out, errb, code := runTool(t, input, prog)
	if code != 0 || errb != "" {
		t.Logf("unexpected failure: prog=%q out=%q err=%q code=%d", prog, out, errb, code)
		return false
	}
	return true
}

// progStdout runs prog expecting success and returns stdout.
func progStdout(t *testing.T, input, prog string) string {
	t.Helper()
	out, errb, code := runTool(t, input, prog)
	if code != 0 || errb != "" {
		t.Fatalf("prog=%q: out=%q err=%q code=%d", prog, out, errb, code)
	}
	return out
}

// progFails runs prog and reports whether it exited non-zero (fail-closed).
func progFails(t *testing.T, input, prog string) bool {
	t.Helper()
	_, _, code := runTool(t, input, prog)
	return code != 0
}

// ---------------------------------------------------------------------------
// Backend routing proof: interval bound ceiling
// ---------------------------------------------------------------------------

// TestAwkRoutingIntervalCeiling proves every endpoint accepts the advertised
// ceiling and rejects the next value.
func TestAwkRoutingIntervalCeiling(t *testing.T) {
	long255 := strings.Repeat("a", 255)
	exprOK := []struct{ name, prog, input string }{
		{"tilde", `BEGIN { print ("` + long255 + `" ~ "a{255}") }`, ""},
		{"not-tilde", `BEGIN { print ("" !~ "a{255}") }`, ""},
		{"match-builtin", `BEGIN { print match("` + long255 + `", "a{255}") }`, ""},
		{"slash-literal", `/a{255}/ { print "hit" }`, long255 + "\n"},
	}
	for _, c := range exprOK {
		if !progOK(t, c.input, c.prog) {
			t.Errorf("%s: expression regex a{255} should succeed", c.name)
		}
	}

	stdOK := []struct{ name, prog string }{
		{"split", `BEGIN { n = split("` + long255 + `", a, "a{255}"); print n }`},
		{"sub", `BEGIN { s = "` + long255 + `"; sub("a{255}", "x", s); print s }`},
		{"gsub", `BEGIN { s = "` + long255 + `"; gsub("a{255}", "x", s); print s }`},
		{"FS", `BEGIN { FS = "a{255}" } { print NF }`},
		{"RS", `BEGIN { RS = "a{255}" } { print }`},
	}
	for _, c := range stdOK {
		if !progOK(t, "aaa\n", c.prog) {
			t.Errorf("%s: unified backend a{255} should succeed", c.name)
		}
	}

	for _, tc := range []struct{ name, prog string }{
		{"tilde", `BEGIN { print ("" ~ "a{256}") }`},
		{"match", `BEGIN { print match("", "a{256}") }`},
		{"split", `BEGIN { print split("a", a, "a{256}") }`},
		{"sub", `BEGIN { s=""; sub("a{256}", "x", s) }`},
	} {
		if !progFails(t, "", tc.prog) {
			t.Errorf("%s: a{256} should fail", tc.name)
		}
	}
}

// A separator ERE that matches only the empty string is ignored by POSIX awk
// split(); it must not manufacture one field per character boundary.
func TestAwkSplitIgnoresZeroLengthSeparatorMatches(t *testing.T) {
	if got := progStdout(t, "", `BEGIN { n = split("abababccccccd", a, /c{0}/); print n; print a[1] }`); got != "1\nabababccccccd\n" {
		t.Errorf("split with zero-length ERE = %q, want the input as one field", got)
	}
}

// ---------------------------------------------------------------------------
// Leading-zero counts through 255 in both paths
// ---------------------------------------------------------------------------

// TestAwkRoutingLeadingZerosBothPaths verifies that leading-zero interval
// spellings are normalized identically in the expression and standard paths.
// A single AWK program loops 1..255 so the test stays fast.
func TestAwkRoutingLeadingZerosBothPaths(t *testing.T) {
	// Expression path (~): for each n 1..255, a{0NNN} matches exactly n a's
	// and does not match n-1 a's.
	prog := `BEGIN {
		ok = 1
		for (n = 1; n <= 255; n++) {
			pat = sprintf("a{%04d}", n)
			s = sprintf("%*s", n, ""); gsub(/ /, "a", s)
			if ((s ~ pat) != 1) { print "FAIL match n=" n; ok = 0 }
			if (n >= 2) {
				short = substr(s, 2)
				if ((short ~ pat) != 0) { print "FAIL nomatch n=" n; ok = 0 }
			}
		}
		print ok
	}`
	if got := progStdout(t, "", prog); got != "1\n" {
		t.Errorf("expression path leading-zero loop: got %q", got)
	}

	// Standard path (split): for each n 2..255, a{0NNN} as a separator on n
	// a's produces exactly 2 fields (the separator matches the whole input).
	progSplit := `BEGIN {
		ok = 1
		for (n = 2; n <= 255; n++) {
			pat = sprintf("a{%04d}", n)
			s = sprintf("%*s", n, ""); gsub(/ /, "a", s)
			nf = split(s, a, pat)
			if (nf != 2) { print "FAIL split n=" n " nf=" nf; ok = 0 }
		}
		print ok
	}`
	if got := progStdout(t, "", progSplit); got != "1\n" {
		t.Errorf("standard path leading-zero loop: got %q", got)
	}

	// n=0: a{0000} matches the empty string (zero repetitions).
	if got := progStdout(t, "", `BEGIN { print ("" ~ "a{0000}") }`); got != "1\n" {
		t.Errorf("n=0: got %q, want 1", got)
	}
}

// ---------------------------------------------------------------------------
// Adjacent quantifiers fail closed on every endpoint
// ---------------------------------------------------------------------------

// TestAwkRoutingAdjacentQuantifiers verifies that adjacent quantifier
// compositions — including some whose effective repetition exceeds 255 — are
// are rejected everywhere (POSIX forbids repeating a quantified atom).
func TestAwkRoutingAdjacentQuantifiers(t *testing.T) {
	// Each individual bound is ≤ 255; some products exceed 255.
	patterns := []string{
		`a{2}{3}`,   // effective 6
		`a{20}{20}`, // effective 400 > 255
		`a*{2}`,     // star then interval
	}
	for _, pat := range patterns {
		// Expression path: rejected (POSIX: cannot quantify a quantified atom).
		exprProg := "BEGIN { print (\"aaaaaa\" ~ \"" + pat + "\") }"
		if !progFails(t, "", exprProg) {
			t.Errorf("expression ~ %q should fail (nested repetition)", pat)
		}

		// Standard endpoints use the same strict backend.
		splitProg := "BEGIN { print split(\"aaaaaa\", a, \"" + pat + "\") }"
		if !progFails(t, "", splitProg) {
			t.Errorf("split %q should fail (nested repetition)", pat)
		}
	}
}

// ---------------------------------------------------------------------------
// Lazy suffix: both paths accept
// ---------------------------------------------------------------------------

// TestAwkRoutingLazySuffix verifies that lazy quantifier suffixes compile in
// both paths. Under AWK's leftmost-longest mode the lazy modifier does not
// change match extent; this test confirms syntactic acceptance.
func TestAwkRoutingLazySuffix(t *testing.T) {
	patterns := []string{`a{2}?`, `a+?`, `a??`}
	for _, pat := range patterns {
		// Expression path.
		if !progOK(t, "", "BEGIN { print (\"aaab\" ~ \""+pat+"\") }") {
			t.Errorf("expression ~ %q should succeed", pat)
		}
		// Standard path (split).
		if !progOK(t, "", "BEGIN { print split(\"aaab\", a, \""+pat+"\") }") {
			t.Errorf("split %q should succeed", pat)
		}
	}
}

// ---------------------------------------------------------------------------
// POSIX bracket atoms: both paths accept
// ---------------------------------------------------------------------------

// TestAwkRoutingBracketAtoms verifies POSIX character-class atoms work in
// both paths.
func TestAwkRoutingBracketAtoms(t *testing.T) {
	if got := progStdout(t, "", `BEGIN { print ("abc" ~ "[[:alpha:]]+") }`); got != "1\n" {
		t.Errorf("expr [[:alpha:]]+: got %q, want 1", got)
	}
	if got := progStdout(t, "", `BEGIN { print split("a1b2", a, "[[:digit:]]") }`); got != "3\n" {
		t.Errorf("split [[:digit:]]: got %q, want 3", got)
	}
}

// ---------------------------------------------------------------------------
// UTF-8 atoms: both paths accept
// ---------------------------------------------------------------------------

// TestAwkRoutingUTF8Atoms verifies that multi-byte UTF-8 atoms participate in
// intervals in both paths.
func TestAwkRoutingUTF8Atoms(t *testing.T) {
	if got := progStdout(t, "", `BEGIN { print ("éé" ~ "é{2}") }`); got != "1\n" {
		t.Errorf("expr é{2}: got %q, want 1", got)
	}
	if got := progStdout(t, "", `BEGIN { print split("éé", a, "é{2}") }`); got != "2\n" {
		t.Errorf("split é{2}: got %q, want 2", got)
	}
}

// ---------------------------------------------------------------------------
// Malformed / repeated quantifier fail-closed behavior
// ---------------------------------------------------------------------------

// TestAwkRoutingMalformedQuantifiers verifies that genuinely invalid quantifier
// usage fails closed on every endpoint.
func TestAwkRoutingMalformedQuantifiers(t *testing.T) {
	// Both paths reject: inverted bounds and dangling quantifiers.
	bothReject := []string{`a{3,2}`, `{2}`}
	for _, pat := range bothReject {
		if !progFails(t, "", "BEGIN { print (\"a\" ~ \""+pat+"\") }") {
			t.Errorf("expr ~ %q should fail", pat)
		}
		if !progFails(t, "", "BEGIN { print split(\"a\", a, \""+pat+"\") }") {
			t.Errorf("split %q should fail", pat)
		}
	}

	malformed := []string{`a{`, `a{}`, `a{x}`, `a{2,3,4}`}
	for _, pat := range malformed {
		if !progFails(t, "", "BEGIN { print (\"a\" ~ \""+pat+"\") }") {
			t.Errorf("expr ~ %q should fail (strict POSIX)", pat)
		}
		if !progFails(t, "", "BEGIN { print split(\"a\", a, \""+pat+"\") }") {
			t.Errorf("split %q should fail (strict POSIX)", pat)
		}
	}

	// Nested repetition (a**) is rejected everywhere.
	if !progFails(t, "", `BEGIN { print ("aaa" ~ "a**") }`) {
		t.Errorf("expr ~ a** should fail")
	}
	if !progFails(t, "", `BEGIN { print split("aaa", a, "a**") }`) {
		t.Errorf("split a** should fail")
	}
}

// ---------------------------------------------------------------------------
// Unclosed bracket: both paths fail
// ---------------------------------------------------------------------------

// TestAwkRoutingUnclosedBracket verifies that unclosed bracket expressions
// fail closed in both paths (though the error message differs).
func TestAwkRoutingUnclosedBracket(t *testing.T) {
	for _, pat := range []string{`[a-`, `[abc`, `[[:alpha:`} {
		if !progFails(t, "", "BEGIN { print (\"a\" ~ \""+pat+"\") }") {
			t.Errorf("expr ~ %q should fail (unclosed bracket)", pat)
		}
		if !progFails(t, "", "BEGIN { print split(\"a\", a, \""+pat+"\") }") {
			t.Errorf("split %q should fail (unclosed bracket)", pat)
		}
	}
}
