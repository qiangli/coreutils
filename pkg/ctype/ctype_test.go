// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package ctype

import "testing"

// TestNormalizeLocale is platform-independent: it exercises the gate every
// platform shares, before any libc is touched.
func TestNormalizeLocale(t *testing.T) {
	tests := []struct {
		name     string
		wantOK   bool
		wantCano string
	}{
		{"C", true, "C"},
		{"POSIX", true, "C"},
		{"de_DE.ISO-8859-1", true, "de_DE.ISO-8859-1"},
		{"de_DE.iso88591", true, "de_DE.ISO-8859-1"},
		{"DE_DE.iso-8859-1", true, "de_DE.ISO-8859-1"},
		{"de_de.ISO88591", true, "de_DE.ISO-8859-1"},

		// "C"/"POSIX" are exact, case-sensitive matches — not aliased the
		// way the de_DE names are.
		{"c", false, ""},
		{"posix", false, ""},
		{"Posix", false, ""},

		{"", false, ""},
		{"de_DE", false, ""},
		{"de_DE.UTF-8", false, ""},
		{"UTF-8", false, ""},
		{"en_US.UTF-8", false, ""},
		{"de_DE.ISO-8859-15", false, ""},
		{"Latin-9", false, ""},
		{"de_DE.latin9", false, ""},
		{"de_DE.ISO8859-15", false, ""},
		{"garbage", false, ""},
		{"de_DE.ISO-8859-1x", false, ""},

		// A NUL byte can never match one of the fixed cases, so it falls
		// through the same way any other unsupported name does.
		{"C\x00", false, ""},
		{"de_DE.ISO-8859-1\x00", false, ""},
		{"\x00", false, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			canonical, codesets, ok := normalizeLocale(tc.name)
			if ok != tc.wantOK {
				t.Fatalf("normalizeLocale(%q) ok = %v, want %v", tc.name, ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if canonical != tc.wantCano {
				t.Errorf("normalizeLocale(%q) canonical = %q, want %q", tc.name, canonical, tc.wantCano)
			}
			if len(codesets) == 0 {
				t.Errorf("normalizeLocale(%q) returned no expected codesets", tc.name)
			}
		})
	}
}

// TestOpenRejectsLocalesBeforeLibc holds on every platform (glibc or stub):
// the locale gate runs before any provider-specific work.
func TestOpenRejectsLocalesBeforeLibc(t *testing.T) {
	for _, name := range []string{
		"", "c", "posix", "de_DE", "de_DE.UTF-8", "UTF-8", "en_US.UTF-8",
		"de_DE.ISO-8859-15", "Latin-9", "de_DE.latin9", "de_DE.ISO8859-15",
		"garbage", "de_DE.ISO-8859-1x", "C\x00", "\x00",
	} {
		if _, err := Open(name); err == nil {
			t.Errorf("Open(%q) = nil error, want a rejection", name)
		} else if err != ErrUnsupportedLocale {
			t.Errorf("Open(%q) = %v, want ErrUnsupportedLocale", name, err)
		}
	}
}
