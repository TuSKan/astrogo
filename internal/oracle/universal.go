package oracle

import (
	"math"
	"math/big"
)

// stumpffGuard is the precision the Stumpff series are summed at beyond Prec:
// an alternating series for z > 0 loses about log₂ of its largest term, eight
// bits at z = 4π², and a hyperbolic one loses nothing.
const stumpffGuard = 64

// Stumpff is Danby's c₀ … c₃ at z, at Prec, by their Maclaurin series
//
//	c_k(z) = Σ_j (−z)^j / (2j + k)!
//
// so c₀ = cos √z, c₁ = sin √z / √z, c₂ = (1 − cos √z)/z and
// c₃ = (√z − sin √z)/z^{3/2} for z > 0, and the hyperbolic functions of
// √−z for z < 0, with no closed form and so no cancellation anywhere: the
// float64 z is taken as the exact rational it is.
func Stumpff(z float64) [4]*big.Float { return stumpffAt(newF(z)) }

// stumpffAt is [Stumpff] at a z held at Prec.
func stumpffAt(bz *big.Float) [4]*big.Float {
	w := uint(Prec + stumpffGuard)
	negZ := new(big.Float).SetPrec(w).Neg(bz)
	zf, _ := bz.Float64()

	var out [4]*big.Float

	for k := range out {
		// term_0 = 1/k!, term_{j+1} = term_j·(−z)/((2j+k+1)(2j+k+2)).
		term := new(big.Float).SetPrec(w).SetInt64(1)
		for i := int64(2); i <= int64(k); i++ {
			term.Quo(term, new(big.Float).SetPrec(w).SetInt64(i))
		}

		sum := new(big.Float).SetPrec(w).Set(term)

		for j := range int64(4000) {
			term.Mul(term, negZ)
			term.Quo(term, new(big.Float).SetPrec(w).SetInt64((2*j+int64(k)+1)*(2*j+int64(k)+2)))
			sum.Add(sum, term)

			// The terms fall factorially once 2j passes √|z|; stop when one
			// is below the sum's last bit.
			if term.Sign() == 0 || (float64(2*j) > math.Sqrt(math.Abs(zf)) && term.MantExp(nil) < sum.MantExp(nil)-int(w)) {
				break
			}
		}

		out[k] = new(big.Float).SetPrec(Prec).Set(sum)
	}

	return out
}

// UniversalExact is the universal anomaly χ at Δt = dt from perihelion, at
// Prec: the root of
//
//	q·χ + e·χ³·c₃(αχ²) = √μ·Δt,   α = (1 − e)/q,
//
// with q, e, μ and dt the exact rationals their float64s are. The left side
// is odd in χ and, for χ > 0 inside a closed orbit's first half period or on
// any open one, convex, so Newton's method from a lower bound overshoots once
// and then descends; a bracket keeps it honest.
func UniversalExact(q, e, mu, dt float64) *big.Float {
	if dt == 0 {
		return newF(dt)
	}

	bq, be := newF(q), newF(e)
	alpha := new(big.Float).SetPrec(Prec).Sub(newI(1), be)
	alpha.Quo(alpha, bq)

	target := new(big.Float).SetPrec(Prec).Sqrt(newF(mu))
	target.Mul(target, newF(math.Abs(dt)))

	tf, _ := target.Float64()
	af, _ := alpha.Float64()

	// The float64 lower bounds production starts from: √a·M for an ellipse,
	// √(−a)·asinh(M/e) for a hyperbola.
	guess := tf / q

	switch {
	case af > 0:
		guess = tf * af
	case af < 0:
		sqrtNegA := math.Sqrt(-1 / af)
		guess = sqrtNegA * math.Asinh(tf*(-af)/sqrtNegA/e)
	}

	lo := newF(0)
	hi := new(big.Float).SetPrec(Prec).Quo(target, bq)

	chi := newF(guess)
	if chi.Cmp(lo) <= 0 || chi.Cmp(hi) >= 0 {
		chi.Add(lo, hi)
		chi.Mul(chi, bigHalfPrec())
	}

	for range 4000 {
		z := new(big.Float).SetPrec(Prec).Mul(chi, chi)
		z.Mul(z, alpha)
		c := stumpffAt(z)

		chi2 := new(big.Float).SetPrec(Prec).Mul(chi, chi)
		chi3 := new(big.Float).SetPrec(Prec).Mul(chi2, chi)

		f := new(big.Float).SetPrec(Prec).Mul(chi3, c[3])
		f.Mul(f, be)
		f.Add(f, new(big.Float).SetPrec(Prec).Mul(bq, chi))
		f.Sub(f, target)

		fp := new(big.Float).SetPrec(Prec).Mul(chi2, c[2])
		fp.Mul(fp, be)
		fp.Add(fp, bq)

		if f.Sign() == 0 {
			break
		}

		if f.Sign() < 0 {
			lo.Set(chi)
		} else {
			hi.Set(chi)
		}

		next := new(big.Float).SetPrec(Prec).Quo(f, fp)
		next.Sub(chi, next)

		if next.Cmp(lo) <= 0 || next.Cmp(hi) >= 0 {
			next.Add(lo, hi)
			next.Mul(next, bigHalfPrec())
		}

		step := new(big.Float).SetPrec(Prec).Sub(next, chi)
		chi = next

		if step.Sign() == 0 || step.MantExp(nil) < chi.MantExp(nil)-int(Prec)+8 {
			break
		}
	}

	if dt < 0 {
		chi.Neg(chi)
	}

	return chi
}

