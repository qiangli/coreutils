package trcmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// ---------------------------------------------------------------------------
// fake provider -----------------------------------------------------------
// ---------------------------------------------------------------------------

// fakeProvider implements ctypeProvider with configurable class membership
// and case mapping.  It tracks Open/Close counts atomically.
type fakeProvider struct {
	classes map[string]map[byte]bool // class name → byte membership
	lower   [256]byte
	upper   [256]byte
	closed  atomic.Bool
	closeN  *atomic.Int32 // shared counter
	isErr   error         // if non-nil, every Is* returns this
	caseErr error         // if non-nil, ToLower/ToUpper return this
	closErr error         // if non-nil, Close returns this
	shortLo bool          // ToLower returns short slice
	shortUp bool          // ToUpper returns short slice
}

func (f *fakeProvider) check(class string, b byte) (bool, error) {
	if f.isErr != nil {
		return false, f.isErr
	}
	m, ok := f.classes[class]
	if !ok {
		return false, nil
	}
	return m[b], nil
}
func (f *fakeProvider) IsAlnum(b byte) (bool, error)  { return f.check("alnum", b) }
func (f *fakeProvider) IsAlpha(b byte) (bool, error)  { return f.check("alpha", b) }
func (f *fakeProvider) IsBlank(b byte) (bool, error)  { return f.check("blank", b) }
func (f *fakeProvider) IsCntrl(b byte) (bool, error)  { return f.check("cntrl", b) }
func (f *fakeProvider) IsDigit(b byte) (bool, error)  { return f.check("digit", b) }
func (f *fakeProvider) IsGraph(b byte) (bool, error)  { return f.check("graph", b) }
func (f *fakeProvider) IsLower(b byte) (bool, error)  { return f.check("lower", b) }
func (f *fakeProvider) IsPrint(b byte) (bool, error)  { return f.check("print", b) }
func (f *fakeProvider) IsPunct(b byte) (bool, error)  { return f.check("punct", b) }
func (f *fakeProvider) IsSpace(b byte) (bool, error)  { return f.check("space", b) }
func (f *fakeProvider) IsUpper(b byte) (bool, error)  { return f.check("upper", b) }
func (f *fakeProvider) IsXDigit(b byte) (bool, error) { return f.check("xdigit", b) }

func (f *fakeProvider) ToLower(in []byte) ([]byte, error) {
	if f.caseErr != nil {
		return nil, f.caseErr
	}
	out := make([]byte, len(in))
	for i, b := range in {
		out[i] = f.lower[b]
	}
	if f.shortLo {
		return out[:len(out)/2], nil
	}
	return out, nil
}

func (f *fakeProvider) ToUpper(in []byte) ([]byte, error) {
	if f.caseErr != nil {
		return nil, f.caseErr
	}
	out := make([]byte, len(in))
	for i, b := range in {
		out[i] = f.upper[b]
	}
	if f.shortUp {
		return out[:len(out)/2], nil
	}
	return out, nil
}

func (f *fakeProvider) Close() error {
	f.closed.Store(true)
	if f.closeN != nil {
		f.closeN.Add(1)
	}
	return f.closErr
}

