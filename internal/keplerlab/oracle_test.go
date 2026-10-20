package keplerlab_test

import (
	"math"
	"math/big"
	"sync"
	"testing"
)

// prec is the oracle's working precision: 256 bits, about 77 digits. The
// worst conditioning measured here is 1/(1−e) ≈ 2⁵³ at e = 1 − 2⁻⁵³, which
// still leaves two hundred bits below a float64's last one.
const prec = 256

func newF(v float64) *big.Float { return new(big.Float).SetPrec(prec).SetFloat64(v) }

func newI(v int64) *big.Float { return new(big.Float).SetPrec(prec).SetInt64(v) }

// atanInv is atan(1/x) for an integer x > 1, by its alternating series, at
// precision p.
func atanInv(x int64, p uint) *big.Float {
	sum := new(big.Float).SetPrec(p)
	xx := new(big.Float).SetPrec(p).SetInt64(x * x)
	term := new(big.Float).SetPrec(p).Quo(new(big.Float).SetPrec(p).SetInt64(1), new(big.Float).SetPrec(p).SetInt64(x))

	for n := int64(0); ; n++ {
		t := new(big.Float).SetPrec(p).Quo(term, new(big.Float).SetPrec(p).SetInt64(2*n+1))
		if n%2 == 0 {
			sum.Add(sum, t)
		} else {
			sum.Sub(sum, t)
		}

		term.Quo(term, xx)

		if term.MantExp(nil) < -int(p)-8 {
			return sum
		}
	}
}

// machinPi is π = 16·atan(1/5) − 4·atan(1/239) at precision p.
func machinPi(p uint) *big.Float {
	a := atanInv(5, p+16)
	a.Mul(a, new(big.Float).SetPrec(p+16).SetInt64(16))
	b := atanInv(239, p+16)
	b.Mul(b, new(big.Float).SetPrec(p+16).SetInt64(4))

	return new(big.Float).SetPrec(p).Sub(a, b)
}

// agmPi is π by the Gauss–Legendre arithmetic–geometric mean at precision p:
// an algorithm sharing nothing with machinPi but the arithmetic.
func agmPi(p uint) *big.Float {
	w := p + 32
	f := func(v float64) *big.Float { return new(big.Float).SetPrec(w).SetFloat64(v) }

	a, b, t, pw := f(1), new(big.Float).SetPrec(w).Sqrt(f(0.5)), f(0.25), f(1)

	for range 20 {
		an := new(big.Float).SetPrec(w).Add(a, b)
		an.Quo(an, f(2))

		b = new(big.Float).SetPrec(w).Sqrt(new(big.Float).SetPrec(w).Mul(a, b))

		d := new(big.Float).SetPrec(w).Sub(a, an)
		d.Mul(d, d)
		d.Mul(d, pw)
		t.Sub(t, d)

		a = an
		pw.Mul(pw, f(2))
	}

	s := new(big.Float).SetPrec(w).Add(a, b)
	s.Mul(s, s)

	den := new(big.Float).SetPrec(w).Mul(t, f(4))

	return new(big.Float).SetPrec(p).Quo(s, den)
}

// reducePrec is the precision π is held at for range reduction: enough that
// reducing any finite float64 — whose exponent reaches 1023 — leaves prec
// bits of the remainder.
const reducePrec = prec + 1100

var (
	piOnce  sync.Once
	bigPi   *big.Float // at reducePrec
	big2Pi  *big.Float
	bigHalf *big.Float
)

func pi() *big.Float {
	piOnce.Do(func() {
		bigPi = machinPi(reducePrec)
		big2Pi = new(big.Float).SetPrec(reducePrec).Mul(bigPi, new(big.Float).SetPrec(reducePrec).SetInt64(2))
		bigHalf = new(big.Float).SetPrec(prec).SetFloat64(0.5)
	})

	return bigPi
}

