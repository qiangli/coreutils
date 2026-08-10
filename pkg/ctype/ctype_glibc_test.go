// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build linux && (amd64 || arm64)

package ctype

import (
	"errors"
	"runtime"
	"sync"
	"testing"

	"github.com/ebitengine/purego"
)

// --- an independent oracle -------------------------------------------------
//
// The provider is validated against a SEPARATE, direct binding to glibc's
// *_l ctype functions — resolved here in the test, not through the package
// under test — so a bug in the package's own binding cannot mask itself. If
// the locale is not installed the oracle reports so and the locale-dependent
// tests skip.

type oracle struct {
	isalphaL, isalnumL, isblankL, iscntrlL, isdigitL, isgraphL  func(c int32, loc uintptr) int32
	islowerL, isprintL, ispunctL, isspaceL, isupperL, isxdigitL func(c int32, loc uintptr) int32
	tolowerL, toupperL                                          func(c int32, loc uintptr) int32
	newlocale                                                   func(mask int32, locale string, base uintptr) uintptr
	freelocale                                                  func(loc uintptr)
	errno                                                       func() *int32
	loc                                                         uintptr
}

func newOracle(t *testing.T, localeName string) *oracle {
	t.Helper()
	h, err := purego.Dlopen("libc.so.6", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil || h == 0 {
		t.Fatalf("oracle: glibc not loadable: %v", err)
	}
	t.Cleanup(func() { purego.Dlclose(h) })

	bind := func(fptr any, name string) {
		sym, err := purego.Dlsym(h, name)
		if err != nil || sym == 0 {
			t.Fatalf("oracle: symbol %s not found: %v", name, err)
		}
		purego.RegisterFunc(fptr, sym)
	}

	o := &oracle{}
	bind(&o.isalphaL, "isalpha_l")
	bind(&o.isalnumL, "isalnum_l")
	bind(&o.isblankL, "isblank_l")
	bind(&o.iscntrlL, "iscntrl_l")
	bind(&o.isdigitL, "isdigit_l")
	bind(&o.isgraphL, "isgraph_l")
	bind(&o.islowerL, "islower_l")
	bind(&o.isprintL, "isprint_l")
	bind(&o.ispunctL, "ispunct_l")
	bind(&o.isspaceL, "isspace_l")
	bind(&o.isupperL, "isupper_l")
	bind(&o.isxdigitL, "isxdigit_l")
	bind(&o.tolowerL, "tolower_l")
	bind(&o.toupperL, "toupper_l")
	bind(&o.newlocale, "newlocale")
	bind(&o.freelocale, "freelocale")
	bind(&o.errno, "__errno_location")

	runtime.LockOSThread()
	errno := o.errno()
	if errno == nil {
		runtime.UnlockOSThread()
		t.Fatal("oracle: __errno_location returned nil")
	}
	*errno = 0
	o.loc = o.newlocale(lcCtypeMask, localeName, 0)
	newlocaleErrno := *errno
	runtime.UnlockOSThread()
	if o.loc == 0 {
		if errors.Is(classifyNewlocaleErrno(newlocaleErrno), ErrMissingLocale) {
			t.Skipf("oracle: locale %q not installed (errno %d)", localeName, newlocaleErrno)
		}
		t.Fatalf("oracle: newlocale(%q) failed with errno %d", localeName, newlocaleErrno)
	}
	t.Cleanup(func() { o.freelocale(o.loc) })
	return o
}

// openProvider opens the real provider, skipping when glibc / the locale is
// unavailable so the suite stays green on a slim box while still failing on
// real defects when the locale IS present.
func openProvider(t *testing.T, localeName string) *Provider {
	t.Helper()
	_ = newOracle(t, localeName) // If oracle fails, it skips; if it succeeds, glibc/locale must be present.
	p, err := Open(localeName)
	if err != nil {
		t.Fatalf("Open(%q): %v (but oracle succeeded!)", localeName, err)
	}
	return p
}

func TestClassifyAgainstOracle(t *testing.T) {
	for _, localeName := range []string{"C", "de_DE.ISO-8859-1"} {
		t.Run(localeName, func(t *testing.T) {
			o := newOracle(t, localeName)
			p := openProvider(t, localeName)
			t.Cleanup(func() { _ = p.Close() })

			checks := []struct {
				name   string
				provFn func(byte) (bool, error)
				oracle func(c int32, loc uintptr) int32
			}{
				{"IsAlpha", p.IsAlpha, o.isalphaL},
				{"IsAlnum", p.IsAlnum, o.isalnumL},
				{"IsBlank", p.IsBlank, o.isblankL},
				{"IsCntrl", p.IsCntrl, o.iscntrlL},
				{"IsDigit", p.IsDigit, o.isdigitL},
				{"IsGraph", p.IsGraph, o.isgraphL},
				{"IsLower", p.IsLower, o.islowerL},
				{"IsPrint", p.IsPrint, o.isprintL},
				{"IsPunct", p.IsPunct, o.ispunctL},
				{"IsSpace", p.IsSpace, o.isspaceL},
				{"IsUpper", p.IsUpper, o.isupperL},
				{"IsXDigit", p.IsXDigit, o.isxdigitL},
			}
			for c := range 256 {
				b := byte(c)
				for _, chk := range checks {
					got, err := chk.provFn(b)
					if err != nil {
						t.Fatalf("%s(%d): %v", chk.name, c, err)
					}
					want := chk.oracle(int32(b), o.loc) != 0
					if got != want {
						t.Errorf("%s(%d) = %v, oracle = %v", chk.name, c, got, want)
					}
				}
			}

			all := make([]byte, 256)
			for i := range all {
				all[i] = byte(i)
			}
			gotLower, err := p.ToLower(all)
			if err != nil {
				t.Fatalf("ToLower: %v", err)
			}
			gotUpper, err := p.ToUpper(all)
			if err != nil {
				t.Fatalf("ToUpper: %v", err)
			}
			for i, b := range all {
				if want := byte(o.tolowerL(int32(b), o.loc)); gotLower[i] != want {
					t.Errorf("ToLower(%d) = %d, oracle = %d", i, gotLower[i], want)
				}
				if want := byte(o.toupperL(int32(b), o.loc)); gotUpper[i] != want {
					t.Errorf("ToUpper(%d) = %d, oracle = %d", i, gotUpper[i], want)
				}
			}
		})
	}
}

func TestOpenRejectsLocalesBeforeLibcGlibc(t *testing.T) {
	for _, name := range []string{
		"", "c", "posix", "de_DE", "de_DE.UTF-8", "UTF-8", "en_US.UTF-8",
		"de_DE.ISO-8859-15", "Latin-9", "de_DE.latin9", "de_DE.ISO8859-15",
		"garbage", "de_DE.ISO-8859-1x",
	} {
		if _, err := Open(name); !errors.Is(err, ErrUnsupportedLocale) {
			t.Errorf("Open(%q) = %v, want ErrUnsupportedLocale", name, err)
		}
	}
}

func TestOpenAcceptsAllLocales(t *testing.T) {
	for _, name := range []string{
		"C", "POSIX",
		"de_DE.ISO-8859-1", "de_DE.iso88591", "DE_DE.iso-8859-1", "de_de.ISO88591",
	} {
		canonical, _, _ := normalizeLocale(name)
		_ = newOracle(t, canonical) // Only genuine locale absence may skip this test.
		p, err := Open(name)
		if err != nil {
			t.Errorf("Open(%q): unexpected error %v", name, err)
			continue
		}
		_ = p.Close()
	}
}

func TestCloseIdempotentAndFencesCalls(t *testing.T) {
	p := openProvider(t, "C")

	if err := p.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Errorf("third Close: %v", err)
	}
	if _, err := p.IsAlpha('a'); !errors.Is(err, ErrClosed) {
		t.Errorf("IsAlpha after Close: got %v, want ErrClosed", err)
	}
	if _, err := p.ToLower([]byte("A")); !errors.Is(err, ErrClosed) {
		t.Errorf("ToLower after Close: got %v, want ErrClosed", err)
	}
}

