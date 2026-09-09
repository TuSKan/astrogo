package coord_test

import (
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
)

// TestFK4ToICRSAgreesWithSOFAsPublishedVector is the end-to-end check against
// a number that came from outside this library.
//
// The inputs are SOFA's own Fk425 test vector (t_sofa_c.c) and the expected
// output is SOFA's published FK5 J2000 answer for it. This function does not
// stop at FK5 — it goes on to the Hipparcos frame — so the comparison is not
// exact, and the size of the gap is itself the assertion: FK5 J2000 and the
// ICRS differ by a fixed rotation of about 25 mas, so anything under 0.1"
// confirms both stages ran and neither did something large and wrong.
//
// A conversion that skipped the FK5→Hipparcos step would land within a
// milliarcsecond, and one that skipped FK4→FK5 would be 0.7 degrees away.
func TestFK4ToICRSAgreesWithSOFAsPublishedVector(t *testing.T) {
	t.Parallel()

	fk4 := coord.NewFK4WithProperMotion(
		angle.Rad(0.07626899753879587532), angle.Rad(-1.137405378399605780),
		angle.Rad(0.1973749217849087460e-4), angle.Rad(0.5659714913272723189e-5),
		angle.Arcsec(0.134), 8.7,
	)

	got := coord.FK4ToICRS(fk4)

	// SOFA's published FK5 J2000 answer for the same input.
	wantFK5 := coord.NewICRS(angle.Rad(0.08757989933556446040), angle.Rad(-1.132279113042091895))

	sep := coord.Separation(got, wantFK5).Arcseconds()

	t.Logf("FK4 -> ICRS lands %.4f\" from SOFA's FK5 J2000 answer", sep)

	if sep > 0.1 {
		t.Errorf("separation from SOFA's FK5 answer is %.4f\", want under 0.1\" — "+
			"FK5 and the ICRS differ by about 25 mas, so this much means a stage "+
			"is missing or wrong", sep)
	}

	if sep < 1e-4 {
		t.Errorf("separation from SOFA's FK5 answer is %.6f\", which is too good: "+
			"the FK5 to Hipparcos rotation is real and about 25 mas, so landing "+
			"exactly on the FK5 answer means that stage did not run", sep)
	}

	// The kinematics have to survive too — a conversion that dropped them
	// would still place the star correctly today and drift afterwards.
	if got.PmRA() == angle.Zero() || got.PmDec() == angle.Zero() {
		t.Error("proper motion was lost in conversion")
	}

	// Parallax and radial velocity against SOFA's FK5 answers, with the
	// tolerance set by the second stage rather than guessed: FK5 to Hipparcos
	// perturbs the radial velocity in the sixth decimal (SOFA's own Fk52h
	// vector takes -7.6 to -7.6000000940000254), and leaves parallax alone.
	testutil.AssertNear(t, "parallax (arcsec)", got.Parallax().Arcseconds(), 0.1339919950582767871, 1e-9)
	testutil.AssertNear(t, "radial velocity (km/s)", got.RV(), 8.736999669183529069, 1e-5)
}

// TestFK4ToICRSMovesByPrecession pins the magnitude anyone would notice if
// this conversion were skipped entirely.
//
// Fifty years of general precession at about 50.29"/yr is close to 0.7°, and
// the shift varies across the sky between roughly a half and a full degree
// depending on where the star sits relative to the pole and equinox. A
// telescope pointed with a B1950 position as though it were J2000 misses by
// that much, which is far outside any field and reads as a pointing fault
// rather than a frame fault.
func TestFK4ToICRSMovesByPrecession(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		ra, dec  angle.Angle
		min, max float64 // degrees
	}{
		{"near the equator", angle.Hour(5), angle.Deg(0), 0.5, 0.9},
		{"mid-northern", angle.Hour(12), angle.Deg(45), 0.3, 0.9},
		{"southern", angle.Hour(18), angle.Deg(-30), 0.3, 0.9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fk4 := coord.NewFK4(tc.ra, tc.dec, coord.B1950)
			icrs := coord.FK4ToICRS(fk4)

			asIf := coord.NewICRS(tc.ra, tc.dec)
			moved := coord.Separation(icrs, asIf).Degrees()

			t.Logf("%s: B1950 -> ICRS moves %.4f deg", tc.name, moved)

			if moved < tc.min || moved > tc.max {
				t.Errorf("moved %.4f deg, want between %.1f and %.1f — fifty years of "+
					"precession is about 0.7 deg and this is the whole reason the "+
					"conversion exists", moved, tc.min, tc.max)
			}
		})
	}
}

