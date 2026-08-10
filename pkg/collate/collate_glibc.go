// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build linux && (amd64 || arm64)

package collate

import (
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/ebitengine/purego"
)

// glibc category masks for newlocale (from bits/locale.h): LC_CTYPE = 0,
// LC_COLLATE = 3, so the masks are 1<<category.
const (
	lcCtypeMask   = 1 << 0
	lcCollateMask = 1 << 3

	// codesetItem is nl_langinfo's CODESET item on glibc (langinfo.h). Its value
	// has been 14 for the life of glibc's ABI.
	codesetItem = 14

	// codesetLimit bounds the CODESET copy: glibc's codeset names are short
	// static strings, so a NUL must appear well within this window. If it does
	// not, we refuse rather than read unbounded foreign memory.
	codesetLimit = 64

	// glibc errno values used to classify a newlocale failure.
	errENOENT = 2
	errEINVAL = 22
)

// libcBinding is the fully-resolved set of glibc symbols. It is published
// exactly once, atomically, and ONLY after every required symbol has resolved —
// a partially-populated binding is never observable.
type libcBinding struct {
	handle uintptr

	newlocale     func(mask int32, locale string, base uintptr) uintptr
	freelocale    func(loc uintptr)
	strcollL      func(a, b string, loc uintptr) int32
	nlLanginfoL   func(item int32, loc uintptr) *byte
	errnoLocation func() *int32
}

var (
	libcOnce sync.Once
	libcPtr  atomic.Pointer[libcBinding]
	libcErr  error
)

// libc loads glibc once and returns the resolved binding. The load is atomic:
// on any failure the handle is closed and nil,err is returned; on success the
// completed binding is published via libcPtr and returned on every later call.
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