// newFakeProvider builds a deterministic fake provider with non-ASCII
// class members, suitable for testing that provider-backed tables are
// actually used instead of the hardcoded C-locale tables.
//
// Class membership:
//
//	alpha:  {0x41('A'), 0x61('a'), 0x80, 0x81}
//	upper:  {0x41('A'), 0x80}
//	lower:  {0x61('a'), 0x81}
//	digit:  {0x30('0'), 0x90}
//	alnum:  {0x41, 0x61, 0x80, 0x81, 0x30, 0x90}
//	blank:  {0x20(' '), 0x09('\t'), 0xA0}
//	space:  {0x20, 0x09, 0x0A, 0xA0}
//	cntrl:  {0x00, 0x7F, 0x9F}
//	graph:  {0x21('!'), 0x41, 0x61, 0x30, 0x80, 0x81, 0x90, 0xB0}
//	print:  {0x20, 0x21, 0x41, 0x61, 0x30, 0x80, 0x81, 0x90, 0xB0}
//	punct:  {0x21('!'), 0xB0}
//	xdigit: {0x30, 0x41, 0x61, 0x90}
//
// Case maps: identity everywhere except A↔a, 0x80↔0x81.
func newFakeProvider(closeN *atomic.Int32) *fakeProvider {
	fp := &fakeProvider{
		closeN: closeN,
		classes: map[string]map[byte]bool{
			"alpha":  {0x41: true, 0x61: true, 0x80: true, 0x81: true},
			"upper":  {0x41: true, 0x80: true},
			"lower":  {0x61: true, 0x81: true},
			"digit":  {0x30: true, 0x90: true},
			"alnum":  {0x41: true, 0x61: true, 0x80: true, 0x81: true, 0x30: true, 0x90: true},
			"blank":  {0x20: true, 0x09: true, 0xA0: true},
			"space":  {0x20: true, 0x09: true, 0x0A: true, 0xA0: true},
			"cntrl":  {0x00: true, 0x7F: true, 0x9F: true},
			"graph":  {0x21: true, 0x41: true, 0x61: true, 0x30: true, 0x80: true, 0x81: true, 0x90: true, 0xB0: true},
			"print":  {0x20: true, 0x21: true, 0x41: true, 0x61: true, 0x30: true, 0x80: true, 0x81: true, 0x90: true, 0xB0: true},
			"punct":  {0x21: true, 0xB0: true},
			"xdigit": {0x30: true, 0x41: true, 0x61: true, 0x90: true},
		},
	}
	// Identity case map everywhere.
	for i := 0; i < 256; i++ {
		fp.lower[i] = byte(i)
		fp.upper[i] = byte(i)
	}
	// Paired cases: A(0x41)↔a(0x61), 0x80↔0x81.
	fp.lower[0x41] = 0x61
	fp.lower[0x80] = 0x81
	fp.upper[0x61] = 0x41
	fp.upper[0x81] = 0x80
	return fp
}

// ---------------------------------------------------------------------------
// helper: run with a specific opener/env
// ---------------------------------------------------------------------------

func runWithOpener(t *testing.T, env []string, stdin string, opener ctypeOpener, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code = runWithCType(rc, args, opener)
	return out.String(), errb.String(), code
}

// panicReader panics on any Read call — proves no I/O occurred.
type panicReader struct{}

func (panicReader) Read([]byte) (int, error) { panic("stdin was read") }

// spyWriter records whether any Write call occurred.
type spyWriter struct{ written bool }

func (s *spyWriter) Write(p []byte) (int, error) { s.written = true; return len(p), nil }

// ---------------------------------------------------------------------------
// TestCTypeLifecycle — provider lifecycle, locale resolution, tables
// ---------------------------------------------------------------------------

