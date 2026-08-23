package sedcmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

type fakeSedCType struct {
	classCalls int
	lowerCalls int
	closeCalls int
	classErr   error
	lowerErr   error
	closeErr   error
}

func (p *fakeSedCType) classify(b byte, member bool) (bool, error) {
	p.classCalls++
	if p.classErr != nil {
		return false, p.classErr
	}
	return member, nil
}
func sedTestAlpha(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b == 0xc9 || b == 0xe9
}
func (p *fakeSedCType) IsAlpha(b byte) (bool, error) { return p.classify(b, sedTestAlpha(b)) }
func (p *fakeSedCType) IsAlnum(b byte) (bool, error) {
	return p.classify(b, sedTestAlpha(b) || b >= '0' && b <= '9')
}
func (p *fakeSedCType) IsBlank(b byte) (bool, error) { return p.classify(b, b == ' ' || b == '\t') }
func (p *fakeSedCType) IsCntrl(b byte) (bool, error) { return p.classify(b, b < 0x20 || b == 0x7f) }
func (p *fakeSedCType) IsDigit(b byte) (bool, error) { return p.classify(b, b >= '0' && b <= '9') }
func (p *fakeSedCType) IsGraph(b byte) (bool, error) { return p.classify(b, b > 0x20 && b != 0x7f) }
func (p *fakeSedCType) IsLower(b byte) (bool, error) {
	return p.classify(b, b >= 'a' && b <= 'z' || b == 0xe9)
}
func (p *fakeSedCType) IsPrint(b byte) (bool, error) { return p.classify(b, b >= 0x20 && b != 0x7f) }
func (p *fakeSedCType) IsPunct(b byte) (bool, error) { return p.classify(b, b == '!' || b == '.') }
func (p *fakeSedCType) IsSpace(b byte) (bool, error) {
	return p.classify(b, b == ' ' || b == '\t' || b == '\n' || b == '\r')
}
func (p *fakeSedCType) IsUpper(b byte) (bool, error) {
	return p.classify(b, b >= 'A' && b <= 'Z' || b == 0xc9)
}
func (p *fakeSedCType) IsXDigit(b byte) (bool, error) {
	return p.classify(b, b >= '0' && b <= '9' || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F')
}
func (p *fakeSedCType) ToLower(in []byte) ([]byte, error) {
	p.lowerCalls++
	if p.lowerErr != nil {
		return nil, p.lowerErr
	}
	out := append([]byte(nil), in...)
	for i, b := range out {
		if b >= 'A' && b <= 'Z' {
			out[i] = b + 32
		} else if b == 0xc9 {
			out[i] = 0xe9
		}
	}
	return out, nil
}
func (p *fakeSedCType) Close() error { p.closeCalls++; return p.closeErr }

func runSedWithCType(env []string, in io.Reader, out io.Writer, args []string, opener ctypeOpener) (string, int) {
	var errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: ".", Env: env, Stdio: tool.Stdio{In: in, Out: out, Err: &errOut}}
	code := runCommandWithCType(rc, args, opener)
	return errOut.String(), code
}

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) { panic("input read before validation") }

type panicWriter struct{}

func (panicWriter) Write([]byte) (int, error) { panic("output written before validation") }

