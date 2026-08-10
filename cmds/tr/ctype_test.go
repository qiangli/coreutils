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
	classes  map[string]map[byte]bool // class name → byte membership
	lower    [256]byte
	upper    [256]byte
	closed   atomic.Bool
	closeN   *atomic.Int32 // shared counter
	isErr    error         // if non-nil, every Is* returns this
	lowerErr error         // if non-nil, ToLower returns this
	upperErr error         // if non-nil, ToUpper returns this
	closErr  error         // if non-nil, Close returns this
	loLen    *int          // if non-nil, ToLower returns exactly this many bytes
	upLen    *int          // if non-nil, ToUpper returns exactly this many bytes

	// probe instrumentation (buildProviderTables is single-goroutine, so
	// plain ints/slices are race-free here).
	isCalls map[string]int // per-class Is* invocation count (nil = untracked)
	loCalls int            // ToLower invocation count
	upCalls int            // ToUpper invocation count
	loRecv  []byte         // exact bytes handed to the last ToLower call
	upRecv  []byte         // exact bytes handed to the last ToUpper call
}

func (f *fakeProvider) check(class string, b byte) (bool, error) {
	if f.isCalls != nil {
		f.isCalls[class]++
	}
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
	f.loCalls++
	f.loRecv = append([]byte(nil), in...)
	if f.lowerErr != nil {
		return nil, f.lowerErr
	}
	out := make([]byte, len(in))
	for i, b := range in {
		out[i] = f.lower[b]
	}
	if f.loLen != nil {
		return resizeBytes(out, *f.loLen), nil
	}
	return out, nil
}

func (f *fakeProvider) ToUpper(in []byte) ([]byte, error) {
	f.upCalls++
	f.upRecv = append([]byte(nil), in...)
	if f.upperErr != nil {
		return nil, f.upperErr
	}
	out := make([]byte, len(in))
	for i, b := range in {
		out[i] = f.upper[b]
	}
	if f.upLen != nil {
		return resizeBytes(out, *f.upLen), nil
	}
	return out, nil
}

