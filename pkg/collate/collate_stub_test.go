// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !(linux && (amd64 || arm64))

package collate

import (
	"errors"
	"testing"
)

func TestStubOpenRejectsUnacceptedLocaleFirst(t *testing.T) {
	// The locale gate is platform-independent, so the stub rejects the same set
	// with ErrUnsupportedLocale, before reporting the platform.
	for _, name := range []string{"C", "", "de_DE", "de_DE.UTF-8", "UTF-8", "Latin-9", "garbage"} {
		if _, err := Open(name); !errors.Is(err, ErrUnsupportedLocale) {
			t.Errorf("Open(%q) = %v, want ErrUnsupportedLocale", name, err)
		}
	}
}

func TestStubOpenAcceptedLocaleReportsPlatform(t *testing.T) {
	for _, name := range []string{"de_DE.ISO-8859-1", "de_DE.iso88591", "DE_DE.ISO88591"} {
		if _, err := Open(name); !errors.Is(err, ErrUnsupportedPlatform) {
			t.Errorf("Open(%q) = %v, want ErrUnsupportedPlatform", name, err)
		}
	}
}

func TestStubMethodsNeverPanic(t *testing.T) {
	var p Provider
	if _, err := p.Compare("a", "b"); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Errorf("Compare = %v, want ErrUnsupportedPlatform", err)
	}
	if err := p.Close(); err != nil {
		t.Errorf("Close = %v, want nil", err)
	}
}
