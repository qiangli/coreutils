package printfcmd

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

// GNU printf converts numeric ARGUMENTs with the C strtol/strtod family: it
// consumes the leading numeric token (base auto-detected via a 0x/0 prefix),
// clamps an out-of-range magnitude to the target type's limit, and still
// formats the parsed value while raising a diagnostic (non-zero exit) when the
// token is empty, overflowed, or trailed by other characters. The helpers below
// reproduce that behaviour; a leading ' or " character-constant is handled by
// charConstant before any of this runs.

func isSpaceByte(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\v', '\f', '\r':
		return true
	}
	return false
}

// digitValue maps a byte to its value as a base-36 digit, or 99 when it is not a
// digit at all (so it never satisfies d < base for any supported base).
func digitValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'z':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'Z':
		return int(c-'A') + 10
	}
	return 99
}

// cScan mimics C strtoumax/strtoimax with base 0 over the leading token of s:
// optional whitespace, an optional sign, an optional 0x/0 base prefix, then the
// value digits. It reports the magnitude (clamped to math.MaxUint64 on
// overflow), whether a '-' sign was present, whether the magnitude overflowed
// uint64, the number of value digits consumed (0 means "no numeric value"), and
// whether any characters remained after the token.
func cScan(s string) (mag uint64, neg, overflow bool, nDigits int, leftover bool) {
	i := 0
	for i < len(s) && isSpaceByte(s[i]) {
		i++
	}
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		neg = s[i] == '-'
		i++
	}
	base := 10
	if i < len(s) && s[i] == '0' {
		if i+1 < len(s) && (s[i+1]|0x20) == 'x' && i+2 < len(s) && digitValue(s[i+2]) < 16 {
			base = 16
			i += 2
		} else {
			// Octal: the leading '0' itself is a value digit, so leave i on it.
			base = 8
		}
	}
	start := i
	for i < len(s) {
		d := digitValue(s[i])
		if d >= base {
			break
		}
		if !overflow {
			if mag > (math.MaxUint64-uint64(d))/uint64(base) {
				overflow = true
				mag = math.MaxUint64
			} else {
				mag = mag*uint64(base) + uint64(d)
			}
		}
		i++
	}
	nDigits = i - start
	leftover = i < len(s)
	return
}

func cQuote(s string) string { return "'" + s + "'" }

// numDiag emits the GNU-format diagnostic (if any) for a scanned numeric token
// and returns true when a diagnostic was raised (which makes printf exit 1).
func numDiag(rc *tool.RunContext, s string, nDigits int, overflow, leftover bool) bool {
	switch {
	case nDigits == 0:
		fmt.Fprintf(rc.Err, "%s: %s: expected a numeric value\n", cmd.Name, cQuote(s))
	case overflow:
		fmt.Fprintf(rc.Err, "%s: %s: Numerical result out of range\n", cmd.Name, cQuote(s))
	case leftover:
		fmt.Fprintf(rc.Err, "%s: %s: value not completely converted\n", cmd.Name, cQuote(s))
	default:
		return false
	}
	return true
}

func warnTrailingConst(rc *tool.RunContext, s string) {
	fmt.Fprintf(rc.Err, "%s: warning: %s: character(s) following character constant have been ignored\n", cmd.Name, s)
}

// parseSigned converts an ARGUMENT for the %d/%i conversions (and the numeric
// %*/%.* width and precision operands). The value is clamped to the int64 range
// on overflow; ok is false when a diagnostic was raised.
func parseSigned(rc *tool.RunContext, s string) (value int64, hadErr bool) {
	if r, ok, warn := charConstant(s); ok {
		if warn {
			warnTrailingConst(rc, s)
		}
		return int64(r), false
	}
	mag, neg, overflow, nDigits, leftover := cScan(s)
	if neg {
		switch {
		case mag > 1<<63:
			value, overflow = math.MinInt64, true
		case mag == 1<<63:
			value = math.MinInt64
		default:
			value = -int64(mag)
		}
	} else if mag > math.MaxInt64 {
		value, overflow = math.MaxInt64, true
	} else {
		value = int64(mag)
	}
	return value, numDiag(rc, s, nDigits, overflow, leftover)
}

// parseUnsigned converts an ARGUMENT for the %o/%u/%x/%X conversions. A leading
// '-' wraps two's-complement (matching C strtoumax); the magnitude is clamped to
// math.MaxUint64 on overflow.
func parseUnsigned(rc *tool.RunContext, s string) (value uint64, hadErr bool) {
	if r, ok, warn := charConstant(s); ok {
		if warn {
			warnTrailingConst(rc, s)
		}
		return uint64(r), false
	}
	mag, neg, overflow, nDigits, leftover := cScan(s)
	if neg {
		value = -mag
	} else {
		value = mag
	}
	return value, numDiag(rc, s, nDigits, overflow, leftover)
}