func TestSedCTypeResolverLifecycleAndBypass(t *testing.T) {
	for _, localeName := range []string{"C", "POSIX"} {
		opener := func(string) (ctypeProvider, error) { panic("C/POSIX opened provider") }
		var out bytes.Buffer
		errOut, code := runSedWithCType([]string{"LC_ALL=" + localeName}, strings.NewReader("a\n"), &out, []string{"s/a/b/"}, opener)
		if code != 0 || errOut != "" || out.String() != "b\n" {
			t.Fatalf("%s bypass=(%q,%q,%d)", localeName, out.String(), errOut, code)
		}
	}
	provider := &fakeSedCType{}
	opened := ""
	opener := func(name string) (ctypeProvider, error) { opened = name; return provider, nil }
	var out bytes.Buffer
	errOut, code := runSedWithCType([]string{"LANG=C", "LC_CTYPE=de_DE.iso88591"}, strings.NewReader(string([]byte{0xe9, '\n'})), &out, []string{"s/[[:upper:]]/X/i"}, opener)
	if code != 0 || errOut != "" || out.String() != "X\n" || opened != "de_DE.iso88591" {
		t.Fatalf("locale run=(%q,%q,%d) opened=%q", out.String(), errOut, code, opened)
	}
	if provider.classCalls != 12*256 || provider.lowerCalls != 1 || provider.closeCalls != 1 {
		t.Fatalf("calls class=%d lower=%d close=%d", provider.classCalls, provider.lowerCalls, provider.closeCalls)
	}
	provider = &fakeSedCType{}
	out.Reset()
	errOut, code = runSedWithCType([]string{"LC_ALL=de_DE.iso88591"}, strings.NewReader(string([]byte{0xe9, '\n'})), &out, []string{`/[[:alpha:]]/s/./Y/`}, func(string) (ctypeProvider, error) { return provider, nil })
	if code != 0 || errOut != "" || out.String() != "Y\n" {
		t.Fatalf("high-byte address=(%q,%q,%d)", out.String(), errOut, code)
	}
	provider = &fakeSedCType{}
	out.Reset()
	errOut, code = runSedWithCType([]string{"LC_ALL=de_DE.iso88591"}, strings.NewReader(string([]byte{0xe9, '\n'})), &out, []string{"-E", `s/([[:alpha:]])/Z/`}, func(string) (ctypeProvider, error) { return provider, nil })
	if code != 0 || errOut != "" || out.String() != "Z\n" {
		t.Fatalf("high-byte ERE=(%q,%q,%d)", out.String(), errOut, code)
	}
	provider = &fakeSedCType{}
	out.Reset()
	rawFoldScript := string([]byte{'s', '/', 0xc9, '/', 'X', '/', 'I', 'g'})
	errOut, code = runSedWithCType([]string{"LC_ALL=de_DE.iso88591"}, strings.NewReader(string([]byte{0xc9, 0xe9, '\n'})), &out, []string{rawFoldScript}, func(string) (ctypeProvider, error) { return provider, nil })
	if code != 0 || errOut != "" || out.String() != "XX\n" || provider.closeCalls != 1 {
		t.Fatalf("raw high-byte fold=(%x,%q,%d) close=%d", out.Bytes(), errOut, code, provider.closeCalls)
	}
	provider = &fakeSedCType{}
	out.Reset()
	rawFastScript := string([]byte{'s', '/', 0xc9, '/', 0xe9, '/', 'g'})
	errOut, code = runSedWithCType([]string{"LC_ALL=de_DE.iso88591"}, strings.NewReader(string([]byte{0xc9, 0xc9, '\n'})), &out, []string{rawFastScript}, func(string) (ctypeProvider, error) { return provider, nil })
	if code != 0 || errOut != "" || !bytes.Equal(out.Bytes(), []byte{0xe9, 0xe9, '\n'}) || provider.closeCalls != 1 {
		t.Fatalf("raw fast pattern/replacement=(%x,%q,%d) close=%d", out.Bytes(), errOut, code, provider.closeCalls)
	}
	provider = &fakeSedCType{}
	out.Reset()
	rawFastDelimiterScript := string([]byte{'s', 0xc9, 'A', 0xc9, 'Q', 0xc9, 'g'})
	errOut, code = runSedWithCType([]string{"LC_ALL=de_DE.iso88591"}, strings.NewReader("AA\n"), &out, []string{rawFastDelimiterScript}, func(string) (ctypeProvider, error) { return provider, nil })
	if code != 0 || errOut != "" || out.String() != "QQ\n" || provider.closeCalls != 1 {
		t.Fatalf("raw fast delimiter=(%x,%q,%d) close=%d", out.Bytes(), errOut, code, provider.closeCalls)
	}
}

