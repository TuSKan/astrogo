package oracle

import (
	"math"
	"math/big"
	"testing"
)

// relErr is |a − b|/|b| at the oracle's precision, as a float64.
func relErr(a, b *big.Float) float64 {
	d := new(big.Float).SetPrec(Prec).Sub(a, b)
	if b.Sign() == 0 {
		f, _ := d.Float64()

		return math.Abs(f)
	}

	d.Quo(d, b)
	f, _ := d.Float64()

	return math.Abs(f)
}

// TestStateOracleIsThePerihelionOracleAtPerihelion: from a perihelion state,
// σ₀ = 0 and r₀ = q, and the state oracle's equation is the perihelion
// oracle's, so the two give one state to their working precision. The two
// solve different equations by different code, so agreement checks both.
//
// The inputs are chosen so that e = v₀²·q/μ − 1 is exact in float64, which
// makes the two oracles' inputs the same exact orbit. With a rounded v₀ they
// would differ by that rounding, 1e-15, and hide everything below it.
func TestStateOracleIsThePerihelionOracleAtPerihelion(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		q, mu, v0, dt float64
	}{
		{1, 1, 1, 3},           // e = 0, a circle, under half its period
		{1, 1, 1.25, -10},      // e = 0.5625
		{1, 1, 1.375, 80},      // e = 0.890625
		{1, 2, 2, 400},         // e = 1, a parabola
		{1, 1, 1.5, -1000},     // e = 1.25
		{2, 1, 1.5, 1_000_000}, // e = 3.5
	} {
		e := tc.v0*tc.v0*tc.q/tc.mu - 1

		r, v := StateExact([3]float64{tc.q, 0, 0}, [3]float64{0, tc.v0, 0}, tc.mu, tc.dt)
		x, y, vx, vy := UniversalStateExact(tc.q, e, tc.mu, tc.dt)

		// 2^−256 is 1e-77; the series and the root give up some bits.
		const tol = 1e-60

		for _, c := range []struct {
			name      string
			got, want *big.Float
		}{
			{"x", r[0], x}, {"y", r[1], y}, {"vx", v[0], vx}, {"vy", v[1], vy},
		} {
			if err := relErr(c.got, c.want); err > tol {
				t.Errorf("e %g, Δt %g: %s differs by %.3g relative", e, tc.dt, c.name, err)
			}
		}

		if r[2].Sign() != 0 || v[2].Sign() != 0 {
			t.Errorf("e %g, Δt %g: an orbit in the xy-plane left it", e, tc.dt)
		}
	}
}

// TestStateOracleRetracesItsSteps: propagated forward and back by the same Δt,
// a general state returns to itself, and the motion keeps its energy and
// angular momentum, to within the oracle's working precision.
func TestStateOracleRetracesItsSteps(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		r0, v0 [3]float64
		dt     float64
	}{
		{"an inclined ellipse", [3]float64{0.9, 0.4, -0.2}, [3]float64{-0.008, 0.015, 0.003}, 120},
		{"a near-circular orbit", [3]float64{1, 0, 0}, [3]float64{0, gaussSpeed(1), 0}, 90},
		{"a hyperbola", [3]float64{1.2, -0.3, 0.5}, [3]float64{0.012, 0.025, -0.004}, -800},
		{"inbound, near-parabolic", [3]float64{3, 2, 1}, [3]float64{-0.007, -0.006, -0.002}, 400},
	} {
		r1, v1 := StateExact(tc.r0, tc.v0, gaussMu, tc.dt)

		var a, b [3]float64
		for i := range 3 {
			a[i], _ = r1[i].Float64()
			b[i], _ = v1[i].Float64()
		}

		// Back from the float64 nearest the forward state: that rounding,
		// carried back, is what bounds the return, not the oracle. Measured,
		// it is at most 7.9e-15 here, on the hyperbola.
		r2, _ := StateExact(a, b, gaussMu, -tc.dt)

		for i := range 3 {
			want := new(big.Float).SetPrec(Prec).SetFloat64(tc.r0[i])
			if err := relErr(r2[i], want); err > 1e-13 {
				t.Errorf("%s: component %d returns %.3g away", tc.name, i, err)
			}
		}

		e0, h0 := invariants(tc.r0, tc.v0)
		e1, h1 := invariantsBig(r1, v1)

		if err := math.Abs(e1/e0 - 1); err > 1e-15 {
			t.Errorf("%s: energy changes by %.3g", tc.name, err)
		}

		if err := math.Abs(h1/h0 - 1); err > 1e-15 {
			t.Errorf("%s: angular momentum changes by %.3g", tc.name, err)
		}
	}
}

func gaussSpeed(r float64) float64 { return math.Sqrt(gaussMu / r) }

// invariants are the specific energy and the magnitude of the specific
// angular momentum.
func invariants(r, v [3]float64) (energy, h float64) {
	rn := math.Sqrt(r[0]*r[0] + r[1]*r[1] + r[2]*r[2])
	v2 := v[0]*v[0] + v[1]*v[1] + v[2]*v[2]

	hx := r[1]*v[2] - r[2]*v[1]
	hy := r[2]*v[0] - r[0]*v[2]
	hz := r[0]*v[1] - r[1]*v[0]

	return v2/2 - gaussMu/rn, math.Sqrt(hx*hx + hy*hy + hz*hz)
}

func invariantsBig(r, v [3]*big.Float) (energy, h float64) {
	var a, b [3]float64
	for i := range 3 {
		a[i], _ = r[i].Float64()
		b[i], _ = v[i].Float64()
	}

	return invariants(a, b)
}
