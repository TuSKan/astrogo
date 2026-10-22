package keplerlab

import (
	"errors"
	"math"
)

// errStateNoConverge is StatePropagate's failure to find χ.
var errStateNoConverge = errors.New("keplerlab: universal anomaly from a state did not converge")

// StatePropagate is the float64 candidate for propagating a state vector by
// universal variables, measured against [StateExact]: χ by Newton's method
// inside a bracket grown from below, then
//
//	f = 1 − χ²·c₂/r₀              g = (r₀·χ·c₁ + σ₀·χ²·c₂)/√μ
//	ḟ = −√μ·χ·c₁/(r·r₀)           ġ = (r₀·c₀ + σ₀·χ·c₁)/r
//
// g and ġ in the forms that do not subtract: Δt − χ³·c₃/√μ and 1 − χ²·c₂/r
// are the same quantities, and they cancel once r ≫ r₀, which is what cost
// the perihelion path 317 ulps before #842.
func StatePropagate(r0, v0 [3]float64, mu, dt float64) (r, v [3]float64, chi float64, err error) {
	rn := math.Sqrt(r0[0]*r0[0] + r0[1]*r0[1] + r0[2]*r0[2])
	sqrtMu := math.Sqrt(mu)
	sigma := (r0[0]*v0[0] + r0[1]*v0[1] + r0[2]*v0[2]) / sqrtMu
	alpha := 2/rn - (v0[0]*v0[0]+v0[1]*v0[1]+v0[2]*v0[2])/mu

	chi, err = stateChi64(rn, sigma, alpha, sqrtMu*dt)
	if err != nil {
		return r, v, 0, err
	}

	c0, c1, c2, _ := stumpff64(alpha * chi * chi)

	chi2c2 := chi * chi * c2
	rr := rn*c0 + sigma*chi*c1 + chi2c2

	f := 1 - chi2c2/rn
	g := (rn*chi*c1 + sigma*chi2c2) / sqrtMu
	fdot := -sqrtMu * chi * c1 / (rr * rn)
	gdot := (rn*c0 + sigma*chi*c1) / rr

	for i := range 3 {
		r[i] = f*r0[i] + g*v0[i]
		v[i] = fdot*r0[i] + gdot*v0[i]
	}

	return r, v, chi, nil
}

// StatePropagateViaPerihelion is the second candidate, for a hyperbola: the
// state's own place on the orbit measured from perihelion, and the perihelion
// path from there.
//
//   - e⃗ = (v²/μ − 1/r₀)·r₀ − (r₀·v₀/μ)·v₀, and P = e⃗/e, Q = h⃗×P/|h⃗|.
//   - The state's χ₀ from perihelion in closed form, sinh H₀ = σ₀·√(−α)/e,
//     χ₀ = H₀/√(−α), with nothing subtracted.
//   - Its time from perihelion from the perihelion equation's left side,
//     √μ·τ = q·χ₀ + e·χ₀³·c₃, whose terms share χ₀'s sign.
//   - The perihelion path to τ + Δt, as ephemeris/kepler builds it since
//     #842, turned into place by P and Q.
//
// The direct path's equation, r₀·χ·c₁ + σ₀·χ²·c₂ + χ³·c₃, has terms that grow
// like e^√|z| and cancel when a hyperbola's step crosses perihelion from far
// out: 528 ulps of χ from 104 au. From perihelion σ₀ = 0 and nothing cancels.
//
// Measured against [StateSensitivity] on hyperbolas from e = 1.00001 to 5,
// started from perihelion out to 104 au: within 6.1 of the input's own
// error, where the direct path reaches 463. With the textbook eccentricity
// vector in place of v⃗×h⃗/μ − r̂ it reached 32.6.
func StatePropagateViaPerihelion(r0, v0 [3]float64, mu, dt float64) (r, v [3]float64, err error) {
	rn := math.Sqrt(r0[0]*r0[0] + r0[1]*r0[1] + r0[2]*r0[2])
	rv := r0[0]*v0[0] + r0[1]*v0[1] + r0[2]*v0[2]
	sqrtMu := math.Sqrt(mu)
	alpha := 2/rn - (v0[0]*v0[0]+v0[1]*v0[1]+v0[2]*v0[2])/mu

	h := [3]float64{r0[1]*v0[2] - r0[2]*v0[1], r0[2]*v0[0] - r0[0]*v0[2], r0[0]*v0[1] - r0[1]*v0[0]}
	hn := math.Sqrt(h[0]*h[0] + h[1]*h[1] + h[2]*h[2])

	// e⃗ = v⃗×h⃗/μ − r̂: far out on a hyperbola its two terms are about e + 1
	// and 1. The textbook (v²/μ − 1/r)·r⃗ − (r⃗·v⃗/μ)·v⃗ is the same vector
	// as the difference of two terms near v²·r/μ, 418 at 104 au for e = 5.
	vxh := [3]float64{v0[1]*h[2] - v0[2]*h[1], v0[2]*h[0] - v0[0]*h[2], v0[0]*h[1] - v0[1]*h[0]}

	var ev [3]float64

	for i := range 3 {
		ev[i] = vxh[i]/mu - r0[i]/rn
	}

	e := math.Sqrt(ev[0]*ev[0] + ev[1]*ev[1] + ev[2]*ev[2])
	q := hn * hn / (mu * (1 + e))

	var p, qv [3]float64
	for i := range 3 {
		p[i] = ev[i] / e
	}

	qv = [3]float64{(h[1]*p[2] - h[2]*p[1]) / hn, (h[2]*p[0] - h[0]*p[2]) / hn, (h[0]*p[1] - h[1]*p[0]) / hn}

	sqrtNegA := math.Sqrt(-alpha)
	chi0 := math.Asinh(rv/sqrtMu*sqrtNegA/e) / sqrtNegA

	_, _, _, c3 := stumpff64(alpha * chi0 * chi0)
	tau := (q*chi0 + e*chi0*chi0*chi0*c3) / sqrtMu

	chi, err := stateChi64(q, 0, alpha, sqrtMu*(tau+dt))
	if err != nil {
		return r, v, err
	}

	c0, c1, c2, _ := stumpff64(alpha * chi * chi)

	vp := math.Sqrt(mu * (1 + e) / q)
	rr := q + e*chi*chi*c2

	x, y := q-chi*chi*c2, vp*q*chi*c1/sqrtMu
	vx, vy := -sqrtMu*chi*c1/rr, vp*q*c0/rr

	for i := range 3 {
		r[i] = x*p[i] + y*qv[i]
		v[i] = vx*p[i] + vy*qv[i]
	}

	return r, v, nil
}

