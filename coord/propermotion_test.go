package coord_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
)

// TestACataloguesProperMotionMovesAStarByWhatItSays is the regression test for
// #281, and it is written the way the bug was found rather than the way the
// code is shaped.
//
// A catalogue's pmra column is μα* — the rate the star moves *across the sky*.
// So a star with 1000 mas/yr and nothing in declination travels 20 arcseconds
// in twenty years, at any declination, and that is the whole assertion. It is
// convention-independent: whatever the library stores internally, the star has
// to end up 20 arcseconds from where it started.
//
// Before the fix it travelled 20·cos δ instead, so the measured displacement
// tracked the cosine exactly:
//
//	dec  0: 20.0000″   dec 45: 14.1421″   dec 60: 10.0000″   dec 80: 3.4730″
//
// Sixteen and a half arcseconds lost at δ = 80, on a value taken straight from
// Gaia. Kapteyn's Star came out 49″ from where it is.
func TestACataloguesProperMotionMovesAStarByWhatItSays(t *testing.T) {
	t.Parallel()

	const (
		pmRAMasPerYear = 1000.0
		years          = 20.0
		wantArcsec     = pmRAMasPerYear / 1000 * years
	)

	from := time.J2000
	to := from.AddDays(years * 365.25)

	for _, dec := range []float64{0, 30, 45, 60, 80, -70} {
		src := coord.NewICRSWithKinematics(
			angle.Deg(120), angle.Deg(dec),
			angle.Arcsec(pmRAMasPerYear/1000), 0, 0, 0,
		)

		got, err := coord.PropagateEpoch(src, from, to)
		if err != nil {
			t.Fatalf("PropagateEpoch at dec %g: %v", dec, err)
		}

		moved := coord.Separation(coord.NewICRS(src.RA(), src.Dec()), got).Arcseconds()

		t.Logf("dec %+3.0f: moved %8.4f arcsec, want %.4f (cos dec = %.4f)",
			dec, moved, wantArcsec, math.Cos(dec*math.Pi/180))

		// A milliarcsecond over twenty years. The residual is the second-order
		// geometry of moving along a great circle, not the factor this is
		// about — a missing cos(dec) is 5.9 arcseconds at δ = 45.
		testutil.AssertNear(t, "displacement (arcsec)", moved, wantArcsec, 1e-3)
	}
}

// TestProperMotionSurvivesAFrameConversion checks the same claim through the
// catalogue frames, since those cross the SOFA boundary twice and could get
// the factor right once and wrong the other way.
//
// The magnitudes are not preserved exactly — FK4's E-terms and both
// catalogues' spin genuinely change a star's motion — so this asserts the
// thing that must not happen: the RA component must not come back scaled by
// cos δ, which at δ = −65° would be a factor of 2.4.
func TestProperMotionSurvivesAFrameConversion(t *testing.T) {
	t.Parallel()

	const pmRAArcsec = 0.5

	for _, dec := range []float64{0, -65, 75} {
		src := coord.NewICRSWithKinematics(
			angle.Deg(200), angle.Deg(dec),
			angle.Arcsec(pmRAArcsec), angle.Arcsec(0.1), angle.Arcsec(0.05), 10,
		)

		for _, tc := range []struct {
			name string
			back coord.ICRS
		}{
			{"through FK5", coord.FK5ToICRS(coord.ICRSToFK5(src, coord.J2000Epoch))},
			{"through FK4", coord.FK4ToICRS(coord.ICRSToFK4(src, coord.B1950))},
		} {
			ratio := tc.back.PmRA().Arcseconds() / pmRAArcsec

			t.Logf("dec %+3.0f %-13s pmRA ratio %.6f", dec, tc.name, ratio)

			// A percent covers the real physical difference the round trip
			// introduces; a lost or doubled cos(dec) is 58% at δ = 55 and
			// 137% at δ = -65, so nothing here is a close call.
			if math.Abs(ratio-1) > 0.01 {
				t.Errorf("at dec %g, %s returned a proper motion %.4f times the "+
					"original — a cos(dec) factor is being applied or dropped "+
					"somewhere in the round trip", dec, tc.name, ratio)
			}
		}
	}
}

// TestProperMotionAtThePoleDoesNotExplode covers the one place the conversion
// has no answer.
//
// dRA/dt is undefined at a pole — right ascension itself has no meaning there,
// so no rate of change of it does either — and the conversion divides by
// cos δ to get it. A star exactly at the pole must produce a finite result
// rather than an infinity that propagates into a NaN position.
func TestProperMotionAtThePoleDoesNotExplode(t *testing.T) {
	t.Parallel()

	for _, dec := range []float64{90, -90} {
		src := coord.NewICRSWithKinematics(
			angle.Deg(0), angle.Deg(dec),
			angle.Arcsec(1), angle.Arcsec(1), 0, 0,
		)

		got, err := coord.PropagateEpoch(src, time.J2000, time.J2000.AddDays(3652.5))
		if err != nil {
			t.Fatalf("PropagateEpoch at the pole: %v", err)
		}

		if math.IsNaN(got.RA().Radians()) || math.IsNaN(got.Dec().Radians()) ||
			math.IsInf(got.RA().Radians(), 0) || math.IsInf(got.Dec().Radians(), 0) {
			t.Errorf("a star at dec %g propagated to RA %v Dec %v", dec, got.RA(), got.Dec())
		}

		t.Logf("dec %+3.0f: propagated to RA %v Dec %v", dec, got.RA(), got.Dec())
	}
}
