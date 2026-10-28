package kepler

import (
	"fmt"
	"math"
	"math/big"
	"testing"

	"github.com/TuSKan/astrogo/internal/oracle"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// FromState's contract (#803), held against the 256-bit oracle in
// internal/oracle: the propagated position and velocity each within
// contractStateCond times what the input's own rounding causes,
// oracle.StateSensitivity — Σ|∂r/∂x|·|x|·ε and Σ|∂v/∂x|·|x|·ε over the six
// components.
//
// That scale and not ulps: a state far out propagated to near perihelion is
// ill-conditioned in the problem itself, and ulps of |r| would charge the path
// for it; ulps of |v| do the same when a perihelion state is carried out to
// where it moves slowly, 303 of them at e = 0.999 for an error the input's own
// rounding explains.
//
// Measured on the dense sweep, four intervals per decade: the direct path on
// ellipses at most 1.75 in position and 1.65 in velocity, the perihelion path
// on open orbits at most 4.69 (e = 5) and 3.21 (e = 2). The test samples that
// sweep thinner, since every sample is eight 256-bit propagations.
const contractStateCond = 8

// stateContractStart is a start state for the sweep: the orbit with
// perihelion 1 au and eccentricity e, tau days from perihelion, turned into an
// inclined plane so that no component is zero.
func stateContractStart(t *testing.T, mu, e, tau float64) (r, v [3]float64) {
	t.Helper()

	v0 := math.Sqrt(mu * (1 + e))

	br, bv := oracle.StateExact([3]float64{1, 0, 0}, [3]float64{0, v0, 0}, mu, tau)

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

// TestFromStateContract propagates states on fourteen conics, from the circle
// to e = 5, started at four places on each, over intervals from 10⁻⁶ to 10⁶
// days either side, within half a period on an ellipse, and holds every one
// to the oracle: every second decade, or under -short every third and from
// two of the four places. One subtest per conic, in parallel.
func TestFromStateContract(t *testing.T) {
	t.Parallel()

	epoch := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	decades, places := 2, []int{0, 1, 2, 3}
	if testing.Short() {
		decades, places = 3, []int{0, 3}
	}

	probe, err := FromState(epoch, vector.V3(1, 0, 0), vector.V3(0, 0.017, 0), sunCenter)
	if err != nil {
		t.Fatal(err)
	}

	mu := probe.prep.mu

	for _, e := range []float64{0, 0.5, 0.9, 0.99, 0.999, 0.99999, 1 - 1e-8, 1, 1 + 1e-8, 1.00001, 1.001, 1.2, 2, 5} {
		t.Run(fmt.Sprintf("e=%g", e), func(t *testing.T) {
			t.Parallel()

			period := 0.0

			if e < 1 {
				a := 1 / (1 - e)
				period = 2 * math.Pi * math.Sqrt(a*a*a/mu)
			}

			taus := []float64{0, 30, -300, 3000}
			if period > 0 {
				taus = []float64{0, period / 8, -period / 4, 3 * period / 8}
			}

			var worstCond, worstVel float64

			for _, i := range places {
				tau := taus[i]
				r0, v0 := stateContractStart(t, mu, e, tau)

				el, err := FromState(epoch, vector.V3(r0[0], r0[1], r0[2]), vector.V3(v0[0], v0[1], v0[2]), sunCenter)
				if err != nil {
					t.Fatalf("τ %g: %v", tau, err)
				}

				for k := -6; k <= 6; k += decades {
					dt := math.Pow(10, float64(k))
					if period > 0 && dt > period/2*(1-1e-6) {
						continue
					}

					for _, d := range []float64{dt, -dt} {
						pos, vel, err := el.stateAt(d)
						if err != nil {
							t.Fatalf("τ %g, Δt %g: %v", tau, d, err)
						}

						wr, wv := oracle.StateExact(r0, v0, mu, d)
						posSens, velSens := oracle.StateSensitivity(r0, v0, mu, d)

						cond := dist3(pos, wr) / posSens
						velCond := dist3(vel, wv) / velSens

						worstCond, worstVel = math.Max(worstCond, cond), math.Max(worstVel, velCond)

						if !(cond <= contractStateCond) || !(velCond <= contractStateCond) {
							t.Errorf("τ %g, Δt %g: position %.3g and velocity %.3g times the input's own error",
								tau, d, cond, velCond)
						}
					}
				}
			}

			t.Logf("position %.2f, velocity %.2f times the input's own error", worstCond, worstVel)
		})
	}
}

// dist3 is |got − want|, the difference taken at the oracle's precision.
func dist3(got vector.Vec3, want [3]*big.Float) float64 {
	var s float64

	for i, g := range [3]float64{got.X, got.Y, got.Z} {
		d := new(big.Float).SetPrec(oracle.Prec).SetFloat64(g)
		d.Sub(d, want[i])
		f, _ := d.Float64()
		s += f * f
	}

	return math.Sqrt(s)
}