// TestFK4RoundTripCloses checks that the two directions are inverses.
//
// The conversion is not a rotation — E-terms of aberration come out and go
// back in, and a fictitious proper motion is subtracted and restored — so
// closure is a real constraint rather than an algebraic identity. A sign error
// in either of those non-rotational parts breaks it while leaving each
// direction individually plausible.
func TestFK4RoundTripCloses(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		ra, dec angle.Angle
	}{
		{"equator", angle.Hour(5), angle.Deg(0)},
		{"north", angle.Hour(12), angle.Deg(60)},
		{"south", angle.Hour(20), angle.Deg(-60)},
		{"near the RA wrap", angle.Deg(359.9), angle.Deg(10)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			start := coord.NewFK4(tc.ra, tc.dec, coord.B1950)

			icrs := coord.FK4ToICRS(start)
			back := coord.ICRSToFK4(icrs, coord.B1950)

			sep := coord.Separation(
				coord.NewICRS(start.RA(), start.Dec()),
				coord.NewICRS(back.RA(), back.Dec()),
			).Arcseconds()

			t.Logf("%s: round trip closes to %.6f\"", tc.name, sep)

			// A milliarcsecond. Not a physical bound — the transformation is
			// analytic and should close far tighter — but tight enough that
			// any dropped term shows, and loose enough not to trip on the
			// E-term removal's own iteration.
			if sep > 1e-3 {
				t.Errorf("round trip is off by %.6f\", want under 0.001\"", sep)
			}
		})
	}
}

// TestFK4PositionOnlyIsNotZeroProperMotion is the error-vs-absence case, and
// the reason this package has two constructors instead of one with defaults.
//
// "The catalogue recorded no proper motion" and "the proper motion is zero"
// are different statements, and they convert to different places. FK4's
// equinox drifts, so a star at rest *in FK4* has a real proper motion in FK5;
// SOFA supplies that fictitious component through Fk45z. Passing zeroes to the
// six-element routine instead asserts the star is at rest in the inertial
// sense, which is a claim almost no old catalogue makes.
//
// The two answers differ by enough to matter, which is what makes the API
// distinction worth having rather than pedantry.
func TestFK4PositionOnlyIsNotZeroProperMotion(t *testing.T) {
	t.Parallel()

	ra, dec := angle.Hour(5), angle.Deg(20)

	recorded := coord.FK4ToICRS(coord.NewFK4(ra, dec, coord.B1950))

	assertedZero := coord.FK4ToICRS(coord.NewFK4WithProperMotion(
		ra, dec, angle.Zero(), angle.Zero(), angle.Zero(), 0))

	sep := coord.Separation(recorded, assertedZero).Arcseconds()

	t.Logf("position-only vs asserted-zero proper motion: %.3f\" apart", sep)

	// Measured across the sky on a 2h by 20° grid: 0.037" to 0.249", and
	// 0.064" at this position. The size is what it should be — FK4's equinox
	// drift is a few tenths of an arcsecond per century and B1950 to J2000 is
	// half of one — so it is small, real, and not a rounding difference.
	//
	// Small enough to argue about, which is the reason for the bounds rather
	// than a bare "they differ": a floor of 0.01" keeps the distinction
	// observable, and a ceiling of 1" catches one of the two paths going
	// somewhere else entirely. My first draft guessed 0.5" and was wrong by
	// an order of magnitude, which is how the grid came to be measured.
	if sep < 0.01 || sep > 1 {
		t.Errorf("the two readings of \"no proper motion\" differ by %.3f\", want "+
			"between 0.01\" and 1\" — the fictitious motion over fifty years is "+
			"tens of milliarcseconds, so far outside that band one of the two "+
			"paths is wrong", sep)
	}
}

// TestFK4ProperMotionReportsWhetherItWasRecorded covers the accessor that
// carries the distinction outward, so a caller reading an FK4 back can tell
// which of the two it holds.
func TestFK4ProperMotionReportsWhetherItWasRecorded(t *testing.T) {
	t.Parallel()

	_, _, ok := coord.NewFK4(angle.Hour(1), angle.Deg(2), coord.B1950).ProperMotion()
	if ok {
		t.Error("a position-only FK4 claims a recorded proper motion")
	}

	pmRA, pmDec, ok := coord.NewFK4WithProperMotion(
		angle.Hour(1), angle.Deg(2),
		angle.Arcsec(0.5), angle.Arcsec(-0.25), angle.Arcsec(0.01), 12,
	).ProperMotion()

	if !ok {
		t.Fatal("a six-element FK4 reports no recorded proper motion")
	}

	testutil.AssertNear(t, "pmRA (arcsec/yr)", pmRA.Arcseconds(), 0.5, 1e-12)
	testutil.AssertNear(t, "pmDec (arcsec/yr)", pmDec.Arcseconds(), -0.25, 1e-12)
}

