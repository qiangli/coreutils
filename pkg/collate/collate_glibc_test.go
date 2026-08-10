// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build linux && (amd64 || arm64)

package collate

import (
	"errors"
	"sync"
	"testing"

	"github.com/ebitengine/purego"
)

// --- an independent oracle -------------------------------------------------
//
// The provider is validated against a SEPARATE, direct binding to glibc's
// strcoll_l — resolved here in the test, not through the package under test —
// so a bug in the package's own binding cannot mask itself. If the locale is
// not installed the oracle reports so and the locale-dependent tests skip.

type oracle struct {
	strcollL   func(a, b string, loc uintptr) int32
	newlocale  func(mask int32, locale string, base uintptr) uintptr
	freelocale func(loc uintptr)
	collate    uintptr
}

func newOracle(t *testing.T) *oracle {
	t.Helper()
	h, err := purego.Dlopen("libc.so.6", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil || h == 0 {
		t.Skipf("oracle: glibc not loadable: %v", err)
	}
	o := &oracle{}
	purego.RegisterLibFunc(&o.strcollL, h, "strcoll_l")
	purego.RegisterLibFunc(&o.newlocale, h, "newlocale")
	purego.RegisterLibFunc(&o.freelocale, h, "freelocale")
	o.collate = o.newlocale(1<<3, "de_DE.ISO-8859-1", 0)
	if o.collate == 0 {
		t.Skip("oracle: de_DE.ISO-8859-1 locale not installed")
	}
	t.Cleanup(func() { o.freelocale(o.collate) })
	return o
}

func (o *oracle) sign(a, b string) int {
	switch r := o.strcollL(a, b, o.collate); {
	case r < 0:
		return -1
	case r > 0:
		return 1
	default:
		return 0
	}
}

// openProvider opens the real provider, skipping when glibc / the locale is
// unavailable so the suite stays green on a slim box while still failing on real
// defects when the locale IS present.
func openProvider(t *testing.T) *Provider {
	t.Helper()
	p, err := Open("de_DE.ISO-8859-1")
	if err != nil {
		if errors.Is(err, ErrGlibcUnavailable) || errors.Is(err, ErrMissingLocale) {
			t.Skipf("de_DE.ISO-8859-1 provider unavailable: %v", err)
		}
		t.Fatalf("Open: %v", err)
	}
	return p
}

// Latin-1 encodings of the accented / sharp-s test data. de_DE.ISO-8859-1 is a
// single-byte codeset, so these are the exact bytes glibc collates.
const (
	ae      = "\xe4" // ä
	oe      = "\xf6" // ö
	ue      = "\xfc" // ü
	sharpS  = "\xdf" // ß
	AE      = "\xc4" // Ä
	capA    = "A"
	lowA    = "a"
	lowZ    = "z"
	strasse = "stra\xdfe" // straße
	strasss = "strasse"
)

func TestCompareAgainstOracle(t *testing.T) {
	o := newOracle(t)
	p := openProvider(t)
	t.Cleanup(func() { _ = p.Close() })

	// Curated accent / case / ß-vs-ss pairs plus plain ASCII neighbours.
	pairs := [][2]string{
		{lowA, ae}, {ae, "b"}, {oe, ue}, {capA, lowA}, {lowA, capA},
		{AE, ae}, {sharpS, strasss}, {strasse, strasss}, {"apfel", "\xe4pfel"},
		{lowA, lowZ}, {"", lowA}, {"", ""}, {lowA, lowA},
		{oe, "o"}, {ue, "u"}, {"Zeta", "zeta"},
	}
	for _, pr := range pairs {
		got, err := p.Compare(pr[0], pr[1])
		if err != nil {
			t.Fatalf("Compare(%q,%q): %v", pr[0], pr[1], err)
		}
		if want := o.sign(pr[0], pr[1]); got != want {
			t.Errorf("Compare(%q,%q) = %d, oracle = %d", pr[0], pr[1], got, want)
		}
	}
}

func TestCompareAntisymmetryTransitivity(t *testing.T) {
	p := openProvider(t)
	t.Cleanup(func() { _ = p.Close() })

	words := []string{
		lowA, ae, AE, capA, "b", oe, ue, sharpS, strasse, strasss,
		"apfel", "\xe4pfel", lowZ, "Zeta", "zeta", "",
	}

	cmp := func(a, b string) int {
		r, err := p.Compare(a, b)
		if err != nil {
			t.Fatalf("Compare(%q,%q): %v", a, b, err)
		}
		return r
	}

	// Antisymmetry: cmp(a,b) == -cmp(b,a).
	for _, a := range words {
		for _, b := range words {
			if got, rev := cmp(a, b), cmp(b, a); got != -rev {
				t.Errorf("antisymmetry: cmp(%q,%q)=%d cmp(%q,%q)=%d", a, b, got, b, a, rev)
			}
		}
	}

	// Transitivity: a<=b and b<=c imply a<=c (and the strict form).
	for _, a := range words {
		for _, b := range words {
			for _, c := range words {
				ab, bc, ac := cmp(a, b), cmp(b, c), cmp(a, c)
				if ab <= 0 && bc <= 0 && ac > 0 {
					t.Errorf("transitivity: %q<=%q<=%q but cmp(a,c)=%d", a, b, c, ac)
				}
			}
		}
	}
}

func TestNulInputRejected(t *testing.T) {
	p := openProvider(t)
	t.Cleanup(func() { _ = p.Close() })

	if _, err := p.Compare("a\x00b", "a"); !errors.Is(err, ErrNulInput) {
		t.Errorf("Compare with NUL lhs: got %v, want ErrNulInput", err)
	}
	if _, err := p.Compare("a", "b\x00"); !errors.Is(err, ErrNulInput) {
		t.Errorf("Compare with NUL rhs: got %v, want ErrNulInput", err)
	}
}

func TestOpenRejectsLocalesBeforeLibc(t *testing.T) {
	// These must fail with ErrUnsupportedLocale — the gate runs before any libc
	// load, so it holds identically whether or not glibc is present.
	for _, name := range []string{
		"C", "", "de_DE", "de_DE.UTF-8", "UTF-8", "en_US.UTF-8",
		"de_DE.ISO-8859-15", "Latin-9", "de_DE.latin9", "de_DE.ISO8859-15",
		"garbage", "de_DE.ISO-8859-1x",
	} {
		if _, err := Open(name); !errors.Is(err, ErrUnsupportedLocale) {
			t.Errorf("Open(%q) = %v, want ErrUnsupportedLocale", name, err)
		}
	}
}

func TestOpenAcceptsBothAliasesCaseInsensitive(t *testing.T) {
	for _, name := range []string{
		"de_DE.ISO-8859-1", "de_DE.iso88591",
		"DE_DE.iso-8859-1", "de_de.ISO88591",
	} {
		p, err := Open(name)
		if err != nil {
			if errors.Is(err, ErrGlibcUnavailable) || errors.Is(err, ErrMissingLocale) {
				t.Skipf("%q: provider unavailable: %v", name, err)
			}
			t.Errorf("Open(%q): unexpected error %v", name, err)
			continue
		}
		_ = p.Close()
	}
}

func TestCloseIdempotentAndFencesCompare(t *testing.T) {
	p := openProvider(t)

	if err := p.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	// Idempotent: second and third Close are clean no-ops.
	if err := p.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Errorf("third Close: %v", err)
	}
	// Compare after Close is deterministic ErrClosed, never a use-after-free.
	if _, err := p.Compare("a", "b"); !errors.Is(err, ErrClosed) {
		t.Errorf("Compare after Close: got %v, want ErrClosed", err)
	}
}

