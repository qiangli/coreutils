package awkcmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

type fakeAwkCType struct {
	classErr   error
	closeErr   error
	closeCalls int
}

type fakeAwkCollate struct{ closeCalls int }

func (p *fakeAwkCollate) Equivalents(value byte) ([]byte, error) {
	if value == 0 {
		return nil, nil
	}
	if value == 'e' || value == 'E' || value == 0xe9 || value == 0xc9 {
		return []byte{'E', 'e', 0xc9, 0xe9}, nil
	}
	return []byte{value}, nil
}
func (p *fakeAwkCollate) EquivalenceClasses() ([]bool, error) {
	result := make([]bool, 256)
	for i := 1; i < len(result); i++ {
		result[i] = true
	}
	return result, nil
}
func (p *fakeAwkCollate) CollationWeights() ([]byte, error) {
	result := make([]byte, 256)
	for i := range result {
		result[i] = byte(i)
	}
	return result, nil
}
func (p *fakeAwkCollate) CollatingElements() ([]bool, error) {
	result := make([]bool, 256)
	for i := 1; i < len(result); i++ {
		result[i] = true
	}
	return result, nil
}
func (p *fakeAwkCollate) Close() error { p.closeCalls++; return nil }

func (p *fakeAwkCType) classify(b byte, member bool) (bool, error) {
	if p.classErr != nil {
		return false, p.classErr
	}
	return member, nil
}
func awkAlpha(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b == 0xc9 || b == 0xe9
}
func (p *fakeAwkCType) IsAlpha(b byte) (bool, error) { return p.classify(b, awkAlpha(b)) }
func (p *fakeAwkCType) IsAlnum(b byte) (bool, error) {
	return p.classify(b, awkAlpha(b) || b >= '0' && b <= '9')
}
func (p *fakeAwkCType) IsBlank(b byte) (bool, error) { return p.classify(b, b == ' ' || b == '\t') }
func (p *fakeAwkCType) IsCntrl(b byte) (bool, error) { return p.classify(b, b < 0x20 || b == 0x7f) }
func (p *fakeAwkCType) IsDigit(b byte) (bool, error) { return p.classify(b, b >= '0' && b <= '9') }
func (p *fakeAwkCType) IsGraph(b byte) (bool, error) { return p.classify(b, b > 0x20 && b != 0x7f) }
func (p *fakeAwkCType) IsLower(b byte) (bool, error) {
	return p.classify(b, b >= 'a' && b <= 'z' || b == 0xe9)
}
func (p *fakeAwkCType) IsPrint(b byte) (bool, error) { return p.classify(b, b >= 0x20 && b != 0x7f) }
func (p *fakeAwkCType) IsPunct(b byte) (bool, error) { return p.classify(b, b == '!' || b == '.') }
func (p *fakeAwkCType) IsSpace(b byte) (bool, error) {
	return p.classify(b, b == ' ' || b == '\t' || b == '\n' || b == '\r')
}
func (p *fakeAwkCType) IsUpper(b byte) (bool, error) {
	return p.classify(b, b >= 'A' && b <= 'Z' || b == 0xc9)
}
func (p *fakeAwkCType) IsXDigit(b byte) (bool, error) {
	return p.classify(b, b >= '0' && b <= '9' || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F')
}
func (p *fakeAwkCType) ToLower(in []byte) ([]byte, error) {
	if p.classErr != nil {
		return nil, p.classErr
	}
	out := append([]byte(nil), in...)
	for i, b := range out {
		if b >= 'A' && b <= 'Z' {
			out[i] = b + 32
		}
		if b == 0xc9 {
			out[i] = 0xe9
		}
	}
	return out, nil
}
func (p *fakeAwkCType) Close() error { p.closeCalls++; return p.closeErr }

func runAwkLocale(env []string, input io.Reader, args []string, opener ctypeOpener) (string, string, int) {
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Env: env, Stdio: tool.Stdio{In: input, Out: &out, Err: &errOut}}
	code := runWithCType(rc, args, opener)
	return out.String(), errOut.String(), code
}