// UniversalStateExact is the perifocal state at dt from perihelion, at Prec:
// χ from [UniversalExact], then, with z = αχ² and r = q + e·χ²·c₂,
//
//	x  = q − χ²·c₂             vx = −√μ·χ·c₁ / r
//	y  = v₀·q·χ·c₁ / √μ        vy = v₀·q·c₀ / r
//
// v₀ = √(μ(1 + e)/q) being the speed at perihelion: the f and g functions
// with the identities 1 − z·c₃ = c₁ and 1 − z·c₂ = c₀ applied, which at Prec
// changes nothing and makes the oracle independent of the forms a float64
// path chooses.
func UniversalStateExact(q, e, mu, dt float64) (x, y, vx, vy *big.Float) {
	return UniversalStateFromChi(q, e, mu, UniversalExact(q, e, mu, dt))
}

// UniversalStateFromChi is [UniversalStateExact]'s state at a given χ, at
// Prec. Given the float64 nearest the exact χ, it is the floor: what a path
// returning the correctly rounded χ would give if everything after it were
// exact.
func UniversalStateFromChi(q, e, mu float64, chi *big.Float) (x, y, vx, vy *big.Float) {
	bq, be := newF(q), newF(e)
	alpha := new(big.Float).SetPrec(Prec).Sub(newI(1), be)
	alpha.Quo(alpha, bq)

	z := new(big.Float).SetPrec(Prec).Mul(chi, chi)
	z.Mul(z, alpha)
	c := stumpffAt(z)

	sqrtMu := new(big.Float).SetPrec(Prec).Sqrt(newF(mu))

	v0 := new(big.Float).SetPrec(Prec).Add(newI(1), be)
	v0.Mul(v0, newF(mu))
	v0.Quo(v0, bq)
	v0.Sqrt(v0)

	chi2 := new(big.Float).SetPrec(Prec).Mul(chi, chi)

	r := new(big.Float).SetPrec(Prec).Mul(chi2, c[2])
	r.Mul(r, be)
	r.Add(r, bq)

	x = new(big.Float).SetPrec(Prec).Mul(chi2, c[2])
	x.Sub(bq, x)

	y = new(big.Float).SetPrec(Prec).Mul(v0, bq)
	y.Mul(y, chi)
	y.Mul(y, c[1])
	y.Quo(y, sqrtMu)

	vx = new(big.Float).SetPrec(Prec).Mul(sqrtMu, chi)
	vx.Mul(vx, c[1])
	vx.Quo(vx, r)
	vx.Neg(vx)

	vy = new(big.Float).SetPrec(Prec).Mul(v0, bq)
	vy.Mul(vy, c[0])
	vy.Quo(vy, r)

	return x, y, vx, vy
}

func bigHalfPrec() *big.Float { return new(big.Float).SetPrec(Prec).SetFloat64(0.5) }
