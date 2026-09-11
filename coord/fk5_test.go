package coord_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
)

// FK5 and ICRS agree to about 20 milliarcseconds, which is why they are easy
// to conflate and why conflating them costs exactly that much. The values below
// are SOFA's own, from the published tests for iauFk52h and iauFk5hz, so these
// check the transformation rather than checking a round trip against itself.

// masTolerance is a tenth of a milliarcsecond, well inside the ~20 mas the
// transformation is about and well outside float noise.
const masTolerance = 1e-4 / 3600 * math.Pi / 180

// TestFK5ToICRSMatchesSOFAWithKinematics is the six-element case, against
// iauFk52h's published result.
func TestFK5ToICRSMatchesSOFAWithKinematics(t *testing.T) {
	t.Parallel()

	// SOFA's inputs, in radians, radians/year, arcseconds and km/s.
	src := coord.NewFK5WithProperMotion(
		angle.Rad(1.76779433),
		angle.Rad(-0.2917517103),
		angle.Rad(-1.91851572e-7),
		angle.Rad(-5.8468475e-6),
		angle.Arcsec(0.379210),
		-7.6,
	)

	got := coord.FK5ToICRS(src)

	// iauFk52h -> ra 1.767794226299947632, dec -0.2917516070530391757
	assertRadians(t, got.RA().Radians(), 1.767794226299947632, "RA")
	assertRadians(t, got.Dec().Radians(), -0.2917516070530391757, "Dec")

	// The kinematics come through too, or a caller loses the space motion the
	// catalogue recorded.
	assertClose(t, got.PmRA().Radians(), -0.1961874125605721270e-6, 1e-18, "pmRA")
	assertClose(t, got.PmDec().Radians(), -0.58459905176693911e-5, 1e-18, "pmDec")
	assertClose(t, got.Parallax().Arcseconds(), 0.37921, 1e-12, "parallax")

	// Not the -7.6 that went in. The rotation acts on the whole space-motion
	// vector, so a little of the transverse motion lands in the radial
	// component; SOFA publishes -7.6000000940000254 for exactly that reason.
	// Asserting the input here would have passed for a transformation that
	// copied the field across and skipped the rotation.
	assertClose(t, got.RV(), -7.6000000940000254, 1e-11, "RV")
}

// TestFK5ToICRSMatchesSOFAPositionOnly is the other constructor, against
// iauFk5hz, and is the one where the epoch matters.
func TestFK5ToICRSMatchesSOFAPositionOnly(t *testing.T) {
	t.Parallel()

	// SOFA evaluates at JD 2400000.5 + 54479.0, which is MJD 54479 — Julian
	// epoch 2008.0189...; the epoch is passed as such here and converted back
	// inside, so this also pins that conversion.
	const mjd = 54479.0

	jepoch := 2000.0 + ((2400000.5+mjd)-2451545.0)/365.25

	src := coord.NewFK5(angle.Rad(1.76779433), angle.Rad(-0.2917517103), jepoch)

	got := coord.FK5ToICRS(src)

	// iauFk5hz -> ra 1.767794191464423978, dec -0.2917516001679884419
	assertRadians(t, got.RA().Radians(), 1.767794191464423978, "RA")
	assertRadians(t, got.Dec().Radians(), -0.2917516001679884419, "Dec")
}

// TestFK5DiffersFromICRSByAboutTwentyMilliarcseconds states the size of the
// thing, because "FK5 is basically ICRS" is true right up to the precision
// this library claims.
func TestFK5DiffersFromICRSByAboutTwentyMilliarcseconds(t *testing.T) {
	t.Parallel()

	src := coord.NewFK5(angle.Deg(101.2871), angle.Deg(-16.7161), coord.J2000Epoch) // Sirius

	got := coord.FK5ToICRS(src)

	sep := coord.Separation(
		coord.NewICRS(src.RA(), src.Dec()),
		got,
	)

	mas := sep.Arcseconds() * 1000

	if mas < 1 || mas > 100 {
		t.Errorf("FK5 to ICRS moved the position by %.2f mas, want the tens of mas "+
			"the frame bias is.\n"+
			"  Far less would mean the transformation is doing nothing; far more "+
			"would mean it is doing something else.", mas)
	}
}