func TestAwkLocaleRegexEveryEndpoint(t *testing.T) {
	alpha := `[[:alpha:]]`
	tests := []struct{ name, program, input, want string }{
		{"literal", `/[[:alpha:]]/ { print "yes" }`, string([]byte{0xe9, '\n'}), "yes\n"},
		{"tilde-match", `BEGIN { s="\351"; print (s ~ "` + alpha + `"), match(s,"` + alpha + `") }`, "", "1 1\n"},
		{"FS", `BEGIN { FS="` + alpha + `" } { print NF }`, string([]byte{'1', 0xe9, '2', '\n'}), "2\n"},
		{"RS", `BEGIN { RS="` + alpha + `+" } { print }`, string([]byte{'1', 0xe9, '2'}), "1\n2\n"},
		{"split", `BEGIN { print split("1\3512", a, "` + alpha + `") }`, "", "2\n"},
		{"sub-gsub", `BEGIN { s="\351\351"; sub("` + alpha + `","1",s); gsub("` + alpha + `","2",s); print s }`, "", "12\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := &fakeAwkCType{}
			out, errOut, code := runAwkLocale([]string{"LC_ALL=de_DE.iso88591"}, strings.NewReader(tc.input), []string{tc.program}, func(string) (ctypeProvider, error) { return provider, nil })
			if code != 0 || errOut != "" || out != tc.want || provider.closeCalls != 1 {
				t.Fatalf("got=(%q,%q,%d) close=%d want=%q", out, errOut, code, provider.closeCalls, tc.want)
			}
		})
	}
}

func TestAwkResolvesCTypeAndCollateIndependently(t *testing.T) {
	tests := []struct {
		name, ctypeName, collateName, want string
	}{
		{"locale-ctype-c-collation", "de_DE.iso88591", "C", "1 0\n"},
		{"c-ctype-locale-collation", "C", "de_DE.iso88591", "0 1\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			rc := &tool.RunContext{Ctx: context.Background(), Env: []string{"LANG=C", "LC_CTYPE=" + tc.ctypeName, "LC_COLLATE=" + tc.collateName}, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errOut}}
			ctypeFake := &fakeAwkCType{}
			collateFake := &fakeAwkCollate{}
			program := `BEGIN { s="\351"; print (s ~ "[[:alpha:]]"), (s ~ "[[=e=]]") }`
			code := runWithLocales(rc, []string{program}, func(string) (ctypeProvider, error) {
				return ctypeFake, nil
			}, func(string) (collateProvider, error) {
				return collateFake, nil
			})
			if code != 0 || errOut.String() != "" || out.String() != tc.want {
				t.Fatalf("got=(%q,%q,%d), want %q", out.String(), errOut.String(), code, tc.want)
			}
		})
	}
}

type panicAwkReader struct{}

func (panicAwkReader) Read([]byte) (int, error) { panic("input read before locale snapshot") }

func TestAwkLocaleLifecycle(t *testing.T) {
	for _, name := range []string{"C", "POSIX"} {
		out, errOut, code := runAwkLocale([]string{"LC_ALL=" + name}, strings.NewReader(""), []string{`BEGIN { print "ok" }`}, func(string) (ctypeProvider, error) { panic("provider opened") })
		if code != 0 || errOut != "" || out != "ok\n" {
			t.Fatalf("%s=(%q,%q,%d)", name, out, errOut, code)
		}
	}
	openErr := errors.New("open failed")
	_, errOut, code := runAwkLocale([]string{"LC_ALL=de_DE.iso88591"}, panicAwkReader{}, []string{"-f", "missing-program-must-not-be-opened"}, func(string) (ctypeProvider, error) { return nil, openErr })
	if code != 2 || !strings.Contains(errOut, openErr.Error()) || strings.Contains(errOut, "missing-program") {
		t.Fatalf("open-before-program-io=(%q,%d)", errOut, code)
	}
	want := errors.New("snapshot failed")
	provider := &fakeAwkCType{classErr: want}
	_, errOut, code = runAwkLocale([]string{"LC_ALL=de_DE.iso88591"}, panicAwkReader{}, []string{`{print}`}, func(string) (ctypeProvider, error) { return provider, nil })
	if code != 2 || !strings.Contains(errOut, want.Error()) || provider.closeCalls != 1 {
		t.Fatalf("snapshot=(%q,%d) close=%d", errOut, code, provider.closeCalls)
	}
	closeErr := errors.New("close failed")
	provider = &fakeAwkCType{closeErr: closeErr}
	_, errOut, code = runAwkLocale([]string{"LC_ALL=de_DE.iso88591"}, panicAwkReader{}, []string{`{print}`}, func(string) (ctypeProvider, error) { return provider, nil })
	if code != 2 || !strings.Contains(errOut, closeErr.Error()) || provider.closeCalls != 1 {
		t.Fatalf("close=(%q,%d) close=%d", errOut, code, provider.closeCalls)
	}
}