// TestFK4EpochIsCarried covers the Besselian epoch, which is not always 1950
// even in a B1950 catalogue: several plate surveys record positions for the
// epoch of the plate and the equinox of B1950, and Fk45z takes both.
func TestFK4EpochIsCarried(t *testing.T) {
	t.Parallel()

	ra, dec := angle.Hour(9), angle.Deg(-15)

	atEquinox := coord.FK4ToICRS(coord.NewFK4(ra, dec, coord.B1950))
	atPlate := coord.FK4ToICRS(coord.NewFK4(ra, dec, 1975.0))

	sep := coord.Separation(atEquinox, atPlate).Arcseconds()

	t.Logf("the same B1950 position at epoch 1950 vs 1975 lands %.3f\" apart", sep)

	if sep == 0 {
		t.Error("the epoch of observation had no effect; it is an input to the " +
			"conversion, not decoration")
	}

	if c := coord.NewFK4(ra, dec, 1975.0); c.Epoch() != 1975.0 {
		t.Errorf("Epoch() = %v, want 1975", c.Epoch())
	}

	// And the same in reverse. The inverse takes the epoch as an argument
	// rather than reading it off a struct, which is exactly the shape that
	// invites the argument being accepted and ignored.
	icrs := coord.NewICRS(ra, dec)

	toEquinox := coord.ICRSToFK4(icrs, coord.B1950)
	toPlate := coord.ICRSToFK4(icrs, 1975.0)

	back := coord.Separation(
		coord.NewICRS(toEquinox.RA(), toEquinox.Dec()),
		coord.NewICRS(toPlate.RA(), toPlate.Dec()),
	).Arcseconds()

	t.Logf("ICRS -> FK4 at epoch 1950 vs 1975 lands %.3f\" apart", back)

	if back == 0 {
		t.Error("ICRSToFK4 ignored its bepoch argument")
	}

	if toEquinox.Epoch() != coord.B1950 || toPlate.Epoch() != 1975.0 {
		t.Errorf("the returned FK4 does not carry the epoch it was built for: %v and %v",
			toEquinox.Epoch(), toPlate.Epoch())
	}
}

// TestFK4StringNamesTheFrameAndEpoch keeps the rendered form unambiguous. A
// coordinate printed without its frame is the thing this whole file exists to
// prevent someone mistaking.
func TestFK4StringNamesTheFrameAndEpoch(t *testing.T) {
	t.Parallel()

	s := coord.NewFK4(angle.Hour(5.5), angle.Deg(-5.4), coord.B1950).String()

	for _, want := range []string{"FK4", "B1950"} {
		if !strings.Contains(s, want) {
			t.Errorf("String() = %q, want it to contain %q", s, want)
		}
	}
}

// TestICRSToFK4RestoresTheFictitiousProperMotion pins the direction that
// surprises people: converting a star at rest in the ICRS into FK4 gives it a
// proper motion.
//
// That is not an artefact. FK4's equinox drifts, so a genuinely inertial
// object appears to move in it, and the returned proper motion is what a
// B1950 catalogue would have had to record to describe the same star.
func TestICRSToFK4RestoresTheFictitiousProperMotion(t *testing.T) {
	t.Parallel()

	icrs := coord.NewICRS(angle.Hour(15), angle.Deg(30))

	fk4 := coord.ICRSToFK4(icrs, coord.B1950)

	pmRA, pmDec, ok := fk4.ProperMotion()
	if !ok {
		t.Fatal("the result reports no proper motion; FK4's drifting equinox gives " +
			"even a fixed star one")
	}

	if pmRA == angle.Zero() && pmDec == angle.Zero() {
		t.Error("both components are exactly zero, which a drifting equinox does not produce")
	}

	// Order of magnitude: FK4's equinox drift is a few tenths of an arcsecond
	// per century, so per year this is tens of milliarcseconds at most.
	for _, c := range []struct {
		name string
		v    angle.Angle
	}{{"pmRA", pmRA}, {"pmDec", pmDec}} {
		if a := math.Abs(c.v.Arcseconds()); a > 0.1 {
			t.Errorf("%s = %.6f\"/yr, which is far larger than the equinox drift "+
				"that produces it", c.name, a)
		}
	}
}

