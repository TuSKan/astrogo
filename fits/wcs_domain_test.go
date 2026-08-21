package fits

import (
	"math"
	"testing"
)

// A projection must answer a direction or an error, never a NaN.
//
// deproject takes intermediate coordinates that come from a pixel, and a pixel
// can be anywhere in an image — including far outside the region its projection
// describes. Three of the five zenithal cases feed a combination straight into
// math.Asin, whose argument is sin(dec) for a valid rotation but can exceed one
// by rounding, and ARC additionally has no bound on its radius at all, so a
// pixel more than pi radians from the reference point is off the sphere.
//
// A NaN here does not stop anything. It propagates into a coordinate, into a
// catalogue match, into a plot, and looks like a missing value rather than a
// bug in the transform.
func TestDeprojectNeverReturnsNaN(t *testing.T) {
	t.Parallel()

	projections := []string{"TAN", "SIN", "ARC", "STG", "AIT"}

	// Reference points including both poles, where sin/cos of the reference
	// declination are degenerate.
	references := []struct{ alpha0, delta0 float64 }{
		{0, 0},
		{math.Pi, 0},
		{0, math.Pi / 2},
		{0, -math.Pi / 2},
		{1.2, 0.9},
		{5.5, -1.1},
	}

	// Intermediate coordinates from just off the reference point out to well
	// beyond any sane image, in radians.
	offsets := []float64{
		0, 1e-12, 1e-6, 0.001, 0.1, 0.5, 1, 1.5,
		math.Pi / 2, 3, math.Pi, 3.5, 6, 100,
	}

	for _, proj := range projections {
		for _, ref := range references {
			for _, x := range offsets {
				for _, y := range offsets {
					for _, sx := range []float64{1, -1} {
						for _, sy := range []float64{1, -1} {
							ra, dec, err := deproject(proj, sx*x, sy*y, ref.alpha0, ref.delta0)
							if err != nil {
								continue // refusing is a correct answer
							}

							if math.IsNaN(ra) || math.IsNaN(dec) ||
								math.IsInf(ra, 0) || math.IsInf(dec, 0) {
								t.Errorf("%s at x=%+.6g y=%+.6g from (%.3f, %.3f) returned "+
									"ra=%v dec=%v with no error",
									proj, sx*x, sy*y, ref.alpha0, ref.delta0, ra, dec)

								continue
							}

							if math.Abs(dec) > math.Pi/2+1e-9 {
								t.Errorf("%s at x=%+.6g y=%+.6g returned declination %.6f rad, "+
									"which is off the sphere", proj, sx*x, sy*y, dec)
							}
						}
					}
				}
			}
		}
	}
}

// The forward projection has the same obligation.
func TestProjectNeverReturnsNaN(t *testing.T) {
	t.Parallel()

	projections := []string{"TAN", "SIN", "ARC", "STG", "AIT"}

	for _, proj := range projections {
		for _, delta0 := range []float64{0, 0.7, -0.7, math.Pi / 2, -math.Pi / 2} {
			for decStep := range 37 {
				dec := (float64(decStep)/36)*math.Pi - math.Pi/2

				for raStep := range 25 {
					ra := (float64(raStep) / 24) * 2 * math.Pi

					x, y, err := project(proj, ra, dec, 0, delta0)
					if err != nil {
						continue
					}

					if math.IsNaN(x) || math.IsNaN(y) {
						t.Errorf("%s at ra=%.4f dec=%+.4f from delta0=%+.4f returned "+
							"x=%v y=%v with no error", proj, ra, dec, delta0, x, y)
					}
				}
			}
		}
	}
}

// Projecting a direction and deprojecting the result must return the direction.
//
// This is the property that catches a sign, a swapped axis or a wrong quadrant
// in either half — each of which produces a perfectly finite coordinate
// somewhere else on the sky.
func TestProjectAndDeprojectRoundTrip(t *testing.T) {
	t.Parallel()

	projections := []string{"TAN", "SIN", "ARC", "STG", "AIT"}

	for _, proj := range projections {
		for _, delta0 := range []float64{0, 0.5, -0.8} {
			const alpha0 = 1.0

			// Stay within a few degrees of the reference point, where every
			// one of these projections is well conditioned.
			for _, dRA := range []float64{-0.05, -0.01, 0, 0.01, 0.05} {
				for _, dDec := range []float64{-0.05, -0.01, 0, 0.01, 0.05} {
					ra, dec := alpha0+dRA, delta0+dDec

					x, y, err := project(proj, ra, dec, alpha0, delta0)
					if err != nil {
						continue
					}

					gotRA, gotDec, err := deproject(proj, x, y, alpha0, delta0)
					if err != nil {
						t.Errorf("%s: projected (%.4f, %+.4f) then failed to invert: %v",
							proj, ra, dec, err)

						continue
					}

					// Compare as directions, so the right-ascension wrap does
					// not read as a disagreement.
					if sep := angularSeparation(ra, dec, gotRA, gotDec); sep > 1e-9 {
						t.Errorf("%s: (%.6f, %+.6f) round-tripped to (%.6f, %+.6f), %.3g rad away",
							proj, ra, dec, gotRA, gotDec, sep)
					}
				}
			}
		}
	}
}

// angularSeparation is the great-circle distance between two directions, in
// radians, via the haversine form so it stays accurate for small separations.
func angularSeparation(ra1, dec1, ra2, dec2 float64) float64 {
	dRA := ra2 - ra1
	dDec := dec2 - dec1

	h := math.Sin(dDec/2)*math.Sin(dDec/2) +
		math.Cos(dec1)*math.Cos(dec2)*math.Sin(dRA/2)*math.Sin(dRA/2)

	return 2 * math.Asin(math.Sqrt(math.Min(1, h)))
}
