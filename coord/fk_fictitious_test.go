package coord_test

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/gofaext"
)

// TestTheRoundTripClosesAtEveryEpoch is #341.
//
// A star with no recorded motion still moves in FK4 and in FK5, because both
// frames drift against the inertial one, and the conversions hand that motion
// back rather than pretending the star is at rest. Marking it as a *recorded*
// proper motion sent the inverse down the six-element branch — Fk425 for FK4,
// Fk52h for FK5 — and both of those take no epoch and assume the catalogue
// equinox, while the position they were handed is at the caller's epoch.
//
// The cost was linear in the distance from the equinox, and zero at it, which
// is why every existing round-trip test missed it:
//
//	bepoch   ICRS → FK4 → ICRS        jepoch   ICRS → FK5 → ICRS
//	1900     0.2368"                  1950     0.0480"
//	1950     0.0000"                  2000     0.0000"
//	1975     0.1184"                  2050     0.0480"
//	2000     0.2367"                  2100     0.0960"
//	2050     0.4735"
//
// 4.7 mas per year for FK4 and 0.96 for FK5. Both now close at every epoch.
func TestTheRoundTripClosesAtEveryEpoch(t *testing.T) {
	t.Parallel()

	star := coord.NewICRS(angle.Deg(123.4), angle.Deg(-35.6))

	// The FK4 route's floor is SOFA's own: the E-terms of aberration are
	// removed and restored by an iteration rather than inverted exactly, and
	// the raw Fk54z → Fk45z pair leaves the same 0.000023 arcsec. Anything
	// above a milliarcsecond is this defect returning.
	const tol = 1e-3

	for _, epoch := range []float64{1900, coord.B1950, 1975, 2000, 2050} {
		back := coord.FK4ToICRS(coord.ICRSToFK4(star, epoch))
		if sep := coord.Separation(star, back).Arcseconds(); sep > tol {
			t.Errorf("ICRS -> FK4 -> ICRS at bepoch %g: moved %.6f arcsec", epoch, sep)
		}
	}

	// The FK5 route has no E-terms and closes exactly.
	for _, epoch := range []float64{1950, coord.J2000Epoch, 2050, 2100} {
		back := coord.FK5ToICRS(coord.ICRSToFK5(star, epoch))
		if sep := coord.Separation(star, back).Arcseconds(); sep > 1e-9 {
			t.Errorf("ICRS -> FK5 -> ICRS at jepoch %g: moved %.9f arcsec", epoch, sep)
		}
	}

	// And the leg between the two catalogues, which is where the defect was
	// first localised.
	fk5 := coord.NewFK5(angle.Deg(123.4), angle.Deg(-35.6), coord.J2000Epoch)

	for _, epoch := range []float64{1900, coord.B1950, 2000, 2050} {
		back := coord.FK4ToFK5(coord.FK5ToFK4(fk5, epoch))

		sep := coord.Separation(
			coord.NewICRS(fk5.RA(), fk5.Dec()),
			coord.NewICRS(back.RA(), back.Dec()),
		).Arcseconds()
		if sep > tol {
			t.Errorf("FK5 -> FK4 -> FK5 at bepoch %g: moved %.6f arcsec", epoch, sep)
		}
	}
}

// TestAstrogoMatchesRawSOFAAtEveryEpoch is the strongest form of the claim, and
// the measurement that localised #341 in the first place.
//
// SOFA's Fk54z and Fk45z are matched inverses and close to 0.000023 arcsec at
// any epoch — which was true before this fix too. The defect was never in SOFA;
// it was astrogo choosing the wrong pair of routines. So the test that catches
// a recurrence compares the two implementations directly rather than measuring
// astrogo against a tolerance of its own.
func TestAstrogoMatchesRawSOFAAtEveryEpoch(t *testing.T) {
	t.Parallel()

	ra := angle.Deg(123.4)
	dec := angle.Deg(-35.6)

	for _, epoch := range []float64{1900, coord.B1950, 1975, 2000, 2050} {
		// Raw SOFA, with no astrogo types in the way.
		r1950, d1950, _, _ := gofaext.Fk54z(ra.Radians(), dec.Radians(), epoch)
		wantRA, wantDec := gofaext.Fk45z(r1950, d1950, epoch)

		// The same trip through astrogo.
		fk5 := coord.NewFK5(ra, dec, coord.J2000Epoch)
		got := coord.FK4ToFK5(coord.FK5ToFK4(fk5, epoch))

		sep := coord.Separation(
			coord.NewICRS(angle.Rad(wantRA), angle.Rad(wantDec)),
			coord.NewICRS(got.RA(), got.Dec()),
		).Arcseconds()

		// A nanoarcsecond: astrogo should be doing the identical arithmetic,
		// not merely a comparably accurate one.
		if sep > 1e-9 {
			t.Errorf("bepoch %g: astrogo is %.9f arcsec from the raw SOFA pair, "+
				"which means it is no longer calling the same routines", epoch, sep)
		}
	}
}