func TestCTypeLifecycle(t *testing.T) {
	// neverOpener asserts that it is never called.
	neverOpener := func(name string) (ctypeProvider, error) {
		t.Fatalf("opener called for %q; C/POSIX should never open", name)
		return nil, nil
	}

	t.Run("C/POSIX never opens", func(t *testing.T) {
		envs := [][]string{
			nil,                            // empty env → POSIX default
			{},                             // empty slice → POSIX default
			{"LC_CTYPE=C"},                 // explicit C
			{"LC_CTYPE=POSIX"},             // explicit POSIX
			{"LC_ALL=C"},                   // LC_ALL override
			{"LANG=C"},                     // LANG fallback
			{"LC_CTYPE=de_DE", "LC_ALL=C"}, // LC_ALL wins
		}
		for _, env := range envs {
			out, errb, code := runWithOpener(t, env, "abc", neverOpener, "-d", "[:digit:]")
			if code != 0 {
				t.Errorf("env=%v: code=%d err=%q", env, code, errb)
			}
			if out != "abc" {
				t.Errorf("env=%v: out=%q, want \"abc\"", env, out)
			}
		}
	})

	t.Run("env precedence LC_ALL > LC_CTYPE > LANG", func(t *testing.T) {
		var lastOpened string
		errOpener := func(name string) (ctypeProvider, error) {
			lastOpened = name
			return nil, fmt.Errorf("test: reject %s", name)
		}

		// LC_ALL beats LC_CTYPE beats LANG.
		cases := []struct {
			env  []string
			want string
		}{
			{[]string{"LANG=lang1", "LC_CTYPE=ctype1", "LC_ALL=all1"}, "all1"},
			{[]string{"LANG=lang1", "LC_CTYPE=ctype1"}, "ctype1"},
			{[]string{"LANG=lang1"}, "lang1"},
		}
		for _, c := range cases {
			lastOpened = ""
			_, _, _ = runWithOpener(t, c.env, "", errOpener, "-d", "a")
			if lastOpened != c.want {
				t.Errorf("env=%v: opened %q, want %q", c.env, lastOpened, c.want)
			}
		}
	})

	t.Run("empty values fall through", func(t *testing.T) {
		var lastOpened string
		errOpener := func(name string) (ctypeProvider, error) {
			lastOpened = name
			return nil, fmt.Errorf("test: reject %s", name)
		}
		// LC_CTYPE= (empty) falls through to LANG.
		_, _, _ = runWithOpener(t, []string{"LC_CTYPE=", "LANG=fr_FR"}, "", errOpener, "-d", "a")
		if lastOpened != "fr_FR" {
			t.Errorf("opened %q, want fr_FR", lastOpened)
		}
	})

	t.Run("duplicate last wins", func(t *testing.T) {
		var lastOpened string
		errOpener := func(name string) (ctypeProvider, error) {
			lastOpened = name
			return nil, fmt.Errorf("test: reject %s", name)
		}
		_, _, _ = runWithOpener(t, []string{"LC_CTYPE=first", "LC_CTYPE=second"}, "", errOpener, "-d", "a")
		if lastOpened != "second" {
			t.Errorf("opened %q, want second", lastOpened)
		}
	})

	t.Run("exact alias reaches opener", func(t *testing.T) {
		var opened string
		errOpener := func(name string) (ctypeProvider, error) {
			opened = name
			return nil, fmt.Errorf("test: reject %s", name)
		}
		_, errb, code := runWithOpener(t, []string{"LC_CTYPE=de_DE.ISO-8859-1"}, "", errOpener, "-d", "a")
		if opened != "de_DE.ISO-8859-1" {
			t.Errorf("opened %q, want de_DE.ISO-8859-1", opened)
		}
		if code != 2 {
			t.Errorf("code=%d, want 2 (open failure)", code)
		}
		if !strings.Contains(errb, "de_DE.ISO-8859-1") {
			t.Errorf("stderr %q should mention locale", errb)
		}
	})
}

// ---------------------------------------------------------------------------
// TestCTypeProviderBacked — deterministic fake provider proves tables used
// ---------------------------------------------------------------------------

