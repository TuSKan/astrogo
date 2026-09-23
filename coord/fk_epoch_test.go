package coord_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// kinematicStarForEpoch is an ordinary catalogue entry with everything
// recorded, so the six-element route is the one that runs.
func kinematicStarForEpoch() coord.ICRS {
	return coord.NewICRSWithKinematics(
		angle.Deg(123.4), angle.Deg(-35.6),
		angle.Arcsec(0.150), angle.Arcsec(0.220),
		angle.Arcsec(0.020), unit.KmPerSec(-22.4),
	)
}

// positionOnlyStarForEpoch is the same direction with nothing recorded, so the
// position-only route runs instead.
func positionOnlyStarForEpoch() coord.ICRS {
	return coord.NewICRS(angle.Deg(123.4), angle.Deg(-35.6))
}

// TestTheSixElementRouteLabelsItsOutputWithTheCatalogueEquinox is #330.
//
// SOFA's Fk524 and H2fk5 answer at B1950.0 and J2000.0 and take no epoch
// argument. The conversions that call them used to store the caller's epoch in
// the returned struct anyway, so ICRSToFK4(star, 1975) returned B1950 numbers
// reporting Epoch() == 1975.
//
// The numbers were right and only the label was wrong, which is the worse of
// the two failures: numbers get compared, labels get believed.
func TestTheSixElementRouteLabelsItsOutputWithTheCatalogueEquinox(t *testing.T) {
	t.Parallel()

	star := kinematicStarForEpoch()
	fk5Star := coord.ICRSToFK5(star, coord.J2000Epoch)

	// Epochs far from the catalogue equinoxes, so a stored argument cannot be
	// mistaken for the right answer.
	for _, epoch := range []float64{1900, 1950, 1975, 2000, 2050} {
		if got := coord.ICRSToFK4(star, epoch).Epoch(); got != coord.B1950 {
			t.Errorf("ICRSToFK4(star, %g).Epoch() = %g, want B1950 — the six-element route "+
				"answers at the catalogue equinox and must say so", epoch, got)
		}

		if got := coord.FK5ToFK4(fk5Star, epoch).Epoch(); got != coord.B1950 {
			t.Errorf("FK5ToFK4(star, %g).Epoch() = %g, want B1950", epoch, got)
		}

		if got := coord.ICRSToFK5(star, epoch).Epoch(); got != coord.J2000Epoch {
			t.Errorf("ICRSToFK5(star, %g).Epoch() = %g, want J2000 — #330 recorded this "+
				"defect for FK4 only, and FK5 had it too", epoch, got)
		}
	}
}

// TestThePositionOnlyRouteStillHonoursItsEpoch is the other half, and the
// reason the argument cannot simply be removed.
//
// SOFA's Fk54z and Hfk5z do take a date, because a star with no recorded motion
// acquires one from the frames' relative spin, and where it has drifted to
// depends on when it is asked about. So on that route the epoch is used, the
// label is true, and the answer genuinely changes with it.
func TestThePositionOnlyRouteStillHonoursItsEpoch(t *testing.T) {
	t.Parallel()

	star := positionOnlyStarForEpoch()

	at1950 := coord.ICRSToFK4(star, coord.B1950)
	at2000 := coord.ICRSToFK4(star, 2000)

	if at1950.Epoch() != coord.B1950 || at2000.Epoch() != 2000 {
		t.Fatalf("the position-only route stopped recording its epoch: %g and %g",
			at1950.Epoch(), at2000.Epoch())
	}

	// Half a century of fictitious proper motion. For this star that motion is
	// about 4.7 mas/yr, so fifty years is 0.237 arcsec — small, but four
	// orders of magnitude above the round trip's own residual, and the point
	// is that it is not zero.
	moved := coord.Separation(
		coord.NewICRS(at1950.RA(), at1950.Dec()),
		coord.NewICRS(at2000.RA(), at2000.Dec()),
	).Arcseconds()

	if moved < 0.05 {
		t.Errorf("the position moved %.4f arcsec between B1950 and B2000, expected about 0.237 "+
			"— if this is near zero the epoch stopped being used", moved)
	}
}

