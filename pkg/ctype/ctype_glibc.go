// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build linux && (amd64 || arm64)

package ctype

import (
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/ebitengine/purego"
)

// lcCtypeMask is glibc's LC_CTYPE category mask for newlocale (bits/locale.h):
// LC_CTYPE = 0, so the mask is 1<<0. This provider owns exactly one
// newlocale call using this mask — no other category is ever requested.
const lcCtypeMask = 1 << 0

const (
	// codesetItem is nl_langinfo's CODESET item on glibc (langinfo.h). Its
	// value has been 14 for the life of glibc's ABI.
	codesetItem = 14

	// codesetLimit bounds the CODESET copy: glibc's codeset names are short
	// static strings, so a NUL must appear well within this window. If it
	// does not, we refuse rather than read unbounded foreign memory.
	codesetLimit = 64

	// glibc errno values used to classify a newlocale failure.
	errENOENT = 2
	errEINVAL = 22
)

// libcBinding is the fully-resolved set of glibc symbols. It is published
// exactly once, atomically, and ONLY after every required symbol has
// resolved — a partially-populated binding is never observable.
type libcBinding struct {
	handle uintptr

	newlocale     func(mask int32, locale string, base uintptr) uintptr
	freelocale    func(loc uintptr)
	nlLanginfoL   func(item int32, loc uintptr) *byte
	errnoLocation func() *int32

	isalphaL  func(c int32, loc uintptr) int32
	isalnumL  func(c int32, loc uintptr) int32
	isblankL  func(c int32, loc uintptr) int32
	iscntrlL  func(c int32, loc uintptr) int32
	isdigitL  func(c int32, loc uintptr) int32
	isgraphL  func(c int32, loc uintptr) int32
	islowerL  func(c int32, loc uintptr) int32
	isprintL  func(c int32, loc uintptr) int32
	ispunctL  func(c int32, loc uintptr) int32
	isspaceL  func(c int32, loc uintptr) int32
	isupperL  func(c int32, loc uintptr) int32
	isxdigitL func(c int32, loc uintptr) int32
	tolowerL  func(c int32, loc uintptr) int32
	toupperL  func(c int32, loc uintptr) int32
}

var (
	libcOnce sync.Once
	libcPtr  atomic.Pointer[libcBinding]
	libcErr  error
)

// libc loads glibc once and returns the resolved binding. The load is
// atomic: on any failure the handle is closed and nil,err is returned; on
// success the completed binding is published via libcPtr and returned on
// every later call.
func libc() (*libcBinding, error) {
	libcOnce.Do(func() {
		b, err := loadLibc("libc.so.6")
		if err != nil {
			libcErr = err
			return
		}
		libcPtr.Store(b)
	})
	if b := libcPtr.Load(); b != nil {
		return b, nil
	}
	return nil, libcErr
}

type libcLoaderOps struct {
	open     func(path string, mode int) (uintptr, error)
	sym      func(handle uintptr, name string) (uintptr, error)
	close    func(handle uintptr) error
	register func(fptr any, cfn uintptr)
}

var puregoLibcLoaderOps = libcLoaderOps{
	open:     purego.Dlopen,
	sym:      purego.Dlsym,
	close:    purego.Dlclose,
	register: purego.RegisterFunc,
}

// loadLibc opens the specified libc library, confirms it is glibc, resolves
// every required symbol, and returns a complete binding. Every failure path
// Dlcloses the handle before returning so a rejected load leaks nothing.
func loadLibc(libName string) (*libcBinding, error) {
	return loadLibcWith(libName, puregoLibcLoaderOps)
}