// resizeBytes returns b at exactly length n: truncated when n<=len(b),
// zero-padded when longer.  Used to simulate a provider that returns the
// wrong number of case-mapped bytes.
func resizeBytes(b []byte, n int) []byte {
	if n <= len(b) {
		return b[:n]
	}
	return append(b, make([]byte, n-len(b))...)
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

// newCaseTestProvider builds a fresh deterministic provider for paired case tests:
// lower={a,b,DF,FF}, upper={A,B}, punct={!,'\xB0'}; all 256 maps identity
// except upper a->A,b->B and lower A->a,B->b.
func newCaseTestProvider(closeN *atomic.Int32) *fakeProvider {
	fp := &fakeProvider{
		closeN: closeN,
		classes: map[string]map[byte]bool{
			"lower": {0x61: true, 0x62: true, 0xDF: true, 0xFF: true},
			"upper": {0x41: true, 0x42: true},
			"punct": {0x21: true, 0xB0: true},
		},
	}
	for i := 0; i < 256; i++ {
		fp.lower[i] = byte(i)
		fp.upper[i] = byte(i)
	}
	fp.upper[0x61] = 0x41
	fp.upper[0x62] = 0x42
	fp.lower[0x41] = 0x61
	fp.lower[0x42] = 0x62
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

	t.Run("pre-open exits bypass opener", func(t *testing.T) {
		// help/version, an unknown flag, and operand-count errors are all
		// resolved before LC_CTYPE is opened.  Under a non-C locale (which
		// WOULD open a provider) the opener must still never be called.
		env := []string{"LC_CTYPE=fake_LOCALE"}
		panicOpener := func(name string) (ctypeProvider, error) {
			t.Fatalf("opener called for %q; pre-open exit must bypass it", name)
			return nil, nil
		}
		cases := []struct {
			name     string
			args     []string
			wantCode int
		}{
			{"help", []string{"--help"}, 0},
			{"version", []string{"--version"}, 0},
			{"unknown flag", []string{"-Z", "a"}, 2},
			{"missing operand", nil, 2},
			{"extra operand", []string{"a", "b", "c"}, 2},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				_, errb, code := runWithOpener(t, env, "", panicOpener, c.args...)
				if code != c.wantCode {
					t.Errorf("code=%d, want %d (err=%q)", code, c.wantCode, errb)
				}
			})
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

	t.Run("provider closed exactly once before stream read and write on success", func(t *testing.T) {
		// Both the input reader and the output writer must observe the
		// provider as already closed, and Close must fire exactly once.
		closeCount.Store(0)
		var providerClosed atomic.Bool
		var stdinReadBeforeClose atomic.Bool
		var stdoutWriteBeforeClose atomic.Bool
		trackOpener := func(name string) (ctypeProvider, error) {
			fp := newFakeProvider(&closeCount)
			// wrap close to set flag
			return &closeTracker{fp, &providerClosed}, nil
		}

		pr := &trackingReader{closed: &providerClosed, readBeforeClose: &stdinReadBeforeClose}
		pw := &trackingWriter{closed: &providerClosed, writeBeforeClose: &stdoutWriteBeforeClose}
		var errb bytes.Buffer
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			Dir:   t.TempDir(),
			Env:   env,
			Stdio: tool.Stdio{In: pr, Out: pw, Err: &errb},
		}
		// "test" contains no digits, so all four bytes pass through and
		// reach the writer — proving the write path also runs post-close.
		code := runWithCType(rc, []string{"-d", "[:digit:]"}, trackOpener)
		if code != 0 {
			t.Fatalf("code=%d err=%q", code, errb.String())
		}
		if stdinReadBeforeClose.Load() {
			t.Error("stdin was read before provider was closed")
		}
		if stdoutWriteBeforeClose.Load() {
			t.Error("stdout was written before provider was closed")
		}
		if pw.calls != 1 || string(pw.data) != "test" {
			t.Errorf("stdout writes=%d data=%q, want 1 and %q", pw.calls, pw.data, "test")
		}
		if closeCount.Load() != 1 {
			t.Errorf("closeCount=%d, want 1", closeCount.Load())
		}
	})

	t.Run("buildProviderTables probes every predicate over 00..FF exactly", func(t *testing.T) {
		var closeCount atomic.Int32
		// One unique sentinel byte per class proves each Is* predicate
		// populates its own table slot with no cross-wiring.
		sentinel := map[string]byte{
			"alnum": 0x01, "alpha": 0x02, "blank": 0x03, "cntrl": 0x04,
			"digit": 0x05, "graph": 0x06, "lower": 0x07, "print": 0x08,
			"punct": 0x09, "space": 0x0A, "upper": 0x0B, "xdigit": 0x0C,
		}
		classes := map[string]map[byte]bool{}
		for name, b := range sentinel {
			classes[name] = map[byte]bool{b: true}
		}
		fp := &fakeProvider{
			classes: classes,
			closeN:  &closeCount,
			isCalls: map[string]int{},
		}
		// Distinct, non-identity case maps so the copied tables can be
		// asserted byte-for-byte.
		for i := 0; i < 256; i++ {
			fp.lower[i] = byte(255 - i)
			fp.upper[i] = byte((i + 7) & 0xFF)
		}

		tables, err := buildProviderTables(fp)
		if err != nil {
			t.Fatalf("buildProviderTables: %v", err)
		}

		got := map[string][]byte{
			"alnum": tables.alnum, "alpha": tables.alpha, "blank": tables.blank,
			"cntrl": tables.cntrl, "digit": tables.digit, "graph": tables.graph,
			"lower": tables.lower, "print": tables.print, "punct": tables.punct,
			"space": tables.space, "upper": tables.upper, "xdigit": tables.xdigit,
		}
		for name, b := range sentinel {
			if !bytes.Equal(got[name], []byte{b}) {
				t.Errorf("class %s table=%v, want [%d]", name, got[name], b)
			}
			members, known := tables.classFromTable(name)
			if !known || !bytes.Equal(members, []byte{b}) {
				t.Errorf("classFromTable(%q)=(%v,%v), want ([%d],true)", name, members, known, b)
			}
			if fp.isCalls[name] != 256 {
				t.Errorf("Is%s probed %d times, want 256", name, fp.isCalls[name])
			}
		}

		want := make([]byte, 256)
		for i := range want {
			want[i] = byte(i)
		}
		if fp.loCalls != 1 || fp.upCalls != 1 {
			t.Errorf("case calls lo=%d up=%d, want 1 and 1", fp.loCalls, fp.upCalls)
		}
		if !bytes.Equal(fp.loRecv, want) {
			t.Errorf("ToLower received %v, want 00..FF", fp.loRecv)
		}
		if !bytes.Equal(fp.upRecv, want) {
			t.Errorf("ToUpper received %v, want 00..FF", fp.upRecv)
		}
		for i := 0; i < 256; i++ {
			if tables.toLower[i] != fp.lower[i] {
				t.Fatalf("toLower[%d]=%d, want %d", i, tables.toLower[i], fp.lower[i])
			}
			if tables.toUpper[i] != fp.upper[i] {
				t.Fatalf("toUpper[%d]=%d, want %d", i, tables.toUpper[i], fp.upper[i])
			}
		}
		if closeCount.Load() != 1 {
			t.Errorf("closeCount=%d, want 1", closeCount.Load())
		}
	})
}

// closeTracker wraps a fakeProvider and sets a flag when Close is called.
type closeTracker struct {
	*fakeProvider
	closed *atomic.Bool
}

func (ct *closeTracker) Close() error {
	err := ct.fakeProvider.Close()
	ct.closed.Store(true)
	return err
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

// trackingWriter records whether Write was called before the provider closed.
type trackingWriter struct {
	closed           *atomic.Bool
	writeBeforeClose *atomic.Bool
	calls            int
	data             []byte
}

func (w *trackingWriter) Write(p []byte) (int, error) {
	if !w.closed.Load() {
		w.writeBeforeClose.Store(true)
	}
	w.calls++
	w.data = append(w.data, p...)
	return len(p), nil
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

	t.Run("case-map build failures close once with no stream I/O", func(t *testing.T) {
		// Method-specific ToLower/ToUpper errors and wrong-length returns
		// (255 short, 257 long) must each fail pre-I/O, close the provider
		// exactly once, and surface the stable diagnostic.
		l255, l257 := 255, 257
		loErr := errors.New("tolower broke")
		upErr := errors.New("toupper broke")
		cases := []struct {
			name    string
			mutate  func(*fakeProvider)
			wantErr string
		}{
			{"ToLower error", func(fp *fakeProvider) { fp.lowerErr = loErr }, "ToLower: tolower broke"},
			{"ToUpper error", func(fp *fakeProvider) { fp.upperErr = upErr }, "ToUpper: toupper broke"},
			{"ToLower short 255", func(fp *fakeProvider) { fp.loLen = &l255 }, "ToLower returned 255 bytes, want 256"},
			{"ToLower long 257", func(fp *fakeProvider) { fp.loLen = &l257 }, "ToLower returned 257 bytes, want 256"},
			{"ToUpper short 255", func(fp *fakeProvider) { fp.upLen = &l255 }, "ToUpper returned 255 bytes, want 256"},
			{"ToUpper long 257", func(fp *fakeProvider) { fp.upLen = &l257 }, "ToUpper returned 257 bytes, want 256"},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				var closeCount atomic.Int32
				opener := func(name string) (ctypeProvider, error) {
					fp := newFakeProvider(&closeCount)
					c.mutate(fp)
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
				if !strings.Contains(errb.String(), c.wantErr) {
					t.Errorf("stderr %q should contain %q", errb.String(), c.wantErr)
				}
				if closeCount.Load() != 1 {
					t.Errorf("closeCount=%d, want 1", closeCount.Load())
				}
			})
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
// TestCTypeCaseTranslation — provider-backed and C-locale case occurrences.
// ---------------------------------------------------------------------------

func TestCTypeCaseTranslation(t *testing.T) {
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

	t.Run("C locale mixed paired and unpaired class fill", func(t *testing.T) {
		out, errb, code := runTool(t, "abAB12", "[:lower:][:upper:]12", "[:upper:][x*]yz")
		if code != 0 || errb != "" || out != "ABxxyz" {
			t.Errorf("code=%d stdout=%q stderr=%q, want 0, %q, empty", code, out, errb, "ABxxyz")
		}
	})

	t.Run("C locale fill candidate at raw capacity boundary", func(t *testing.T) {
		out, errb, code := runTool(t, "12az", "12[:lower:]", "[x*][:upper:]")
		if code != 0 || errb != "" || out != "xxAZ" {
			t.Errorf("code=%d stdout=%q stderr=%q, want 0, %q, empty", code, out, errb, "xxAZ")
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

	t.Run("provider paired-case translation rows", func(t *testing.T) {
		tests := []struct {
			name       string
			args       []string
			stdin      string
			wantStdout string
			wantStderr string
			wantCode   int
			reject     bool
		}{
			{
				name:       "whole lower->upper",
				args:       []string{"[:lower:]", "[:upper:]"},
				stdin:      "ab\xdf\xffAB",
				wantStdout: "AB\xdf\xffAB",
				wantCode:   0,
			},
			{
				name:       "prefix and suffix preserve logical case position",
				args:       []string{"1[:lower:]3", "2[:upper:]4"},
				stdin:      "1ab\xdf\xff3",
				wantStdout: "2AB\xdf\xff4",
				wantCode:   0,
			},
			{
				name:       "adjacent lower+upper -> upper+lower",
				args:       []string{"[:lower:][:upper:]", "[:upper:][:lower:]"},
				stdin:      "ab\xdf\xffAB",
				wantStdout: "AB\xdf\xffab",
				wantCode:   0,
			},
			{
				name:       "repeated lower+lower -> upper+lower unchanged (later identity)",
				args:       []string{"[:lower:][:lower:]", "[:upper:][:lower:]"},
				stdin:      "ab\xdf\xff",
				wantStdout: "ab\xdf\xff",
				wantCode:   0,
			},
			{
				name:       "A+upper -> x+lower gives a",
				args:       []string{"A[:upper:]", "x[:lower:]"},
				stdin:      "A",
				wantStdout: "a",
				wantCode:   0,
			},
			{
				name:       "upper+A -> lower+x gives x",
				args:       []string{"[:upper:]A", "[:lower:]x"},
				stdin:      "A",
				wantStdout: "x",
				wantCode:   0,
			},
			{
				name:       "fill-after lower12 -> upper[x*] gives mapped class plus exactly xx",
				args:       []string{"[:lower:]12", "[:upper:][x*]"},
				stdin:      "ab\xdf\xff12",
				wantStdout: "AB\xdf\xffxx",
				wantCode:   0,
			},
			{
				name:       "fill-before 12lower -> [x*]upper gives exactly xx then mapped class",
				args:       []string{"12[:lower:]", "[x*][:upper:]"},
				stdin:      "12ab\xdf\xff",
				wantStdout: "xxAB\xdf\xff",
				wantCode:   0,
			},
			{
				name:       "ordinary provider class before paired fill keeps raw cardinality",
				args:       []string{"[:punct:][:lower:]", "[x*][:upper:]"},
				stdin:      "!\xb0ab\xdf\xff",
				wantStdout: "xxAB\xdf\xff",
				wantCode:   0,
			},
			{
				name:       "unpaired lower fill keeps raw cardinality",
				args:       []string{"[:lower:]12", "[x*]yz"},
				stdin:      "ab\xdf\xff12",
				wantStdout: "xxxxyz",
				wantCode:   0,
			},
			{
				name:       "default padding after paired class repeats final literal",
				args:       []string{"[:lower:]12", "[:upper:]x"},
				stdin:      "ab\xdf\xff12",
				wantStdout: "AB\xdf\xffxx",
				wantCode:   0,
			},
			{
				name:       "truncate after paired class leaves unmatched source unchanged",
				args:       []string{"-t", "[:lower:]12", "[:upper:]x"},
				stdin:      "ab\xdf\xff12",
				wantStdout: "AB\xdf\xffx2",
				wantCode:   0,
			},
			{
				name:       "unpaired lower->x gives xxxx",
				args:       []string{"[:lower:]", "x"},
				stdin:      "ab\xdf\xff",
				wantStdout: "xxxx",
				wantCode:   0,
			},
			{
				name:       "target inside source reject",
				args:       []string{"[:lower:]", "x[:upper:]"},
				stdin:      "",
				wantStderr: "misaligned [:upper:] and/or [:lower:] construct",
				reject:     true,
			},
			{
				name:       "target at EOF reject",
				args:       []string{"a", "x[:upper:]"},
				stdin:      "",
				wantStderr: "misaligned [:upper:] and/or [:lower:] construct",
				reject:     true,
			},
			{
				name:       "target class tail before source suffix rejects exactly",
				args:       []string{"1[:lower:]3", "2[:upper:]"},
				stdin:      "",
				wantStderr: "when translating with string1 longer than string2,\nthe latter string must not end with a character class",
				reject:     true,
			},
			{
				name:       "fill cannot exceed zero raw capacity to reach case class",
				args:       []string{"12[:upper:]", "[x*][:lower:]"},
				stdin:      "",
				wantStderr: "misaligned [:upper:] and/or [:lower:] construct",
				reject:     true,
			},
		}

		for _, tt := range tests {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				var openCount, closeCount atomic.Int32
				opener := func(name string) (ctypeProvider, error) {
					openCount.Add(1)
					return newCaseTestProvider(&closeCount), nil
				}

				env := []string{"LC_CTYPE=fake_locale"}
				out, errb, code := runWithOpener(t, env, tt.stdin, opener, tt.args...)

				if openCount.Load() != 1 || closeCount.Load() != 1 {
					t.Errorf("open=%d close=%d, want 1 and 1", openCount.Load(), closeCount.Load())
				}

				if tt.reject {
					if code != 1 {
						t.Errorf("code=%d, want 1", code)
					}
					wantErr := "tr: " + tt.wantStderr + "\n"
					if errb != wantErr {
						t.Errorf("stderr=%q, want %q", errb, wantErr)
					}
				} else {
					if code != tt.wantCode {
						t.Errorf("code=%d, want %d (stderr=%q)", code, tt.wantCode, errb)
					}
					if out != tt.wantStdout {
						t.Errorf("out=%q, want %q", out, tt.wantStdout)
					}
					if errb != "" {
						t.Errorf("stderr=%q, want empty", errb)
					}
				}
			})
		}
	})
}

// ---------------------------------------------------------------------------
// TestCTypeComplementFlags — -c/-C remain complement aliases with provider classes
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

	t.Run("lowercase -c is independent of non-C LC_COLLATE", func(t *testing.T) {
		// The removed blanket gate must not resurrect: -c works regardless
		// of LC_COLLATE. -c -d a keeps only the complement's complement {a}.
		env := []string{"LC_COLLATE=de_DE"}
		out, errb, code := runWithOpener(t, env, "abc", neverOpener(), "-c", "-d", "a")
		if code != 0 {
			t.Errorf("code=%d err=%q", code, errb)
		}
		if out != "a" {
			t.Errorf("out=%q, want %q", out, "a")
		}
	})

	t.Run("--complement long form is independent of non-C LC_COLLATE", func(t *testing.T) {
		env := []string{"LC_COLLATE=fr_FR.UTF-8"}
		out, errb, code := runWithOpener(t, env, "abc", neverOpener(), "--complement", "-d", "a")
		if code != 0 {
			t.Errorf("code=%d err=%q", code, errb)
		}
		if out != "a" {
			t.Errorf("out=%q, want %q", out, "a")
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

	for _, args := range [][]string{{"-C", "-d", "[:digit:]"}, {"-Cd", "[:digit:]"}} {
		args := args
		t.Run("provider-backed "+strings.Join(args, " "), func(t *testing.T) {
			var openCount, closeCount atomic.Int32
			opener := func(name string) (ctypeProvider, error) {
				openCount.Add(1)
				return newFakeProvider(&closeCount), nil
			}
			out, errb, code := runWithOpener(t, []string{"LC_CTYPE=fake_locale"}, "0\x90x", opener, args...)
			if code != 0 || errb != "" || out != "0\x90" {
				t.Fatalf("out=%q err=%q code=%d, want %q, empty stderr, 0", out, errb, code, "0\x90")
			}
			if openCount.Load() != 1 || closeCount.Load() != 1 {
				t.Fatalf("open=%d close=%d, want 1 and 1", openCount.Load(), closeCount.Load())
			}
		})
	}
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

	t.Run("all known empty provider classes are recognized", func(t *testing.T) {
		tables := &ctypeTables{}
		for _, name := range []string{"alnum", "alpha", "blank", "cntrl", "digit", "graph", "lower", "print", "punct", "space", "upper", "xdigit"} {
			members, known := tables.classFromTable(name)
			if !known || len(members) != 0 {
				t.Errorf("classFromTable(%q)=(%v,%v), want empty,true", name, members, known)
			}
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