// TestARecordedMotionStillTakesTheSixElementRoute is the other side of the
// dispatch, and the reason it cannot simply always use the position-only pair.
//
// A catalogue's measured proper motion, parallax and radial velocity must
// survive the conversion. Routing those through Fk45z would silently drop all
// three, which is a worse bug than the one being fixed.
func TestARecordedMotionStillTakesTheSixElementRoute(t *testing.T) {
	t.Parallel()

	const (
		pmRAIn  = 150.0 // mas/yr
		pmDecIn = 220.0
		pxIn    = 0.020 // arcsec
		rvIn    = -22.4 // km/s
	)

	star := coord.NewICRSWithKinematics(
		angle.Deg(123.4), angle.Deg(-35.6),
		angle.Arcsec(pmRAIn/1000), angle.Arcsec(pmDecIn/1000),
		angle.Arcsec(pxIn), rvIn,
	)

	back := coord.FK4ToICRS(coord.ICRSToFK4(star, coord.B1950))

	if sep := coord.Separation(star, back).Arcseconds(); sep > 1e-3 {
		t.Errorf("position moved %.6f arcsec", sep)
	}

	if got := back.PmRA().Arcseconds() * 1000; got < pmRAIn-0.01 || got > pmRAIn+0.01 {
		t.Errorf("proper motion in RA came back %.4f mas/yr, want %.1f — a recorded motion "+
			"was routed through the position-only pair, which discards it", got, pmRAIn)
	}

	if got := back.PmDec().Arcseconds() * 1000; got < pmDecIn-0.01 || got > pmDecIn+0.01 {
		t.Errorf("proper motion in Dec came back %.4f mas/yr, want %.1f", got, pmDecIn)
	}

	if got := back.Parallax().Arcseconds(); got < pxIn-1e-6 || got > pxIn+1e-6 {
		t.Errorf("parallax came back %.8f arcsec, want %.3f", got, pxIn)
	}

	if got := back.RV(); got < rvIn-1e-4 || got > rvIn+1e-4 {
		t.Errorf("radial velocity came back %.6f km/s, want %.1f", got, rvIn)
	}
}

// TestAFictitiousMotionIsNotTreatedAsRecorded checks the distinction head-on.
//
// Feeding the fictitious motion back in through NewFK4WithProperMotion asserts
// it as a measurement, which is a different claim about the star and must give
// a different answer. If it did not, the flag would be inert and every test
// above would be passing for the wrong reason.
func TestAFictitiousMotionIsNotTreatedAsRecorded(t *testing.T) {
	t.Parallel()

	star := coord.NewICRS(angle.Deg(123.4), angle.Deg(-35.6))

	// The FK4 a conversion produces: position plus the frame's own motion.
	converted := coord.ICRSToFK4(star, 2000)

	pmRA, pmDec, ok := converted.ProperMotion()
	if !ok {
		t.Fatal("the position-only route stopped reporting the fictitious motion at all")
	}

	if pmRA == 0 && pmDec == 0 {
		t.Fatal("the fictitious motion is zero, so this test cannot distinguish anything")
	}

	// The same numbers, but asserted as a catalogue measurement.
	asRecorded := coord.NewFK4WithProperMotion(
		converted.RA(), converted.Dec(), pmRA, pmDec,
		converted.Parallax(), converted.RV(),
	)

	viaFictitious := coord.FK4ToICRS(converted)
	viaRecorded := coord.FK4ToICRS(asRecorded)

	if sep := coord.Separation(viaFictitious, viaRecorded).Arcseconds(); sep < 1e-3 {
		t.Errorf("declaring the same motion as recorded changed the answer by only %.9f arcsec — "+
			"the two claims must convert differently, and if they do not the dispatch is inert", sep)
	}

	// Only the first is the star we started from.
	if back := coord.Separation(star, viaFictitious).Arcseconds(); back > 1e-3 {
		t.Errorf("the fictitious route did not return the original star: %.6f arcsec", back)
	}
}
