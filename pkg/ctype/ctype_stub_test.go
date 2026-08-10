// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !(linux && (amd64 || arm64))

package ctype

import (
	"errors"
	"testing"
)

func TestStubOpenAcceptedLocaleReportsPlatform(t *testing.T) {
	for _, name := range []string{"C", "POSIX", "de_DE.ISO-8859-1", "de_DE.iso88591", "DE_DE.ISO88591"} {
		if _, err := Open(name); !errors.Is(err, ErrUnsupportedPlatform) {
			t.Errorf("Open(%q) = %v, want ErrUnsupportedPlatform", name, err)
		}
	}
}

func TestStubMethodsNeverPanic(t *testing.T) {
	var p Provider

	classifiers := []struct {
		name string
		fn   func(byte) (bool, error)
	}{
		{"IsAlpha", p.IsAlpha}, {"IsAlnum", p.IsAlnum}, {"IsBlank", p.IsBlank},
		{"IsCntrl", p.IsCntrl}, {"IsDigit", p.IsDigit}, {"IsGraph", p.IsGraph},
		{"IsLower", p.IsLower}, {"IsPrint", p.IsPrint}, {"IsPunct", p.IsPunct},
		{"IsSpace", p.IsSpace}, {"IsUpper", p.IsUpper}, {"IsXDigit", p.IsXDigit},
	}
	for _, c := range classifiers {
		if _, err := c.fn('a'); !errors.Is(err, ErrUnsupportedPlatform) {
			t.Errorf("%s = %v, want ErrUnsupportedPlatform", c.name, err)
		}
	}

	if _, err := p.ToLower([]byte("AB")); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Errorf("ToLower = %v, want ErrUnsupportedPlatform", err)
	}
	if _, err := p.ToUpper([]byte("ab")); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Errorf("ToUpper = %v, want ErrUnsupportedPlatform", err)
	}
	if err := p.Close(); err != nil {
		t.Errorf("Close = %v, want nil", err)
	}
}