func TestCTypeProviderBacked(t *testing.T) {
	var closeCount atomic.Int32

	fakeOpener := func(name string) (ctypeProvider, error) {
		return newFakeProvider(&closeCount), nil
	}

	env := []string{"LC_CTYPE=fake_LOCALE"}

	t.Run("delete with provider alpha class", func(t *testing.T) {
		closeCount.Store(0)
		// Fake alpha includes 0x80 and 0x81, which the C-locale alpha does NOT include.
		input := "A\x80\x61\x81xyz"
		out, errb, code := runWithOpener(t, env, input, fakeOpener, "-d", "[:alpha:]")
		if code != 0 {
			t.Fatalf("code=%d err=%q", code, errb)
		}
		// Only alpha members should be deleted; x,y,z are not alpha in the fake.
		if out != "xyz" {
			t.Errorf("out=%q, want %q", out, "xyz")
		}
		if closeCount.Load() != 1 {
			t.Errorf("closeCount=%d, want 1", closeCount.Load())
		}
	})

	t.Run("squeeze with provider digit class", func(t *testing.T) {
		closeCount.Store(0)
		// Fake digit: {0x30('0'), 0x90}. Feed consecutive 0x90 bytes.
		input := "\x90\x90\x90abc"
		out, _, code := runWithOpener(t, env, input, fakeOpener, "-s", "[:digit:]")
		if code != 0 {
			t.Fatalf("code=%d", code)
		}
		// Consecutive 0x90 squeezed to one.
		want := "\x90abc"
		if out != want {
			t.Errorf("out=%q, want %q", out, want)
		}
	})

	t.Run("complement delete with provider blank class", func(t *testing.T) {
		closeCount.Store(0)
		// Fake blank: {0x20, 0x09, 0xA0}. Complement deletes everything NOT blank.
		input := "AB \t\xA0CD"
		out, _, code := runWithOpener(t, env, input, fakeOpener, "-c", "-d", "[:blank:]")
		if code != 0 {
			t.Fatalf("code=%d", code)
		}
		want := " \t\xA0"
		if out != want {
			t.Errorf("out=%q, want %q", out, want)
		}
	})

	t.Run("SET1-to-literal translation with provider class", func(t *testing.T) {
		closeCount.Store(0)
		// Translate [:punct:] to 'X'. Fake punct: {0x21('!'), 0xB0}.
		input := "!\xB0abc"
		out, _, code := runWithOpener(t, env, input, fakeOpener, "[:punct:]", "X")
		if code != 0 {
			t.Fatalf("code=%d", code)
		}
		want := "XXabc"
		if out != want {
			t.Errorf("out=%q, want %q", out, want)
		}
	})

	t.Run("provider closed before stream read on success", func(t *testing.T) {
		// Verify that the provider is closed before any stdin read.
		var providerClosed atomic.Bool
		var stdinReadBeforeClose atomic.Bool
		trackOpener := func(name string) (ctypeProvider, error) {
			fp := newFakeProvider(&closeCount)
			// wrap close to set flag
			return &closeTracker{fp, &providerClosed}, nil
		}

		pr := &trackingReader{closed: &providerClosed, readBeforeClose: &stdinReadBeforeClose}
		var out, errb bytes.Buffer
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			Dir:   t.TempDir(),
			Env:   env,
			Stdio: tool.Stdio{In: pr, Out: &out, Err: &errb},
		}
		code := runWithCType(rc, []string{"-d", "[:digit:]"}, trackOpener)
		if code != 0 {
			t.Fatalf("code=%d err=%q", code, errb.String())
		}
		if stdinReadBeforeClose.Load() {
			t.Error("stdin was read before provider was closed")
		}
	})
}

// closeTracker wraps a fakeProvider and sets a flag when Close is called.
type closeTracker struct {
	*fakeProvider
	closed *atomic.Bool
}

func (ct *closeTracker) Close() error {
	ct.closed.Store(true)
	return ct.fakeProvider.Close()
}

// trackingReader records whether Read was called before the provider closed.
type trackingReader struct {
	closed          *atomic.Bool
	readBeforeClose *atomic.Bool
	done            bool
}

func (r *trackingReader) Read(p []byte) (int, error) {
	if !r.closed.Load() {
		r.readBeforeClose.Store(true)
	}
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	return copy(p, "test"), nil
}

// ---------------------------------------------------------------------------
// TestCTypeFailures — Open/build/Close failure handling
// ---------------------------------------------------------------------------

