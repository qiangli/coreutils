package bre

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func syntheticBytePatternTables() bytePatternTables {
	classes := make(map[string][256]bool)
	var word, space, alpha, digit [256]bool
	for b := byte('a'); b <= 'z'; b++ {
		word[b], alpha[b] = true, true
	}
	for b := byte('A'); b <= 'Z'; b++ {
		word[b], alpha[b] = true, true
	}
	for b := byte('0'); b <= '9'; b++ {
		word[b], digit[b] = true, true
	}
	word['_'] = true
	// Synthetic single-byte locale letters.
	word[0xc9], alpha[0xc9] = true, true
	word[0xe9], alpha[0xe9] = true, true
	for _, b := range []byte{' ', '\t', '\n', '\r'} {
		space[b] = true
	}
	classes["word"] = word
	classes["space"] = space
	classes["alpha"] = alpha
	classes["digit"] = digit
	var fold [256]byte
	for i := range fold {
		fold[i] = byte(i)
	}
	for b := byte('A'); b <= 'Z'; b++ {
		fold[b] = b + ('a' - 'A')
	}
	fold[0xc9] = 0xe9
	return bytePatternTables{classes: classes, fold: fold}
}

func compileSyntheticBytePattern(t *testing.T, pattern []byte, fold bool) *localeBytePattern {
	t.Helper()
	p, err := compileLocaleBytePattern(pattern, syntheticBytePatternTables(), fold)
	if err != nil {
		t.Fatalf("compileLocaleBytePattern(%q): %v", pattern, err)
	}
	return p
}

func TestLocaleBytePatternRawCapturesAndOffsets(t *testing.T) {
	p := compileSyntheticBytePattern(t, []byte(`^\(A\+\)\(.\)$`), true)
	raw := []byte{'a', 'A', 0xe9}
	got, err := p.findSubmatchIndex(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{0, 3, 0, 2, 2, 3}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("submatch indices=%v, want %v", got, want)
	}
	if !bytes.Equal(raw[got[4]:got[5]], []byte{0xe9}) {
		t.Fatalf("high-byte capture=%v", raw[got[4]:got[5]])
	}
}

