// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Package collate compares strings using compiled Go locale data (OpenEnv),
// or the bounded host ISO-8859-1 provider (Open). Compiled providers are pure
// Go on every platform and carry explicit order and multi-level weights.
// The host provider uses purego (Apache-2.0) on Linux; see THIRD_PARTY_LICENSES.md.
package collate

import (
	"errors"
	"strings"
)

// Sentinel errors. These are platform-independent so callers can switch on them
// identically on Linux and on the stub platforms.
var (
	// ErrUnsupportedPlatform is returned by Open when no glibc provider is
	// built: outside linux/amd64 and linux/arm64, or under bashy_scratch.
	ErrUnsupportedPlatform = errors.New("collate: glibc collation provider is only built for linux/amd64 and linux/arm64")

	// ErrUnsupportedLocale is returned when the requested locale name is not one
	// of the two accepted ISO-8859-1 aliases. Reported BEFORE any libc is loaded.
	ErrUnsupportedLocale = errors.New("collate: unsupported locale; only the ISO-8859-1 aliases \"de_DE.ISO-8859-1\" and \"de_DE.iso88591\" are accepted")

	// ErrGlibcUnavailable is returned when libc.so.6 cannot be loaded, or the
	// loaded C library is not glibc (honest detection via gnu_get_libc_version),
	// or a required symbol is missing.
	ErrGlibcUnavailable = errors.New("collate: glibc runtime not detected")

	// ErrMissingLocale is returned when glibc is present but the requested locale
	// data is not installed (newlocale failed).
	ErrMissingLocale = errors.New("collate: locale data is not installed")

	// ErrInitFailure is returned when glibc newlocale fails due to ENOMEM or other initialization failure.
	ErrInitFailure = errors.New("collate: initialization failure")

	// ErrCodeset is returned when the opened locale's CODESET is not ISO-8859-1,
	// which would mean the byte-oriented Compare contract does not hold.
	ErrCodeset = errors.New("collate: locale codeset is not ISO-8859-1")

	// ErrNulInput is returned by Compare when either operand contains a NUL byte,
	// which a C string cannot represent unambiguously.
	ErrNulInput = errors.New("collate: input contains a NUL byte")

	// ErrClosed is returned by Compare after the provider has been closed.
	ErrClosed = errors.New("collate: provider is closed")

	ErrInvalidCollatingElement = errors.New("collate: byte is not a valid single-byte collating element")
)

// normalizeLocale validates a requested locale name and, on success, returns the
// canonical glibc locale string to hand to newlocale.
//
// Only two aliases are accepted, matched case-insensitively (documented):
//
//	de_DE.ISO-8859-1
//	de_DE.iso88591
//
// Both normalize to "de_DE.ISO-8859-1", which glibc resolves for either
// charmap spelling. Everything else is rejected — this is the whole gate that
// keeps a non-Latin-1 locale from ever reaching libc.
func normalizeLocale(name string) (string, bool) {
	switch strings.ToLower(name) {
	case "de_de.iso-8859-1", "de_de.iso88591":
		return "de_DE.ISO-8859-1", true
	default:
		return "", false
	}
}
