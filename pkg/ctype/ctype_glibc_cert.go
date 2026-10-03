//go:build bashy_cert && linux && (amd64 || arm64) && cgo && !bashy_scratch

package ctype

/*
#define _GNU_SOURCE
#include <ctype.h>
#include <errno.h>
#include <langinfo.h>
#include <locale.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

static uintptr_t cert_newlocale(int mask, const char *name, uintptr_t base) {
	return (uintptr_t)newlocale(mask, name, (locale_t)base);
}
static void cert_freelocale(uintptr_t loc) { freelocale((locale_t)loc); }
static const char *cert_langinfo(int item, uintptr_t loc) {
	return nl_langinfo_l((nl_item)item, (locale_t)loc);
}
// Static glibc's nl_langinfo_l(CODESET) can expose the C locale before its
// thread-local locale state has been selected. Read the same locale through
// nl_langinfo while it is installed only on this thread, then restore it.
static char *cert_codeset(uintptr_t loc) {
	locale_t previous = uselocale((locale_t)loc);
	if (previous == (locale_t)0) return NULL;
	const char *value = nl_langinfo(CODESET);
	char *copy = value ? strdup(value) : NULL;
	uselocale(previous);
	return copy;
}
static int *cert_errno_location(void) { return __errno_location(); }
static int cert_ctype(int kind, int c, uintptr_t loc) {
	locale_t l = (locale_t)loc;
	switch (kind) {
	case 0: return isalpha_l(c, l);
	case 1: return isalnum_l(c, l);
	case 2: return isblank_l(c, l);
	case 3: return iscntrl_l(c, l);
	case 4: return isdigit_l(c, l);
	case 5: return isgraph_l(c, l);
	case 6: return islower_l(c, l);
	case 7: return isprint_l(c, l);
	case 8: return ispunct_l(c, l);
	case 9: return isspace_l(c, l);
	case 10: return isupper_l(c, l);
	case 11: return isxdigit_l(c, l);
	case 12: return tolower_l(c, l);
	case 13: return toupper_l(c, l);
	default: return 0;
	}
}
*/
import "C"

import "unsafe"

// The certification ELF links libc statically. Calling purego.Dlopen on
// libc.so.6 would load another libc with separate TLS; cold locale opens have
// intermittently crashed there. These bindings call the linked libc directly.
func init() { linkedLibc = certLinkedLibc }

func certLinkedLibc() (*libcBinding, error) {
	b := &libcBinding{
		newlocale: func(mask int32, name string, base uintptr) uintptr {
			cname := C.CString(name)
			defer C.free(unsafe.Pointer(cname))
			return uintptr(C.cert_newlocale(C.int(mask), cname, C.uintptr_t(base)))
		},
		freelocale: func(loc uintptr) { C.cert_freelocale(C.uintptr_t(loc)) },
		nlLanginfoL: func(item int32, loc uintptr) *byte {
			return (*byte)(unsafe.Pointer(C.cert_langinfo(C.int(item), C.uintptr_t(loc))))
		},
		codesetL: func(loc uintptr) string {
			value := C.cert_codeset(C.uintptr_t(loc))
			if value == nil {
				return ""
			}
			defer C.free(unsafe.Pointer(value))
			return C.GoString(value)
		},
		errnoLocation: func() *int32 {
			return (*int32)(unsafe.Pointer(C.cert_errno_location()))
		},
	}
	classify := func(kind C.int) func(int32, uintptr) int32 {
		return func(c int32, loc uintptr) int32 {
			return int32(C.cert_ctype(kind, C.int(c), C.uintptr_t(loc)))
		}
	}
	b.isalphaL = classify(0)
	b.isalnumL = classify(1)
	b.isblankL = classify(2)
	b.iscntrlL = classify(3)
	b.isdigitL = classify(4)
	b.isgraphL = classify(5)
	b.islowerL = classify(6)
	b.isprintL = classify(7)
	b.ispunctL = classify(8)
	b.isspaceL = classify(9)
	b.isupperL = classify(10)
	b.isxdigitL = classify(11)
	b.tolowerL = classify(12)
	b.toupperL = classify(13)
	return b, nil
}
