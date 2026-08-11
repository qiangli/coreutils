package bre

import (
	"bytes"
	"errors"
	"reflect"
	"regexp"
	"testing"
)

type fakeByteCtype struct {
	tables     bytePatternTables
	calls      map[string]int
	lowerCalls int
	failClass  string
	failErr    error
	lower      []byte
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

func TestCompileLocaleByteRegexpProviderErrors(t *testing.T) {
	sentinel := errors.New("provider failed")
	provider := newFakeByteCtype()
	provider.failClass, provider.failErr = "graph", sentinel
	if _, err := CompileLocaleByteRegexp([]byte("a"), provider, ByteRegexpOptions{}); !errors.Is(err, sentinel) {
		t.Fatalf("classification error=%v, want sentinel", err)
	}
	provider = newFakeByteCtype()
	provider.failClass, provider.failErr = "lowercase", sentinel
	if _, err := CompileLocaleByteRegexp([]byte("a"), provider, ByteRegexpOptions{}); !errors.Is(err, sentinel) {
		t.Fatalf("ToLower error=%v, want sentinel", err)
	}
	provider = newFakeByteCtype()
	provider.lower = provider.lower[:255]
	if _, err := CompileLocaleByteRegexp([]byte("a"), provider, ByteRegexpOptions{}); err == nil {
		t.Fatal("short lowercase table accepted")
	}
	if _, err := CompileLocaleByteRegexp([]byte("a"), nil, ByteRegexpOptions{}); err == nil {
		t.Fatal("nil provider accepted")
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
	if _, err := re.Expand(nil, nil, src, []int{0, 2}); err == nil {
		t.Fatal("invalid expansion offsets accepted")
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
}