// reduce is x less the nearest multiple of 2π, exactly (to prec bits), in
// [−π, π]. The float64 x is taken as the exact rational it is.
func reduce(x float64) *big.Float {
	pi()

	bx := new(big.Float).SetPrec(reducePrec).SetFloat64(x)
	k := new(big.Float).SetPrec(reducePrec).Quo(bx, big2Pi)

	ki, _ := k.Int(nil)
	// Round to nearest: Int truncates toward zero.
	frac := new(big.Float).SetPrec(reducePrec).Sub(k, new(big.Float).SetPrec(reducePrec).SetInt(ki))

	switch {
	case frac.Cmp(new(big.Float).SetFloat64(0.5)) > 0:
		ki.Add(ki, big.NewInt(1))
	case frac.Cmp(new(big.Float).SetFloat64(-0.5)) < 0:
		ki.Sub(ki, big.NewInt(1))
	}

	r := new(big.Float).SetPrec(reducePrec).Mul(new(big.Float).SetPrec(reducePrec).SetInt(ki), big2Pi)
	r.Sub(bx, r)

	return new(big.Float).SetPrec(prec).Set(r)
}

// sinCos is sin x and cos x for |x| <= 4, by their Taylor series.
func sinCos(x *big.Float) (s, c *big.Float) {
	x2 := new(big.Float).SetPrec(prec+32).Mul(x, x)

	s = new(big.Float).SetPrec(prec + 32).Set(x)
	c = new(big.Float).SetPrec(prec + 32).SetInt64(1)

	ts := new(big.Float).SetPrec(prec + 32).Set(x)
	tc := new(big.Float).SetPrec(prec + 32).SetInt64(1)

	for k := int64(1); k < 200; k++ {
		ts.Mul(ts, x2)
		ts.Quo(ts, new(big.Float).SetInt64((2*k)*(2*k+1)))
		ts.Neg(ts)

		tc.Mul(tc, x2)
		tc.Quo(tc, new(big.Float).SetInt64((2*k-1)*(2*k)))
		tc.Neg(tc)

		s.Add(s, ts)
		c.Add(c, tc)

		if ts.Sign() == 0 || (ts.MantExp(nil) < s.MantExp(nil)-int(prec)-16 && tc.MantExp(nil) < -int(prec)-16) {
			break
		}
	}

	return new(big.Float).SetPrec(prec).Set(s), new(big.Float).SetPrec(prec).Set(c)
}

// keplerResidual is E − e·sin E − M at prec.
func keplerResidual(bigE, e, m *big.Float) (f, fp *big.Float) {
	s, c := sinCos(bigE)

	f = new(big.Float).SetPrec(prec).Mul(e, s)
	f.Sub(bigE, f)
	f.Sub(f, m)

	fp = new(big.Float).SetPrec(prec).Mul(e, c)
	fp.Sub(newI(1), fp)

	return f, fp
}

// solveExact is the exact eccentric anomaly for the float64 inputs: the root
// in [−π, π] of E − e·sin E = M, with M the exact reduction of m. M = ±0
// gives E = ±0, which is the root.
func solveExact(m, e float64) *big.Float {
	if m == 0 {
		return newF(m) // keeps the sign of zero
	}

	bm := reduce(m)
	be := newF(e)

	neg := bm.Sign() < 0
	if neg {
		bm.Neg(bm)
	}

	// 0 <= E − M = e·sin E <= e on [0, π], so the root is bracketed by
	// [M, min(M + e, π)].
	lo := new(big.Float).SetPrec(prec).Set(bm)

	hi := new(big.Float).SetPrec(prec).Add(bm, be)
	if p := new(big.Float).SetPrec(prec).Set(pi()); hi.Cmp(p) > 0 {
		hi = p
	}

	bigE := new(big.Float).SetPrec(prec).Add(lo, hi)
	bigE.Mul(bigE, bigHalf)

	tol := -int(prec) + 8

	for range 2000 {
		f, fp := keplerResidual(bigE, be, bm)

		if f.Sign() == 0 {
			break
		}

		if f.Sign() > 0 {
			hi.Set(bigE)
		} else {
			lo.Set(bigE)
		}

		next := new(big.Float).SetPrec(prec).Quo(f, fp)
		next.Sub(bigE, next)

		if next.Cmp(lo) <= 0 || next.Cmp(hi) >= 0 {
			next.Add(lo, hi)
			next.Mul(next, bigHalf)
		}

		step := new(big.Float).SetPrec(prec).Sub(next, bigE)
		bigE = next

		if step.Sign() == 0 || step.MantExp(nil) < bigE.MantExp(nil)+tol {
			break
		}
	}

	if neg {
		bigE.Neg(bigE)
	}

	return bigE
}

