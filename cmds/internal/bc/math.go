package bc

import (
	"fmt"
	"math/big"
)

func mathPrec(scale int) uint {
	if scale < 20 {
		scale = 20
	}
	return uint((scale + 32) * 4)
}

func numberFloat(n Number, prec uint) *big.Float {
	f := new(big.Float).SetPrec(prec).SetInt(n.n)
	if n.scale != 0 {
		f.Quo(f, new(big.Float).SetPrec(prec).SetInt(pow10(n.scale)))
	}
	return f
}

func floatNumber(f *big.Float, scale int) Number {
	r, _ := f.Rat(nil)
	r.Mul(r, new(big.Rat).SetInt(pow10(scale)))
	n := new(big.Int).Quo(r.Num(), r.Denom())
	return Number{n: n, scale: scale}
}

func epsilon(prec uint) *big.Float {
	return new(big.Float).SetPrec(prec).SetMantExp(new(big.Float).SetPrec(prec).SetInt64(1), -int(prec)+8)
}

func absFloat(x *big.Float) *big.Float {
	if x.Sign() < 0 {
		return new(big.Float).SetPrec(x.Prec()).Neg(x)
	}
	return new(big.Float).SetPrec(x.Prec()).Set(x)
}

func atanSeries(x *big.Float, prec uint) *big.Float {
	one := new(big.Float).SetPrec(prec).SetInt64(1)
	abs := absFloat(x)
	if abs.Cmp(new(big.Float).SetPrec(prec).SetFloat64(.5)) > 0 {
		// atan(x)=pi/4+atan((x-1)/(x+1)); the transformed argument
		// converges toward zero without the x=1 reciprocal cycle.
		z := new(big.Float).SetPrec(prec).Quo(
			new(big.Float).SetPrec(prec).Sub(abs, one),
			new(big.Float).SetPrec(prec).Add(abs, one))
		r := atanSeries(z, prec)
		quarterPi := new(big.Float).SetPrec(prec).Quo(piFloat(prec), new(big.Float).SetPrec(prec).SetInt64(4))
		r.Add(quarterPi, r)
		if x.Sign() < 0 {
			r.Neg(r)
		}
		return r
	}
	x2 := new(big.Float).SetPrec(prec).Mul(x, x)
	term := new(big.Float).SetPrec(prec).Set(x)
	sum := new(big.Float).SetPrec(prec).Set(x)
	eps := epsilon(prec)
	for k := int64(1); k < 100000; k++ {
		term.Mul(term, x2)
		frac := new(big.Float).SetPrec(prec).Quo(term, new(big.Float).SetPrec(prec).SetInt64(2*k+1))
		if k&1 == 1 {
			sum.Sub(sum, frac)
		} else {
			sum.Add(sum, frac)
		}
		if absFloat(frac).Cmp(eps) < 0 {
			break
		}
	}
	return sum
}

func piFloat(prec uint) *big.Float {
	a := atanSeries(new(big.Float).SetPrec(prec).Quo(new(big.Float).SetPrec(prec).SetInt64(1), new(big.Float).SetPrec(prec).SetInt64(5)), prec)
	b := atanSeries(new(big.Float).SetPrec(prec).Quo(new(big.Float).SetPrec(prec).SetInt64(1), new(big.Float).SetPrec(prec).SetInt64(239)), prec)
	a.Mul(a, new(big.Float).SetPrec(prec).SetInt64(16))
	b.Mul(b, new(big.Float).SetPrec(prec).SetInt64(4))
	return a.Sub(a, b)
}

func sincos(x *big.Float, prec uint, cos bool) *big.Float {
	// Range reduce to [-pi,pi] before the Taylor series.
	pi := piFloat(prec)
	twoPi := new(big.Float).SetPrec(prec).Mul(pi, new(big.Float).SetPrec(prec).SetInt64(2))
	q := new(big.Float).SetPrec(prec).Quo(x, twoPi)
	qi, _ := q.Int(nil)
	x = new(big.Float).SetPrec(prec).Sub(x, new(big.Float).SetPrec(prec).Mul(new(big.Float).SetPrec(prec).SetInt(qi), twoPi))
	x2 := new(big.Float).SetPrec(prec).Mul(x, x)
	eps := epsilon(prec)
	var term, sum *big.Float
	start := int64(2)
	if cos {
		term = new(big.Float).SetPrec(prec).SetInt64(1)
		sum = new(big.Float).SetPrec(prec).Set(term)
		start = 1
	} else {
		term = new(big.Float).SetPrec(prec).Set(x)
		sum = new(big.Float).SetPrec(prec).Set(x)
	}
	for n := start; n < 100000; n += 2 {
		den := new(big.Float).SetPrec(prec).SetInt64(n * (n + 1))
		term.Mul(term, x2)
		term.Quo(term, den)
		term.Neg(term)
		sum.Add(sum, term)
		if absFloat(term).Cmp(eps) < 0 {
			break
		}
	}
	return sum
}

