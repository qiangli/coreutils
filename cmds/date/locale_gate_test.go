package datecmd

import "testing"

// A numeric-only format consults no locale data, so an unavailable LC_TIME
// must not fail it. Eager resolution used to reject `date +%Y%m%d` under an
// ordinary LANG, and inside a larger command the failure was swallowed —
// "tag=v$(date -u +%Y%m%d)" printed "tag=v" and exited 0, so `set -e` never
// fired and a date-stamped release tag silently truncated.
func TestFormatNeedsLocale(t *testing.T) {
	for _, f := range []string{
		"%Y%m%d-%H%M", "%F", "%T", "%s", "%Y-%m-%dT%H:%M:%S%z",
		"%D", "%R", "%I:%M", "%j", "%%b", "plain text", "",
	} {
		if formatNeedsLocale(f) {
			t.Errorf("formatNeedsLocale(%q) = true, want false — it reads no locale data", f)
		}
	}
	for _, f := range []string{
		"%a", "%A", "%b", "%B", "%h", "%c", "%p", "%P", "%r", "%x", "%X",
		"%Y %b %d", "%Ex", "%OY", "%%%b",
	} {
		if !formatNeedsLocale(f) {
			t.Errorf("formatNeedsLocale(%q) = false, want true — it prints locale text", f)
		}
	}
}

// %% is an escaped percent and must not let the NEXT byte be read as a
// conversion: "%%b" is a literal "%b", not the month name.
func TestEscapedPercentDoesNotConsumeTheNextSpecifier(t *testing.T) {
	if formatNeedsLocale("%%b") {
		t.Error("formatNeedsLocale of an escaped percent = true: %% is a literal percent, so the b is text")
	}
	if !formatNeedsLocale("%%%b") {
		t.Error("formatNeedsLocale of escaped-percent-then-conversion = false: the third %% starts a real month-name conversion")
	}
}