func TestCTypeFailures(t *testing.T) {
	env := []string{"LC_CTYPE=test_locale"}

	t.Run("Open failure", func(t *testing.T) {
		openErr := errors.New("open failed")
		var openCount int32
		opener := func(name string) (ctypeProvider, error) {
			openCount++
			return nil, openErr
		}

		spy := &spyWriter{}
		var errb bytes.Buffer
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			Dir:   t.TempDir(),
			Env:   env,
			Stdio: tool.Stdio{In: panicReader{}, Out: spy, Err: &errb},
		}
		code := runWithCType(rc, []string{"-d", "a"}, opener)
		if code != 2 {
			t.Errorf("code=%d, want 2", code)
		}
		if spy.written {
			t.Error("stdout was written to despite open failure")
		}
		if openCount != 1 {
			t.Errorf("openCount=%d, want 1", openCount)
		}
		if !strings.Contains(errb.String(), "open failed") {
			t.Errorf("stderr %q should mention 'open failed'", errb.String())
		}
	})

	t.Run("build error (Is* fails)", func(t *testing.T) {
		var closeCount atomic.Int32
		buildErr := errors.New("classification failed")
		opener := func(name string) (ctypeProvider, error) {
			fp := newFakeProvider(&closeCount)
			fp.isErr = buildErr
			return fp, nil
		}

		spy := &spyWriter{}
		var errb bytes.Buffer
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			Dir:   t.TempDir(),
			Env:   env,
			Stdio: tool.Stdio{In: panicReader{}, Out: spy, Err: &errb},
		}
		code := runWithCType(rc, []string{"-d", "a"}, opener)
		if code != 2 {
			t.Errorf("code=%d, want 2", code)
		}
		if spy.written {
			t.Error("stdout written despite build failure")
		}
		if closeCount.Load() != 1 {
			t.Errorf("closeCount=%d, want 1 (must close even on build failure)", closeCount.Load())
		}
	})

	t.Run("build error wins over Close error", func(t *testing.T) {
		var closeCount atomic.Int32
		opener := func(name string) (ctypeProvider, error) {
			fp := newFakeProvider(&closeCount)
			fp.isErr = errors.New("build broke")
			fp.closErr = errors.New("close broke")
			return fp, nil
		}

		var errb bytes.Buffer
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			Dir:   t.TempDir(),
			Env:   env,
			Stdio: tool.Stdio{In: panicReader{}, Out: &spyWriter{}, Err: &errb},
		}
		code := runWithCType(rc, []string{"-d", "a"}, opener)
		if code != 2 {
			t.Errorf("code=%d, want 2", code)
		}
		if !strings.Contains(errb.String(), "build broke") {
			t.Errorf("stderr %q should contain build error, not close error", errb.String())
		}
		if closeCount.Load() != 1 {
			t.Errorf("closeCount=%d, want 1", closeCount.Load())
		}
	})

	t.Run("successful build + Close error fails", func(t *testing.T) {
		var closeCount atomic.Int32
		opener := func(name string) (ctypeProvider, error) {
			fp := newFakeProvider(&closeCount)
			fp.closErr = errors.New("close broke")
			return fp, nil
		}

		var errb bytes.Buffer
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			Dir:   t.TempDir(),
			Env:   env,
			Stdio: tool.Stdio{In: panicReader{}, Out: &spyWriter{}, Err: &errb},
		}
		code := runWithCType(rc, []string{"-d", "a"}, opener)
		if code != 2 {
			t.Errorf("code=%d, want 2", code)
		}
		if !strings.Contains(errb.String(), "close broke") {
			t.Errorf("stderr %q should mention 'close broke'", errb.String())
		}
	})

	t.Run("short ToLower map fails", func(t *testing.T) {
		var closeCount atomic.Int32
		opener := func(name string) (ctypeProvider, error) {
			fp := newFakeProvider(&closeCount)
			fp.shortLo = true
			return fp, nil
		}

		var errb bytes.Buffer
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			Dir:   t.TempDir(),
			Env:   env,
			Stdio: tool.Stdio{In: panicReader{}, Out: &spyWriter{}, Err: &errb},
		}
		code := runWithCType(rc, []string{"-d", "a"}, opener)
		if code != 2 {
			t.Errorf("code=%d, want 2", code)
		}
		if !strings.Contains(errb.String(), "128 bytes, want 256") {
			t.Errorf("stderr %q should mention short map", errb.String())
		}
		if closeCount.Load() != 1 {
			t.Errorf("closeCount=%d, want 1", closeCount.Load())
		}
	})

	t.Run("short ToUpper map fails", func(t *testing.T) {
		var closeCount atomic.Int32
		opener := func(name string) (ctypeProvider, error) {
			fp := newFakeProvider(&closeCount)
			fp.shortUp = true
			return fp, nil
		}

		var errb bytes.Buffer
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			Dir:   t.TempDir(),
			Env:   env,
			Stdio: tool.Stdio{In: panicReader{}, Out: &spyWriter{}, Err: &errb},
		}
		code := runWithCType(rc, []string{"-d", "a"}, opener)
		if code != 2 {
			t.Errorf("code=%d, want 2", code)
		}
		if !strings.Contains(errb.String(), "128 bytes, want 256") {
			t.Errorf("stderr %q should mention short map", errb.String())
		}
	})

	t.Run("no I/O on any failure", func(t *testing.T) {
		// Ensure panicReader is never read on open failure.
		opener := func(name string) (ctypeProvider, error) {
			return nil, errors.New("nope")
		}
		spy := &spyWriter{}
		var errb bytes.Buffer
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			Dir:   t.TempDir(),
			Env:   env,
			Stdio: tool.Stdio{In: panicReader{}, Out: spy, Err: &errb},
		}
		code := runWithCType(rc, []string{"-d", "a"}, opener)
		if code != 2 {
			t.Errorf("code=%d, want 2", code)
		}
		if spy.written {
			t.Error("stdout was written despite failure")
		}
	})
}

