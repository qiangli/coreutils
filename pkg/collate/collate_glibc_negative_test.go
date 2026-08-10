// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build linux && (amd64 || arm64)

package collate

import (
	"errors"
	"testing"
)

func TestLoadLibcNegative(t *testing.T) {
	// missing libc
	if _, err := loadLibc("lib_does_not_exist.so"); !errors.Is(err, ErrGlibcUnavailable) {
		t.Errorf("loadLibc(missing_libc) = %v, want ErrGlibcUnavailable", err)
	}
	// missing symbol (libm is a valid library but lacks gnu_get_libc_version)
	// If libm.so.6 is missing, it will still return ErrGlibcUnavailable from dlopen,
	// which is acceptable for the test, but typically it resolves and fails on dlsym.
	if _, err := loadLibc("libm.so.6"); !errors.Is(err, ErrGlibcUnavailable) {
		t.Errorf("loadLibc(missing_symbol) = %v, want ErrGlibcUnavailable", err)
	}
}

func TestVerifyCodesetNegative(t *testing.T) {
	overlong := make([]byte, 65)
	for i := range overlong {
		overlong[i] = 'A'
	}

	tests := []struct {
		name string
		data []byte
	}{
		{"wrong_codeset", []byte("UTF-8\x00")},
		{"unterminated", []byte("ISO-8859-1")},
		{"overlong", overlong},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := &libcBinding{
				nlLanginfoL: func(item int32, loc uintptr) *byte {
					return &tc.data[0]
				},
			}
			if err := b.verifyCodeset(1); !errors.Is(err, ErrCodeset) {
				t.Errorf("verifyCodeset(%s) = %v, want ErrCodeset", tc.name, err)
			}
		})
	}
}

func TestClassifyNewlocaleErrnoExact(t *testing.T) {
	if err := classifyNewlocaleErrno(errENOENT); !errors.Is(err, ErrMissingLocale) {
		t.Errorf("ENOENT = %v, want ErrMissingLocale", err)
	}
	if err := classifyNewlocaleErrno(errEINVAL); !errors.Is(err, ErrMissingLocale) {
		t.Errorf("EINVAL = %v, want ErrMissingLocale", err)
	}
	if err := classifyNewlocaleErrno(12); !errors.Is(err, ErrInitFailure) { // ENOMEM
		t.Errorf("ENOMEM = %v, want ErrInitFailure", err)
	}
}
