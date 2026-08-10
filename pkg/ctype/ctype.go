// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Package ctype is a provider-only character-classification engine: it
// classifies and cases bytes using glibc's locale-aware *_l ctype functions,
// reached over dlopen/dlsym rather than cgo. It exists to give agent tooling
// faithful, deterministic POSIX character classes and case mapping without
// linking libc and without shelling out.
//
// # Scope, on purpose
//
// This package is a LIBRARY. It wires to no applet, registers no verb, and is
// deliberately narrow: it accepts ONLY the "C"/"POSIX" locale and the two
// explicit ISO-8859-1 locale aliases already reviewed for
// [github.com/qiangli/coreutils/pkg/collate] (see [Open]). Everything else —
// a bare "de_DE" with no codeset, "de_DE.UTF-8", ISO-8859-15/"Latin-9", and
// arbitrary names — is rejected up front, before any libc is loaded, with
// [ErrUnsupportedLocale].
//
// The real provider is built only on linux/amd64 and linux/arm64
// (ctype_glibc.go). Every other platform gets a stub (ctype_stub.go) whose
// [Open] returns [ErrUnsupportedPlatform] after the same locale validation,
// so callers get one consistent, honest contract everywhere.
//
// # Third-party provenance
//
// The dlopen/dlsym FFI is provided by github.com/ebitengine/purego (v0.10.0),
// upstream https://github.com/ebitengine/purego, Apache-2.0 licensed. purego
// is used directly — no cgo — via purego.Dlopen/Dlsym/RegisterFunc, as
// already declared in THIRD_PARTY_LICENSES.md for pkg/collate.
package ctype

import "errors"

// Sentinel errors. These are platform-independent so callers can switch on
// them identically on Linux and on the stub platforms.
var (
	// ErrUnsupportedPlatform is returned by Open on any platform other than
	// linux/amd64 and linux/arm64, where no glibc provider is built.
	ErrUnsupportedPlatform = errors.New("ctype: glibc ctype provider is only built for linux/amd64 and linux/arm64")

	// ErrUnsupportedLocale is returned when the requested locale name is not
	// "C", "POSIX", or one of the two accepted ISO-8859-1 aliases. Reported
	// BEFORE any libc is loaded.
	ErrUnsupportedLocale = errors.New("ctype: unsupported locale; only \"C\", \"POSIX\", and the ISO-8859-1 aliases \"de_DE.ISO-8859-1\" / \"de_DE.iso88591\" are accepted")

	// ErrGlibcUnavailable is returned when libc.so.6 cannot be loaded, or the
	// loaded C library is not glibc (honest detection via
	// gnu_get_libc_version), or a required symbol is missing.
	ErrGlibcUnavailable = errors.New("ctype: glibc runtime not detected")

	// ErrMissingLocale is returned when glibc is present but the requested
	// locale data is not installed (newlocale failed).
	ErrMissingLocale = errors.New("ctype: locale data is not installed")

	// ErrInitFailure is returned when glibc newlocale fails due to ENOMEM or
	// other initialization failure.
	ErrInitFailure = errors.New("ctype: initialization failure")

	// ErrCodeset is returned when the opened locale's CODESET does not match
	// the codeset this package expects for that locale (ANSI_X3.4-1968 for
	// "C"/"POSIX", ISO-8859-1 for the de_DE aliases).
	ErrCodeset = errors.New("ctype: locale codeset does not match the expected codeset for this locale")

	// ErrClosed is returned by the classification and casing methods after
	// the provider has been closed.
	ErrClosed = errors.New("ctype: provider is closed")
)

// codesetC is glibc's CODESET value for the "C"/"POSIX" locale.
const codesetC = "ANSI_X3.4-1968"

// codesetISO88591 and codesetISO88591Alt are the two CODESET spellings glibc
// uses for ISO-8859-1, matching pkg/collate's accepted set exactly.
const (
	codesetISO88591    = "ISO-8859-1"
	codesetISO88591Alt = "ISO8859-1"
)

// normalizeLocale validates a requested locale name and, on success, returns
// the canonical glibc locale string to hand to newlocale plus the codeset(s)
// that locale is expected to report.
//
// Three cases are accepted:
//
//	C                    (exact, case-sensitive)
//	POSIX                (exact, case-sensitive; glibc's alias for "C")
//	de_DE.ISO-8859-1      (case-insensitive, reviewed pkg/collate alias)
//	de_DE.iso88591        (case-insensitive, reviewed pkg/collate alias)
//
// Everything else — including a locale name containing a NUL byte, which can
// never match one of the fixed cases above — is rejected. This is the whole
// gate that keeps a non-reviewed locale from ever reaching glibc.
func normalizeLocale(name string) (canonical string, codesets []string, ok bool) {
	switch name {
	case "C", "POSIX":
		return "C", []string{codesetC}, true
	}
	switch lower(name) {
	case "de_de.iso-8859-1", "de_de.iso88591":
		return "de_DE.ISO-8859-1", []string{codesetISO88591, codesetISO88591Alt}, true
	default:
		return "", nil, false
	}
}

// lower ASCII-lowercases s. A hand-rolled loop avoids pulling strings.ToLower's
// Unicode case-folding machinery for what is always a short ASCII locale name.
func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}
