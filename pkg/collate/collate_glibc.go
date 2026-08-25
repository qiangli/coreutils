// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build linux && (amd64 || arm64)

package collate

import (
	"fmt"
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
	codesetItem   = 14
	collseqMBItem = (3 << 16) | 16
	regExtended   = 1
	regNoSub      = 1 << 3
	regNoMatch    = 1
	regECollate   = 3

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
	uselocale     func(loc uintptr) uintptr
	regcomp       func(preg unsafe.Pointer, pattern *byte, flags int32) int32
	regexec       func(preg unsafe.Pointer, subject *byte, nmatch uintptr, matches unsafe.Pointer, flags int32) int32
	regfree       func(preg unsafe.Pointer)
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

	// Honest glibc detection: gnu_get_libc_version is a glibc-only symbol. musl
	// and other C libraries do not export it, so its presence is proof of glibc
	// before we trust any of the locale ABI below.
	var gnuGetLibcVersion func() *byte
	if err := bindFuncWith(&gnuGetLibcVersion, handle, "gnu_get_libc_version", ops); err != nil {
		return fail()
	}

	b := &libcBinding{handle: handle}
	if err := bindFuncWith(&b.newlocale, handle, "newlocale", ops); err != nil {
		return fail()
	}
	if err := bindFuncWith(&b.freelocale, handle, "freelocale", ops); err != nil {
		return fail()
	}
	if err := bindFuncWith(&b.strcollL, handle, "strcoll_l", ops); err != nil {
		return fail()
	}
	if err := bindFuncWith(&b.uselocale, handle, "uselocale", ops); err != nil {
		return fail()
	}
	if err := bindFuncWith(&b.regcomp, handle, "regcomp", ops); err != nil {
		return fail()
	}
	if err := bindFuncWith(&b.regexec, handle, "regexec", ops); err != nil {
		return fail()
	}
	if err := bindFuncWith(&b.regfree, handle, "regfree", ops); err != nil {
		return fail()
	}
	if err := bindFuncWith(&b.nlLanginfoL, handle, "nl_langinfo_l", ops); err != nil {
		return fail()
	}
	if err := bindFuncWith(&b.errnoLocation, handle, "__errno_location", ops); err != nil {
		return fail()
	}
	return b, nil
}

// bindFuncWith resolves one symbol and installs it into fptr. A missing symbol
// is caught before RegisterFunc, and the caller closes the owning handle.
func bindFuncWith(fptr any, handle uintptr, name string, ops libcLoaderOps) error {
	sym, err := ops.sym(handle, name)
	if err != nil || sym == 0 {
		return ErrGlibcUnavailable
	}
	ops.register(fptr, sym)
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

	collate     uintptr // locale_t with LC_COLLATE set
	ctype       uintptr // locale_t with LC_CTYPE set
	regexLocale uintptr // locale_t with LC_CTYPE and LC_COLLATE set
	equivalent  [256][256]bool
	equivValid  [256]bool
	collseq     [256]byte
	collating   [256]bool

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

	collate, ctype, regexLocale, err := lib.newLocales(canonical)
	if err != nil {
		return nil, err
	}
	if err := lib.verifyCodeset(ctype); err != nil {
		lib.freelocale(collate)
		lib.freelocale(ctype)
		lib.freelocale(regexLocale)
		return nil, err
	}
	equivalent, equivValid, collseq, collating, err := lib.snapshotRegexCollation(regexLocale, regexLocale)
	if err != nil {
		lib.freelocale(collate)
		lib.freelocale(ctype)
		lib.freelocale(regexLocale)
		return nil, err
	}
	return &Provider{locale: canonical, lib: lib, collate: collate, ctype: ctype, regexLocale: regexLocale, equivalent: equivalent, equivValid: equivValid, collseq: collseq, collating: collating}, nil
}

