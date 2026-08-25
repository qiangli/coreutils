package writecmd

import (
	"errors"
	"strings"
	"testing"
)

type trackingCType struct {
	print, space, control  map[byte]bool
	printCalls, spaceCalls int
	controlCalls           int
	classErr, closeErr     error
	closed                 bool
}

func (p *trackingCType) IsPrint(b byte) (bool, error) {
	p.printCalls++
	if p.classErr != nil {
		return false, p.classErr
	}
	return p.print[b], nil
}
func (p *trackingCType) IsSpace(b byte) (bool, error) {
	p.spaceCalls++
	return p.space[b], nil
}
func (p *trackingCType) IsCntrl(b byte) (bool, error) {
	p.controlCalls++
	return p.control[b], nil
}
func (p *trackingCType) Close() error { p.closed = true; return p.closeErr }

func TestPOSIXSingleByteLocaleUsesAllClassesAndPrecedence(t *testing.T) {
	w := install(t, fixture{
		uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}},
	})
	p := &trackingCType{
		print:   map[byte]bool{0xe9: true},
		space:   map[byte]bool{0xa0: true, '\t': true, '\n': true, '\v': true, '\f': true, '\r': true, ' ': true},
		control: map[byte]bool{0x9b: true},
	}
	openCTypeFn = func(name string) (ctypeProvider, error) {
		if name != "chosen_8bit" {
			t.Fatalf("resolved locale = %q, want chosen_8bit", name)
		}
		return p, nil
	}
	env := []string{"POSIXLY_CORRECT=", "LANG=ignored", "LC_CTYPE=ignored_too", "LC_ALL=chosen_8bit"}
	out, errOut, code := execEnv(t, env, "\xe9\xa0\x9b\n", "bob")
	if code != 0 || out != "" || errOut != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	if p.printCalls != 256 || p.spaceCalls != 256 || p.controlCalls != 256 || !p.closed {
		t.Fatalf("provider calls print=%d space=%d control=%d closed=%v", p.printCalls, p.spaceCalls, p.controlCalls, p.closed)
	}
	got := w.read(t, "pts/9")
	if !strings.Contains(got, "\xe9\xa0M-^[\nEOT\n") {
		t.Fatalf("recipient bytes = %q", got)
	}
}

func TestPOSIXLocaleFailuresAreDiagnosedBeforeDelivery(t *testing.T) {
	w := install(t, fixture{
		uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}},
	})
	openCTypeFn = func(string) (ctypeProvider, error) { return nil, errors.New("locale unavailable") }
	_, errOut, code := execEnv(t, []string{"POSIXLY_CORRECT=", "LC_ALL=x"}, "secret\n", "bob")
	if code != 1 || !strings.Contains(errOut, `LC_CTYPE "x": locale unavailable`) {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	if got := w.read(t, "pts/9"); got != "" {
		t.Fatalf("locale failure wrote recipient data %q", got)
	}

	p := &trackingCType{classErr: errors.New("classification failed")}
	openCTypeFn = func(string) (ctypeProvider, error) { return p, nil }
	_, errOut, code = execEnv(t, []string{"POSIXLY_CORRECT=1", "LC_ALL=x"}, "", "bob")
	if code != 1 || !p.closed || !strings.Contains(errOut, "classification failed") {
		t.Fatalf("classification: code=%d closed=%v stderr=%q", code, p.closed, errOut)
	}

	p = &trackingCType{closeErr: errors.New("close failed")}
	openCTypeFn = func(string) (ctypeProvider, error) { return p, nil }
	_, errOut, code = execEnv(t, []string{"POSIXLY_CORRECT=1", "LC_ALL=x"}, "", "bob")
	if code != 1 || !p.closed || !strings.Contains(errOut, "close failed") {
		t.Fatalf("close: code=%d closed=%v stderr=%q", code, p.closed, errOut)
	}
}

func TestPOSIXCLocaleUsesCClassesWithoutProvider(t *testing.T) {
	w := install(t, fixture{
		uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}},
	})
	openCTypeFn = func(string) (ctypeProvider, error) {
		t.Fatal("C locale must not open a locale provider")
		return nil, nil
	}
	_, errOut, code := execEnv(t, []string{"POSIXLY_CORRECT=", "LC_ALL=POSIX"}, "\xe9\n", "bob")
	if code != 0 || errOut != "" {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	if got := w.read(t, "pts/9"); !strings.Contains(got, "M-i\nEOT\n") {
		t.Fatalf("recipient bytes = %q", got)
	}
}

func TestOutsidePOSIXModePreservesDocumentedCFallback(t *testing.T) {
	w := install(t, fixture{
		uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}},
	})
	openCTypeFn = func(string) (ctypeProvider, error) { return nil, errors.New("unsupported locale") }
	_, errOut, code := execEnv(t, []string{"LC_ALL=unsupported"}, "\xe9\n", "bob")
	if code != 0 || errOut != "" {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	if got := w.read(t, "pts/9"); !strings.Contains(got, "M-i\nEOT\n") {
		t.Fatalf("recipient bytes = %q", got)
	}
}