// relErr is |got − want| / |want| in units of 2⁻⁵², the float64 epsilon —
// "ulps of E". A zero want must be matched by a zero of the same sign.
func relErr(got float64, want *big.Float) float64 {
	if want.Sign() == 0 {
		if got == 0 && math.Signbit(got) == want.Signbit() {
			return 0
		}

		return math.Inf(1)
	}

	d := new(big.Float).SetPrec(prec).Sub(newF(got), want)
	d.Quo(d, want)

	r, _ := d.Float64()

	return math.Abs(r) / 0x1p-52
}

// TestOraclePiAgrees: two unrelated algorithms for π agree to the precision
// range reduction needs, and both round to math.Pi.
func TestOraclePiAgrees(t *testing.T) {
	a, b := machinPi(reducePrec), agmPi(reducePrec)

	d := new(big.Float).SetPrec(reducePrec).Sub(a, b)
	if d.Sign() != 0 && d.MantExp(nil) > -int(reducePrec)+8 {
		t.Errorf("Machin and AGM disagree at 2^%d", d.MantExp(nil))
	}

	if f, _ := a.Float64(); f != math.Pi {
		t.Errorf("π rounds to %v, want math.Pi", f)
	}
}

// TestOracleSinCos: sin²+cos² = 1 to the working precision, sin(π/6) = 1/2,
// and the series agrees with math.Sin and math.Cos to a float64 ulp.
func TestOracleSinCos(t *testing.T) {
	sixth := new(big.Float).SetPrec(prec).Quo(new(big.Float).SetPrec(prec).Set(pi()), newI(6))

	s, _ := sinCos(sixth)
	if d := new(big.Float).SetPrec(prec).Sub(s, newF(0.5)); d.Sign() != 0 && d.MantExp(nil) > -int(prec)+8 {
		t.Errorf("sin(π/6) − 1/2 = 2^%d", d.MantExp(nil))
	}

	for _, x := range []float64{1e-300, 1e-8, 0.3, 1, 2.5, 3.14159, -3, 4} {
		s, c := sinCos(newF(x))

		one := new(big.Float).SetPrec(prec).Mul(s, s)
		one.Add(one, new(big.Float).SetPrec(prec).Mul(c, c))
		one.Sub(one, newI(1))

		if one.Sign() != 0 && one.MantExp(nil) > -int(prec)+8 {
			t.Errorf("x = %v: sin²+cos²−1 = 2^%d", x, one.MantExp(nil))
		}

		if sf, _ := s.Float64(); math.Abs(sf-math.Sin(x)) > 2*math.Abs(math.Nextafter(math.Sin(x), 2)-math.Sin(x)) {
			t.Errorf("sin(%v): oracle %v, math.Sin %v", x, sf, math.Sin(x))
		}
	}
}

// TestOracleSolvesKepler: the oracle's root satisfies the equation to the
// working precision across the whole domain, including the critical corner
// and the largest finite mean anomaly.
func TestOracleSolvesKepler(t *testing.T) {
	for _, e := range []float64{0, 0.5, 0.99, 1 - 0x1p-30, math.Nextafter(1, 0)} {
		for _, m := range []float64{1e-300, 1e-12, 1e-4, 0.5, 3, -2, math.Pi, 2 * math.Pi, 1e10, math.MaxFloat64} {
			bigE := solveExact(m, e)

			f, _ := keplerResidual(bigE, newF(e), reduce(m))
			if f.Sign() != 0 && f.MantExp(nil) > bigE.MantExp(nil)-int(prec)+12 {
				t.Errorf("e = %v, M = %v: residual 2^%d at E = %s", e, m, f.MantExp(nil), bigE.Text('g', 20))
			}
		}
	}
}