func TestLocaleBytePatternLiteralHighByteAndFold(t *testing.T) {
	for _, tc := range []struct {
		name    string
		pattern []byte
		input   []byte
		fold    bool
		match   bool
	}{
		{"raw high literal", []byte{0xe9}, []byte{0xe9}, false, true},
		{"high fold", []byte{0xc9}, []byte{0xe9}, true, true},
		{"high no fold", []byte{0xc9}, []byte{0xe9}, false, false},
		{"ASCII fold", []byte{'A'}, []byte{'a'}, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := compileSyntheticBytePattern(t, tc.pattern, tc.fold)
			got, err := p.findSubmatchIndex(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if (got != nil) != tc.match {
				t.Fatalf("indices=%v, want match=%v", got, tc.match)
			}
		})
	}
	tables := syntheticBytePatternTables()
	codec, err := newByteTokenCodec(tables.classes["word"])
	if err != nil {
		t.Fatal(err)
	}
	translated, err := translateLocaleByteBRE([]byte{'A'}, codec, tables, true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(translated, "(?i") {
		t.Fatalf("locale fold leaked RE2 case flag into %q", translated)
	}
}

func TestLocaleBytePatternDoesNotMatchAcrossTokenSeam(t *testing.T) {
	tables := syntheticBytePatternTables()
	word := tables.classes["word"]
	word[0xfd], word[0xc4], word['B'] = true, true, true
	tables.classes["word"] = word
	p, err := compileLocaleBytePattern([]byte{'B'}, tables, false)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := p.findSubmatchIndex([]byte{0xfd, 0xc4}); err != nil || got != nil {
		t.Fatalf("false seam probe indices=%v err=%v, want no match", got, err)
	}
	if got, err := p.findSubmatchIndex([]byte{0xfd, 0xc4, 'B'}); err != nil || !reflect.DeepEqual(got, []int{2, 3}) {
		t.Fatalf("later literal probe indices=%v err=%v, want [2 3]", got, err)
	}
}

func TestLocaleBytePatternDotAndLiteralNewline(t *testing.T) {
	dot := compileSyntheticBytePattern(t, []byte{'.'}, false)
	for _, tc := range []struct {
		input []byte
		match bool
	}{
		{[]byte{0xff}, true},
		{[]byte{'\n'}, false},
	} {
		got, err := dot.findSubmatchIndex(tc.input)
		if err != nil {
			t.Fatal(err)
		}
		if (got != nil) != tc.match {
			t.Errorf("dot input=%v indices=%v, want match=%v", tc.input, got, tc.match)
		}
	}
	literal := compileSyntheticBytePattern(t, []byte{'\n'}, false)
	if got, err := literal.findSubmatchIndex([]byte{'\n'}); err != nil || !reflect.DeepEqual(got, []int{0, 1}) {
		t.Fatalf("literal newline indices=%v err=%v, want [0 1]", got, err)
	}
}

func TestLocaleBytePatternClasses(t *testing.T) {
	for _, tc := range []struct {
		pattern string
		input   byte
		fold    bool
		match   bool
	}{
		{`[[:alpha:]]`, 0xe9, false, true},
		{`[^[:alpha:]]`, '!', false, true},
		{`[^[:alpha:]]`, 'a', false, false},
		{`\w`, 0xc9, false, true},
		{`\W`, '!', false, true},
		{`\s`, '\n', false, true},
		{`\S`, '\n', false, false},
		{`[A]`, 'a', true, true},
		{`[^A]`, 'a', true, false},
	} {
		p := compileSyntheticBytePattern(t, []byte(tc.pattern), tc.fold)
		got, err := p.findSubmatchIndex([]byte{tc.input})
		if err != nil {
			t.Fatal(err)
		}
		if (got != nil) != tc.match {
			t.Errorf("%q input=%#x indices=%v, want match=%v", tc.pattern, tc.input, got, tc.match)
		}
	}
}

func TestLocaleBytePatternGroupsAlternationAndRepeats(t *testing.T) {
	for _, tc := range []struct {
		pattern string
		input   string
		want    []int
	}{
		{`^\(ab\|a\)$`, "ab", []int{0, 2, 0, 2}},
		{`^a\{2,3\}$`, "aaa", []int{0, 3}},
		{`^ab\?$`, "a", []int{0, 1}},
		{`^ab*$`, "abbb", []int{0, 4}},
	} {
		p := compileSyntheticBytePattern(t, []byte(tc.pattern), false)
		got, err := p.findSubmatchIndex([]byte(tc.input))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q on %q indices=%v, want %v", tc.pattern, tc.input, got, tc.want)
		}
	}
}

func TestLocaleBytePatternTablesAreSnapshot(t *testing.T) {
	tables := syntheticBytePatternTables()
	p, err := compileLocaleBytePattern([]byte(`[[:alpha:]]`), tables, false)
	if err != nil {
		t.Fatal(err)
	}
	alpha := tables.classes["alpha"]
	alpha['a'] = false
	tables.classes["alpha"] = alpha
	if got, err := p.findSubmatchIndex([]byte{'a'}); err != nil || got == nil {
		t.Fatalf("compiled class changed with caller tables: indices=%v err=%v", got, err)
	}
}

func TestLocaleBytePatternFailsClosed(t *testing.T) {
	patterns := []string{
		`\1`, `\b`, `\B`, `\<`, `\>`,
		`[a-z]`, `[[=a=]]`, `[[.a.]]`, `[[:bogus:]]`, `[[:alpha:]`,
		`\{2\}`, `\}`, `a**`, `\q`, `\`,
	}
	for _, pattern := range patterns {
		t.Run(strings.ReplaceAll(pattern, "/", "_"), func(t *testing.T) {
			if _, err := compileLocaleBytePattern([]byte(pattern), syntheticBytePatternTables(), false); err == nil {
				t.Fatalf("compileLocaleBytePattern(%q) succeeded", pattern)
			}
		})
	}
}

func TestLocaleBytePatternRequiresCoreTables(t *testing.T) {
	tables := syntheticBytePatternTables()
	delete(tables.classes, "word")
	if _, err := compileLocaleBytePattern([]byte("a"), tables, false); err == nil {
		t.Fatal("missing word class accepted")
	}
	tables = syntheticBytePatternTables()
	delete(tables.classes, "space")
	if _, err := compileLocaleBytePattern([]byte("a"), tables, false); err == nil {
		t.Fatal("missing space class accepted")
	}
}