// loadLibcWith is the injectable loader seam used by failure-path tests. The
// production path always supplies puregoLibcLoaderOps.
func loadLibcWith(libName string, ops libcLoaderOps) (*libcBinding, error) {
	handle, err := ops.open(libName, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil || handle == 0 {
		return nil, ErrGlibcUnavailable
	}

	fail := func() (*libcBinding, error) {
		_ = ops.close(handle)
		return nil, ErrGlibcUnavailable
	}

	// Honest glibc detection: gnu_get_libc_version is a glibc-only symbol.
	// musl and other C libraries do not export it, so its presence is proof
	// of glibc before we trust any of the locale ABI below.
	var gnuGetLibcVersion func() *byte
	if err := bindFuncWith(&gnuGetLibcVersion, handle, "gnu_get_libc_version", ops); err != nil {
		return fail()
	}

	b := &libcBinding{handle: handle}
	binds := []struct {
		fptr any
		name string
	}{
		{&b.newlocale, "newlocale"},
		{&b.freelocale, "freelocale"},
		{&b.nlLanginfoL, "nl_langinfo_l"},
		{&b.errnoLocation, "__errno_location"},
		{&b.isalphaL, "isalpha_l"},
		{&b.isalnumL, "isalnum_l"},
		{&b.isblankL, "isblank_l"},
		{&b.iscntrlL, "iscntrl_l"},
		{&b.isdigitL, "isdigit_l"},
		{&b.isgraphL, "isgraph_l"},
		{&b.islowerL, "islower_l"},
		{&b.isprintL, "isprint_l"},
		{&b.ispunctL, "ispunct_l"},
		{&b.isspaceL, "isspace_l"},
		{&b.isupperL, "isupper_l"},
		{&b.isxdigitL, "isxdigit_l"},
		{&b.tolowerL, "tolower_l"},
		{&b.toupperL, "toupper_l"},
	}
	for _, bd := range binds {
		if err := bindFuncWith(bd.fptr, handle, bd.name, ops); err != nil {
			return fail()
		}
	}
	return b, nil
}

// bindFuncWith resolves one symbol and installs it into fptr. A missing
// symbol is caught before RegisterFunc, and the caller closes the owning
// handle.
func bindFuncWith(fptr any, handle uintptr, name string, ops libcLoaderOps) error {
	sym, err := ops.sym(handle, name)
	if err != nil || sym == 0 {
		return ErrGlibcUnavailable
	}
	ops.register(fptr, sym)
	return nil
}

// Provider classifies and cases bytes in a glibc locale via the *_l ctype
// functions. It holds one locale_t handle carrying LC_CTYPE, obtained from a
// single newlocale(LC_CTYPE_MASK, ...) call owned by this Provider. A
// RWMutex fences concurrent classification/casing calls against Close; Close
// is idempotent.
type Provider struct {
	locale string
	lib    *libcBinding

	ctype uintptr // locale_t with LC_CTYPE set

	mu     sync.RWMutex
	closed bool
}

// Open returns a Provider for "C", "POSIX", or one of the accepted
// ISO-8859-1 locale aliases.
//
// The locale name is validated before any libc is touched; an unaccepted
// name returns ErrUnsupportedLocale. It then requires glibc
// (ErrGlibcUnavailable if absent), the locale data to be installed
// (ErrMissingLocale), and the locale's CODESET to exactly match what this
// package expects for that locale (ErrCodeset). The caller owns the
// returned Provider and must Close it.
func Open(name string) (*Provider, error) {
	canonical, codesets, ok := normalizeLocale(name)
	if !ok {
		return nil, ErrUnsupportedLocale
	}
	lib, err := libc()
	if err != nil {
		return nil, err
	}

	loc, err := lib.newCtypeLocale(canonical)
	if err != nil {
		return nil, err
	}
	if err := lib.verifyCodeset(loc, codesets); err != nil {
		lib.freelocale(loc)
		return nil, err
	}
	return &Provider{locale: canonical, lib: lib, ctype: loc}, nil
}

// newCtypeLocale builds the LC_CTYPE locale_t handle. errno is cleared and
// read around newlocale on a locked OS thread, because glibc's errno is
// thread-local and newlocale reports failure only through it plus a NULL
// return.
func (b *libcBinding) newCtypeLocale(name string) (uintptr, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	errno := b.errnoLocation()
	*errno = 0
	loc := b.newlocale(lcCtypeMask, name, 0)
	if loc == 0 {
		return 0, classifyNewlocaleErrno(*errno)
	}
	return loc, nil
}

// classifyNewlocaleErrno maps a newlocale errno to a sentinel. ENOENT/EINVAL
// are the "locale not installed / unusable" cases; anything else is still
// treated as an initialization failure.
func classifyNewlocaleErrno(errno int32) error {
	switch errno {
	case errENOENT, errEINVAL:
		return ErrMissingLocale
	default:
		return ErrInitFailure
	}
}

// verifyCodeset reads CODESET from the locale and requires it to be exactly
// one of want. The read is bounded and the value is copied into Go memory
// immediately, so nothing later dereferences the libc-owned pointer.
func (b *libcBinding) verifyCodeset(loc uintptr, want []string) error {
	cs, ok := goStringBounded(b.nlLanginfoL(codesetItem, loc), codesetLimit)
	if !ok || !slices.Contains(want, cs) {
		return ErrCodeset
	}
	return nil
}

// goStringBounded copies a NUL-terminated C string into a Go string, reading
// at most limit bytes. It returns ok=false if p is nil or no NUL appears
// within the limit, so a malformed or unexpectedly-long value is refused
// rather than read past its bound. The slice is scanned only up to the
// terminator, so only the live bytes of the string are ever touched.
func goStringBounded(p *byte, limit int) (string, bool) {
	if p == nil {
		return "", false
	}
	buf := unsafe.Slice(p, limit)
	for i := range limit {
		if buf[i] == 0 {
			return string(buf[:i]), true
		}
	}
	return "", false
}

// classify runs a single *_l predicate against one byte under the
// provider's locale, fenced against Close. err is ErrClosed once the
// provider has been closed; otherwise it is always nil.
func (p *Provider) classify(fn func(c int32, loc uintptr) int32, c byte) (bool, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return false, ErrClosed
	}
	return fn(int32(c), p.ctype) != 0, nil
}