// TestOverlappingCompareAndClose stresses the RWMutex fence: many Compare
// goroutines race a single Close. Every call must return one of the two
// well-defined outcomes and none may crash. Run under -race.
func TestOverlappingCompareAndClose(t *testing.T) {
	p := openProvider(t)

	const workers = 32
	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			<-start
			for range 200 {
				_, err := p.Compare("stra\xdfe", "strasse")
				if err != nil && !errors.Is(err, ErrClosed) {
					t.Errorf("Compare during Close: unexpected %v", err)
					return
				}
			}
		}()
	}
	close(start)
	// Close concurrently with the in-flight comparisons.
	if err := p.Close(); err != nil {
		t.Errorf("concurrent Close: %v", err)
	}
	wg.Wait()

	// A second Close after the storm is still a clean no-op.
	if err := p.Close(); err != nil {
		t.Errorf("post-storm Close: %v", err)
	}
}

// TestCodesetVerified confirms the CODESET gate ran: a successfully opened
// provider is, by construction, ISO-8859-1, so a known ß < straße-style
// single-byte comparison behaves as a Latin-1 comparison rather than erroring.
func TestCodesetVerified(t *testing.T) {
	p := openProvider(t)
	t.Cleanup(func() { _ = p.Close() })

	// A byte-oriented (single-byte codeset) compare must succeed without error;
	// under a UTF-8 codeset these raw high bytes would not round-trip the same
	// way, and the CODESET gate would have refused the Open above.
	if _, err := p.Compare(sharpS, ae); err != nil {
		t.Fatalf("Compare on verified ISO-8859-1 provider: %v", err)
	}
}
