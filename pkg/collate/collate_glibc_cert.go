//go:build bashy_cert && linux && (amd64 || arm64) && cgo && !bashy_scratch

package collate

/*
#define _GNU_SOURCE
#include <errno.h>
#include <langinfo.h>
#include <locale.h>
#include <regex.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

static uintptr_t cert_newlocale(int mask, const char *name, uintptr_t base) {
	return (uintptr_t)newlocale(mask, name, (locale_t)base);
}
static void cert_freelocale(uintptr_t loc) { freelocale((locale_t)loc); }
static int cert_strcoll(const char *a, const char *b, uintptr_t loc) {
	return strcoll_l(a, b, (locale_t)loc);
}
static uintptr_t cert_uselocale(uintptr_t loc) {
	return (uintptr_t)uselocale((locale_t)loc);
}
static int cert_regcomp(void *preg, const char *pattern, int flags) {
	return regcomp((regex_t *)preg, pattern, flags);
}
static int cert_regexec(void *preg, const char *subject, size_t nmatch, void *matches, int flags) {
	return regexec((const regex_t *)preg, subject, nmatch, (regmatch_t *)matches, flags);
}
static void cert_regfree(void *preg) { regfree((regex_t *)preg); }
static const char *cert_langinfo(int item, uintptr_t loc) {
	return nl_langinfo_l((nl_item)item, (locale_t)loc);
}
static int *cert_errno_location(void) { return __errno_location(); }
*/
import "C"

import "unsafe"

// The certification ELF links libc statically. Resolve its locale and regex
// functions at link time instead of loading a second libc.so.6 at runtime.
func init() { linkedLibc = certLinkedLibc }

func certLinkedLibc() (*libcBinding, error) {
	return &libcBinding{
		newlocale: func(mask int32, name string, base uintptr) uintptr {
			cname := C.CString(name)
			defer C.free(unsafe.Pointer(cname))
			return uintptr(C.cert_newlocale(C.int(mask), cname, C.uintptr_t(base)))
		},
		freelocale: func(loc uintptr) { C.cert_freelocale(C.uintptr_t(loc)) },
		strcollL: func(a, b string, loc uintptr) int32 {
			ca, cb := C.CString(a), C.CString(b)
			defer C.free(unsafe.Pointer(ca))
			defer C.free(unsafe.Pointer(cb))
			return int32(C.cert_strcoll(ca, cb, C.uintptr_t(loc)))
		},
		uselocale: func(loc uintptr) uintptr {
			return uintptr(C.cert_uselocale(C.uintptr_t(loc)))
		},
		regcomp: func(preg unsafe.Pointer, pattern *byte, flags int32) int32 {
			return int32(C.cert_regcomp(preg, (*C.char)(unsafe.Pointer(pattern)), C.int(flags)))
		},
		regexec: func(preg unsafe.Pointer, subject *byte, nmatch uintptr, matches unsafe.Pointer, flags int32) int32 {
			return int32(C.cert_regexec(preg, (*C.char)(unsafe.Pointer(subject)), C.size_t(nmatch), matches, C.int(flags)))
		},
		regfree: func(preg unsafe.Pointer) { C.cert_regfree(preg) },
		nlLanginfoL: func(item int32, loc uintptr) *byte {
			return (*byte)(unsafe.Pointer(C.cert_langinfo(C.int(item), C.uintptr_t(loc))))
		},
		errnoLocation: func() *int32 {
			return (*int32)(unsafe.Pointer(C.cert_errno_location()))
		},
	}, nil
}
