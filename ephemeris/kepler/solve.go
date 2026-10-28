package kepler

import "math"

// The pieces of SolveKepler: Markley's starter and fifth-order correction
// (Markley 1995, "Kepler Equation Solver", NASA GSFC, NTRS 19950021346), on
// a mean anomaly reduced by 2π exactly, with the residual and slope written
// so that nothing cancels near perihelion as e → 1. docs/kepler.md measures
// it against the alternatives and against a 256-bit oracle.

// twoPi is 2π as an unevaluated sum of three float64s, each the rounding of
// what the earlier ones leave. TestTwoPiIsThreeExactParts holds it to 2π
// from internal/oracle.
var twoPi = [3]float64{0x1.921fb54442d18p+02, 0x1.1a62633145c07p-52, -0x1.f1976b7ed8fbcp-108}

// reduceExact is m less the nearest multiple of 2π, for |m| < 2⁵²: the
// product k·2π carried exactly through fused multiply-adds, so the result
// keeps its relative precision however close it is to a whole revolution.
func reduceExact(m float64) float64 {
	if math.Abs(m) <= math.Pi {
		return m
	}

	k := math.RoundToEven(m / twoPi[0])

	// The conversion is load-bearing. Go may fuse a product into a later
	// addition, across statements, and arm64 does: unconverted, k·twoPi[0]
	// was folded into m − p as one fused operation, so pe below was taken
	// from a product that had never been rounded, and every phase past a
	// revolution came out wrong on macOS alone (#814). A conversion rounds,
	// and the spec forbids fusing across it.
	p := float64(k * twoPi[0])
	pe := math.FMA(k, twoPi[0], -p) // k·twoPi[0] = p + pe exactly

	r := m - p // exact: p is within a factor of two of m
	r -= pe
	r = math.FMA(-k, twoPi[1], r)

	return math.FMA(-k, twoPi[2], r)
}

// eMinusSinCoeffs are 1/(2k+3)!, the coefficients of E − sin E's Maclaurin
// series, computed rather than transcribed.
var eMinusSinCoeffs = func() [9]float64 {
	var c [9]float64

	f := 6.0 // 3!
	for k := range c {
		c[k] = 1 / f
		f *= float64((2*k + 4) * (2*k + 5))
	}

	return c
}()

// eMinusSin is E − sin E for |E| < 1 by its series, which has no
// cancellation: its terms are its own.
func eMinusSin(x float64) float64 {
	x2 := x * x

	s := eMinusSinCoeffs[8]
	for k := 7; k >= 0; k-- {
		s = s*(-x2) + eMinusSinCoeffs[k]
	}

	return x * x2 * s
}

// stableResidual is E − e·sin E − M with the cancellation removed where it
// occurs: for e > 1/2 and |E| < 1 it is (1−e)·E + e·(E − sin E) − M, both
// terms of M's sign, so its rounding is relative to M rather than to E.
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

// markleyStart is Markley's starting formula, eqs. (5)–(15) and (20), for m
// in [0, π]: the root of a cubic built on a Padé approximation to sin E.
func markleyStart(m, e float64) float64 {
	const pi2 = math.Pi * math.Pi

	alpha := (3*pi2 + 1.6*math.Pi*(math.Pi-m)/(1+e)) / (pi2 - 6) // (20)
	d := 3*(1-e) + alpha*e                                       // (5)
	q := 2*alpha*d*(1-e) - m*m                                   // (9)
	r := 3*alpha*d*(d-1+e)*m + m*m*m                             // (10)
	w := math.Cbrt(math.Abs(r) + math.Sqrt(q*q*q+r*r))           // (14), as a cube root
	w *= w                                                       // and its square: Pow(·, 2/3) costs more

	if w == 0 { // e = 1 and M = 0 together, the one singular point
		return m
	}

	return (2*r*w/(w*w+w*q+q*q) + m) / d // (15)
}

// markleyDelta5 is Markley's fifth-order correction, eqs. (22)–(24), given the
// residual f and its first three derivatives; the fourth is −f2, eq. (28).
func markleyDelta5(f, f1, f2, f3 float64) float64 {
	f4 := -f2
	d3 := -f / (f1 - 0.5*f*f2/f1)                             // (22)
	d4 := -f / (f1 + 0.5*d3*f2 + d3*d3*f3/6)                  // (23)
	d5 := -f / (f1 + 0.5*d4*f2 + d4*d4*f3/6 + d4*d4*d4*f4/24) // (24)

	return d5
}