// IsAlpha reports whether c is alphabetic under the provider's locale.
func (p *Provider) IsAlpha(c byte) (bool, error) { return p.classify(p.lib.isalphaL, c) }

// IsAlnum reports whether c is alphanumeric under the provider's locale.
func (p *Provider) IsAlnum(c byte) (bool, error) { return p.classify(p.lib.isalnumL, c) }

// IsBlank reports whether c is a blank (space or tab) character.
func (p *Provider) IsBlank(c byte) (bool, error) { return p.classify(p.lib.isblankL, c) }

// IsCntrl reports whether c is a control character.
func (p *Provider) IsCntrl(c byte) (bool, error) { return p.classify(p.lib.iscntrlL, c) }

// IsDigit reports whether c is a decimal digit.
func (p *Provider) IsDigit(c byte) (bool, error) { return p.classify(p.lib.isdigitL, c) }

// IsGraph reports whether c has a visible glyph.
func (p *Provider) IsGraph(c byte) (bool, error) { return p.classify(p.lib.isgraphL, c) }

// IsLower reports whether c is a lowercase letter under the provider's
// locale.
func (p *Provider) IsLower(c byte) (bool, error) { return p.classify(p.lib.islowerL, c) }

// IsPrint reports whether c is printable (including space).
func (p *Provider) IsPrint(c byte) (bool, error) { return p.classify(p.lib.isprintL, c) }

// IsPunct reports whether c is punctuation.
func (p *Provider) IsPunct(c byte) (bool, error) { return p.classify(p.lib.ispunctL, c) }

// IsSpace reports whether c is whitespace under the provider's locale.
func (p *Provider) IsSpace(c byte) (bool, error) { return p.classify(p.lib.isspaceL, c) }

// IsUpper reports whether c is an uppercase letter under the provider's
// locale.
func (p *Provider) IsUpper(c byte) (bool, error) { return p.classify(p.lib.isupperL, c) }

// IsXDigit reports whether c is a hexadecimal digit.
func (p *Provider) IsXDigit(c byte) (bool, error) { return p.classify(p.lib.isxdigitL, c) }

// ToLower returns a copy of b with every byte case-mapped through the
// provider's locale via tolower_l. b is never mutated. Once the provider is
// closed, ToLower returns (nil, ErrClosed) and leaves b untouched.
func (p *Provider) ToLower(b []byte) ([]byte, error) { return p.caseMap(p.lib.tolowerL, b) }

// ToUpper returns a copy of b with every byte case-mapped through the
// provider's locale via toupper_l. b is never mutated. Once the provider is
// closed, ToUpper returns (nil, ErrClosed) and leaves b untouched.
func (p *Provider) ToUpper(b []byte) ([]byte, error) { return p.caseMap(p.lib.toupperL, b) }

// Equivalents returns the single-byte members of c's primary collating
// equivalence class. The only non-C locale admitted by Open is German
// ISO-8859-1, so its reviewed Latin-1 base-letter groups are finite and
// deterministic. Multi-character collating elements remain out of scope.
func (p *Provider) Equivalents(c byte) ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return nil, ErrClosed
	}
	if p.locale == "C" {
		return []byte{c}, nil
	}
	for _, group := range [][]byte{
		{'A', 'a', 0xc0, 0xc1, 0xc2, 0xc3, 0xc4, 0xc5, 0xe0, 0xe1, 0xe2, 0xe3, 0xe4, 0xe5},
		{'C', 'c', 0xc7, 0xe7},
		{'E', 'e', 0xc8, 0xc9, 0xca, 0xcb, 0xe8, 0xe9, 0xea, 0xeb},
		{'I', 'i', 0xcc, 0xcd, 0xce, 0xcf, 0xec, 0xed, 0xee, 0xef},
		{'N', 'n', 0xd1, 0xf1},
		{'O', 'o', 0xd2, 0xd3, 0xd4, 0xd5, 0xd6, 0xd8, 0xf2, 0xf3, 0xf4, 0xf5, 0xf6, 0xf8},
		{'U', 'u', 0xd9, 0xda, 0xdb, 0xdc, 0xf9, 0xfa, 0xfb, 0xfc},
		{'Y', 'y', 0xdd, 0xfd, 0xff},
	} {
		for _, member := range group {
			if member == c {
				return append([]byte(nil), group...), nil
			}
		}
	}
	return []byte{c}, nil
}

// caseMap applies a single *_l case-mapping function byte-by-byte, fenced
// against Close.
func (p *Provider) caseMap(fn func(c int32, loc uintptr) int32, b []byte) ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return nil, ErrClosed
	}
	out := make([]byte, len(b))
	for i, c := range b {
		out[i] = byte(fn(int32(c), p.ctype))
	}
	return out, nil
}

// Close frees the provider's locale_t handle. It is safe to call more than
// once and safe against concurrent classification/casing calls: the write
// lock waits for in-flight calls to finish, and the closed flag stops later
// ones deterministically.
func (p *Provider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	if p.ctype != 0 {
		p.lib.freelocale(p.ctype)
		p.ctype = 0
	}
	return nil
}