// ---------------------------------------------------------------------------
// TestCTypeCaseTranslation — non-C paired-case rejects pre-I/O;
// existing C occurrence tests unchanged/green.
// ---------------------------------------------------------------------------

func TestCTypeCaseTranslation(t *testing.T) {
	t.Run("non-C paired-case translation fails pre-IO", func(t *testing.T) {
		env := []string{"LC_CTYPE=de_DE.ISO-8859-1"}
		fakeOpener := func(name string) (ctypeProvider, error) {
			var closeCount atomic.Int32
			return newFakeProvider(&closeCount), nil
		}

		spy := &spyWriter{}
		var errb bytes.Buffer
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			Dir:   t.TempDir(),
			Env:   env,
			Stdio: tool.Stdio{In: panicReader{}, Out: spy, Err: &errb},
		}
		code := runWithCType(rc, []string{"[:lower:]", "[:upper:]"}, fakeOpener)
		if code != 2 {
			t.Errorf("code=%d, want 2", code)
		}
		if spy.written {
			t.Error("stdout written despite rejection")
		}
		if !strings.Contains(errb.String(), "not yet supported") {
			t.Errorf("stderr %q should contain unsupported message", errb.String())
		}
		if !strings.Contains(errb.String(), "de_DE.ISO-8859-1") {
			t.Errorf("stderr %q should mention locale", errb.String())
		}
	})

	t.Run("C locale paired-case still works", func(t *testing.T) {
		out, errb, code := runTool(t, "Hello World", "[:lower:]", "[:upper:]")
		if code != 0 {
			t.Errorf("code=%d err=%q", code, errb)
		}
		if out != "HELLO WORLD" {
			t.Errorf("out=%q, want %q", out, "HELLO WORLD")
		}
	})

	t.Run("C locale upper-to-lower still works", func(t *testing.T) {
		out, errb, code := runTool(t, "HELLO", "[:upper:]", "[:lower:]")
		if code != 0 {
			t.Errorf("code=%d err=%q", code, errb)
		}
		if out != "hello" {
			t.Errorf("out=%q, want %q", out, "hello")
		}
	})

	t.Run("non-C ordinary class in delete is OK", func(t *testing.T) {
		// Ordinary classes (non-case) should work under non-C.
		env := []string{"LC_CTYPE=fake_locale"}
		fakeOpener := func(name string) (ctypeProvider, error) {
			var closeCount atomic.Int32
			return newFakeProvider(&closeCount), nil
		}

		out, errb, code := runWithOpener(t, env, "!\xB0abc", fakeOpener, "-d", "[:punct:]")
		if code != 0 {
			t.Fatalf("code=%d err=%q", code, errb)
		}
		if out != "abc" {
			t.Errorf("out=%q, want %q", out, "abc")
		}
	})
}

