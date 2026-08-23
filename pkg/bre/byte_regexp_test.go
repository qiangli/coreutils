package bre

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sync"
	"testing"
)

type fakeByteCtype struct {
	tables     bytePatternTables
	calls      map[string]int
	lowerCalls int
	failClass  string
	failErr    error
	lower      []byte
	closed     bool
}

type fakeByteEquivalence struct{ *fakeByteCtype }

func (p fakeByteEquivalence) Equivalents(value byte) ([]byte, error) {
	if value == 'a' {
		return []byte{'a', 0xe4}, nil
	}
	return []byte{value}, nil
}

func newFakeByteCtype() *fakeByteCtype {
	tables := syntheticBytePatternTables()
	// Supply every named CType class, including empty synthetic classes.
	for _, name := range []string{"blank", "cntrl", "graph", "lower", "print", "punct", "upper", "xdigit"} {
		if _, ok := tables.classes[name]; !ok {
			tables.classes[name] = [256]bool{}
		}
	}
	lower := make([]byte, 256)
	copy(lower, tables.fold[:])
	return &fakeByteCtype{tables: tables, calls: make(map[string]int), lower: lower}
}

func (p *fakeByteCtype) classify(name string, value byte) (bool, error) {
	p.calls[name]++
	if p.closed {
		return false, errors.New("provider is closed")
	}
	if p.failClass == name {
		return false, p.failErr
	}
	return p.tables.classes[name][value], nil
}

func (p *fakeByteCtype) IsAlpha(b byte) (bool, error)  { return p.classify("alpha", b) }
func (p *fakeByteCtype) IsAlnum(b byte) (bool, error)  { return p.classify("alnum", b) }
func (p *fakeByteCtype) IsBlank(b byte) (bool, error)  { return p.classify("blank", b) }
func (p *fakeByteCtype) IsCntrl(b byte) (bool, error)  { return p.classify("cntrl", b) }
func (p *fakeByteCtype) IsDigit(b byte) (bool, error)  { return p.classify("digit", b) }
func (p *fakeByteCtype) IsGraph(b byte) (bool, error)  { return p.classify("graph", b) }
func (p *fakeByteCtype) IsLower(b byte) (bool, error)  { return p.classify("lower", b) }
func (p *fakeByteCtype) IsPrint(b byte) (bool, error)  { return p.classify("print", b) }
func (p *fakeByteCtype) IsPunct(b byte) (bool, error)  { return p.classify("punct", b) }
func (p *fakeByteCtype) IsSpace(b byte) (bool, error)  { return p.classify("space", b) }
func (p *fakeByteCtype) IsUpper(b byte) (bool, error)  { return p.classify("upper", b) }
func (p *fakeByteCtype) IsXDigit(b byte) (bool, error) { return p.classify("xdigit", b) }
func (p *fakeByteCtype) ToLower(in []byte) ([]byte, error) {
	p.lowerCalls++
	if p.closed {
		return nil, errors.New("provider is closed")
	}
	if p.failClass == "lowercase" {
		return nil, p.failErr
	}
	return append([]byte(nil), p.lower...), nil
}

func TestCompileLocaleByteRegexpSnapshotsAllCtypeTables(t *testing.T) {
	provider := newFakeByteCtype()
	re, err := CompileLocaleByteRegexp([]byte(`[[:alpha:]]`), provider, ByteRegexpOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alpha", "alnum", "blank", "cntrl", "digit", "graph", "lower", "print", "punct", "space", "upper", "xdigit"} {
		if got := provider.calls[name]; got != 256 {
			t.Errorf("%s calls=%d, want 256", name, got)
		}
	}
	if provider.lowerCalls != 1 {
		t.Fatalf("ToLower calls=%d, want 1", provider.lowerCalls)
	}
	tables, err := buildBytePatternTables(provider)
	if err != nil {
		t.Fatal(err)
	}
	if len(tables.classes) != 13 {
		t.Fatalf("class map length=%d, want 12 CType classes plus word", len(tables.classes))
	}
	alpha := provider.tables.classes["alpha"]
	alpha['a'] = false
	provider.tables.classes["alpha"] = alpha
	if matched, err := re.MatchString("a"); err != nil || !matched {
		t.Fatalf("compiled snapshot changed: matched=%v err=%v", matched, err)
	}
}

