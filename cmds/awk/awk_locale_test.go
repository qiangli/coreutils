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
