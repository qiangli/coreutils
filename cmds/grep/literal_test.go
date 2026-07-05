package grepcmd

import (
	"strings"
	"testing"
)

func TestLiteralPattern(t *testing.T) {
	tests := []struct {
		pats       []string
		fixed      bool
		ignoreCase bool
		word       bool
		want       bool
	}{
		{[]string{"ERROR"}, false, false, false, true},
		{[]string{""}, false, false, false, true},
		{[]string{"a b/c-d_e"}, false, false, false, true},
		{[]string{"ERROR", "WARN"}, false, false, false, false}, // multiple patterns
		{[]string{"ERROR"}, false, true, false, false},          // -i
		{[]string{"ERROR"}, false, false, true, false},          // -w
		{[]string{"a.b"}, false, false, false, false},           // metachar
		{[]string{"a*"}, false, false, false, false},
		{[]string{`a\+`}, false, false, false, false},
		{[]string{"a+b"}, false, false, false, false}, // ERE metachar, conservative for BRE too
		{[]string{"a{2}"}, false, false, false, false},
		{[]string{"a|b"}, false, false, false, false},
		{[]string{"a.b"}, true, false, false, true}, // -F: everything is literal
		{[]string{"a|b"}, true, false, false, true},
	}
	for _, tt := range tests {
		lit, ok := literalPattern(tt.pats, tt.fixed, tt.ignoreCase, tt.word)
		if ok != tt.want {
			t.Errorf("literalPattern(%q, fixed=%v, i=%v, w=%v) = %v, want %v",
				tt.pats, tt.fixed, tt.ignoreCase, tt.word, ok, tt.want)
		}
		if ok && string(lit) != tt.pats[0] {
			t.Errorf("literalPattern(%q) lit = %q", tt.pats, lit)
		}
	}
}

// TestLiteralFastPathDifferential proves the fast path is byte-identical
// to the RE2 path: the same pattern given twice via -e compiles to an
// equivalent regexp but disqualifies the fast path (len(split) != 1), so
// each case runs once per engine and the outputs must match exactly.
func TestLiteralFastPathDifferential(t *testing.T) {
	inputs := map[string]string{
		"plain":       "hello world\nfoo bar\nHELLO again\nfoo\n",
		"empty":       "",
		"emptyLines":  "\n\nfoo\n\n",
		"noTrailNL":   "one foo\ntwo",
		"crlf":        "foo\r\nbar\r\n",
		"binary":      "abc\x00def foo\nfoo again\nmore\n",
		"longLines":   strings.Repeat("x", 200000) + "foo" + strings.Repeat("y", 200000) + "\nplain foo line\n" + strings.Repeat("z", 300000) + "\n",
		"manyMatches": strings.Repeat("a foo b\nno match\n", 5000),
		"lastLineHit": "aaa\nbbb\nccc foo",
	}
	patterns := []string{"foo", "", "o", "xyz-not-there", "two"}
	flagSets := [][]string{
		{},
		{"-v"},
		{"-c"},
		{"-n"},
		{"-x"},
		{"-l"},
		{"-L"},
		{"-q"},
		{"-H"},
		{"-m", "1"},
		{"-m", "3"},
		{"-n", "-v"},
		{"-c", "-v"},
		{"-n", "-m", "2"},
		{"-x", "-v"},
		{"-F"},
		{"-F", "-x"},
	}
	for inName, input := range inputs {
		for _, pat := range patterns {
			for _, flags := range flagSets {
				fast := append(append([]string{}, flags...), "-e", pat)
				slow := append(append([]string{}, flags...), "-e", pat, "-e", pat)
				fo, fe, fc := runGrep(t, "", input, fast...)
				so, se, sc := runGrep(t, "", input, slow...)
				if fo != so || fe != se || fc != sc {
					t.Errorf("input=%s pat=%q flags=%v:\n fast: out=%q err=%q code=%d\n  re2: out=%q err=%q code=%d",
						inName, pat, flags, fo, fe, fc, so, se, sc)
				}
			}
		}
	}
}

// TestLiteralFastPathXWholeLine pins -x semantics on the fast path.
func TestLiteralFastPathXWholeLine(t *testing.T) {
	out, _, code := runGrep(t, "", "foo\nfoobar\nfoo\r\n", "-x", "-n", "foo")
	if out != "1:foo\n" || code != 0 {
		t.Errorf("-x fast path: out=%q code=%d", out, code)
	}
	// -x with the empty pattern selects only empty lines.
	out, _, code = runGrep(t, "", "a\n\nb\n", "-x", "-n", "-e", "")
	if out != "2:\n" || code != 0 {
		t.Errorf("-x empty pattern: out=%q code=%d", out, code)
	}
}