// loadLibc opens the specified libc library, confirms it is glibc, resolves every required
// symbol, and returns a complete binding. Every failure path Dlcloses the
// handle before returning so a rejected load leaks nothing.
func loadLibc(libName string) (*libcBinding, error) {
	handle, err := purego.Dlopen(libName, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil || handle == 0 {
		return nil, ErrGlibcUnavailable
	}

	fail := func() (*libcBinding, error) {
		_ = purego.Dlclose(handle)
		return nil, ErrGlibcUnavailable
	}

	// Honest glibc detection: gnu_get_libc_version is a glibc-only symbol. musl
	// and other C libraries do not export it, so its presence is proof of glibc
	// before we trust any of the locale ABI below.
	var gnuGetLibcVersion func() *byte
	if err := bindFunc(&gnuGetLibcVersion, handle, "gnu_get_libc_version"); err != nil {
		return fail()
	}

	b := &libcBinding{handle: handle}
	if err := bindFunc(&b.newlocale, handle, "newlocale"); err != nil {
		return fail()
	}
	if err := bindFunc(&b.freelocale, handle, "freelocale"); err != nil {
		return fail()
	}
	if err := bindFunc(&b.strcollL, handle, "strcoll_l"); err != nil {
		return fail()
	}
	if err := bindFunc(&b.nlLanginfoL, handle, "nl_langinfo_l"); err != nil {
		return fail()
	}
	if err := bindFunc(&b.errnoLocation, handle, "__errno_location"); err != nil {
		return fail()
	}
	return b, nil
}

// bindFunc resolves one symbol and installs it into fptr. purego.RegisterFunc
// panics on a bad function type, so a missing symbol is caught here as an error
// (Dlsym returns a nil address) and never reaches RegisterFunc.
func bindFunc(fptr any, handle uintptr, name string) error {
	sym, err := purego.Dlsym(handle, name)
	if err != nil || sym == 0 {
		return ErrGlibcUnavailable
	}
	purego.RegisterFunc(fptr, sym)
	return nil
}

// Provider compares strings in a glibc ISO-8859-1 locale via strcoll_l.
//
// It holds two independent locale_t handles — one carrying LC_COLLATE (used by
// strcoll_l) and one carrying LC_CTYPE (used to read and verify CODESET). A
// RWMutex fences concurrent Compare calls against Close; Close is idempotent.
type Provider struct {
	locale string
	lib    *libcBinding

	collate uintptr // locale_t with LC_COLLATE set
	ctype   uintptr // locale_t with LC_CTYPE set

	mu     sync.RWMutex
	closed bool
}

// Open returns a Provider for one of the accepted ISO-8859-1 locale aliases.
//
// The locale name is validated before any libc is touched; an unaccepted name
// returns ErrUnsupportedLocale. It then requires glibc (ErrGlibcUnavailable if
// absent), the locale data to be installed (ErrMissingLocale), and the locale's
// CODESET to be exactly ISO-8859-1 (ErrCodeset). The caller owns the returned
// Provider and must Close it.
func Open(name string) (*Provider, error) {
	canonical, ok := normalizeLocale(name)
	if !ok {
		return nil, ErrUnsupportedLocale
	}
	lib, err := libc()
	if err != nil {
		return nil, err
	}

	collate, ctype, err := lib.newLocales(canonical)
	if err != nil {
		return nil, err
	}
	if err := lib.verifyCodeset(ctype); err != nil {
		lib.freelocale(collate)
		lib.freelocale(ctype)
		return nil, err
	}
	return &Provider{locale: canonical, lib: lib, collate: collate, ctype: ctype}, nil
}

// newLocales builds the two independent locale_t handles. errno is cleared and
// read around each newlocale on a locked OS thread, because glibc's errno is
// thread-local and newlocale reports failure only through it plus a NULL return.
func (b *libcBinding) newLocales(name string) (collate, ctype uintptr, err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	errno := b.errnoLocation()

	*errno = 0
	collate = b.newlocale(lcCollateMask, name, 0)
	if collate == 0 {
		return 0, 0, classifyNewlocaleErrno(*errno)
	}

	*errno = 0
	ctype = b.newlocale(lcCtypeMask, name, 0)
	if ctype == 0 {
		e := *errno
		b.freelocale(collate)
		return 0, 0, classifyNewlocaleErrno(e)
	}
	return collate, ctype, nil
}

// classifyNewlocaleErrno maps a newlocale errno to a sentinel. ENOENT/EINVAL are
// the "locale not installed / unusable" cases; anything else is still treated as
// an initialization failure.
func classifyNewlocaleErrno(errno int32) error {
	switch errno {
	case errENOENT, errEINVAL:
		return ErrMissingLocale
	default:
		return ErrInitFailure
	}
}

// verifyCodeset reads CODESET from the CTYPE locale and requires it to be
// exactly ISO-8859-1 (either accepted spelling). The read is bounded and the
// value is copied into Go memory immediately, so nothing later dereferences the
// libc-owned pointer.
func (b *libcBinding) verifyCodeset(ctype uintptr) error {
	cs, ok := goStringBounded(b.nlLanginfoL(codesetItem, ctype), codesetLimit)
	if !ok {
		return ErrCodeset
	}
	if cs != "ISO-8859-1" && cs != "ISO8859-1" {
		return ErrCodeset
	}
	return nil
}

// goStringBounded copies a NUL-terminated C string into a Go string, reading at
// most limit bytes. It returns ok=false if p is nil or no NUL appears within the
// limit, so a malformed or unexpectedly-long value is refused rather than read
// past its bound. The slice is scanned only up to the terminator, so only the
// live bytes of the string are ever touched.
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

// hasNul reports whether s contains a NUL byte.
func hasNul(s string) bool { return strings.IndexByte(s, 0) >= 0 }

// Compare returns -1, 0, or +1 as a sorts before, equal to, or after b under
// the provider's LC_COLLATE order. The operands must be ISO-8859-1 (single-byte)
// encoded, matching the locale's codeset. A NUL byte in either operand returns
// ErrNulInput; a closed provider returns ErrClosed.
func (p *Provider) Compare(a, b string) (int, error) {
	if hasNul(a) || hasNul(b) {
		return 0, ErrNulInput
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return 0, ErrClosed
	}

	switch r := p.lib.strcollL(a, b, p.collate); {
	case r < 0:
		return -1, nil
	case r > 0:
		return 1, nil
	default:
		return 0, nil
	}
}

// Close frees the provider's locale_t handles. It is safe to call more than once
// and safe against concurrent Compare calls: the write lock waits for in-flight
// comparisons to finish, and the closed flag stops later ones deterministically.
func (p *Provider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	if p.collate != 0 {
		p.lib.freelocale(p.collate)
		p.collate = 0
	}
	if p.ctype != 0 {
		p.lib.freelocale(p.ctype)
		p.ctype = 0
	}
	return nil
}