func TestAwkLCNumericInputOutputAndPrecedence(t *testing.T) {
	panicOpen := func(string) (ctypeProvider, error) { panic("numeric-only locale must not open LC_CTYPE") }
	for _, tc := range []struct {
		name    string
		env     []string
		program string
		input   string
		args    []string
		want    string
	}{
		{"input and print", []string{"LANG=C", "LC_NUMERIC=de_DE.iso88591"}, `{ print $1 + 1 }`, "4,5\n", nil, "5,5\n"},
		{"printf keeps literal period", []string{"LANG=C", "LC_NUMERIC=de_DE.ISO-8859-1"}, `BEGIN { printf "v.=%0.2f\n", 1.5 }`, "", nil, "v.=1,50\n"},
		{"source and assignment use period", []string{"LANG=C", "LC_NUMERIC=de_DE.iso88591"}, `BEGIN { print 1.5 + x }`, "", []string{"-v", "x=4.5"}, "6\n"},
		{"string conversion uses locale", []string{"LANG=C", "LC_NUMERIC=de_DE.iso88591"}, `BEGIN { print "4,5" + 0 }`, "", nil, "4,5\n"},
		{"assignment comma remains nonnumeric", []string{"LANG=C", "LC_NUMERIC=de_DE.iso88591"}, `BEGIN { print x + 0 }`, "", []string{"-v", "x=4,5"}, "4\n"},
		{"LC_ALL overrides numeric", []string{"LANG=de_DE.iso88591", "LC_NUMERIC=de_DE.iso88591", "LC_ALL=POSIX"}, `{ print $1 + 1 }`, "4.5\n", nil, "5.5\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append(append([]string(nil), tc.args...), tc.program)
			out, errOut, code := runAwkLocale(tc.env, strings.NewReader(tc.input), args, panicOpen)
			if code != 0 || errOut != "" || out != tc.want {
				t.Fatalf("got=(%q,%q,%d), want=(%q,empty,0)", out, errOut, code, tc.want)
			}
		})
	}
}

func TestAwkLCNumericUnsupportedFailsBeforeInput(t *testing.T) {
	out, errOut, code := runAwkLocale([]string{"LANG=C", "LC_NUMERIC=de_DE.UTF-8"}, panicAwkReader{}, []string{`{ print }`}, func(string) (ctypeProvider, error) {
		panic("unsupported LC_NUMERIC must fail before LC_CTYPE")
	})
	if code != 2 || out != "" || !strings.Contains(errOut, `LC_NUMERIC "de_DE.UTF-8"`) {
		t.Fatalf("got=(%q,%q,%d), want fail-closed LC_NUMERIC diagnostic", out, errOut, code)
	}
}

// TestAwkLocaleEquivalenceClassMatches pins POSIX XBD 9.3.5 for awk, which has
// no BRE mode: every awk ERE went through the compiler that dropped the locale
// equivalence table, so `/[[=a=]]/` under a non-C LC_CTYPE silently matched
// nothing — the literal 'a' included — with no diagnostic and exit 0.
func TestAwkLocaleEquivalenceClassMatches(t *testing.T) {
	for _, tc := range []struct{ name, program, input, want string }{
		{"equivalence-member", `/[[=a=]]/ { print "yes" }`, "bab\n", "yes\n"},
		{"equivalence-nonmember", `/[[=a=]]/ { print "yes" }`, "bbb\n", ""},
		{"collating-element", `/[[.a.]]/ { print "yes" }`, "bab\n", "yes\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &fakeAwkCType{}
			out, errOut, code := runAwkLocale([]string{"LC_ALL=de_DE.iso88591"}, strings.NewReader(tc.input), []string{tc.program}, func(string) (ctypeProvider, error) { return provider, nil })
			if code != 0 || errOut != "" || out != tc.want {
				t.Fatalf("got=(%q,%q,%d) want=%q", out, errOut, code, tc.want)
			}
		})
	}
}