// stateChi64 solves r₀·χ·c₁ + σ₀·χ²·c₂ + χ³·c₃ = target for χ.
func stateChi64(rn, sigma, alpha, target float64) (float64, error) {
	if target == 0 {
		return 0, nil
	}

	eval := func(chi float64) (f, fp float64) {
		c0, c1, c2, c3 := stumpff64(alpha * chi * chi)
		chi2 := chi * chi

		return rn*chi*c1 + sigma*chi2*c2 + chi2*chi*c3 - target, rn*c0 + sigma*chi*c1 + chi2*c2
	}

	start := math.Abs(target / rn)
	if alpha != 0 {
		start = math.Min(start, 1/math.Sqrt(math.Abs(alpha)))
	}

	edge, near := math.Copysign(start, target), 0.0

	for range 2000 {
		f, _ := eval(edge)
		if f == 0 || (f > 0) == (target > 0) {
			break
		}

		near, edge = edge, 2*edge
	}

	lo, hi := near, edge
	if target < 0 {
		lo, hi = edge, near
	}

	chi := lo + (hi-lo)/2
	width := hi - lo

	for range 400 {
		f, fp := eval(chi)
		if f == 0 {
			return chi, nil
		}

		if f < 0 {
			lo = chi
		} else {
			hi = chi
		}

		step := f / fp
		next := chi - step

		// A converged step is taken whatever the bracket says, as in the
		// perihelion path's solver: at the root the iterate is one end of
		// the bracket, and a step of a few ulps cannot land inside it.
		if math.Abs(step) <= 1e-13*math.Max(1, math.Abs(chi)) {
			return next, nil
		}

		if !(next > lo && next < hi) || hi-lo > width/2 {
			next = lo + (hi-lo)/2
		}

		width = hi - lo
		chi = next
	}

	return 0, errStateNoConverge
}

// stumpff64 is ephemeris/kepler's stumpff as #842 shipped it: the series
// below |z| = 4, the closed forms with the half-angle c₂ above.
func stumpff64(z float64) (c0, c1, c2, c3 float64) {
	if math.Abs(z) < 4 {
		t0, t1, t2, t3 := 1.0, 1.0, 0.5, 1.0/6
		c0, c1, c2, c3 = t0, t1, t2, t3

		for j := 1; j < 30; j++ {
			fj := float64(j)
			t0 *= -z / ((2*fj - 1) * (2 * fj))
			t1 *= -z / ((2 * fj) * (2*fj + 1))
			t2 *= -z / ((2*fj + 1) * (2*fj + 2))
			t3 *= -z / ((2*fj + 2) * (2*fj + 3))
			c0, c1, c2, c3 = c0+t0, c1+t1, c2+t2, c3+t3

			if math.Abs(t0) < 0x1p-60*math.Abs(c0) && math.Abs(t3) < 0x1p-60*c3 {
				break
			}
		}

		return c0, c1, c2, c3
	}

	if z > 0 {
		x := math.Sqrt(z)
		s, c := math.Sincos(x)
		h := math.Sin(x / 2)

		return c, s / x, 2 * h * h / (x * x), (x - s) / (x * x * x)
	}

	x := math.Sqrt(-z)
	h := math.Sinh(x / 2)
	sh := math.Sinh(x)

	return math.Cosh(x), sh / x, 2 * h * h / (x * x), (sh - x) / (x * x * x)
}