// TestFK5RoundTripsThroughICRS covers the inverse, and is written so that a
// transformation doing nothing would fail it: the position-only form comes back
// carrying the proper motion FK5's spin gives it, which a no-op cannot produce.
func TestFK5RoundTripsThroughICRS(t *testing.T) {
	t.Parallel()

	src := coord.NewFK5WithProperMotion(
		angle.Deg(101.2871), angle.Deg(-16.7161),
		angle.Arcsec(-0.5460), angle.Arcsec(-1.2231),
		angle.Arcsec(0.37921), -5.5,
	)

	back := coord.ICRSToFK5(coord.FK5ToICRS(src), coord.J2000Epoch)

	assertRadians(t, back.RA().Radians(), src.RA().Radians(), "RA round trip")
	assertRadians(t, back.Dec().Radians(), src.Dec().Radians(), "Dec round trip")

	pmRA, pmDec, ok := back.ProperMotion()
	if !ok {
		t.Fatal("the round trip lost the recorded proper motion")
	}

	assertClose(t, pmRA.Arcseconds(), -0.5460, 1e-6, "pmRA round trip")
	assertClose(t, pmDec.Arcseconds(), -1.2231, 1e-6, "pmDec round trip")
}

// TestICRSToFK5MatchesSOFAWithKinematics covers the inverse against iauH2fk5's
// published result.
func TestICRSToFK5MatchesSOFAWithKinematics(t *testing.T) {
	t.Parallel()

	src := coord.NewICRSWithKinematics(
		angle.Rad(1.767794352),
		angle.Rad(-0.2917512594),
		angle.Rad(-2.76413026e-6),
		angle.Rad(-5.92994449e-6),
		angle.Arcsec(0.379210),
		-7.6,
	)

	got := coord.ICRSToFK5(src, coord.J2000Epoch)

	pmRA, pmDec, ok := got.ProperMotion()
	if !ok {
		t.Fatal("the six-element conversion came back with no recorded proper motion")
	}

	// iauH2fk5 -> ra 1.767794455700065506, dec -0.2917513626469638890
	assertRadians(t, got.RA().Radians(), 1.767794455700065506, "RA")
	assertRadians(t, got.Dec().Radians(), -0.2917513626469638890, "Dec")

	assertClose(t, pmRA.Radians(), -0.27597945024511204e-5, 1e-18, "pmRA")
	assertClose(t, pmDec.Radians(), -0.59308014093262838e-5, 1e-18, "pmDec")
	assertClose(t, got.Parallax().Arcseconds(), 0.37921, 1e-13, "parallax")
	assertClose(t, got.RV(), -7.6000001309071126, 1e-11, "RV")
}

// TestICRSToFK5SuppliesTheSpinMotion pins the claim [NewFK5] makes: a star at
// rest in ICRS is not at rest in FK5, so converting a position with no
// kinematics returns one that has them — and the values are iauHfk5z's own,
// not merely "something nonzero".
func TestICRSToFK5SuppliesTheSpinMotion(t *testing.T) {
	t.Parallel()

	// SOFA evaluates at JD 2400000.5 + 54479.0; the epoch goes in as the
	// Julian epoch that is, so this pins the conversion as well.
	const mjd = 54479.0

	jepoch := 2000.0 + ((2400000.5+mjd)-2451545.0)/365.25

	got := coord.ICRSToFK5(coord.NewICRS(angle.Rad(1.767794352), angle.Rad(-0.2917512594)), jepoch)

	pmRA, pmDec, ok := got.ProperMotion()
	if !ok {
		t.Fatal("no proper motion was supplied; a star at rest in ICRS is not at rest in FK5")
	}

	// iauHfk5z -> ra 1.767794490535581026, dec -0.2917513695320114258
	assertRadians(t, got.RA().Radians(), 1.767794490535581026, "RA")
	assertRadians(t, got.Dec().Radians(), -0.2917513695320114258, "Dec")

	// The spin, in full. About 0.9 mas/yr in RA here — small, and an order of
	// magnitude larger than the 0.1 mas this library's other tests resolve, so
	// "too small to matter" is not available as an excuse for dropping it.
	assertClose(t, pmRA.Radians(), 0.4335890983539243029e-8, 1e-22, "pmRA")
	assertClose(t, pmDec.Radians(), -0.8569648841237745902e-9, 1e-23, "pmDec")

	if pmRA == angle.Zero() && pmDec == angle.Zero() {
		t.Error("the supplied proper motion is exactly zero, which is the answer " +
			"a transformation that did nothing would give")
	}
}

// assertRadians compares two angles to a tenth of a milliarcsecond.
func assertRadians(t *testing.T, got, want float64, what string) {
	t.Helper()

	if diff := math.Abs(got - want); diff > masTolerance {
		t.Errorf("%s = %.15f rad, want %.15f (differ by %.4f mas)",
			what, got, want, diff*180/math.Pi*3600*1000)
	}
}

// assertClose compares two numbers to an explicit tolerance.
func assertClose(t *testing.T, got, want, tol float64, what string) {
	t.Helper()

	if diff := math.Abs(got - want); diff > tol {
		t.Errorf("%s = %g, want %g (differ by %g)", what, got, want, diff)
	}
}