// TestOverlappingCallsAndClose stresses the RWMutex fence: many classifier
// goroutines race a single Close. Every call must return one of the two
// well-defined outcomes and none may crash. Run under -race.
func TestOverlappingCallsAndClose(t *testing.T) {
	p := openProvider(t, "de_DE.ISO-8859-1")

	const workers = 32
	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			<-start
			for range 200 {
				if _, err := p.IsAlpha(0xe4); err != nil && !errors.Is(err, ErrClosed) {
					t.Errorf("IsAlpha during Close: unexpected %v", err)
					return
				}
				if _, err := p.ToUpper([]byte{0xe4}); err != nil && !errors.Is(err, ErrClosed) {
					t.Errorf("ToUpper during Close: unexpected %v", err)
					return
				}
			}
		}()
	}
	close(start)
	if err := p.Close(); err != nil {
		t.Errorf("concurrent Close: %v", err)
	}
	wg.Wait()

	if err := p.Close(); err != nil {
		t.Errorf("post-storm Close: %v", err)
	}
}

// TestCodesetVerified confirms the CODESET gate ran: a successfully opened
// de_DE.ISO-8859-1 provider classifies the Latin-1 letters ä/ö/ü/ß as
// alphabetic, which only holds under an ISO-8859-1 (not ASCII/UTF-8) ctype
// table.
func TestCodesetVerified(t *testing.T) {
	p := openProvider(t, "de_DE.ISO-8859-1")
	t.Cleanup(func() { _ = p.Close() })

	for _, b := range []byte{0xe4, 0xf6, 0xfc, 0xdf} { // ä ö ü ß
		alpha, err := p.IsAlpha(b)
		if err != nil {
			t.Fatalf("IsAlpha(%#x): %v", b, err)
		}
		if !alpha {
			t.Errorf("IsAlpha(%#x) = false, want true under ISO-8859-1", b)
		}
	}
}
