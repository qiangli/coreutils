// Package bc implements the arithmetic and language engine for the bc applet.
//
// Gavin D. Howard's BSD-2-Clause bc is used as a behavioral oracle. No C
// source was copied or translated: this is a native Go engine built around
// math/big and the coreutils RunContext contract.
package bc

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

const (
	MaxBase  = 99
	MaxDim   = 2048
	MaxScale = 99
)

// Number is an arbitrary-precision fixed-point decimal: n * 10^-scale.
// Keeping scale explicit is essential because bc makes it observable.
type Number struct {
	n     *big.Int
	scale int
}

func Zero() Number { return Number{n: new(big.Int)} }

func fromInt64(v int64) Number { return Number{n: big.NewInt(v)} }

func pow10(n int) *big.Int {
	if n <= 0 {
		return big.NewInt(1)
	}
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
}

func ParseNumber(s string, base int) (Number, error) {
	if base < 2 || base > 16 {
		return Zero(), fmt.Errorf("invalid input base %d", base)
	}
	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return Zero(), fmt.Errorf("invalid number %q", s)
	}
	digits := strings.ToUpper(parts[0])
	frac := ""
	if len(parts) == 2 {
		frac = strings.ToUpper(parts[1])
	}
	if digits == "" {
		digits = "0"
	}
	n := new(big.Int)
	b := big.NewInt(int64(base))
	for _, r := range digits + frac {
		var d int
		switch {
		case r >= '0' && r <= '9':
			d = int(r - '0')
		case r >= 'A' && r <= 'F':
			d = int(r-'A') + 10
		default:
			return Zero(), fmt.Errorf("invalid digit %q", r)
		}
		// The value of an out-of-range digit is undefined by POSIX. Clamping
		// matches the historical implementations used as our differential
		// oracle without changing any conforming program.
		if d >= base {
			d = base - 1
		}
		n.Mul(n, b)
		n.Add(n, big.NewInt(int64(d)))
	}
	if base == 10 {
		return Number{n: n, scale: len(frac)}, nil
	}
	if len(frac) == 0 {
		return Number{n: n}, nil
	}
	// Convert a non-decimal fraction to a decimal with enough digits to
	// preserve bc's input precision, truncating rather than rounding.
	den := new(big.Int).Exp(b, big.NewInt(int64(len(frac))), nil)
	intDigits := len(digits)
	intPart := new(big.Int)
	fracPart := new(big.Int)
	div := new(big.Int).Exp(b, big.NewInt(int64(len(frac))), nil)
	intPart.QuoRem(n, div, fracPart)
	scale := len(frac)
	fracPart.Mul(fracPart, pow10(scale))
	fracPart.Quo(fracPart, den)
	intPart.Mul(intPart, pow10(scale))
	intPart.Add(intPart, fracPart)
	_ = intDigits
	return Number{n: intPart, scale: scale}, nil
}

func (x Number) clone() Number { return Number{n: new(big.Int).Set(x.n), scale: x.scale} }
func (x Number) Sign() int     { return x.n.Sign() }
func (x Number) Scale() int    { return x.scale }

func align(a, b Number) (*big.Int, *big.Int, int) {
	s := a.scale
	if b.scale > s {
		s = b.scale
	}
	ax := new(big.Int).Set(a.n)
	bx := new(big.Int).Set(b.n)
	if a.scale < s {
		ax.Mul(ax, pow10(s-a.scale))
	}
	if b.scale < s {
		bx.Mul(bx, pow10(s-b.scale))
	}
	return ax, bx, s
}

func (x Number) Add(y Number) Number {
	a, b, s := align(x, y)
	return Number{n: new(big.Int).Add(a, b), scale: s}
}
func (x Number) Sub(y Number) Number {
	a, b, s := align(x, y)
	return Number{n: new(big.Int).Sub(a, b), scale: s}
}
func (x Number) Neg() Number      { return Number{n: new(big.Int).Neg(x.n), scale: x.scale} }
func (x Number) Cmp(y Number) int { a, b, _ := align(x, y); return a.Cmp(b) }

func (x Number) Mul(y Number, globalScale int) Number {
	s := x.scale + y.scale
	min := x.scale
	if y.scale > min {
		min = y.scale
	}
	if globalScale > min {
		min = globalScale
	}
	n := new(big.Int).Mul(x.n, y.n)
	if s > min {
		n.Quo(n, pow10(s-min))
		s = min
	}
	return Number{n: n, scale: s}
}

func (x Number) Div(y Number, scale int) (Number, error) {
	if y.Sign() == 0 {
		return Zero(), fmt.Errorf("divide by zero")
	}
	n := new(big.Int).Mul(x.n, pow10(scale+y.scale))
	n.Quo(n, new(big.Int).Mul(y.n, pow10(x.scale)))
	return Number{n: n, scale: scale}, nil
}

func (x Number) Mod(y Number, scale int) (Number, error) {
	q, err := x.Div(y, scale)
	if err != nil {
		return Zero(), err
	}
	target := x.scale
	if scale+y.scale > target {
		target = scale + y.scale
	}
	product := Number{n: new(big.Int).Mul(q.n, y.n), scale: q.scale + y.scale}
	product = product.withScale(target)
	return x.Sub(product).withScale(target), nil
}