func TestCompileLocaleByteRegexpSnapshotsEquivalenceClasses(t *testing.T) {
	provider := fakeByteEquivalence{newFakeByteCtype()}
	tables, err := SnapshotLocaleByteTables(provider)
	if err != nil {
		t.Fatal(err)
	}
	provider.closed = true
	re, err := CompileLocaleByteRegexpTables([]byte(`[[=a=]]`), tables, ByteRegexpOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if matched, err := re.MatchString(string([]byte{0xe4})); err != nil || !matched {
		t.Fatalf("equivalence class matched=%v err=%v", matched, err)
	}
}

func TestCompileLocaleByteRegexpProviderErrors(t *testing.T) {
	sentinel := errors.New("provider failed")
	provider := newFakeByteCtype()
	provider.failClass, provider.failErr = "graph", sentinel
	if _, err := SnapshotLocaleByteTables(provider); !errors.Is(err, sentinel) {
		t.Fatalf("classification error=%v, want sentinel", err)
	}
	provider = newFakeByteCtype()
	provider.failClass, provider.failErr = "lowercase", sentinel
	if _, err := SnapshotLocaleByteTables(provider); !errors.Is(err, sentinel) {
		t.Fatalf("ToLower error=%v, want sentinel", err)
	}
	provider = newFakeByteCtype()
	provider.lower = provider.lower[:255]
	if _, err := SnapshotLocaleByteTables(provider); err == nil {
		t.Fatal("short lowercase table accepted")
	}
	if _, err := SnapshotLocaleByteTables(nil); err == nil {
		t.Fatal("nil provider accepted")
	}
	if _, err := CompileLocaleByteRegexpTables([]byte("a"), nil, ByteRegexpOptions{}); err == nil {
		t.Fatal("nil tables accepted")
	}
}

func TestLocaleByteTablesReusableAfterProviderClose(t *testing.T) {
	provider := newFakeByteCtype()
	tables, err := SnapshotLocaleByteTables(provider)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alpha", "alnum", "blank", "cntrl", "digit", "graph", "lower", "print", "punct", "space", "upper", "xdigit"} {
		if provider.calls[name] != 256 {
			t.Fatalf("%s calls=%d, want 256", name, provider.calls[name])
		}
	}
	if provider.lowerCalls != 1 {
		t.Fatalf("ToLower calls=%d, want 1", provider.lowerCalls)
	}
	provider.closed = true
	for i := 0; i < 8; i++ {
		options := ByteRegexpOptions{Syntax: ByteRegexpBRE, FoldCase: i%2 == 0}
		pattern := []byte(`^\(A\|b\)\+$`)
		if i%2 != 0 {
			options.Syntax = ByteRegexpERE
			pattern = []byte(`^(a|b)+$`)
		}
		re, err := CompileLocaleByteRegexpTables(pattern, tables, options)
		if err != nil {
			t.Fatal(err)
		}
		if matched, err := re.MatchString("ab"); err != nil || !matched {
			t.Fatalf("compile %d matched=%v err=%v", i, matched, err)
		}
	}
	for _, calls := range provider.calls {
		if calls != 256 {
			t.Fatal("compile called closed provider classification")
		}
	}
	if provider.lowerCalls != 1 {
		t.Fatal("compile called closed provider ToLower")
	}
}

func TestLocaleByteTablesConcurrentCompileAndMatch(t *testing.T) {
	tables, err := SnapshotLocaleByteTables(newFakeByteCtype())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for worker := 0; worker < 16; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			options := ByteRegexpOptions{Syntax: ByteRegexpSyntax(worker % 2), FoldCase: true, DotAll: worker%3 == 0, MultiLine: worker%5 == 0}
			pattern := []byte(`^\(A\|b\)\+$`)
			if options.Syntax == ByteRegexpERE {
				pattern = []byte(`^(A|b)+$`)
			}
			for iteration := 0; iteration < 10; iteration++ {
				re, err := CompileLocaleByteRegexpTables(pattern, tables, options)
				if err != nil {
					errs <- err
					return
				}
				matched, err := re.MatchString("ab")
				if err != nil || !matched {
					errs <- fmt.Errorf("matched=%v: %w", matched, err)
					return
				}
			}
		}(worker)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestLocaleByteRegexpBREAndERE(t *testing.T) {
	provider := newFakeByteCtype()
	bre, err := CompileLocaleByteRegexp([]byte(`^\(a\|b\)\+$`), provider, ByteRegexpOptions{Syntax: ByteRegexpBRE})
	if err != nil {
		t.Fatal(err)
	}
	if matched, err := bre.MatchString("ab"); err != nil || !matched {
		t.Fatalf("BRE matched=%v err=%v", matched, err)
	}
	ere, err := CompileLocaleByteRegexp([]byte(`^(A|b)+$`), newFakeByteCtype(), ByteRegexpOptions{Syntax: ByteRegexpERE, FoldCase: true})
	if err != nil {
		t.Fatal(err)
	}
	if matched, err := ere.MatchString("aB"); err != nil || !matched {
		t.Fatalf("ERE matched=%v err=%v", matched, err)
	}
	if _, err := CompileLocaleByteRegexp([]byte("a"), newFakeByteCtype(), ByteRegexpOptions{Syntax: 99}); err == nil {
		t.Fatal("unknown syntax accepted")
	}
}