func TestSedCTypeBypassesOnEarlyExit(t *testing.T) {
	opener := func(string) (ctypeProvider, error) { panic("provider opened on early exit") }
	for _, args := range [][]string{{"--help"}, {"--version"}, {}, {"-i", "s/a/b/"}} {
		_, _ = runSedWithCType([]string{"LC_ALL=de_DE.iso88591"}, strings.NewReader(""), &bytes.Buffer{}, args, opener)
	}
}

func TestSedCTypeErrorsCloseAndPrecedeIO(t *testing.T) {
	wantBuild := errors.New("build failed")
	wantClose := errors.New("close failed")
	provider := &fakeSedCType{classErr: wantBuild, closeErr: wantClose}
	errOut, code := runSedWithCType([]string{"LC_ALL=de_DE.iso88591"}, panicReader{}, panicWriter{}, []string{"s/a/b/"}, func(string) (ctypeProvider, error) { return provider, nil })
	if code != 2 || !strings.Contains(errOut, wantBuild.Error()) || strings.Contains(errOut, wantClose.Error()) || provider.closeCalls != 1 {
		t.Fatalf("build precedence err=%q code=%d close=%d", errOut, code, provider.closeCalls)
	}
	openErr := errors.New("open failed")
	errOut, code = runSedWithCType([]string{"LC_ALL=de_DE.iso88591"}, panicReader{}, panicWriter{}, []string{"s/a/b/"}, func(string) (ctypeProvider, error) { return nil, openErr })
	if code != 2 || !strings.Contains(errOut, `sed: LC_CTYPE "de_DE.iso88591": open failed`) {
		t.Fatalf("open error=%q code=%d", errOut, code)
	}
	provider = &fakeSedCType{closeErr: wantClose}
	errOut, code = runSedWithCType([]string{"LC_ALL=de_DE.iso88591"}, panicReader{}, panicWriter{}, []string{"s/a/b/"}, func(string) (ctypeProvider, error) { return provider, nil })
	if code != 2 || !strings.Contains(errOut, wantClose.Error()) || provider.closeCalls != 1 {
		t.Fatalf("close error=%q code=%d close=%d", errOut, code, provider.closeCalls)
	}
}

func TestSedCTypeConcurrentInvocationEnvironments(t *testing.T) {
	var wg sync.WaitGroup
	errs := make(chan string, 12)
	for worker := 0; worker < 12; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			var out bytes.Buffer
			if worker%2 == 0 {
				errOut, code := runSedWithCType([]string{"LC_ALL=C"}, strings.NewReader("a\n"), &out, []string{"s/a/b/"}, func(string) (ctypeProvider, error) {
					return nil, errors.New("C invocation opened provider")
				})
				if code != 0 || errOut != "" || out.String() != "b\n" {
					errs <- "C invocation failed"
				}
				return
			}
			provider := &fakeSedCType{}
			errOut, code := runSedWithCType([]string{"LC_ALL=de_DE.iso88591"}, strings.NewReader(string([]byte{0xe9, '\n'})), &out, []string{"s/[[:alpha:]]/X/"}, func(string) (ctypeProvider, error) { return provider, nil })
			if code != 0 || errOut != "" || out.String() != "X\n" || provider.closeCalls != 1 {
				errs <- "locale invocation failed"
			}
		}(worker)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestSedLocaleCompileFailsBeforeOperandIO(t *testing.T) {
	unsupported := []string{`s/\b/x/`, `s/\1/x/`, `s/[a-z]/x/`, `s/[[=ab=]]/x/`, `s/[[.ab.]]/x/`, `s/a\{1001\}/x/`}
	for _, script := range unsupported {
		provider := &fakeSedCType{}
		errOut, code := runSedWithCType([]string{"LC_ALL=de_DE.iso88591"}, panicReader{}, panicWriter{}, []string{script, "definitely-missing"}, func(string) (ctypeProvider, error) { return provider, nil })
		if code != 2 || errOut == "" || strings.Contains(errOut, "definitely-missing") {
			t.Fatalf("script %q err=%q code=%d", script, errOut, code)
		}
		if provider.closeCalls != 1 {
			t.Fatalf("script %q close=%d", script, provider.closeCalls)
		}
	}
}
