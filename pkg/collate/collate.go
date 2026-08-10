// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Package collate is a provider-only collation engine: it compares strings
// using glibc's locale-aware strcoll_l, reached over dlopen/dlsym rather than
// cgo. It exists to give agent tooling a faithful, deterministic ISO-8859-1
// collation order without linking libc and without shelling out.
//
// # Scope, on purpose
//
// This package is a LIBRARY. It wires to no applet, registers no verb, and is
// deliberately narrow: it accepts ONLY the two explicit ISO-8859-1 locale
// aliases (see [Open]). Everything else — the "C" locale, a bare "de_DE" with
// no codeset, "de_DE.UTF-8", ISO-8859-15/"Latin-9", and arbitrary names — is
// rejected up front, before any libc is loaded, with [ErrUnsupportedLocale].
// The narrow surface is the safety story: the only code path that ever reaches
// glibc has already been proven to name a single-byte Latin-1 locale.
//
// The real provider is built only on linux/amd64 and linux/arm64
// (collate_glibc.go). Every other platform gets a stub (collate_stub.go) whose
// [Open] returns [ErrUnsupportedPlatform] after the same locale validation, so
// callers get one consistent, honest contract everywhere.
//
// # Third-party provenance
//
// The dlopen/dlsym FFI is provided by github.com/ebitengine/purego (v0.10.0),
// upstream https://github.com/ebitengine/purego, Apache-2.0 licensed. purego is
// used directly — no cgo — via purego.Dlopen/Dlsym/RegisterFunc. See
// THIRD_PARTY_LICENSES.md.
package collate

import (
	"errors"
	"strings"
)

// Sentinel errors. These are platform-independent so callers can switch on them
// identically on Linux and on the stub platforms.
var (
	// ErrUnsupportedPlatform is returned by Open on any platform other than
	// linux/amd64 and linux/arm64, where no glibc provider is built.
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