// TestTheSixElementRouteIgnoresTheEpochEntirely proves the argument is unused
// rather than partly used, which is what makes returning [coord.B1950] honest
// instead of merely tidier.
//
// If any part of the six-element path consumed the epoch, two calls differing
// only in it would differ in some element, and labeling both B1950 would then
// be a second wrong label rather than a correction of the first.
func TestTheSixElementRouteIgnoresTheEpochEntirely(t *testing.T) {
	t.Parallel()

	star := kinematicStarForEpoch()

	base := coord.ICRSToFK4(star, coord.B1950)

	for _, epoch := range []float64{1900, 1975, 2050} {
		got := coord.ICRSToFK4(star, epoch)

		basePmRA, basePmDec, _ := base.ProperMotion()
		gotPmRA, gotPmDec, _ := got.ProperMotion()

		if got.RA() != base.RA() || got.Dec() != base.Dec() ||
			gotPmRA != basePmRA || gotPmDec != basePmDec ||
			got.Parallax() != base.Parallax() || got.RV() != base.RV() {
			t.Errorf("epoch %g: the six-element route produced different numbers from B1950, "+
				"so the epoch is partly used and B1950 is the wrong label", epoch)
		}
	}
}

// TestPropagateEpochIsWhatTheEpochArgumentCannotDo shows the limitation has
// teeth, and shows the supported way round it.
//
// Ignoring an epoch would be harmless if the epoch did not matter. It does:
// this star moves 0.27 arcsec a year, so a quarter century is arcseconds. What
// the six-element conversion cannot do, [coord.PropagateEpoch] can — rigorously,
// through SOFA's Pmsafe, including parallax and the light-time term that simple
// linear propagation omits.
//
// So the answer to "I want this star in FK4 at B1975" is to propagate first and
// convert second, and this test is that recipe executed.
func TestPropagateEpochIsWhatTheEpochArgumentCannotDo(t *testing.T) {
	t.Parallel()

	star := kinematicStarForEpoch()

	// Twenty-five Julian years after J2000.
	later := time.J2000().Add(unit.JulianYears(25))

	moved, err := coord.PropagateEpoch(star, time.J2000(), later)
	testutil.AssertNoError(t, err)

	atEquinox := coord.ICRSToFK4(star, coord.B1950)
	propagated := coord.ICRSToFK4(moved, coord.B1950)

	// Both are B1950 positions, and they differ because the star moved.
	if propagated.Epoch() != coord.B1950 || atEquinox.Epoch() != coord.B1950 {
		t.Fatalf("both should be labeled B1950, got %g and %g",
			propagated.Epoch(), atEquinox.Epoch())
	}

	sep := coord.Separation(
		coord.NewICRS(atEquinox.RA(), atEquinox.Dec()),
		coord.NewICRS(propagated.RA(), propagated.Dec()),
	).Arcseconds()

	// 0.267 arcsec/yr over 25 years is about 6.7 arcsec, and the conversion is
	// very nearly length-preserving on the sky.
	expected := math.Hypot(0.150, 0.220) * 25

	testutil.AssertRelNear(t, "the propagated star moved", sep, expected, 0.02)

	if sep < 1 {
		t.Errorf("propagation moved the star %.4f arcsec over 25 years, which is too little "+
			"for this proper motion — the limitation would be harmless if this were zero", sep)
	}
}

// TestTheRoundTripStillClosesAfterTheRelabeling guards the thing a label
// change could plausibly break.
//
// FK4ToFK5's position-only route reads FK4.Epoch and passes it to SOFA. Nothing
// reads it on the six-element route, which is why storing B1950 there is safe —
// but "nothing reads it" is exactly the kind of claim that stops being true, so
// it is asserted rather than assumed.
//
// Both routes are checked at every epoch. The position-only one used to close
// only at B1950 — see TestTheRoundTripClosesAtEveryEpoch, which is #341 and
// has the numbers.
func TestTheRoundTripStillClosesAfterTheRelabeling(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		star coord.ICRS
	}{
		{"with recorded kinematics", kinematicStarForEpoch()},
		{"position only", positionOnlyStarForEpoch()},
	} {
		for _, epoch := range []float64{coord.B1950, 1900, 1975, 2000, 2050} {
			back := coord.FK4ToICRS(coord.ICRSToFK4(tc.star, epoch))

			sep := coord.Separation(tc.star, back).Arcseconds()
			if sep > 1e-3 {
				t.Errorf("%s at epoch %g: ICRS -> FK4 -> ICRS moved the position %.6f arcsec",
					tc.name, epoch, sep)
			}
		}
	}
}
