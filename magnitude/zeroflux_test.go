package magnitude_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/magnitude"
)

// TestZeroReflectedFluxIsInfinitelyFaint: where a phase law reflects nothing
// toward the observer, the magnitude is +Inf, as SurfaceBrightness answers a
// zero radiance and as sbpy answers these laws (#835). Until then each gave a
// finite magnitude 75 fainter, from a 10⁻³⁰ floor under the logarithm. Just
// short of the zero, each is finite and still rising, so the +Inf is the
// limit the law approaches and not a cut placed early.
func TestZeroReflectedFluxIsInfinitelyFaint(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name     string
		mag      func(deg float64) float64
		zero     []float64  // phase angles where the law reflects nothing
		approach [2]float64 // two angles short of it, fainter at the second
	}{
		{
			"H,G at 180°",
			func(deg float64) float64 { return magnitude.AsteroidHG(3.34, 0.12, 1, 1, angle.Deg(deg)) },
			[]float64{180},
			[2]float64{170, 179},
		},
		{
			// Φ₁'s extrapolation past its 150° knot reaches zero at 152.3°,
			// where it is clipped, as sbpy clips it.
			"H,G1,G2 with G1 = 1 beyond 152.3°",
			func(deg float64) float64 { return magnitude.AsteroidHG1G2(0, 1, 0, 1, 1, angle.Deg(deg)) },
			[]float64{153, 160, 180},
			[2]float64{150, 152},
		},
		{
			"satellite sphere at 180°",
			func(deg float64) float64 {
				return magnitude.SatelliteApparent(4, magnitude.ConventionMcCants, 1_000_000, angle.Deg(deg), magnitude.PhaseSphere)
			},
			[]float64{180},
			[2]float64{170, 179.9},
		},
	} {
		for _, deg := range c.zero {
			if got := c.mag(deg); !math.IsInf(got, 1) {
				t.Errorf("%s: %v at α = %v°, want +Inf", c.name, got, deg)
			}
		}

		near, nearer := c.mag(c.approach[0]), c.mag(c.approach[1])
		if math.IsInf(near, 0) || math.IsInf(nearer, 0) || !(nearer > near) {
			t.Errorf("%s: %v at %v° and %v at %v°, want finite and rising toward the zero",
				c.name, near, c.approach[0], nearer, c.approach[1])
		}
	}
}
