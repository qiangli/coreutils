package bre

import (
	"reflect"
	"strings"
	"testing"
)

func compileSyntheticByteERE(t *testing.T, pattern []byte, fold bool) *localeBytePattern {
	t.Helper()
	p, err := compileLocaleByteERE(pattern, syntheticBytePatternTables(), fold)
	if err != nil {
		t.Fatalf("compileLocaleByteERE(%q): %v", pattern, err)
	}
	return p
}

func TestLocaleByteEREHighByteCaptureFoldAlternation(t *testing.T) {
	pattern := append([]byte("^("), 0xc9)
	pattern = append(pattern, []byte("+|x)$")...)
	p := compileSyntheticByteERE(t, pattern, true)
	got, err := p.findSubmatchIndex([]byte{0xe9, 0xc9})
	if err != nil {
		t.Fatal(err)
	}
	want := []int{0, 2, 0, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("submatch indices=%v, want %v", got, want)
	}
}

func TestLocaleByteEREOperatorsAndAnchors(t *testing.T) {
	for _, tc := range []struct {
		pattern string
		input   string
		want    []int
	}{
		{`^(ab|a)+$`, "aba", []int{0, 3, 2, 3}},
		{`^a{2,3}$`, "aaa", []int{0, 3}},
		{`^ab?$`, "a", []int{0, 1}},
		{`^ab*$`, "abbb", []int{0, 4}},
		{`^\(\)$`, "()", []int{0, 2}},
	} {
		p := compileSyntheticByteERE(t, []byte(tc.pattern), false)
		got, err := p.findSubmatchIndex([]byte(tc.input))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q on %q indices=%v, want %v", tc.pattern, tc.input, got, tc.want)
		}
	}
}

func TestLocaleByteEREClassesDotAndNewline(t *testing.T) {
	for _, tc := range []struct {
		pattern string
		input   byte
		fold    bool
		match   bool
	}{
		{`[[:alpha:]]`, 0xe9, false, true},
		{`[^A]`, 'a', true, false},
		{`\w`, 0xc9, false, true},
		{`\W`, '!', false, true},
		{`\s`, '\n', false, true},
		{`\S`, '\n', false, false},
		{`.`, 0xff, false, true},
		{`.`, '\n', false, false},
	} {
		p := compileSyntheticByteERE(t, []byte(tc.pattern), tc.fold)
		got, err := p.findSubmatchIndex([]byte{tc.input})
		if err != nil {
			t.Fatal(err)
		}
		if (got != nil) != tc.match {
			t.Errorf("%q input=%#x indices=%v, want match=%v", tc.pattern, tc.input, got, tc.match)
		}
	}
}

func TestLocaleByteEREAndBREDistinguishOperators(t *testing.T) {
	pattern := []byte(`^(a|b)+$`)
	ere := compileSyntheticByteERE(t, pattern, false)
	if got, err := ere.findSubmatchIndex([]byte("ab")); err != nil || got == nil {
		t.Fatalf("ERE operators did not match: indices=%v err=%v", got, err)
	}
	bre := compileSyntheticBytePattern(t, pattern, false)
	if got, err := bre.findSubmatchIndex([]byte("ab")); err != nil || got != nil {
		t.Fatalf("BRE treated unescaped ERE operators as operators: indices=%v err=%v", got, err)
	}
}

func TestLocaleByteEREFailsClosed(t *testing.T) {
	patterns := []string{
		`\1`, `\b`, `\B`, `\<`, `\>`, `\q`, `\`,
		`[a-z]`, `[[=a=]]`, `[[.a.]]`, `[[:bogus:]]`, `[[:alpha:]`,
		`*a`, `a**`, `a+?`, `{2}`, `a{2`, `a{2,1}`, `a{1001}`, `a}`,
		`(a`, `a)`, `()`, `a|`, `|a`, `(a|)`,
	}
	for _, pattern := range patterns {
		t.Run(strings.ReplaceAll(pattern, "/", "_"), func(t *testing.T) {
			if _, err := compileLocaleByteERE([]byte(pattern), syntheticBytePatternTables(), false); err == nil {
				t.Fatalf("compileLocaleByteERE(%q) succeeded", pattern)
			}
		})
	}
}

func TestLocaleByteERERequiresCoreTables(t *testing.T) {
	tables := syntheticBytePatternTables()
	delete(tables.classes, "word")
	if _, err := compileLocaleByteERE([]byte("a"), tables, false); err == nil {
		t.Fatal("missing word class accepted")
	}
	tables = syntheticBytePatternTables()
	delete(tables.classes, "space")
	if _, err := compileLocaleByteERE([]byte("a"), tables, false); err == nil {
		t.Fatal("missing space class accepted")
	}
}
