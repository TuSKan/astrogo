package kepler

import (
	"math"
	"math/big"
	"testing"

	"github.com/TuSKan/astrogo/internal/oracle"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// The universal path's contract (#833), held against the 256-bit oracle in
// internal/oracle, in the classical contract's shape: the universal anomaly
// χ within contractUniversalChiULPs of the exact root, and the perifocal state
// within those ulps carried through ∂state/∂χ, plus contractUniversalStateULPs
// for the construction — of |r| for the position, and of |v| for the
// velocity, floored on an ellipse at √(μ/a) as the classical contract floors
// it at n·a. Since dχ/dt = √μ/r, ∂pos/∂χ = v·r/√μ and |∂vel/∂χ| = √μ/r.
//
// The construction's 4 ulps is the velocity far out on a hyperbola, measured
// at 2.8: there it is insensitive to χ, and what is left is the eight or so
// roundings that build it. Before #833 the same sweep put the position 39.6
// times outside this bound.
const (
	contractUniversalChiULPs   = 2
	contractUniversalStateULPs = 4
)

// TestUniversalContract holds solveUniversal and perifocalUniversal to the
// contract over fourteen eccentricities from the circle to e = 5, the
// parabola and 1 ± 10⁻⁸ among them, and intervals from 10⁻⁶ to 10⁶ days
// either side of perihelion, within half a period on an ellipse — thinned, not
// dropped, under -short.
func TestUniversalContract(t *testing.T) {
	epoch := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	perDecade := 8
	if testing.Short() {
		perDecade = 2
	}

	const eps = 0x1p-52

	for _, e := range []float64{0, 0.5, 0.9, 0.99, 0.999, 0.99999, 1 - 1e-8, 1, 1 + 1e-8, 1.00001, 1.001, 1.2, 2, 5} {
		el, err := FromPerihelion(epoch, epoch, unit.AU(1), e, 0, 0, 0)
		if err != nil {
			t.Fatalf("e = %v: %v", e, err)
		}

		q, mu, alpha, sqrtMu := el.perihelion.AU(), el.prep.mu, el.prep.alpha, el.prep.sqrtMu

		maxDt := 1e6
		if p := el.prep.period; p > 0 {
			maxDt = p / 2 * (1 - 1e-6)
		}

		for k := -6 * perDecade; k <= 6*perDecade; k++ {
			dt := math.Pow(10, float64(k)/float64(perDecade))
			if dt > maxDt {
				continue
			}

			for _, d := range []float64{dt, -dt} {
				wantChi := oracle.UniversalExact(q, e, mu, d)

				chi, err := solveUniversal(q, e, alpha, sqrtMu, d)
				if err != nil {
					t.Fatalf("e = %v, Δt = %v d: %v", e, d, err)
				}

				if u := relULPs(chi, wantChi); u > contractUniversalChiULPs {
					t.Errorf("e = %v, Δt = %v d: χ is %.3g ulps from the exact root", e, d, u)
				}

				pos, vel, err := el.perifocalUniversal(d)
				if err != nil {
					t.Fatalf("e = %v, Δt = %v d: %v", e, d, err)
				}

				wx, wy, wvx, wvy := oracle.UniversalStateExact(q, e, mu, d)
				r, v := oracle.Norm(wx, wy), oracle.Norm(wvx, wvy)

				vScale := v
				if alpha > 0 {
					vScale = math.Max(v, math.Sqrt(mu*alpha))
				}

				absChi, _ := new(big.Float).Abs(wantChi).Float64()
				posBound := contractUniversalChiULPs*eps*absChi*v*r/math.Sqrt(mu) + contractUniversalStateULPs*eps*r
				velBound := contractUniversalChiULPs*eps*absChi*math.Sqrt(mu)/r + contractUniversalStateULPs*eps*vScale

				if got := oracle.AbsErr(pos.X, pos.Y, wx, wy); !(got <= posBound) || pos.Z != 0 {
					t.Errorf("e = %v, Δt = %v d: position %.3g times its bound from the oracle's", e, d, got/posBound)
				}

				if got := oracle.AbsErr(vel.X, vel.Y, wvx, wvy); !(got <= velBound) || vel.Z != 0 {
					t.Errorf("e = %v, Δt = %v d: velocity %.3g times its bound from the oracle's", e, d, got/velBound)
				}
			}
		}
	}
}

// relULPs is |got − want|/|want| in units of 2⁻⁵².
func relULPs(got float64, want *big.Float) float64 {
	d := new(big.Float).SetPrec(oracle.Prec).SetFloat64(got)
	d.Sub(d, want)
	d.Quo(d, want)
	f, _ := d.Float64()

	return math.Abs(f) / 0x1p-52
}

// TestStumpffSeriesMeetsTheClosedForm checks the switch at |z| = 4, where the
// Maclaurin series hands over to the closed forms: on either side of it, for
// both signs of z, the two agree on all four functions to the few ulps each
// is accurate to, as stumpff's doc comment records them against the oracle.
// And at zero the series gives the limits exactly.
func TestStumpffSeriesMeetsTheClosedForm(t *testing.T) {
	closed := func(z float64) [4]float64 {
		if z > 0 {
			x := math.Sqrt(z)
			s, c := math.Sincos(x)
			h := math.Sin(x / 2)

			return [4]float64{c, s / x, 2 * h * h / (x * x), (x - s) / (x * x * x)}
		}

		x := math.Sqrt(-z)
		h := math.Sinh(x / 2)
		sh := math.Sinh(x)

		return [4]float64{math.Cosh(x), sh / x, 2 * h * h / (x * x), (sh - x) / (x * x * x)}
	}

	for _, z := range []float64{math.Nextafter(4, 0), math.Nextafter(-4, 0), 3.9, -3.9, 4, -4} {
		c0, c1, c2, c3 := stumpff(z)
		series := [4]float64{c0, c1, c2, c3}
		cl := closed(z)

		for k := range series {
			// c₀ is cos 2 = −0.42 here: its ulps are taken of 1, as the
			// accuracy table takes them for a function that crosses zero.
			scale := math.Max(math.Abs(cl[k]), 1)
			if k > 1 {
				scale = math.Abs(cl[k])
			}

			if d := math.Abs(series[k]-cl[k]) / scale / 0x1p-52; d > 8 {
				t.Errorf("z = %v: c%d is %.17g by the series, %.17g closed, %.3g ulps apart", z, k, series[k], cl[k], d)
			}
		}
	}

	if c0, c1, c2, c3 := stumpff(0); c0 != 1 || c1 != 1 || c2 != 0.5 || c3 != 1.0/6 {
		t.Errorf("c(0) = %v, %v, %v, %v; want 1, 1, 1/2 and 1/6", c0, c1, c2, c3)
	}
}
