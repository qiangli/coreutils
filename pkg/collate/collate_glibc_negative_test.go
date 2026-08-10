// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build linux && (amd64 || arm64)

package collate

import (
	"errors"
	"fmt"
	"testing"
)

func TestLoadLibcNegative(t *testing.T) {
	const fakeHandle = uintptr(47)

	t.Run("missing_libc", func(t *testing.T) {
		closed := 0
		ops := libcLoaderOps{
			open: func(string, int) (uintptr, error) { return 0, errors.New("missing") },
			sym:  func(uintptr, string) (uintptr, error) { t.Fatal("Dlsym called after Dlopen failure"); return 0, nil },
			close: func(uintptr) error {
				closed++
				return nil
			},
			register: func(any, uintptr) { t.Fatal("RegisterFunc called after Dlopen failure") },
		}
		if _, err := loadLibcWith("missing", ops); !errors.Is(err, ErrGlibcUnavailable) {
			t.Errorf("loadLibcWith(missing) = %v, want ErrGlibcUnavailable", err)
		}
		if closed != 0 {
			t.Errorf("close calls = %d, want 0 when no handle opened", closed)
		}
	})

	t.Run("missing_symbol_closes_handle", func(t *testing.T) {
		closed := 0
		ops := libcLoaderOps{
			open: func(string, int) (uintptr, error) { return fakeHandle, nil },
			sym: func(_ uintptr, name string) (uintptr, error) {
				if name == "strcoll_l" {
					return 0, fmt.Errorf("missing %s", name)
				}
				return 1, nil
			},
			close: func(handle uintptr) error {
				if handle != fakeHandle {
					t.Errorf("closed handle = %d, want %d", handle, fakeHandle)
				}
				closed++
				return nil
			},
			register: func(any, uintptr) {},
		}
		if _, err := loadLibcWith("fake", ops); !errors.Is(err, ErrGlibcUnavailable) {
			t.Errorf("loadLibcWith(missing symbol) = %v, want ErrGlibcUnavailable", err)
		}
		if closed != 1 {
			t.Errorf("close calls = %d, want exactly 1", closed)
		}
	})
}

func TestVerifyCodesetNegative(t *testing.T) {
	unterminated := make([]byte, codesetLimit)
	for i := range unterminated {
		unterminated[i] = 'A'
	}
	overlong := make([]byte, codesetLimit+1)
	for i := range overlong {
		overlong[i] = 'A'
	}

	tests := []struct {
		name string
		data []byte
	}{
		{"wrong_codeset", []byte("UTF-8\x00")},
		{"unterminated", unterminated},
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