// newLocales builds the two independent locale_t handles. errno is cleared and
// read around each newlocale on a locked OS thread, because glibc's errno is
// thread-local and newlocale reports failure only through it plus a NULL return.
func (b *libcBinding) newLocales(name string) (collate, ctype, regexLocale uintptr, err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	errno := b.errnoLocation()

	*errno = 0
	collate = b.newlocale(lcCollateMask, name, 0)
	if collate == 0 {
		return 0, 0, 0, classifyNewlocaleErrno(*errno)
	}

	*errno = 0
	ctype = b.newlocale(lcCtypeMask, name, 0)
	if ctype == 0 {
		e := *errno
		b.freelocale(collate)
		return 0, 0, 0, classifyNewlocaleErrno(e)
	}
	*errno = 0
	regexLocale = b.newlocale(lcCollateMask|lcCtypeMask, name, 0)
	if regexLocale == 0 {
		e := *errno
		b.freelocale(collate)
		b.freelocale(ctype)
		return 0, 0, 0, classifyNewlocaleErrno(e)
	}
	return collate, ctype, regexLocale, nil
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

// snapshotRegexCollation asks glibc's POSIX regex implementation for every
// single-byte equivalence class in the requested locale. This avoids embedding
// an incomplete, locale-specific table: glibc's installed locale provider is
// the sole source of truth. The LC_COLLATE sequence is copied from the same
// locale object for bracket-range expansion.
func (b *libcBinding) snapshotRegexCollation(regexLocale, collate uintptr) ([256][256]bool, [256]bool, [256]byte, [256]bool, error) {
	var equivalent [256][256]bool
	var equivValid [256]bool
	var order [256]byte
	var valid [256]bool

	seq := b.nlLanginfoL(collseqMBItem, collate)
	if seq == nil {
		return equivalent, equivValid, order, valid, ErrInitFailure
	}
	copy(order[:], unsafe.Slice(seq, len(order)))

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	previous := b.uselocale(regexLocale)
	if previous == 0 {
		return equivalent, equivValid, order, valid, ErrInitFailure
	}
	defer b.uselocale(previous)

	for source := 1; source < 256; source++ {
		equivalencePattern := [8]byte{'[', '[', '=', byte(source), '=', ']', ']', 0}
		collatingPattern := [8]byte{'[', '[', '.', byte(source), '.', ']', ']', 0}
		// regex_t is opaque. This aligned buffer is deliberately larger than
		// glibc's public regex_t on every supported ABI; regfree sees the same
		// address that regcomp initialized.
		var compiled, collatingCompiled [128]uintptr
		equivalenceCode := b.regcomp(unsafe.Pointer(&compiled[0]), &equivalencePattern[0], regExtended|regNoSub)
		collatingCode := b.regcomp(unsafe.Pointer(&collatingCompiled[0]), &collatingPattern[0], regExtended|regNoSub)
		if collatingCode == 0 {
			valid[source] = true
			b.regfree(unsafe.Pointer(&collatingCompiled[0]))
		} else if collatingCode != regECollate {
			if equivalenceCode == 0 {
				b.regfree(unsafe.Pointer(&compiled[0]))
			}
			return equivalent, equivValid, order, valid, fmt.Errorf("%w: byte %#02x collating regcomp=%d", ErrInitFailure, source, collatingCode)
		}
		if equivalenceCode == regECollate {
			continue
		}
		if equivalenceCode != 0 {
			return equivalent, equivValid, order, valid, fmt.Errorf("%w: byte %#02x equivalence regcomp=%d", ErrInitFailure, source, equivalenceCode)
		}
		if !valid[source] {
			b.regfree(unsafe.Pointer(&compiled[0]))
			return equivalent, equivValid, order, valid, fmt.Errorf("%w: byte %#02x has equivalence but no collating element", ErrInitFailure, source)
		}
		equivValid[source] = true
		for candidate := 1; candidate < 256; candidate++ {
			subject := [2]byte{byte(candidate), 0}
			switch match := b.regexec(unsafe.Pointer(&compiled[0]), &subject[0], 0, nil, 0); match {
			case 0:
				equivalent[source][candidate] = true
			case regNoMatch:
			default:
				b.regfree(unsafe.Pointer(&compiled[0]))
				return equivalent, equivValid, order, valid, fmt.Errorf("%w: equivalence byte %#02x candidate %#02x regexec=%d", ErrInitFailure, source, candidate, match)
			}
		}
		b.regfree(unsafe.Pointer(&compiled[0]))
		if !equivalent[source][source] {
			return equivalent, equivValid, order, valid, fmt.Errorf("%w: non-reflexive equivalence byte %#02x", ErrInitFailure, source)
		}
	}
	return equivalent, equivValid, order, valid, nil
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

// Equivalents returns the complete single-byte equivalence class for c.
func (p *Provider) Equivalents(c byte) ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return nil, ErrClosed
	}
	if !p.equivValid[c] {
		return nil, nil
	}
	result := make([]byte, 0, 8)
	for candidate, member := range p.equivalent[c] {
		if member {
			result = append(result, byte(candidate))
		}
	}
	return result, nil
}

func (p *Provider) EquivalenceClasses() ([]bool, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return nil, ErrClosed
	}
	return append([]bool(nil), p.equivValid[:]...), nil
}

// CollationWeights returns glibc's single-byte LC_COLLATE sequence.
func (p *Provider) CollationWeights() ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return nil, ErrClosed
	}
	return append([]byte(nil), p.collseq[:]...), nil
}

// CollatingElements reports which individual bytes glibc accepts as named
// collating elements. NUL is always false because POSIX regex strings cannot
// represent it.
func (p *Provider) CollatingElements() ([]bool, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return nil, ErrClosed
	}
	return append([]bool(nil), p.collating[:]...), nil
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
	if p.regexLocale != 0 {
		p.lib.freelocale(p.regexLocale)
		p.regexLocale = 0
	}
	return nil
}
