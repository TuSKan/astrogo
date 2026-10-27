package oracle

import (
	"math"
	"math/big"
	"sync"
)

// Prec is the oracle's working precision: 256 bits, about 77 digits. The
// worst conditioning measured here is 1/(1−e) ≈ 2⁵³ at e = 1 − 2⁻⁵³, which
// still leaves two hundred bits below a float64's last one.
const Prec = 256

// ReducePrec is the precision π is held at for range reduction: enough that
// reducing any finite float64 — whose exponent reaches 1023 — leaves Prec
// bits of the remainder.
const ReducePrec = Prec + 1100

func newF(v float64) *big.Float { return new(big.Float).SetPrec(Prec).SetFloat64(v) }

func newI(v int64) *big.Float { return new(big.Float).SetPrec(Prec).SetInt64(v) }

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

// MachinPi is π = 16·atan(1/5) − 4·atan(1/239) at precision p.
func MachinPi(p uint) *big.Float {
	a := atanInv(5, p+16)
	a.Mul(a, new(big.Float).SetPrec(p+16).SetInt64(16))
	b := atanInv(239, p+16)
	b.Mul(b, new(big.Float).SetPrec(p+16).SetInt64(4))

	return new(big.Float).SetPrec(p).Sub(a, b)
}

// AGMPi is π by the Gauss–Legendre arithmetic–geometric mean at precision p:
// an algorithm sharing nothing with MachinPi but the arithmetic.
func AGMPi(p uint) *big.Float {
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

var (
	piOnce  sync.Once
	bigPi   *big.Float // at ReducePrec
	big2Pi  *big.Float
	bigHalf *big.Float
)

// Pi is π at ReducePrec, by Machin's formula.
func Pi() *big.Float {
	piOnce.Do(func() {
		bigPi = MachinPi(ReducePrec)
		big2Pi = new(big.Float).SetPrec(ReducePrec).Mul(bigPi, new(big.Float).SetPrec(ReducePrec).SetInt64(2))
		bigHalf = new(big.Float).SetPrec(Prec).SetFloat64(0.5)
	})

	return bigPi
}

// TwoPi is 2π as an unevaluated sum of three float64s, each the rounding of
// what the previous ones leave.
var TwoPi = sync.OnceValue(func() [3]float64 {
	Pi()

	var parts [3]float64

	rest := new(big.Float).SetPrec(ReducePrec).Set(big2Pi)

	for i := range parts {
		parts[i], _ = rest.Float64()
		rest.Sub(rest, new(big.Float).SetPrec(ReducePrec).SetFloat64(parts[i]))
	}

	return parts
})

// Reduce is x less the nearest multiple of 2π, exactly (to Prec bits), in
// [−π, π]. The float64 x is taken as the exact rational it is.
func Reduce(x float64) *big.Float {
	Pi()

	bx := new(big.Float).SetPrec(ReducePrec).SetFloat64(x)
	k := new(big.Float).SetPrec(ReducePrec).Quo(bx, big2Pi)

	ki, _ := k.Int(nil)
	// Round to nearest: Int truncates toward zero.
	frac := new(big.Float).SetPrec(ReducePrec).Sub(k, new(big.Float).SetPrec(ReducePrec).SetInt(ki))

	switch {
	case frac.Cmp(new(big.Float).SetFloat64(0.5)) > 0:
		ki.Add(ki, big.NewInt(1))
	case frac.Cmp(new(big.Float).SetFloat64(-0.5)) < 0:
		ki.Sub(ki, big.NewInt(1))
	}

	r := new(big.Float).SetPrec(ReducePrec).Mul(new(big.Float).SetPrec(ReducePrec).SetInt(ki), big2Pi)
	r.Sub(bx, r)

	return new(big.Float).SetPrec(Prec).Set(r)
}

// SinCos is sin x and cos x for |x| <= 4, by their Taylor series.
func SinCos(x *big.Float) (s, c *big.Float) {
	x2 := new(big.Float).SetPrec(Prec+32).Mul(x, x)

	s = new(big.Float).SetPrec(Prec + 32).Set(x)
	c = new(big.Float).SetPrec(Prec + 32).SetInt64(1)

	ts := new(big.Float).SetPrec(Prec + 32).Set(x)
	tc := new(big.Float).SetPrec(Prec + 32).SetInt64(1)

	for k := int64(1); k < 200; k++ {
		ts.Mul(ts, x2)
		ts.Quo(ts, new(big.Float).SetInt64((2*k)*(2*k+1)))
		ts.Neg(ts)

		tc.Mul(tc, x2)
		tc.Quo(tc, new(big.Float).SetInt64((2*k-1)*(2*k)))
		tc.Neg(tc)

		s.Add(s, ts)
		c.Add(c, tc)

		if ts.Sign() == 0 || (ts.MantExp(nil) < s.MantExp(nil)-int(Prec)-16 && tc.MantExp(nil) < -int(Prec)-16) {
			break
		}
	}

	return new(big.Float).SetPrec(Prec).Set(s), new(big.Float).SetPrec(Prec).Set(c)
}

// Residual is E − e·sin E − M and its slope 1 − e·cos E, at Prec.
func Residual(bigE, e, m *big.Float) (f, fp *big.Float) {
	s, c := SinCos(bigE)

	f = new(big.Float).SetPrec(Prec).Mul(e, s)
	f.Sub(bigE, f)
	f.Sub(f, m)

	fp = new(big.Float).SetPrec(Prec).Mul(e, c)
	fp.Sub(newI(1), fp)

	return f, fp
}

// SolveExact is the exact eccentric anomaly for the float64 inputs: the root
// in [−π, π] of E − e·sin E = M, with M the exact reduction of m. M = ±0
// gives E = ±0, which is the root.
func SolveExact(m, e float64) *big.Float {
	if m == 0 {
		return newF(m) // keeps the sign of zero
	}

	bm := Reduce(m)
	be := newF(e)

	neg := bm.Sign() < 0
	if neg {
		bm.Neg(bm)
	}

	// 0 <= E − M = e·sin E <= e on [0, π], so the root is bracketed by
	// [M, min(M + e, π)].
	lo := new(big.Float).SetPrec(Prec).Set(bm)

	hi := new(big.Float).SetPrec(Prec).Add(bm, be)
	if p := new(big.Float).SetPrec(Prec).Set(Pi()); hi.Cmp(p) > 0 {
		hi = p
	}

	bigE := new(big.Float).SetPrec(Prec).Add(lo, hi)
	bigE.Mul(bigE, bigHalf)

	tol := -int(Prec) + 8

	for range 2000 {
		f, fp := Residual(bigE, be, bm)

		if f.Sign() == 0 {
			break
		}

		if f.Sign() > 0 {
			hi.Set(bigE)
		} else {
			lo.Set(bigE)
		}

		next := new(big.Float).SetPrec(Prec).Quo(f, fp)
		next.Sub(bigE, next)

		if next.Cmp(lo) <= 0 || next.Cmp(hi) >= 0 {
			next.Add(lo, hi)
			next.Mul(next, bigHalf)
		}

		step := new(big.Float).SetPrec(Prec).Sub(next, bigE)
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

// StateFromE is the perifocal state at eccentric anomaly bigE, at Prec:
// x = a(cos E − e), y = a√(1−e²)·sin E and their time derivatives with mean
// motion n. Given the float64 nearest the exact root, it is the floor: the
// state a solver returning the correctly rounded E would give if everything
// after it were exact, so no float64 path can do better on average.
func StateFromE(a, e, n float64, bigE *big.Float) (x, y, vx, vy *big.Float) {
	s, c := SinCos(bigE)

	ba, be, bn := newF(a), newF(e), newF(n)

	oneMinusE2 := new(big.Float).SetPrec(Prec).Mul(be, be)
	oneMinusE2.Sub(newI(1), oneMinusE2)

	b := new(big.Float).SetPrec(Prec).Sqrt(oneMinusE2)
	b.Mul(b, ba)

	x = new(big.Float).SetPrec(Prec).Sub(c, be)
	x.Mul(x, ba)

	y = new(big.Float).SetPrec(Prec).Mul(b, s)

	// Ė = n / (1 − e·cos E)
	den := new(big.Float).SetPrec(Prec).Mul(be, c)
	den.Sub(newI(1), den)

	edot := new(big.Float).SetPrec(Prec).Quo(bn, den)

	vx = new(big.Float).SetPrec(Prec).Mul(ba, s)
	vx.Mul(vx, edot)
	vx.Neg(vx)

	vy = new(big.Float).SetPrec(Prec).Mul(b, c)
	vy.Mul(vy, edot)

	return x, y, vx, vy
}

// Sensitivity is |∂(x, y)/∂E| and |∂(vx, vy)/∂E| at the exact root, by a
// central difference at Prec with a step of 2⁻¹⁰⁰ of E: exact to far below
// the float64 quantities it bounds.
func Sensitivity(a, e, n float64, bigE *big.Float) (dPos, dVel float64) {
	h := new(big.Float).SetPrec(Prec).SetMantExp(newF(1), bigE.MantExp(nil)-100)

	hi := new(big.Float).SetPrec(Prec).Add(bigE, h)
	lo := new(big.Float).SetPrec(Prec).Sub(bigE, h)

	x1, y1, vx1, vy1 := StateFromE(a, e, n, hi)
	x0, y0, vx0, vy0 := StateFromE(a, e, n, lo)

	twoH, _ := new(big.Float).SetPrec(Prec).Mul(h, newI(2)).Float64()

	diff := func(p, q *big.Float) float64 {
		d, _ := new(big.Float).SetPrec(Prec).Sub(p, q).Float64()

		return d / twoH
	}

	return math.Hypot(diff(x1, x0), diff(y1, y0)), math.Hypot(diff(vx1, vx0), diff(vy1, vy0))
}

// ErrorOf is a candidate's error against the exact root: relative, in ulps of
// E, and absolute, in radians. The difference is taken modulo 2π, so an
// answer on another branch — [0, 2π) rather than [−π, π] — is judged as the
// angle it is, through the float64 it is: its cos and sin are what a state is
// built from.
func ErrorOf(got float64, want *big.Float) (rel, abs float64) {
	if math.IsNaN(got) || math.IsInf(got, 0) {
		return math.Inf(1), math.Inf(1)
	}

	Pi()

	d := new(big.Float).SetPrec(ReducePrec).SetFloat64(got)
	d.Sub(d, want)

	k := new(big.Float).SetPrec(ReducePrec).Quo(d, big2Pi)
	if kf, _ := k.Float64(); math.Abs(kf) >= 0.5 {
		d.Sub(d, new(big.Float).SetPrec(ReducePrec).Mul(big2Pi, new(big.Float).SetFloat64(math.Round(kf))))
	}

	abs, _ = d.Float64()
	abs = math.Abs(abs)

	if want.Sign() == 0 {
		return 0, abs // M = ±0 is judged by its sign
	}

	d.Quo(d, want)
	rel, _ = d.Float64()

	return math.Abs(rel) / 0x1p-52, abs
}

// AbsErr is |(gx, gy) − (wx, wy)|.
func AbsErr(gx, gy float64, wx, wy *big.Float) float64 {
	dx := new(big.Float).SetPrec(Prec).Sub(newF(gx), wx)
	dy := new(big.Float).SetPrec(Prec).Sub(newF(gy), wy)

	fx, _ := dx.Float64()
	fy, _ := dy.Float64()

	return math.Hypot(fx, fy)
}

// Norm is |(wx, wy)| as a float64.
func Norm(wx, wy *big.Float) float64 {
	fx, _ := wx.Float64()
	fy, _ := wy.Float64()

	return math.Hypot(fx, fy)
}

// Eccentricities spans the elliptic range up to its last float64, densest
// where the equation's conditioning, 1/(1 − e·cos E), is worst.
func Eccentricities() []float64 {
	return []float64{
		0, 1e-3, 0.1, 0.5, 0.7, 0.9, 0.99, 0.999, 0.99999,
		1 - 1e-7, 1 - 1e-10, 1 - 1e-13, math.Nextafter(1, 0),
	}
}

// MeanAnomalies is the sweep: log-spaced from 10⁻²⁰ to π, both signs, the
// float64s on either side of whole revolutions, and phases many revolutions
// out. Under -short it is thinned, not dropped.
func MeanAnomalies(short bool) []float64 {
	perDecade := 12
	if short {
		perDecade = 2
	}

	var ms []float64

	for k := -20 * perDecade; k <= 0; k++ {
		m := math.Pi * math.Pow(10, float64(k)/float64(perDecade))
		ms = append(ms, m, -m)
	}

	for _, rev := range []float64{1, 7, 1e3, 1e6} {
		c := rev * 2 * math.Pi
		ms = append(ms, c, math.Nextafter(c, 0), math.Nextafter(c, math.Inf(1)), c+1e-9, c-1e-9, c+1.5, -c-0.5)
	}

	return append(ms, 0, math.Copysign(0, -1), 1e9, 1e12)
}