// ---------------------------------------------------------------------------
// TestCTypeComplementFlags — -c with provider class; -C checks LC_COLLATE
// ---------------------------------------------------------------------------

func TestCTypeComplementFlags(t *testing.T) {
	fakeOpener := func(name string) (ctypeProvider, error) {
		var closeCount atomic.Int32
		return newFakeProvider(&closeCount), nil
	}

	t.Run("-c works with provider class", func(t *testing.T) {
		// -c complement with fake digit class: delete everything NOT in [:digit:]
		// Fake digit: {0x30('0'), 0x90}
		env := []string{"LC_CTYPE=fake_locale"}
		input := "0\x90abc"
		out, errb, code := runWithOpener(t, env, input, fakeOpener, "-c", "-d", "[:digit:]")
		if code != 0 {
			t.Fatalf("code=%d err=%q", code, errb)
		}
		want := "0\x90"
		if out != want {
			t.Errorf("out=%q, want %q", out, want)
		}
	})

	t.Run("-C succeeds with C/POSIX LC_COLLATE", func(t *testing.T) {
		// -C with C/POSIX collate should work.
		// -C -d a = delete complement of {a} = keep only a.
		for _, collate := range []string{"C", "POSIX", ""} {
			env := []string{"LC_COLLATE=" + collate}
			out, errb, code := runWithOpener(t, env, "abc", neverOpener(), "-C", "-d", "a")
			if code != 0 {
				t.Errorf("LC_COLLATE=%q: code=%d err=%q", collate, code, errb)
			}
			if out != "a" {
				t.Errorf("LC_COLLATE=%q: out=%q, want %q", collate, out, "a")
			}
		}
	})

	t.Run("-C works with non-C LC_COLLATE", func(t *testing.T) {
		// -C must not fail merely because LC_COLLATE is non-C.
		// -C -d a = delete complement of {a} = keep only a.
		env := []string{"LC_COLLATE=de_DE"}
		out, errb, code := runWithOpener(t, env, "abc", neverOpener(), "-C", "-d", "a")
		if code != 0 {
			t.Errorf("code=%d err=%q", code, errb)
		}
		if out != "a" {
			t.Errorf("out=%q, want %q", out, "a")
		}
	})

	t.Run("-C works with non-C LC_COLLATE via LC_ALL", func(t *testing.T) {
		// LC_ALL overrides LC_COLLATE and LC_CTYPE. Supply a working
		// opener so LC_CTYPE resolution doesn't panic.
		fakeOpener := func(name string) (ctypeProvider, error) {
			var closeCount atomic.Int32
			return newFakeProvider(&closeCount), nil
		}
		env := []string{"LC_COLLATE=C", "LC_ALL=de_DE"}
		out, errb, code := runWithOpener(t, env, "abc", fakeOpener, "-C", "-d", "a")
		if code != 0 {
			t.Errorf("code=%d err=%q", code, errb)
		}
		if out != "a" {
			t.Errorf("out=%q, want %q", out, "a")
		}
	})
}

// ---------------------------------------------------------------------------
// TestCTypeClassFromTable — regression: recognised empty classes vs bogus
// ---------------------------------------------------------------------------

