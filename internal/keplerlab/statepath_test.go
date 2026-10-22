package keplerlab_test

import (
	"math"
	"math/big"
	"testing"

	"github.com/TuSKan/astrogo/internal/keplerlab"
)

// stateStart is a start state for the sweep: the orbit with perihelion 1 au
// and eccentricity e, at tau days from perihelion, turned into an inclined
// plane so that no component is zero.
func stateStart(t *testing.T, e, tau float64) (r, v [3]float64) {
	t.Helper()

	q := 1.0
	v0 := math.Sqrt(gaussMu * (1 + e) / q)

	br, bv := keplerlab.StateExact([3]float64{q, 0, 0}, [3]float64{0, v0, 0}, gaussMu, tau)

	var pr, pv [3]float64
	for i := range 3 {
		pr[i], _ = br[i].Float64()
		pv[i], _ = bv[i].Float64()
	}

	// Inclination 0.6 rad about x, then a node of 1.1 rad about z.
	ci, si := math.Cos(0.6), math.Sin(0.6)
	cn, sn := math.Cos(1.1), math.Sin(1.1)

	turn := func(a [3]float64) [3]float64 {
		x, y, z := a[0], a[1]*ci-a[2]*si, a[1]*si+a[2]*ci

		return [3]float64{x*cn - y*sn, x*sn + y*cn, z}
	}

	return turn(pr), turn(pv)
}

// TestStatePathMeasure sweeps StatePropagate against StateExact and logs the
// worst position and velocity error, in ulps of |r| and |v|, per
// eccentricity, beside the floor: the same state built exactly from the
// float64 nearest the exact χ.
func TestStatePathMeasure(t *testing.T) {
	if testing.Short() {
		t.Skip("a measurement, run on its own")
	}

	const eps = 0x1p-52

	for _, e := range []float64{0, 0.5, 0.9, 0.99, 0.999, 0.99999, 1 - 1e-8, 1, 1 + 1e-8, 1.00001, 1.001, 1.2, 2, 5} {
		period := 0.0
		if e < 1 {
			a := 1 / (1 - e)
			period = 2 * math.Pi * math.Sqrt(a*a*a/gaussMu)
		}

		taus := []float64{0, 30, -300, 3000}
		if period > 0 {
			taus = []float64{0, period / 8, -period / 4, 3 * period / 8}
		}

		var worstPos, worstVel, worstChi, floorPos, worstCond float64

		for _, tau := range taus {
			r0, v0 := stateStart(t, e, tau)

			for k := -6 * 4; k <= 6*4; k++ {
				dt := math.Pow(10, float64(k)/4)
				if period > 0 && dt > period/2*(1-1e-6) {
					continue
				}

				for _, d := range []float64{dt, -dt} {
					r, v, chi, err := keplerlab.StatePropagate(r0, v0, gaussMu, d)
					if err != nil {
						t.Fatalf("e %g, τ %g, Δt %g: %v", e, tau, d, err)
					}

					wantChi := keplerlab.StateChiExact(r0, v0, gaussMu, d)
					wr, wv := keplerlab.StateFromChiExact(r0, v0, gaussMu, wantChi)

					rounded, _ := wantChi.Float64()
					fr, _ := keplerlab.StateFromChiExact(r0, v0, gaussMu, new(big.Float).SetPrec(keplerlab.Prec).SetFloat64(rounded))

					rmag, vmag := norm3(wr), norm3(wv)

					worstPos = math.Max(worstPos, dist3(r, wr)/(eps*rmag))
					worstCond = math.Max(worstCond, dist3(r, wr)/keplerlab.StateSensitivity(r0, v0, gaussMu, d))
					worstVel = math.Max(worstVel, dist3(v, wv)/(eps*vmag))
					floorPos = math.Max(floorPos, distBig3(fr, wr)/(eps*rmag))

					if wantChi.Sign() != 0 {
						worstChi = math.Max(worstChi, relErr(new(big.Float).SetPrec(keplerlab.Prec).SetFloat64(chi), wantChi)/eps)
					}
				}
			}
		}

		t.Logf("e %-11g χ %7.2f ulps   position %8.2f ulps of |r| (floor %5.2f), %5.2f of the input's own   velocity %8.2f ulps of |v|",
			e, worstChi, worstPos, floorPos, worstCond, worstVel)
	}
}

func norm3(a [3]*big.Float) float64 {
	var s float64

	for i := range 3 {
		f, _ := a[i].Float64()
		s += f * f
	}

	return math.Sqrt(s)
}

// dist3 is |got − want| with the difference taken at the oracle's precision.
func dist3(got [3]float64, want [3]*big.Float) float64 {
	var s float64

	for i := range 3 {
		d := new(big.Float).SetPrec(keplerlab.Prec).SetFloat64(got[i])
		d.Sub(d, want[i])
		f, _ := d.Float64()
		s += f * f
	}

	return math.Sqrt(s)
}

func distBig3(got, want [3]*big.Float) float64 {
	var s float64

	for i := range 3 {
		d := new(big.Float).SetPrec(keplerlab.Prec).Sub(got[i], want[i])
		f, _ := d.Float64()
		s += f * f
	}

	return math.Sqrt(s)
}
