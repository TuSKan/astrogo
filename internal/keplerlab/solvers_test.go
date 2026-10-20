package keplerlab_test

import (
	"math"
	"math/big"
	"sync"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/ephemeris/kepler"
)

// solver is a candidate: the eccentric anomaly for a finite mean anomaly m
// and an eccentricity e in [0, 1). Every candidate but the production one
// answers in [−π, π].
type solver struct {
	name  string
	solve func(m, e float64) float64
}

var solvers = []solver{
	{"production", production},
	{"markley-published", markleyPublished},
	{"enrke", enrke},
	{"markley-stable", markleyStable},
	{"newton-stable", newtonStable},
	{"mikkola-published", mikkolaPublished},
	{"mikkola-stable", mikkolaStable},
}

// ─── Mikkola 1987 ────────────────────────────────────────────────────────────

// mikkolaStart is Mikkola's starting approximation, "A cubic approximation
// for Kepler's equation" (Celest. Mech. 40, 329, 1987), eqs. (6)–(9), for m
// in [−π, π]: Kepler's equation in s = sin(E/3) with arcsin truncated after
// its cubic term, solved by Cardano, the s⁵ correction (7) added, and E
// recovered through (8).
func mikkolaStart(m, e float64) float64 {
	den := 4*e + 0.5
	alpha := (1 - e) / den // (9a)
	beta := 0.5 * m / den  // (9a)

	z := math.Cbrt(beta + math.Copysign(math.Sqrt(beta*beta+alpha*alpha*alpha), beta)) // (9b)
	if z == 0 {                                                                        // e = 1 and M = 0 together
		return m
	}

	s := z - alpha/z                         // (9c)
	s -= 0.078 * s * s * s * s * s / (1 + e) // (7)

	return m + e*(3*s-4*s*s*s) // (8)
}

// mikkolaPublished is Mikkola's method as published: his starter, then
// Burkhart and Danby's correction carried to dx₄ (his eq. 24) — the same
// recurrence as Markley's δ₃ … δ₅ — on the direct residual, with M reduced by
// the float64 2π.
func mikkolaPublished(m, e float64) float64 {
	m = reduceFloat(m)
	bigE := mikkolaStart(m, e)

	s, c := math.Sincos(bigE)

	return bigE + markleyDelta5(bigE-e*s-m, 1-e*c, e*s, e*c)
}

// mikkolaStable is Mikkola's starter with the same stable residual, slope,
// exact reduction and odd symmetry as markleyStable.
func mikkolaStable(m, e float64) float64 {
	m = reduceExact(m)
	if m == 0 {
		return m
	}

	am := math.Abs(m)
	bigE := mikkolaStart(am, e)

	s, c := math.Sincos(bigE)
	f1 := stableSlope(s, c, e)

	bigE += markleyDelta5(stableResidual(bigE, s, am, e), f1, e*s, 1-f1)

	return math.Copysign(bigE, m)
}

// production is kepler.SolveKepler as it ships: Newton from M + e·sin M (±π
// above e = 0.9) on the direct residual, M wrapped to (−π, π], the answer in
// [0, 2π).
func production(m, e float64) float64 {
	ea, err := kepler.SolveKepler(angle.Rad(m), e)
	if err != nil {
		return math.NaN()
	}

	return ea.Radians()
}

// ─── range reduction ─────────────────────────────────────────────────────────

// twoPi is 2π as an unevaluated sum of three float64s, each the rounding of
// what the previous ones leave, taken from the oracle rather than typed in.
var twoPi = sync.OnceValue(func() [3]float64 {
	pi()

	var parts [3]float64

	rest := new(big.Float).SetPrec(reducePrec).Set(big2Pi)

	for i := range parts {
		parts[i], _ = rest.Float64()
		rest.Sub(rest, new(big.Float).SetPrec(reducePrec).SetFloat64(parts[i]))
	}

	return parts
})

// reduceFloat is the usual reduction: math.Remainder by the float64 nearest
// 2π, which is 2.4×10⁻¹⁶ short of it per revolution removed.
func reduceFloat(m float64) float64 { return math.Remainder(m, 2*math.Pi) }

