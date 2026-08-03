package printfcmd

import (
	"strings"
	"testing"
)

// These cases target the residual conformance gaps against GNU printf: the
// C-strtol numeric parsing (partial conversion, overflow clamping), the
// empty-%c NUL byte, hex-float exponent formatting, and the inf/nan spellings.

// TestPrintfNumericConversionResiduals covers strtol-style parsing: a leading
// numeric prefix is consumed and formatted while trailing garbage yields a
// diagnostic (non-zero exit) rather than discarding the value, and out-of-range
// magnitudes clamp to the target type's limit.
func TestPrintfNumericConversionResiduals(t *testing.T) {
	cases := []struct {
		args   []string
		want   string
		code   int
		errHas string
	}{
		// Partial conversion: value used, warning raised, exit 1.
		{[]string{"%d", "12abc"}, "12", 1, "not completely converted"},
		{[]string{"%d", "5 "}, "5", 1, "not completely converted"},
		// No numeric value: 0 used, diagnostic, exit 1.
		{[]string{"%d", "abc"}, "0", 1, "expected a numeric value"},
		// Signed overflow clamps to INT64_MAX / INT64_MIN.
		{[]string{"%d", "99999999999999999999999"}, "9223372036854775807", 1, "out of range"},
		{[]string{"%d", "-99999999999999999999999"}, "-9223372036854775808", 1, "out of range"},
		// Unsigned overflow clamps to UINT64_MAX.
		{[]string{"%u", "99999999999999999999999"}, "18446744073709551615", 1, "out of range"},
		// Clean octal / hex prefixes still parse without diagnostics.
		{[]string{"%d", "017"}, "15", 0, ""},
		{[]string{"%d", "0x1A"}, "26", 0, ""},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, c.args...)
		if out != c.want || code != c.code {
			t.Errorf("printf %v = (%q, code=%d), want (%q, %d)", c.args, out, code, c.want, c.code)
		}
		if c.errHas != "" && !strings.Contains(errb, c.errHas) {
			t.Errorf("printf %v stderr=%q, want contains %q", c.args, errb, c.errHas)
		}
		if c.errHas == "" && errb != "" {
			t.Errorf("printf %v unexpected stderr=%q", c.args, errb)
		}
	}
}

// TestPrintfEmptyCharNUL: %c of an empty ARGUMENT emits a single NUL byte, as
// GNU printf does (C's *argument == '\0').
func TestPrintfEmptyCharNUL(t *testing.T) {
	out, _, code := runTool(t, "%c", "")
	if code != 0 || out != "\x00" {
		t.Errorf("%%c empty: out=%q code=%d, want NUL byte", out, code)
	}
	// Width still applies, counting the NUL as one character.
	out, _, code = runTool(t, "%3c", "")
	if code != 0 || out != "  \x00" {
		t.Errorf("%%3c empty: out=%q code=%d, want two spaces + NUL", out, code)
	}
}

// TestPrintfHexFloat: %a/%A use the minimum number of exponent digits, not Go's
// zero-padded two-digit exponent.
func TestPrintfHexFloat(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"%a", "1.0"}, "0x1p+0"},
		{[]string{"%a", "0.5"}, "0x1p-1"},
		{[]string{"%a", "0"}, "0x0p+0"},
		{[]string{"%a", "0.1"}, "0x1.999999999999ap-4"},
		{[]string{"%A", "1.0"}, "0X1P+0"},
		{[]string{"%.3a", "1.0"}, "0x1.000p+0"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("printf %v = (%q, err=%q, code=%d), want (%q, 0)", c.args, out, errb, code, c.want)
		}
	}
}

// TestPrintfInfNaN: floating-point conversions print C's inf/nan spellings
// (INF/NAN for uppercase verbs), honour the sign flags, ignore the '0' flag when
// padding, and accept inf/infinity/nan spellings on input.
func TestPrintfInfNaN(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"%f", "nan"}, "nan"},
		{[]string{"%f", "inf"}, "inf"},
		{[]string{"%f", "-inf"}, "-inf"},
		{[]string{"%F", "inf"}, "INF"},
		{[]string{"%E", "nan"}, "NAN"},
		{[]string{"%G", "-inf"}, "-INF"},
		{[]string{"%+f", "inf"}, "+inf"},
		{[]string{"% f", "nan"}, " nan"},
		{[]string{"%5f", "nan"}, "  nan"},
		{[]string{"%-5f|", "inf"}, "inf  |"},
		{[]string{"%05f", "inf"}, "  inf"},
		{[]string{"%f", "infinity"}, "inf"},
		{[]string{"%f", "INF"}, "inf"},
		{[]string{"%g", "nan"}, "nan"},
		{[]string{"%a", "inf"}, "inf"},
		{[]string{"%A", "nan"}, "NAN"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("printf %v = (%q, err=%q, code=%d), want (%q, 0)", c.args, out, errb, code, c.want)
		}
	}
}
