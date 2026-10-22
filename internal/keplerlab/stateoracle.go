package keplerlab

import (
	"math"
	"math/big"
)

// StateExact is two-body motion of the state (r0, v0) about μ over Δt = dt,
// at Prec: the exact answer a float64 propagation from a state vector is held
// to. Units are the caller's, consistently: a length, a length per time unit,
// μ in length³ per time unit², dt in the time unit.
//
// It solves for the universal anomaly χ in
//
//	r₀·χ·c₁(z) + σ₀·χ²·c₂(z) + χ³·c₃(z) = √μ·Δt,
//	z = α·χ²,  α = 2/r₀ − v₀²/μ,  σ₀ = r₀·v₀/√μ,
//
// whose left side has derivative r = r₀·c₀ + σ₀·χ·c₁ + χ²·c₂ > 0, so the root
// is unique and a bracket around it never loses it. At σ₀ = 0 and r₀ = q it is
// [UniversalExact]'s equation, since r₀·c₁ = q − q·z·c₃ and q·α = 1 − e.
//
// The state follows from the f and g functions,
//
//	f  = 1 − χ²·c₂/r₀            g  = (r₀·χ·c₁ + σ₀·χ²·c₂)/√μ
//	ḟ  = −√μ·χ·c₁/(r·r₀)          ġ  = 1 − χ²·c₂/r
//
// with r = f·r₀ + g·v₀ and v = ḟ·r₀ + ġ·v₀. g is written without Δt so that
// the oracle shares no form with the float64 paths it checks; at Prec the
// choice changes nothing.
//
// Like [UniversalExact], the Stumpff series it sums lose about √z·log₂e bits
// to their alternation, which stumpffGuard covers within a half revolution
// (√z ≤ π) and not across many. Hold an ellipse's Δt inside half a period.
func StateExact(r0, v0 [3]float64, mu, dt float64) (r, v [3]*big.Float) {
	br, bv := vecF(r0), vecF(v0)

	chi := stateChi(br, bv, mu, dt)

	return stateFromChi(br, bv, mu, chi)
}

// StateChiExact is the χ [StateExact] solves for, at Prec.
func StateChiExact(r0, v0 [3]float64, mu, dt float64) *big.Float {
	return stateChi(vecF(r0), vecF(v0), mu, dt)
}

// StateFromChiExact is [StateExact]'s state at a given χ, at Prec. Given the
// float64 nearest the exact χ it is the floor: what a path returning the
// correctly rounded χ would give if everything after it were exact.
func StateFromChiExact(r0, v0 [3]float64, mu float64, chi *big.Float) (r, v [3]*big.Float) {
	return stateFromChi(vecF(r0), vecF(v0), mu, chi)
}

// stateTerms are what both the root and the state need from (r₀, v₀, μ).
type stateTerms struct {
	rn, sigma, alpha, sqrtMu *big.Float
}

func termsOf(br, bv [3]*big.Float, mu float64) stateTerms {
	rn := norm(br)

	sqrtMu := new(big.Float).SetPrec(Prec).Sqrt(newF(mu))

	sigma := dot(br, bv)
	sigma.Quo(sigma, sqrtMu)

	// α = 2/r₀ − v₀²/μ.
	alpha := new(big.Float).SetPrec(Prec).Quo(newI(2), rn)
	alpha.Sub(alpha, new(big.Float).SetPrec(Prec).Quo(dot(bv, bv), newF(mu)))

	return stateTerms{rn: rn, sigma: sigma, alpha: alpha, sqrtMu: sqrtMu}
}

