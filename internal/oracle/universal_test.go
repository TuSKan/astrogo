package oracle

import (
	"math"
	"math/big"
	"testing"
)

// gaussMu is k², the Sun's GM in AU³/day² with Gauss's constant.
const gaussMu = 0.01720209895 * 0.01720209895

// ulpsOf is a float64's error against a Prec value, in ulps of the value.
func ulpsOf(got float64, want *big.Float) float64 {
	if want.Sign() == 0 {
		return math.Abs(got) / 0x1p-1074
	}

	d := new(big.Float).SetPrec(Prec).Sub(newF(got), want)
	d.Quo(d, want)
	f, _ := d.Float64()

	return math.Abs(f) / 0x1p-52
}

// TestOracleStumpff: the series against the identities c₀ = 1 − z·c₂,
// c₁ = 1 − z·c₃ and c₀² + z·c₁² = 1, and against math.Cos and math.Sinh
// at points where those are accurate.
func TestOracleStumpff(t *testing.T) {
	for _, z := range []float64{1e-30, -1e-30, 0.1, -0.1, 1, -1, 9.8, -40, -400} {
		c := Stumpff(z)
		bz := newF(z)

		// Each identity is a difference of terms as large as c₀², which is
		// cosh²√−z on a hyperbola: its rounding at Prec is the scale.
		scale := max(c[0].MantExp(nil)*2, 1)

		check := func(name string, got, want *big.Float) {
			d := new(big.Float).SetPrec(Prec).Sub(got, want)
			if d.Sign() != 0 && d.MantExp(nil) > max(want.MantExp(nil), scale)-int(Prec)+16 {
				t.Errorf("z = %v: %s off at 2^%d", z, name, d.MantExp(nil)-want.MantExp(nil))
			}
		}

		check("c0 = 1 − z·c2", c[0], new(big.Float).SetPrec(Prec).Sub(newI(1), new(big.Float).SetPrec(Prec).Mul(bz, c[2])))
		check("c1 = 1 − z·c3", c[1], new(big.Float).SetPrec(Prec).Sub(newI(1), new(big.Float).SetPrec(Prec).Mul(bz, c[3])))

		zc1 := new(big.Float).SetPrec(Prec).Mul(c[1], c[1])
		zc1.Mul(zc1, bz)
		check("c0² + z·c1² = 1", new(big.Float).SetPrec(Prec).Add(new(big.Float).SetPrec(Prec).Mul(c[0], c[0]), zc1), newI(1))

		var want float64
		if z > 0 {
			want = math.Cos(math.Sqrt(z))
		} else {
			want = math.Cosh(math.Sqrt(-z))
		}

		if u := ulpsOf(want, c[0]); u > 4 {
			t.Errorf("z = %v: c0 is %.3g ulps from math's", z, u)
		}
	}
}

// TestOracleUniversalIsKepler: on an ellipse the universal state is the
// classical one, from the oracle's own eccentric anomaly at M = n·Δt — two
// formulations sharing nothing but the arithmetic. M is formed in float64,
// so the agreement is to its rounding, not to Prec.
func TestOracleUniversalIsKepler(t *testing.T) {
	for _, e := range []float64{0, 0.3, 0.9, 0.999} {
		q := 0.7
		a := q / (1 - e)
		n := math.Sqrt(gaussMu / (a * a * a))

		for _, dt := range []float64{1e-3, 3.7, 50, 180} {
			ux, uy, _, _ := UniversalStateExact(q, e, gaussMu, dt)
			kx, ky, _, _ := StateFromE(a, e, n, SolveExact(n*dt, e))

			rel := AbsErr(mustF(ux), mustF(uy), kx, ky) / Norm(kx, ky)
			if rel > 1e-13 {
				t.Errorf("e = %v, Δt = %v: universal and classical differ by %.3g relative", e, dt, rel)
			}
		}
	}
}

func mustF(b *big.Float) float64 {
	f, _ := b.Float64()

	return f
}

// TestOracleUniversalAtPerihelion: at Δt = ±0 the oracle's χ is that same
// signed zero, and its state is the perihelion itself: (q, 0) moving at
// (0, v₀), v₀ = √(μ(1 + e)/q), for an ellipse, the parabola and a hyperbola.
func TestOracleUniversalAtPerihelion(t *testing.T) {
	for _, e := range []float64{0.5, 1, 2} {
		for _, dt := range []float64{0, math.Copysign(0, -1)} {
			chi := UniversalExact(0.7, e, gaussMu, dt)
			if chi.Sign() != 0 || chi.Signbit() != math.Signbit(dt) {
				t.Errorf("e = %v, Δt = %v: χ = %v, want the same zero", e, dt, chi)
			}

			x, y, vx, vy := UniversalStateExact(0.7, e, gaussMu, dt)
			v0 := math.Sqrt(gaussMu * (1 + e) / 0.7)

			if mustF(x) != 0.7 || mustF(y) != 0 || mustF(vx) != 0 || math.Abs(mustF(vy)-v0) > 1e-15*v0 {
				t.Errorf("e = %v, Δt = %v: state (%v, %v) (%v, %v), want the perihelion (0.7, 0) (0, %v)",
					e, dt, mustF(x), mustF(y), mustF(vx), mustF(vy), v0)
			}
		}
	}
}