// reduceExact is m less the nearest multiple of 2π to within a few ulps of the
// result, for |m| < 2⁵²: the product k·2π carried exactly through FMA, so the
// remainder keeps its relative precision however close it is to zero.
func reduceExact(m float64) float64 {
	if math.Abs(m) <= math.Pi {
		return m
	}

	c := twoPi()
	k := math.RoundToEven(m / c[0])

	p := k * c[0]
	pe := math.FMA(k, c[0], -p) // k·c0 = p + pe exactly

	r := m - p // exact: p is within a factor of two of m
	r -= pe
	r = math.FMA(-k, c[1], r)
	r = math.FMA(-k, c[2], r)

	return r
}

// ─── shared pieces ───────────────────────────────────────────────────────────

// eMinusSinSeries is E − sin E for |E| <= 1 by its Maclaurin series, with
// coefficients 1/(2k+3)! computed rather than transcribed. It has no
// cancellation: the series' terms are its own.
var eMinusSinCoeffs = func() [9]float64 {
	var c [9]float64

	f := 6.0 // 3!
	for k := range c {
		c[k] = 1 / f
		f *= float64((2*k + 4) * (2*k + 5))
	}

	return c
}()

func eMinusSin(x float64) float64 {
	x2 := x * x

	s := eMinusSinCoeffs[8]
	for k := 7; k >= 0; k-- {
		s = s*(-x2) + eMinusSinCoeffs[k]
	}

	// The alternating signs: c0 − c1·x² + c2·x⁴ − …
	return x * x2 * s
}

// stableResidual is E − e·sin E − M with the cancellation removed where it
// occurs: for e > 1/2 and |E| < 1 it is (1−e)·E + e·(E − sin E) − M, both
// terms of the same sign and E − sin E from its series.
func stableResidual(bigE, sinE, m, e float64) float64 {
	if e > 0.5 && math.Abs(bigE) < 1 {
		return (1-e)*bigE + e*eMinusSin(bigE) - m
	}

	return bigE - e*sinE - m
}

// stableSlope is 1 − e·cos E without its cancellation: 1 − cos E is
// sin²E/(1 + cos E) while cos E > 0, so near perihelion the slope is
// (1−e) + e·sin²E/(1+cos E), two non-negative terms.
func stableSlope(sinE, cosE, e float64) float64 {
	if cosE > 0 {
		return (1 - e) + e*sinE*sinE/(1+cosE)
	}

	return 1 - e*cosE
}

// ─── Markley 1995 ────────────────────────────────────────────────────────────

// markleyStart is Markley's starting formula, eqs. (5)–(15) and (20) of
// "Kepler Equation Solver" (NASA GSFC, 1995; NTRS 19950021346), for m in
// [−π, π]: the root of a cubic built on a Padé approximation to sin E.
func markleyStart(m, e float64) float64 {
	const pi2 = math.Pi * math.Pi

	am := math.Abs(m)
	alpha := (3*pi2 + 1.6*math.Pi*(math.Pi-am)/(1+e)) / (pi2 - 6) // (20)
	d := 3*(1-e) + alpha*e                                        // (5)
	q := 2*alpha*d*(1-e) - m*m                                    // (9)
	r := 3*alpha*d*(d-1+e)*m + m*m*m                              // (10)
	w := math.Cbrt(math.Abs(r) + math.Sqrt(q*q*q+r*r))            // (14), as a cube root
	w *= w                                                        // and its square: Pow(·, 2/3) costs more

	if w == 0 { // e = 1 and M = 0 together, the one singular point
		return m
	}

	return (2*r*w/(w*w+w*q+q*q) + m) / d // (15)
}

// markleyDelta5 is Markley's fifth-order correction, eqs. (22)–(24), given the
// residual f and its derivatives.
func markleyDelta5(f, f1, f2, f3 float64) float64 {
	f4 := -f2                                                 // (28)
	d3 := -f / (f1 - 0.5*f*f2/f1)                             // (22)
	d4 := -f / (f1 + 0.5*d3*f2 + d3*d3*f3/6)                  // (23)
	d5 := -f / (f1 + 0.5*d4*f2 + d4*d4*f3/6 + d4*d4*d4*f4/24) // (24)

	return d5
}

// markleyPublished is Markley's method as its equations stand before his
// "Numerical Considerations": the direct residual (21) and slope (25), on a
// mean anomaly reduced by the float64 2π.
func markleyPublished(m, e float64) float64 {
	m = reduceFloat(m)
	bigE := markleyStart(m, e)

	s, c := math.Sincos(bigE)

	return bigE + markleyDelta5(bigE-e*s-m, 1-e*c, e*s, e*c)
}