// stateChi solves the universal Kepler equation from a state by Newton's
// method inside a bracket that is widened, by doubling, until it holds the
// root.
func stateChi(br, bv [3]*big.Float, mu, dt float64) *big.Float {
	if dt == 0 {
		return newF(0)
	}

	k := termsOf(br, bv, mu)

	target := new(big.Float).SetPrec(Prec).Mul(k.sqrtMu, newF(dt))

	// F(χ) − √μ·Δt and its derivative r.
	eval := func(chi *big.Float) (f, fp *big.Float) {
		z := new(big.Float).SetPrec(Prec).Mul(chi, chi)
		z.Mul(z, k.alpha)
		c := stumpffAt(z)

		chi2 := new(big.Float).SetPrec(Prec).Mul(chi, chi)

		f = new(big.Float).SetPrec(Prec).Mul(k.rn, chi)
		f.Mul(f, c[1])
		f.Add(f, mul(k.sigma, chi2, c[2]))
		f.Add(f, mul(chi2, chi, c[3]))
		f.Sub(f, target)

		fp = new(big.Float).SetPrec(Prec).Mul(k.rn, c[0])
		fp.Add(fp, mul(k.sigma, chi, c[1]))
		fp.Add(fp, mul(chi2, c[2]))

		return f, fp
	}

	// F is increasing, so F(0) = −√μ·Δt has the sign opposite to Δt's, and
	// the root lies on Δt's side of zero. The bracket's far edge grows from
	// below, by doubling, from a χ at which √|z| is at most 1: no evaluation
	// is ever past twice the root, where the series are summed well. Started
	// at the first-order √μ·Δt/r₀ instead, it put a hyperbola's edge at
	// √|z| = 5.6e5, where the series' term cap leaves them nowhere near
	// converged, and Newton's method crawled from there.
	tf, _ := target.Float64()
	rf, _ := k.rn.Float64()
	af, _ := k.alpha.Float64()

	start := math.Abs(tf / rf)
	if af != 0 {
		start = math.Min(start, 1/math.Sqrt(math.Abs(af)))
	}

	edge := newF(math.Copysign(start, dt))
	near := newF(0)

	for {
		f, _ := eval(edge)
		if f.Sign() == 0 || (f.Sign() > 0) == (dt > 0) {
			break
		}

		near.Set(edge)
		edge.Mul(edge, newI(2))
	}

	lo, hi := near, edge
	if dt < 0 {
		lo, hi = edge, near
	}

	// Newton's method inside the bracket, falling back to bisection whenever
	// a step would leave it or fail to halve it: rtsafe's safeguard, which
	// bounds the iterations by the bracket's bits whatever F looks like.
	chi := new(big.Float).SetPrec(Prec).Add(lo, hi)
	chi.Mul(chi, bigHalfPrec())

	width := new(big.Float).SetPrec(Prec).Sub(hi, lo)

	for range 4000 {
		f, fp := eval(chi)
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

		newWidth := new(big.Float).SetPrec(Prec).Sub(hi, lo)
		halved := new(big.Float).SetPrec(Prec).Mul(width, bigHalfPrec())

		if next.Cmp(lo) <= 0 || next.Cmp(hi) >= 0 || newWidth.Cmp(halved) > 0 {
			next.Add(lo, hi)
			next.Mul(next, bigHalfPrec())
		}

		width = newWidth

		step := new(big.Float).SetPrec(Prec).Sub(next, chi)
		chi = next

		if step.Sign() == 0 || step.MantExp(nil) < chi.MantExp(nil)-int(Prec)+8 {
			break
		}
	}

	return chi
}

func stateFromChi(br, bv [3]*big.Float, mu float64, chi *big.Float) (r, v [3]*big.Float) {
	k := termsOf(br, bv, mu)

	z := new(big.Float).SetPrec(Prec).Mul(chi, chi)
	z.Mul(z, k.alpha)
	c := stumpffAt(z)

	chi2 := new(big.Float).SetPrec(Prec).Mul(chi, chi)
	chi2c2 := mul(chi2, c[2])

	rn := new(big.Float).SetPrec(Prec).Mul(k.rn, c[0])
	rn.Add(rn, mul(k.sigma, chi, c[1]))
	rn.Add(rn, chi2c2)

	f := new(big.Float).SetPrec(Prec).Quo(chi2c2, k.rn)
	f.Sub(newI(1), f)

	g := mul(k.rn, chi, c[1])
	g.Add(g, mul(k.sigma, chi2c2))
	g.Quo(g, k.sqrtMu)

	fdot := mul(k.sqrtMu, chi, c[1])
	fdot.Quo(fdot, rn)
	fdot.Quo(fdot, k.rn)
	fdot.Neg(fdot)

	gdot := new(big.Float).SetPrec(Prec).Quo(chi2c2, rn)
	gdot.Sub(newI(1), gdot)

	for i := range 3 {
		r[i] = mul(f, br[i])
		r[i].Add(r[i], mul(g, bv[i]))

		v[i] = mul(fdot, br[i])
		v[i].Add(v[i], mul(gdot, bv[i]))
	}

	return r, v
}

func vecF(a [3]float64) [3]*big.Float { return [3]*big.Float{newF(a[0]), newF(a[1]), newF(a[2])} }

func dot(a, b [3]*big.Float) *big.Float {
	s := new(big.Float).SetPrec(Prec)
	for i := range 3 {
		s.Add(s, mul(a[i], b[i]))
	}

	return s
}

func norm(a [3]*big.Float) *big.Float { return new(big.Float).SetPrec(Prec).Sqrt(dot(a, a)) }

// mul is the product of its arguments, at Prec.
func mul(xs ...*big.Float) *big.Float {
	p := new(big.Float).SetPrec(Prec).SetInt64(1)
	for _, x := range xs {
		p.Mul(p, x)
	}

	return p
}