func (x Number) Pow(y Number, globalScale int) (Number, error) {
	exponent := new(big.Int).Set(y.n)
	if y.scale != 0 {
		den := pow10(y.scale)
		rem := new(big.Int)
		exponent.QuoRem(exponent, den, rem)
		if rem.Sign() != 0 {
			return Zero(), fmt.Errorf("non-integer exponent")
		}
	}
	if !exponent.IsInt64() {
		return Zero(), fmt.Errorf("exponent too large")
	}
	e := exponent.Int64()
	if e == 0 {
		return fromInt64(1), nil
	}
	neg := e < 0
	if neg {
		e = -e
	}
	n := new(big.Int).Exp(x.n, big.NewInt(e), nil)
	if neg {
		decimalPower := int64(x.scale)*e + int64(globalScale)
		if decimalPower < 0 || int64(int(decimalPower)) != decimalPower {
			return Zero(), fmt.Errorf("exponent result scale too large")
		}
		numerator := pow10(int(decimalPower))
		return Number{n: new(big.Int).Quo(numerator, n), scale: globalScale}, nil
	}
	s := x.scale * int(e)
	maxs := globalScale
	if x.scale > maxs {
		maxs = x.scale
	}
	if s > maxs {
		n.Quo(n, pow10(s-maxs))
		s = maxs
	}
	return Number{n: n, scale: s}, nil
}

func (x Number) Sqrt(scale int) (Number, error) {
	if x.Sign() < 0 {
		return Zero(), fmt.Errorf("square root of negative number")
	}
	target := scale
	if x.scale > target {
		target = x.scale
	}
	p := 2*target - x.scale
	n := new(big.Int).Set(x.n)
	if p >= 0 {
		n.Mul(n, pow10(p))
	} else {
		n.Quo(n, pow10(-p))
	}
	return Number{n: new(big.Int).Sqrt(n), scale: target}, nil
}

func (x Number) Length() int {
	if x.n.Sign() == 0 && x.scale > 0 {
		return x.scale
	}
	return len(new(big.Int).Abs(x.n).String())
}

func (x Number) withScale(scale int) Number {
	n := new(big.Int).Set(x.n)
	if x.scale < scale {
		n.Mul(n, pow10(scale-x.scale))
	} else if x.scale > scale {
		n.Quo(n, pow10(x.scale-scale))
	}
	return Number{n: n, scale: scale}
}

func (x Number) StringBase(base int) string {
	if base < 2 || base > MaxBase {
		base = 10
	}
	if x.n.Sign() == 0 {
		return "0"
	}
	if base != 10 {
		neg := x.n.Sign() < 0
		n := new(big.Int).Abs(x.n)
		den := pow10(x.scale)
		whole, rem := new(big.Int), new(big.Int)
		whole.QuoRem(n, den, rem)
		formatDigit := func(d int64) string {
			if base <= 16 {
				return string("0123456789ABCDEF"[d])
			}
			return fmt.Sprintf("%0*d", len(strconv.Itoa(base-1)), d)
		}
		var integerDigits []string
		if whole.Sign() != 0 {
			bb := big.NewInt(int64(base))
			v := new(big.Int).Set(whole)
			for v.Sign() != 0 {
				q, r := new(big.Int), new(big.Int)
				q.QuoRem(v, bb, r)
				integerDigits = append(integerDigits, formatDigit(r.Int64()))
				v = q
			}
			for i, j := 0, len(integerDigits)-1; i < j; i, j = i+1, j-1 {
				integerDigits[i], integerDigits[j] = integerDigits[j], integerDigits[i]
			}
		}
		sep := ""
		if base > 16 {
			sep = " "
		}
		result := strings.Join(integerDigits, sep)
		if base > 16 && result != "" {
			result = " " + result
		}
		if x.scale > 0 {
			// Smallest d such that base^d >= 10^scale. This avoids
			// floating-point rounding at exact-power boundaries.
			digits := 0
			capacity := big.NewInt(1)
			bb := big.NewInt(int64(base))
			for capacity.Cmp(pow10(x.scale)) < 0 {
				capacity.Mul(capacity, bb)
				digits++
			}
			fracDigits := make([]string, 0, digits)
			for range digits {
				rem.Mul(rem, bb)
				d, next := new(big.Int), new(big.Int)
				d.QuoRem(rem, den, next)
				rem = next
				fracDigits = append(fracDigits, formatDigit(d.Int64()))
			}
			result += "." + strings.Join(fracDigits, sep)
		}
		if neg {
			result = "-" + result
		}
		return result
	}
	neg := x.n.Sign() < 0
	a := new(big.Int).Abs(x.n).String()
	if x.scale == 0 {
		if neg {
			return "-" + a
		}
		return a
	}
	if len(a) <= x.scale {
		a = strings.Repeat("0", x.scale-len(a)+1) + a
	}
	i := len(a) - x.scale
	// bc omits the leading zero for proper fractions.
	if i == 1 && a[0] == '0' {
		a = a[1:]
		i = 0
	}
	r := a[:i] + "." + a[i:]
	if neg {
		return "-" + r
	}
	return r
}

// wrappedStringBase applies the historical POSIX-locale bc line width. A
// continued physical line carries 68 data bytes followed by a backslash; the
// newline is the seventieth output character. Splitting is byte-oriented in
// the POSIX locale, whose numeric alphabet is ASCII.
func (x Number) wrappedStringBase(base int) string {
	s := x.StringBase(base)
	const dataPerContinuedLine = 68
	if len(s) <= dataPerContinuedLine {
		return s
	}
	var out strings.Builder
	for len(s) > dataPerContinuedLine {
		out.WriteString(s[:dataPerContinuedLine])
		out.WriteString("\\\n")
		s = s[dataPerContinuedLine:]
	}
	out.WriteString(s)
	return out.String()
}