func TestCTypeClassFromTable(t *testing.T) {
	// Regression: classFromTable must return true for every recognised
	// class even when its membership is empty, and false only for an
	// unrecognised name.  This proves that a provider whose class has
	// zero members is accepted (no "invalid character class" error),
	// while a genuinely bogus name is still rejected.

	t.Run("known empty provider class is accepted", func(t *testing.T) {
		// Build a fake provider where "punct" has zero members.
		fp := &fakeProvider{
			classes: map[string]map[byte]bool{
				"alnum":  {},
				"alpha":  {},
				"blank":  {},
				"cntrl":  {},
				"digit":  {},
				"graph":  {},
				"lower":  {},
				"print":  {},
				"punct":  {}, // empty — no bytes
				"space":  {},
				"upper":  {},
				"xdigit": {},
			},
		}
		for i := 0; i < 256; i++ {
			fp.lower[i] = byte(i)
			fp.upper[i] = byte(i)
		}

		tables, err := buildProviderTables(fp)
		if err != nil {
			t.Fatalf("buildProviderTables: %v", err)
		}

		// classFromTable should recognise "punct" (return true) and
		// return a zero-length members slice (nil or empty).
		members, ok := tables.classFromTable("punct")
		if !ok {
			t.Error("classFromTable(\"punct\"): ok=false, want true")
		}
		if len(members) != 0 {
			t.Errorf("classFromTable(\"punct\"): len=%d, want 0", len(members))
		}
	})

	t.Run("bogus class is rejected", func(t *testing.T) {
		// A class name not known to the tables must return (nil, false).
		fp := &fakeProvider{
			classes: map[string]map[byte]bool{},
		}
		for i := 0; i < 256; i++ {
			fp.lower[i] = byte(i)
			fp.upper[i] = byte(i)
		}
		tables, err := buildProviderTables(fp)
		if err != nil {
			t.Fatalf("buildProviderTables: %v", err)
		}

		members, ok := tables.classFromTable("bogus")
		if ok {
			t.Error("classFromTable(\"bogus\"): ok=true, want false")
		}
		if members != nil {
			t.Errorf("classFromTable(\"bogus\"): members=%v, want nil", members)
		}
	})

	t.Run("end-to-end: empty class accepted in -d mode", func(t *testing.T) {
		// Full end-to-end: a provider with an empty "punct" class should
		// delete nothing (empty class), not fail with "invalid character
		// class".
		fakeOpener := func(name string) (ctypeProvider, error) {
			fp := &fakeProvider{
				classes: map[string]map[byte]bool{
					"alpha":  {},
					"upper":  {},
					"lower":  {},
					"digit":  {},
					"alnum":  {},
					"blank":  {},
					"space":  {},
					"cntrl":  {},
					"graph":  {},
					"print":  {},
					"punct":  {}, // empty
					"xdigit": {},
				},
			}
			for i := 0; i < 256; i++ {
				fp.lower[i] = byte(i)
				fp.upper[i] = byte(i)
			}
			return fp, nil
		}
		env := []string{"LC_CTYPE=empty_punct_locale"}
		// An empty class deletes nothing, so input should pass through.
		out, errb, code := runWithOpener(t, env, "hello!", fakeOpener, "-d", "[:punct:]")
		if code != 0 {
			t.Fatalf("code=%d err=%q", code, errb)
		}
		if out != "hello!" {
			t.Errorf("out=%q, want %q", out, "hello!")
		}
	})

	t.Run("end-to-end: bogus class rejected", func(t *testing.T) {
		fakeOpener := func(name string) (ctypeProvider, error) {
			return newFakeProvider(nil), nil
		}
		env := []string{"LC_CTYPE=fake_locale"}
		_, errb, code := runWithOpener(t, env, "test", fakeOpener, "-d", "[:bogus:]")
		if code == 0 {
			t.Error("bogus class should fail")
		}
		if !strings.Contains(errb, "invalid character class") {
			t.Errorf("stderr %q should mention 'invalid character class'", errb)
		}
	})
}

// neverOpener returns a ctypeOpener that never opens (for C/POSIX tests).
func neverOpener() ctypeOpener {
	return func(name string) (ctypeProvider, error) {
		panic(fmt.Sprintf("opener called for %q; should never happen", name))
	}
}