// TestFK4RoundTripWithKinematicsCloses exercises the six-element inverse,
// which the position-only round trip never reaches.
//
// ICRSToFK4 has two branches and they do different work: a position with no
// kinematics goes through Hfk5z/Fk54z, and one carrying proper motion, parallax
// and radial velocity goes through H2fk5/Fk524. Only the first was covered, so
// half the function — the half that restores the E-terms of aberration and the
// fictitious proper motion for a *moving* star — was never run.
//
// Closure over all six elements is the assertion. Position closing alone would
// pass with the proper motion mangled, and a star whose position is right today
// and whose motion is wrong is a star that drifts away.
func TestFK4RoundTripWithKinematicsCloses(t *testing.T) {
	t.Parallel()

	// SOFA's own Fk425 input vector: a real star's worth of proper motion,
	// parallax and radial velocity rather than round numbers, so a term that
	// happens to vanish for a tidy input cannot hide.
	start := coord.NewFK4WithProperMotion(
		angle.Rad(0.07626899753879587532), angle.Rad(-1.137405378399605780),
		angle.Rad(0.1973749217849087460e-4), angle.Rad(0.5659714913272723189e-5),
		angle.Arcsec(0.134), 8.7,
	)

	icrs := coord.FK4ToICRS(start)
	back := coord.ICRSToFK4(icrs, coord.B1950)

	sep := coord.Separation(
		coord.NewICRS(start.RA(), start.Dec()),
		coord.NewICRS(back.RA(), back.Dec()),
	).Arcseconds()

	t.Logf("six-element round trip closes to %.6f\" in position", sep)

	if sep > 1e-3 {
		t.Errorf("position closes to only %.6f\", want under 0.001\"", sep)
	}

	startPmRA, startPmDec, ok := start.ProperMotion()
	if !ok {
		t.Fatal("the fixture reports no recorded proper motion")
	}

	backPmRA, backPmDec, ok := back.ProperMotion()
	if !ok {
		t.Fatal("the six-element inverse dropped the proper motion")
	}

	// Proper motions in milliarcseconds per year, where a microarcsecond per
	// year is a thousandth of the tolerance and the values themselves are of
	// order 4000 and 1200 mas/yr.
	testutil.AssertNear(t, "pmRA (mas/yr)",
		backPmRA.Arcseconds()*1000, startPmRA.Arcseconds()*1000, 1e-3)
	testutil.AssertNear(t, "pmDec (mas/yr)",
		backPmDec.Arcseconds()*1000, startPmDec.Arcseconds()*1000, 1e-3)

	// Parallax and radial velocity round-trip through both stages too. They
	// are perturbed on the way out — Fk425 changes the parallax in its fourth
	// decimal and the RV in its second — so closure here is a real constraint
	// rather than two untouched values coming back.
	testutil.AssertNear(t, "parallax (arcsec)", back.Parallax().Arcseconds(), 0.134, 1e-9)
	testutil.AssertNear(t, "radial velocity (km/s)", back.RV(), 8.7, 1e-6)
}

// TestFK4CarriesTheKinematicsItWasGiven covers the accessors that carry the
// two elements the position-only path leaves at zero, so a caller reading an
// FK4 back gets what the catalogue recorded rather than what survived a
// conversion.
func TestFK4CarriesTheKinematicsItWasGiven(t *testing.T) {
	t.Parallel()

	c := coord.NewFK4WithProperMotion(
		angle.Hour(6), angle.Deg(-16),
		angle.Arcsec(-0.546), angle.Arcsec(-1.223),
		angle.Arcsec(0.379), -7.6,
	)

	testutil.AssertNear(t, "parallax (arcsec)", c.Parallax().Arcseconds(), 0.379, 1e-12)
	testutil.AssertNear(t, "radial velocity (km/s)", c.RV(), -7.6, 1e-12)

	// And a position-only FK4 reports neither, rather than a plausible zero
	// that a caller could mistake for a measurement of zero.
	empty := coord.NewFK4(angle.Hour(6), angle.Deg(-16), coord.B1950)

	if empty.Parallax() != angle.Zero() || empty.RV() != 0 {
		t.Errorf("a position-only FK4 reports parallax %v and RV %v; it was given neither",
			empty.Parallax(), empty.RV())
	}
}