func TestLocaleByteRegexpZeroLengthSingleMatch(t *testing.T) {
	empty, err := CompileLocaleByteRegexp(nil, newFakeByteCtype(), ByteRegexpOptions{Syntax: ByteRegexpERE})
	if err != nil {
		t.Fatal(err)
	}
	got, err := empty.FindSubmatchIndex([]byte{0xfd, '\n', 0xc4})
	if err != nil {
		t.Fatal(err)
	}
	if want := []int{0, 0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("zero-length Find=%v, want %v", got, want)
	}
}

func TestLocaleByteRegexpFindAllRawDifferential(t *testing.T) {
	type oracle func([]byte) [][]int
	cases := []struct {
		name    string
		pattern []byte
		want    oracle
	}{
		{"empty", nil, oracleEmptyMatches},
		{"optional", []byte(`(a)?`), oracleOptionalA},
		{"star", []byte(`(a)*`), oracleStarA},
		{"repeat", []byte(`(a)+`), oracleRunA},
		{"alternation-high", []byte{'(', 'a', '|', 0xe9, ')', '+'}, oracleRunAOrHigh},
	}
	for _, tc := range cases {
		re, err := CompileLocaleByteRegexp(tc.pattern, newFakeByteCtype(), ByteRegexpOptions{Syntax: ByteRegexpERE})
		if err != nil {
			t.Fatal(err)
		}
		for _, src := range generatedByteSubjects([]byte{'a', 'b', '\n', 0xe9}, 3) {
			all := tc.want(src)
			for _, n := range []int{0, 1, 2, -1} {
				want := limitOracleMatches(all, n)
				got, err := re.FindAllSubmatchIndex(src, n)
				if err != nil {
					t.Fatalf("%s n=%d src=%v: %v", tc.name, n, src, err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("%s n=%d src=%v: got %v, want %v", tc.name, n, src, got, want)
				}
				stringGot, err := re.FindAllStringSubmatchIndex(string(src), n)
				if err != nil || !reflect.DeepEqual(stringGot, want) {
					t.Fatalf("%s string n=%d src=%v: got %v, want %v err=%v", tc.name, n, src, stringGot, want, err)
				}
			}
		}
	}
}

func limitOracleMatches(all [][]int, n int) [][]int {
	if n == 0 || len(all) == 0 {
		return nil
	}
	if n < 0 || n >= len(all) {
		return all
	}
	return all[:n]
}

func generatedByteSubjects(alphabet []byte, maxLen int) [][]byte {
	result := [][]byte{nil}
	var extend func([]byte, int)
	extend = func(prefix []byte, remaining int) {
		if remaining == 0 {
			return
		}
		for _, b := range alphabet {
			next := append(append([]byte(nil), prefix...), b)
			result = append(result, next)
			extend(next, remaining-1)
		}
	}
	extend(nil, maxLen)
	return result
}

func oracleEmptyMatches(src []byte) [][]int {
	result := make([][]int, len(src)+1)
	for i := range result {
		result[i] = []int{i, i}
	}
	return result
}

func oracleOptionalA(src []byte) [][]int {
	var result [][]int
	previousNonEmptyEnd := -1
	for i := 0; i <= len(src); {
		if i < len(src) && src[i] == 'a' {
			result = append(result, []int{i, i + 1, i, i + 1})
			i++
			previousNonEmptyEnd = i
		} else {
			if i != previousNonEmptyEnd {
				result = append(result, []int{i, i, -1, -1})
			}
			i++
		}
	}
	return result
}

func oracleStarA(src []byte) [][]int {
	var result [][]int
	previousNonEmptyEnd := -1
	for i := 0; i <= len(src); {
		if i < len(src) && src[i] == 'a' {
			start := i
			for i < len(src) && src[i] == 'a' {
				i++
			}
			result = append(result, []int{start, i, i - 1, i})
			previousNonEmptyEnd = i
		} else {
			if i != previousNonEmptyEnd {
				result = append(result, []int{i, i, -1, -1})
			}
			i++
		}
	}
	return result
}

func oracleRunA(src []byte) [][]int { return oracleRuns(src, func(b byte) bool { return b == 'a' }) }
func oracleRunAOrHigh(src []byte) [][]int {
	return oracleRuns(src, func(b byte) bool { return b == 'a' || b == 0xe9 })
}

func oracleRuns(src []byte, member func(byte) bool) [][]int {
	var result [][]int
	for i := 0; i < len(src); {
		if !member(src[i]) {
			i++
			continue
		}
		start := i
		for i < len(src) && member(src[i]) {
			i++
		}
		result = append(result, []int{start, i, i - 1, i})
	}
	return result
}

func TestLocaleByteRegexpDotAllAndMultiLine(t *testing.T) {
	dot, err := CompileLocaleByteRegexp([]byte(`.`), newFakeByteCtype(), ByteRegexpOptions{Syntax: ByteRegexpERE, DotAll: true})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := dot.FindSubmatchIndex([]byte{'\n'}); err != nil || !reflect.DeepEqual(got, []int{0, 1}) {
		t.Fatalf("DotAll newline indices=%v err=%v", got, err)
	}
	multiline, err := CompileLocaleByteRegexp([]byte(`^a$`), newFakeByteCtype(), ByteRegexpOptions{Syntax: ByteRegexpERE, MultiLine: true})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := multiline.FindSubmatchIndex([]byte("x\na\ny")); err != nil || !reflect.DeepEqual(got, []int{2, 3}) {
		t.Fatalf("MultiLine indices=%v err=%v", got, err)
	}
}

func TestLocaleByteRegexpExpandRawBytes(t *testing.T) {
	re, err := CompileLocaleByteRegexp([]byte(`^(.)$`), newFakeByteCtype(), ByteRegexpOptions{Syntax: ByteRegexpERE})
	if err != nil {
		t.Fatal(err)
	}
	src := []byte{0xe9}
	match, err := re.FindSubmatchIndex(src)
	if err != nil {
		t.Fatal(err)
	}
	got, err := re.Expand(nil, []byte(`x${1}y`), src, match)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte{'x', 0xe9, 'y'}) {
		t.Fatalf("Expand=%v", got)
	}
	got, err = re.ExpandString(nil, `<${1}>`, string(src), match)
	if err != nil || !bytes.Equal(got, []byte{'<', 0xe9, '>'}) {
		t.Fatalf("ExpandString=%v err=%v", got, err)
	}
	invalid := map[string][]int{
		"missing capture":   {0, 1},
		"extra capture":     {0, 1, 0, 1, -1, -1},
		"absent whole":      {-1, -1, -1, -1},
		"reversed whole":    {1, 0, -1, -1},
		"whole past source": {0, 2, 0, 1},
		"half absent start": {0, 1, -1, 1},
		"half absent end":   {0, 1, 0, -1},
		"capture before":    {1, 1, 0, 1},
		"capture after":     {0, 0, 0, 1},
	}
	for name, indices := range invalid {
		t.Run(name, func(t *testing.T) {
			if _, err := re.Expand(nil, nil, src, indices); err == nil {
				t.Error("Expand accepted invalid vector")
			}
			if _, err := re.ExpandString(nil, "", string(src), indices); err == nil {
				t.Error("ExpandString accepted invalid vector")
			}
		})
	}
	absent, err := CompileLocaleByteRegexp([]byte(`^(a)?b$`), newFakeByteCtype(), ByteRegexpOptions{Syntax: ByteRegexpERE})
	if err != nil {
		t.Fatal(err)
	}
	absentMatch, err := absent.FindSubmatchIndex([]byte("b"))
	if err != nil || !reflect.DeepEqual(absentMatch, []int{0, 1, -1, -1}) {
		t.Fatalf("absent capture indices=%v err=%v", absentMatch, err)
	}
	if got, err := absent.ExpandString(nil, `x${1}y`, "b", absentMatch); err != nil || string(got) != "xy" {
		t.Fatalf("absent capture expansion=%q err=%v", got, err)
	}
}

func TestLocaleByteRegexpNeverSwallowsInvariantError(t *testing.T) {
	re, err := CompileLocaleByteRegexp([]byte("a"), newFakeByteCtype(), ByteRegexpOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately violate the compiler invariant: one RE2 rune consumes half
	// of the two-rune token. Public methods must surface the mapping failure.
	re.pattern.re = regexp.MustCompile(`.`)
	if _, err := re.FindSubmatchIndex([]byte{'a'}); err == nil {
		t.Fatal("FindSubmatchIndex swallowed invariant error")
	}
	if _, err := re.MatchString("a"); err == nil {
		t.Fatal("MatchString swallowed invariant error")
	}
	if _, err := re.FindAllSubmatchIndex([]byte{'a'}, -1); err == nil {
		t.Fatal("FindAllSubmatchIndex swallowed whole-match boundary error")
	}
	re.pattern.re = regexp.MustCompile(`(.).`)
	if _, err := re.FindAllSubmatchIndex([]byte{'a'}, -1); err == nil {
		t.Fatal("FindAllSubmatchIndex swallowed capture-boundary error")
	}
}