// parseFloat converts an ARGUMENT for the floating-point conversions. It accepts
// the same forms as C strtod (decimal, hex-float, inf/infinity/nan) via Go's
// ParseFloat, consuming the longest valid prefix so trailing garbage yields a
// "value not completely converted" diagnostic rather than discarding the value.
func parseFloat(rc *tool.RunContext, s string) (value float64, hadErr bool) {
	if r, ok, warn := charConstant(s); ok {
		if warn {
			warnTrailingConst(rc, s)
		}
		return float64(r), false
	}
	i := 0
	for i < len(s) && isSpaceByte(s[i]) {
		i++
	}
	t := s[i:]

	best := -1
	var bv float64
	brange := false
	for k := 1; k <= len(t); k++ {
		v, err := strconv.ParseFloat(t[:k], 64)
		if err == nil {
			best, bv, brange = k, v, false
		} else if errors.Is(err, strconv.ErrRange) {
			best, bv, brange = k, v, true
		}
	}
	if best < 0 {
		fmt.Fprintf(rc.Err, "%s: %s: expected a numeric value\n", cmd.Name, cQuote(s))
		return 0, true
	}
	if brange {
		fmt.Fprintf(rc.Err, "%s: %s: Numerical result out of range\n", cmd.Name, cQuote(s))
		return bv, true
	}
	if best < len(t) {
		fmt.Fprintf(rc.Err, "%s: %s: value not completely converted\n", cmd.Name, cQuote(s))
		return bv, true
	}
	return bv, false
}

// writeNonFinite formats a NaN or infinity for a floating-point conversion the
// way C printf does: the lowercase verbs print "inf"/"nan", the uppercase verbs
// (E/F/G/A) print "INF"/"NAN", the sign/space flags and a negative sign apply,
// the '0' flag is ignored (padding is always spaces), and width still pads.
func writeNonFinite(out *bytes.Buffer, conv byte, flags string, width int, hasWidth bool, fv float64) {
	base := "inf"
	if math.IsNaN(fv) {
		base = "nan"
	}
	switch conv {
	case 'E', 'F', 'G', 'A':
		base = strings.ToUpper(base)
	}
	sign := ""
	switch {
	case math.Signbit(fv):
		sign = "-"
	case strings.ContainsRune(flags, '+'):
		sign = "+"
	case strings.ContainsRune(flags, ' '):
		sign = " "
	}
	body := sign + base
	if !hasWidth || len(body) >= width {
		out.WriteString(body)
		return
	}
	pad := strings.Repeat(" ", width-len(body))
	if strings.ContainsRune(flags, '-') {
		out.WriteString(body)
		out.WriteString(pad)
		return
	}
	out.WriteString(pad)
	out.WriteString(body)
}

// writeHexFloat formats a finite value for %a/%A. Go's %x/%X float verb produces
// the same mantissa as C's %a but zero-pads the binary exponent ("p+01"); C uses
// the minimum number of exponent digits ("p+1"), so the exponent is rewritten
// before width padding is applied.
func writeHexFloat(out *bytes.Buffer, conv byte, flags string, width int, hasWidth bool, precision int, hasPrecision bool, fv float64) {
	goConv := "x"
	if conv == 'A' {
		goConv = "X"
	}
	var tmp strings.Builder
	verb := buildVerb(flags, 0, false, precision, hasPrecision, goConv)
	fmt.Fprintf(&tmp, verb, fv)
	padNumeric(out, fixHexExp(tmp.String()), flags, width, hasWidth)
}

// fixHexExp rewrites the binary exponent of a Go-formatted hex float so it uses
// the minimum number of digits (at least one), matching C's %a.
func fixHexExp(s string) string {
	idx := strings.IndexAny(s, "pP")
	if idx < 0 {
		return s
	}
	head, exp := s[:idx+1], s[idx+1:]
	sign := ""
	if len(exp) > 0 && (exp[0] == '+' || exp[0] == '-') {
		sign, exp = exp[:1], exp[1:]
	}
	exp = strings.TrimLeft(exp, "0")
	if exp == "" {
		exp = "0"
	}
	return head + sign + exp
}

// padNumeric applies a field width to an already-formatted numeric string,
// honouring the '-' (left-justify) and '0' (zero-fill after any sign and 0x/0X
// prefix) flags.
func padNumeric(out *bytes.Buffer, s, flags string, width int, hasWidth bool) {
	if !hasWidth || len(s) >= width {
		out.WriteString(s)
		return
	}
	pad := width - len(s)
	if strings.ContainsRune(flags, '-') {
		out.WriteString(s)
		out.WriteString(strings.Repeat(" ", pad))
		return
	}
	if strings.ContainsRune(flags, '0') {
		i := 0
		if i < len(s) && (s[i] == '+' || s[i] == '-' || s[i] == ' ') {
			i++
		}
		if i+1 < len(s) && s[i] == '0' && (s[i+1]|0x20) == 'x' {
			i += 2
		}
		out.WriteString(s[:i])
		out.WriteString(strings.Repeat("0", pad))
		out.WriteString(s[i:])
		return
	}
	out.WriteString(strings.Repeat(" ", pad))
	out.WriteString(s)
}