// markleyStable is Markley with his own roundoff remedies — the slope as
// (1−e) + 2e·sin²(E/2), eq. (30), here without the extra sine, and the
// residual without cancellation near perihelion, eqs. (31)–(33), here from
// an exact-coefficient series rather than his Padé fit — on an exactly
// reduced mean anomaly, solved on |M| and the sign restored so that the
// answer is odd in M bit for bit.
func markleyStable(m, e float64) float64 {
	m = reduceExact(m)
	if m == 0 {
		return m
	}

	am := math.Abs(m)
	bigE := markleyStart(am, e)

	s, c := math.Sincos(bigE)
	f1 := stableSlope(s, c, e)

	bigE += markleyDelta5(stableResidual(bigE, s, am, e), f1, e*s, 1-f1)

	return math.Copysign(bigE, m)
}

// ─── Tommasini & Olivieri 2022 (ENRKE) ───────────────────────────────────────

// enrke is the ENRKE routine as Tommasini & Olivieri (A&A 658, A196, 2022)
// describe it: their rational seed (19), one Odell–Gooding δ131 step (6),
// Newton with their stopping rule (9) at 3×10⁻¹⁵ rad, and bisection in the
// critical region e > 0.99, M < 0.0045 rad (23)–(24), there carried to the
// last representable midpoint. M is folded into [0, π] by the symmetries (2).
func enrke(m, e float64) float64 {
	const (
		tol     = 3e-15
		bSeed   = 0.999999
		eSwitch = 0.99
		mSwitch = 0.0045
	)

	m = reduceFloat(m)
	sign := 1.0

	if m < 0 {
		m, sign = -m, -1
	}

	if e > eSwitch && m < mSwitch {
		lo, hi := m, math.Min(m+e, math.Pi)

		for range 200 {
			mid := lo + (hi-lo)/2
			if mid <= lo || mid >= hi {
				break
			}

			if mid-e*math.Sin(mid)-m > 0 {
				hi = mid
			} else {
				lo = mid
			}
		}

		return sign * (lo + (hi-lo)/2)
	}

	bigE := m + bSeed*4*e*m*(math.Pi-m)/(8*e*m+4*e*(e-math.Pi)+math.Pi*math.Pi) // (19)

	// One δ131 step, eq. (6).
	s, c := math.Sincos(bigE)
	f, f1, f2, f3 := bigE-e*s-m, 1-e*c, e*s, e*c
	num := f1*f1*f1 - 0.5*f*f1*f2 + f*f*f3/3
	den := f1*f1*f1 - f*f1*f2 + 0.5*f*f*f3
	bigE -= f / f1 * num / den

	for range 50 {
		s, c = math.Sincos(bigE)
		f1 = 1 - e*c
		delta := -(bigE - e*s - m) / f1
		bigE += delta

		if delta*delta < 2*f1*tol/(e+0x1p-52) { // (9)
			break
		}
	}

	return sign * bigE
}

// ─── proposed ────────────────────────────────────────────────────────────────

// newtonStable is the proposal: Markley's starting formula, then Newton on
// the stable residual and slope, held inside the bracket the equation proves,
// [M, min(M+e, π)] for M in [0, π], stopping once a step is within two ulps
// of E — on an exactly reduced M, solved on |M| with the sign restored.
func newtonStable(m, e float64) float64 {
	m = reduceExact(m)
	if m == 0 {
		return m
	}

	am := math.Abs(m)
	lo, hi := am, math.Min(am+e, math.Pi)

	bigE := markleyStart(am, e)
	if !(bigE > lo && bigE < hi) {
		bigE = lo + (hi-lo)/2
	}

	for range 64 {
		s, c := math.Sincos(bigE)
		f := stableResidual(bigE, s, am, e)

		if f > 0 {
			hi = bigE
		} else {
			lo = bigE
		}

		next := bigE - f/stableSlope(s, c, e)
		if !(next >= lo && next <= hi) {
			next = lo + (hi-lo)/2
		}

		done := math.Abs(next-bigE) <= 0x1p-51*next
		bigE = next

		if done {
			break
		}
	}

	return math.Copysign(bigE, m)
}
