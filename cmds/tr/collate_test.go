package trcmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

type fakeTrCollator struct {
	weights  []byte
	elements []bool
	valid    []bool
	equiv    map[byte][]byte
	err      error
	closeErr error
	closes   int
}

func germanTrCollator() *fakeTrCollator {
	p := &fakeTrCollator{
		weights: make([]byte, 256), elements: make([]bool, 256),
		valid: make([]bool, 256), equiv: make(map[byte][]byte),
	}
	for i := 0; i < 256; i++ {
		p.weights[i] = 200
		p.elements[i] = true
		p.valid[i] = true
		p.equiv[byte(i)] = []byte{byte(i)}
	}
	p.weights['a'] = 10
	p.weights[0xe4] = 11 // de_DE: a < ä < b, unlike byte order
	p.weights['b'] = 12
	p.equiv['e'] = []byte{'e', 0xe9}
	p.equiv[0xe9] = []byte{'e', 0xe9}
	return p
}

func (p *fakeTrCollator) Equivalents(c byte) ([]byte, error) {
	if p.err != nil {
		return nil, p.err
	}
	return append([]byte(nil), p.equiv[c]...), nil
}
func (p *fakeTrCollator) EquivalenceClasses() ([]bool, error) {
	if p.err != nil {
		return nil, p.err
	}
	return append([]bool(nil), p.valid...), nil
}
func (p *fakeTrCollator) CollationWeights() ([]byte, error) {
	if p.err != nil {
		return nil, p.err
	}
	return append([]byte(nil), p.weights...), nil
}
func (p *fakeTrCollator) CollatingElements() ([]bool, error) {
	if p.err != nil {
		return nil, p.err
	}
	return append([]bool(nil), p.elements...), nil
}
func (p *fakeTrCollator) Close() error { p.closes++; return p.closeErr }

func runTrProviders(t *testing.T, env []string, stdin string, copen ctypeOpener, lopen collateOpener, args ...string) (string, string, int) {
	t.Helper()
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Env: env,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errOut}}
	code := runWithProviders(rc, args, copen, lopen)
	return out.String(), errOut.String(), code
}

func TestTrLCCollateRangesAndEquivalence(t *testing.T) {
	for _, tc := range []struct {
		name, set1, set2, in, want string
	}{
		{"range", "a-b", "123", "a\xe4bc\n", "123c\n"},
		{"range forward only in collation", "\xe4-b", "12", "\xe4b", "12"},
		{"equivalence", "[=e=]", "[X*]", "e\xe9f\n", "XXf\n"},
		{"ordered character complement repeat", "x", "12[Z*]", "a\xe4b", "12Z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := germanTrCollator()
			opened := ""
			args := []string{tc.set1, tc.set2}
			if tc.name == "ordered character complement repeat" {
				args = append([]string{"-C"}, args...)
			}
			out, errOut, code := runTrProviders(t,
				[]string{"POSIXLY_CORRECT=1", "LC_CTYPE=C", "LC_COLLATE=de_DE.iso88591"}, tc.in,
				neverOpener(), func(name string) (collateProvider, error) { opened = name; return p, nil },
				args...)
			if code != 0 || errOut != "" || out != tc.want {
				t.Fatalf("code=%d stdout=%q stderr=%q, want %q", code, out, errOut, tc.want)
			}
			if opened != "de_DE.iso88591" || p.closes != 1 {
				t.Fatalf("opened=%q closes=%d", opened, p.closes)
			}
		})
	}
}

func TestTrLCCollatePrecedence(t *testing.T) {
	p := germanTrCollator()
	var ctypeCloses atomic.Int32
	copen := func(name string) (ctypeProvider, error) {
		if name != "de_DE.iso88591" {
			t.Fatalf("LC_CTYPE open=%q", name)
		}
		return newFakeProvider(&ctypeCloses), nil
	}
	opened := ""
	out, errOut, code := runTrProviders(t,
		[]string{"POSIXLY_CORRECT=1", "LANG=bad", "LC_CTYPE=C", "LC_COLLATE=bad", "LC_ALL=de_DE.iso88591"},
		"a\xe4b\n", copen, func(name string) (collateProvider, error) { opened = name; return p, nil },
		"a-b", "123")
	if code != 0 || errOut != "" || out != "123\n" || opened != "de_DE.iso88591" {
		t.Fatalf("code=%d stdout=%q stderr=%q opened=%q", code, out, errOut, opened)
	}
	if ctypeCloses.Load() != 1 || p.closes != 1 {
		t.Fatalf("ctype closes=%d collate closes=%d", ctypeCloses.Load(), p.closes)
	}
}

func TestTrLCCollateFailsClosed(t *testing.T) {
	t.Run("irrelevant category is not opened", func(t *testing.T) {
		out, errOut, code := runTrProviders(t,
			[]string{"POSIXLY_CORRECT=1", "LC_CTYPE=C", "LC_COLLATE=unsupported"}, "abc", neverOpener(),
			func(string) (collateProvider, error) {
				t.Fatal("LC_COLLATE opened for literal delete")
				return nil, nil
			},
			"-d", "b")
		if code != 0 || out != "ac" || errOut != "" {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
		}
	})
	t.Run("open", func(t *testing.T) {
		want := errors.New("unavailable")
		out, errOut, code := runTrProviders(t,
			[]string{"POSIXLY_CORRECT=1", "LC_CTYPE=C", "LC_COLLATE=x-test"}, "input", neverOpener(),
			func(string) (collateProvider, error) { return nil, want }, "a-b", "12")
		if code != 2 || out != "" || !strings.Contains(errOut, `LC_COLLATE "x-test"`) {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
		}
	})
	for _, tc := range []struct {
		name     string
		provider *fakeTrCollator
		want     string
	}{
		{"snapshot", &fakeTrCollator{err: errors.New("snapshot")}, "snapshot"},
		{"close", func() *fakeTrCollator { p := germanTrCollator(); p.closeErr = errors.New("close"); return p }(), "close"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runTrProviders(t,
				[]string{"POSIXLY_CORRECT=1", "LC_CTYPE=C", "LC_COLLATE=x-test"}, "input", neverOpener(),
				func(string) (collateProvider, error) { return tc.provider, nil }, "a-b", "12")
			if code != 2 || out != "" || !strings.Contains(errOut, tc.want) || tc.provider.closes != 1 {
				t.Fatalf("code=%d stdout=%q stderr=%q closes=%d", code, out, errOut, tc.provider.closes)
			}
		})
	}
}