func expFloat(x *big.Float, prec uint) *big.Float {
	if x.Sign() < 0 {
		positive := expFloat(new(big.Float).SetPrec(prec).Neg(x), prec)
		return new(big.Float).SetPrec(prec).Quo(new(big.Float).SetPrec(prec).SetInt64(1), positive)
	}
	// Repeatedly halve large arguments, evaluate a rapidly converging
	// Taylor series near zero, then square back. This avoids catastrophic
	// cancellation and impractically long series for large magnitudes.
	reduced := new(big.Float).SetPrec(prec).Set(x)
	half := new(big.Float).SetPrec(prec).SetFloat64(.5)
	halvings := 0
	for reduced.Cmp(half) > 0 {
		reduced.Quo(reduced, new(big.Float).SetPrec(prec).SetInt64(2))
		halvings++
	}
	term := new(big.Float).SetPrec(prec).SetInt64(1)
	sum := new(big.Float).SetPrec(prec).SetInt64(1)
	eps := epsilon(prec)
	for n := int64(1); n < 100000; n++ {
		term.Mul(term, reduced)
		term.Quo(term, new(big.Float).SetPrec(prec).SetInt64(n))
		sum.Add(sum, term)
		if absFloat(term).Cmp(eps) < 0 {
			break
		}
	}
	for range halvings {
		sum.Mul(sum, sum)
	}
	return sum
}

func powFloatInt(x *big.Float, n int64, prec uint) *big.Float {
	r := new(big.Float).SetPrec(prec).SetInt64(1)
	b := new(big.Float).SetPrec(prec).Set(x)
	for n > 0 {
		if n&1 != 0 {
			r.Mul(r, b)
		}
		n >>= 1
		if n != 0 {
			b.Mul(b, b)
		}
	}
	return r
}

func besselFloat(order int64, x *big.Float, prec uint) *big.Float {
	negOrder := order < 0
	if negOrder {
		order = -order
	}
	half := new(big.Float).SetPrec(prec).Quo(x, new(big.Float).SetPrec(prec).SetInt64(2))
	term := powFloatInt(half, order, prec)
	for i := int64(2); i <= order; i++ {
		term.Quo(term, new(big.Float).SetPrec(prec).SetInt64(i))
	}
	sum := new(big.Float).SetPrec(prec).Set(term)
	x2quarter := new(big.Float).SetPrec(prec).Mul(half, half)
	eps := epsilon(prec)
	for k := int64(0); k < 100000; k++ {
		den := new(big.Float).SetPrec(prec).SetInt64((k + 1) * (order + k + 1))
		term.Mul(term, x2quarter)
		term.Quo(term, den)
		term.Neg(term)
		sum.Add(sum, term)
		if absFloat(term).Cmp(eps) < 0 {
			break
		}
	}
	if negOrder && order&1 != 0 {
		sum.Neg(sum)
	}
	return sum
}

func logUnit(x *big.Float, prec uint) *big.Float {
	one := new(big.Float).SetPrec(prec).SetInt64(1)
	z := new(big.Float).SetPrec(prec).Quo(new(big.Float).SetPrec(prec).Sub(x, one), new(big.Float).SetPrec(prec).Add(x, one))
	z2 := new(big.Float).SetPrec(prec).Mul(z, z)
	term := new(big.Float).SetPrec(prec).Set(z)
	sum := new(big.Float).SetPrec(prec).Set(z)
	eps := epsilon(prec)
	for n := int64(3); n < 100000; n += 2 {
		term.Mul(term, z2)
		q := new(big.Float).SetPrec(prec).Quo(term, new(big.Float).SetPrec(prec).SetInt64(n))
		sum.Add(sum, q)
		if absFloat(q).Cmp(eps) < 0 {
			break
		}
	}
	return sum.Mul(sum, new(big.Float).SetPrec(prec).SetInt64(2))
}

func logFloat(x *big.Float, prec uint) (*big.Float, error) {
	if x.Sign() <= 0 {
		return nil, fmt.Errorf("logarithm of non-positive number")
	}
	// Normalize x=m*2^e with m in [1,2), then use the atanh series.
	m := new(big.Float).SetPrec(prec)
	e := x.MantExp(m)
	m.Mul(m, new(big.Float).SetPrec(prec).SetInt64(2))
	e--
	sum := logUnit(m, prec)
	if e != 0 {
		ln2 := logUnit(new(big.Float).SetPrec(prec).SetInt64(2), prec)
		ln2.Mul(ln2, new(big.Float).SetPrec(prec).SetInt64(int64(e)))
		sum.Add(sum, ln2)
	}
	return sum, nil
}

func (b *Interpreter) mathCall(name string, v Number) (Number, error) {
	prec := mathPrec(b.Scale)
	x := numberFloat(v, prec)
	var y *big.Float
	var err error
	switch name {
	case "s":
		y = sincos(x, prec, false)
	case "c":
		y = sincos(x, prec, true)
	case "a":
		y = atanSeries(x, prec)
	case "e":
		y = expFloat(x, prec)
	case "l":
		y, err = logFloat(x, prec)
	default:
		return Zero(), fmt.Errorf("unknown math function %s", name)
	}
	if err != nil {
		return Zero(), err
	}
	return floatNumber(y, b.Scale), nil
}

func (b *Interpreter) besselCall(order, arg Number) (Number, error) {
	num := new(big.Int).Set(order.n)
	if order.scale != 0 {
		rem := new(big.Int)
		num.QuoRem(num, pow10(order.scale), rem)
		if rem.Sign() != 0 {
			return Zero(), fmt.Errorf("Bessel order is not an integer")
		}
	}
	if !num.IsInt64() || num.Cmp(big.NewInt(-1<<30)) < 0 || num.Cmp(big.NewInt(1<<30)) > 0 {
		return Zero(), fmt.Errorf("Bessel order is not an integer")
	}
	prec := mathPrec(b.Scale)
	return floatNumber(besselFloat(num.Int64(), numberFloat(arg, prec), prec), b.Scale), nil
}
