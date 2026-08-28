package skybrightness_test

import (
	"context"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/skybrightness"
)

// How much the answer jitters as a feature moves across the sky.
//
// The direct measurement of aliasing, and the one the convergence test cannot
// make: there is no trustworthy reference value for a discontinuous integrand,
// because a finer run has the same kind of error. What can be measured is
// stability. Move a sharp feature by a fraction of a sample spacing and the
// true integral changes smoothly and slightly; a quadrature that aliases
// against it jumps.
//
// Reported as the coefficient of variation over a sweep, which is the number
// to watch when this is changed.
func TestScatteredInIsStableAsAFeatureMoves(t *testing.T) {
	t.Parallel()

	view := coord.NewAltAz(angle.Deg(55), angle.Deg(200))

	band := func(inclinationDeg float64) skybrightness.SkyRadiance {
		inclination := inclinationDeg * math.Pi / 180

		return func(_ context.Context, dst skybrightness.SpectralRadiance, dir coord.AltAz) error {
			alt, az := dir.Alt().Radians(), dir.Az().Radians()
			z := math.Sin(alt)*math.Cos(inclination) -
				math.Cos(alt)*math.Sin(az)*math.Sin(inclination)

			if math.Abs(math.Asin(math.Max(-1, math.Min(1, z)))) > 10*math.Pi/180 {
				return nil
			}

			for i := range dst {
				dst[i] += 1e-9
			}

			return nil
		}
	}

	const (
		rings = 16
		steps = 24
	)

	values := make([]float64, 0, steps)

	// A sweep of two ring spacings, so the feature crosses several sample rows
	// and any lining-up shows as a jump.
	for s := range steps {
		inclination := 60 + 2*(90.0/rings)*float64(s)/float64(steps)
		values = append(values, scatteredAt(t, band(inclination), view, rings))
	}

	var mean float64
	for _, v := range values {
		mean += v
	}

	mean /= float64(len(values))

	var variance, worst float64

	for _, v := range values {
		d := v - mean
		variance += d * d
		worst = math.Max(worst, math.Abs(d)/mean)
	}

	cv := math.Sqrt(variance/float64(len(values))) / mean

	t.Logf("over %d positions at %d rings: coefficient of variation %.4f per cent, "+
		"worst departure %.4f per cent", steps, rings, 100*cv, 100*worst)

	// The true integral barely changes over this sweep — the band covers the
	// same solid angle and moves a few degrees — so essentially all of this
	// spread is the quadrature. Five per cent would mean the rule is
	// controlled by where the samples happen to fall.
	if cv > 0.05 {
		t.Errorf("the answer varies by %.2f per cent as the feature moves; the quadrature "+
			"is aliasing against it", 100*cv)
	}
}
