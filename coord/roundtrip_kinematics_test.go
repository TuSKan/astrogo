package coord_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
)

// Round trips over all six elements, for every conversion between the three
// frames that carry kinematics.
//
// # Why this exists as a matrix rather than as more single-star tests
//
// #278 was a proper motion wrong by 0.6 to 0.9 mas/yr in ICRS → FK4, and it
// shipped because every round-trip test that covered that path compared
// *positions*. The position closed to 19 microarcseconds throughout. A test
// that checks one element cannot find a defect in another, and the elements
// are not independent: the FK4 conversions mix position and velocity through a
// 6x6 matrix, so a sign error in the velocity block moves the position and a
// sign error in the position block moves the velocity.
//
// The frames that carry kinematics are ICRS, FK5 and FK4, giving six directed
// conversions. Each is exercised over a grid of sky positions crossed with a
// range of kinematic profiles, and every round trip asserts all six elements.
//
// # What the grid is for
//
// A single well-chosen star is what the existing tests use, and it is good for
// checking against a published vector. It is poor at finding terms that vanish
// for particular inputs: a declination-dependent term is invisible at the
// equator, an RA-dependent one at RA zero, and anything multiplied by the
// proper motion is invisible for a star at rest. The grid exists so that no
// single value of anything is the only one tried.

// kinematicStar is one row of the grid: a position crossed with a motion.
type kinematicStar struct {
	name        string
	ra, dec     angle.Angle
	pmRA, pmDec angle.Angle // mu_alpha* and mu_delta, per Julian year
	parallax    angle.Angle
	rv          float64 // km/s
}

// skyGrid spreads the positions over the cases that hide terms.
var skyGrid = []struct {
	name    string
	ra, dec angle.Angle
}{
	{"origin", angle.Deg(0), angle.Deg(0)},
	{"equator, mid RA", angle.Deg(123.4), angle.Deg(0)},
	{"north mid", angle.Deg(88.79), angle.Deg(7.41)},
	{"north high", angle.Deg(45), angle.Deg(78)},
	{"south mid", angle.Deg(201.3), angle.Deg(-43.1)},
	{"south high", angle.Deg(310.7), angle.Deg(-81.2)},
	{"just under the RA wrap", angle.Deg(359.99), angle.Deg(10)},
	{"just over the RA wrap", angle.Deg(0.01), angle.Deg(-10)},
}

// motionGrid spreads the kinematics over the magnitudes that occur, including
// the two that make terms vanish: no motion at all, and no parallax.
var motionGrid = []struct {
	name        string
	pmRA, pmDec angle.Angle
	parallax    angle.Angle
	rv          float64
}{
	{
		// At rest but at a known distance, which is an ordinary catalogue
		// entry. A parallax of zero is excluded from this grid for the reason
		// given below it.
		name:     "at rest, with parallax",
		parallax: angle.Arcsec(0.020),
	},
	{
		name:     "typical field star",
		pmRA:     angle.Arcsec(0.020),
		pmDec:    angle.Arcsec(-0.035),
		parallax: angle.Arcsec(0.012),
		rv:       -22.4,
	},
	{
		// Barnard's Star, the largest proper motion known, and a parallax
		// large enough that the light-time and perspective terms are not
		// rounding.
		name:     "Barnard's Star scale",
		pmRA:     angle.Arcsec(-0.79847),
		pmDec:    angle.Arcsec(10.33777),
		parallax: angle.Arcsec(0.54698),
		rv:       -110.6,
	},
	{
		name:     "negative in both components",
		pmRA:     angle.Arcsec(-0.310),
		pmDec:    angle.Arcsec(-0.480),
		parallax: angle.Arcsec(0.089),
		rv:       +45.0,
	},
	{
		// A large motion at a distance, where the transverse velocity is high
		// and the perspective terms are not rounding.
		//
		// The parallax is bounded below on purpose. Proper motion and parallax
		// are not independent: 270 mas/yr at a parallax of 0.0005 arcsec is a
		// transverse velocity of about 2,560 km/s, which no star has, and which
		// pushes Starpv's relativistic solution far enough that the round trip
		// carries a few centimetres per second of radial velocity. 0.005 arcsec
		// puts it at roughly 256 km/s — a fast halo star, and a real one.
		name:     "large motion at distance",
		pmRA:     angle.Arcsec(0.150),
		pmDec:    angle.Arcsec(0.220),
		parallax: angle.Arcsec(0.005),
		rv:       -180.0,
	},
}

// A parallax of zero is deliberately absent from this grid.
//
// The six-element conversions go through a space-motion pv-vector, which needs
// a finite distance, and SOFA clamps any parallax below 1e-7 arcsec. Neither
// proper motion nor radial velocity survives a round trip below that clamp —
// and it is not only the motion that is affected: a star declared *at rest*
// with no parallax comes back with several mas/yr it never had. That is a
// limitation of the representation rather than a property to assert here, so it
// is pinned by TestSixElementConversionsNeedAParallax.

// kinematicGrid is the cross product, built once.
func kinematicGrid() []kinematicStar {
	out := make([]kinematicStar, 0, len(skyGrid)*len(motionGrid))

	for _, p := range skyGrid {
		for _, m := range motionGrid {
			out = append(out, kinematicStar{
				name:     p.name + " / " + m.name,
				ra:       p.ra,
				dec:      p.dec,
				pmRA:     m.pmRA,
				pmDec:    m.pmDec,
				parallax: m.parallax,
				rv:       m.rv,
			})
		}
	}

	return out
}

// elements is the six-element state, normalised so the three frames can be
// compared without caring which type they came from.
type elements struct {
	ra, dec     angle.Angle
	pmRA, pmDec angle.Angle
	parallax    angle.Angle
	rv          float64
}

func icrsElements(c coord.ICRS) elements {
	return elements{
		ra: c.RA(), dec: c.Dec(),
		pmRA: c.PmRA(), pmDec: c.PmDec(),
		parallax: c.Parallax(), rv: c.RV(),
	}
}

func fk5Elements(c coord.FK5) elements {
	pmRA, pmDec, _ := c.ProperMotion()

	return elements{
		ra: c.RA(), dec: c.Dec(),
		pmRA: pmRA, pmDec: pmDec,
		parallax: c.Parallax(), rv: c.RV(),
	}
}

func fk4Elements(c coord.FK4) elements {
	pmRA, pmDec, _ := c.ProperMotion()

	return elements{
		ra: c.RA(), dec: c.Dec(),
		pmRA: pmRA, pmDec: pmDec,
		parallax: c.Parallax(), rv: c.RV(),
	}
}

// tolerances for a round trip through two analytic conversions.
//
// These are not physical bounds. Each conversion is an exact matrix operation
// plus, for FK4, an E-term removal that iterates; a correct round trip closes
// far tighter than this. They are set well below the size of any real defect
// and well above the observed residual, so the band between them is empty and
// a regression cannot hide in it.
//
// The FK4 pair is looser because the E-terms of aberration are removed and
// restored by an iteration rather than inverted exactly.
const (
	tolPosArcsec  = 1e-4 // 0.1 mas
	tolPMMasPerYr = 1e-3 // a microarcsecond a year
	tolRVKmPerS   = 1e-6
	tolPosFK4     = 1e-3 // 1 mas
	tolPMFK4MasYr = 1e-2 // 10 microarcsec a year
	tolRVFK4      = 1e-5

	// The parallax tolerance is SOFA's floor, not a numerical one.
	//
	// Starpv clamps any parallax below PXMIN = 1e-7 arcsec to that value,
	// because the space-motion vector it builds needs a finite distance. A
	// parallax of exactly zero therefore comes back as 1e-7 arcsec — an
	// absolute error equal to the clamp, on every conversion that goes through
	// a pv-vector. Anything above the clamp round-trips to machine precision.
	tolParallaxAs  = 2e-7
	tolParallaxFK4 = 2e-7
)

// assertClosed compares two six-element states and reports every element that
// moved, rather than the first — a conversion that breaks one element usually
// breaks more, and seeing which ones is what localises the term.
func assertClosed(t *testing.T, what string, got, want elements, tol elementTolerance) {
	t.Helper()

	sep := coord.Separation(
		coord.NewICRS(want.ra, want.dec),
		coord.NewICRS(got.ra, got.dec),
	).Arcseconds()

	if sep > tol.pos {
		t.Errorf("%s: position closes to %.3e arcsec, want under %.0e", what, sep, tol.pos)
	}

	if d := math.Abs((got.pmRA.Arcseconds() - want.pmRA.Arcseconds()) * 1000); d > tol.pm {
		t.Errorf("%s: pmRA off by %.3e mas/yr, want under %.0e", what, d, tol.pm)
	}

	if d := math.Abs((got.pmDec.Arcseconds() - want.pmDec.Arcseconds()) * 1000); d > tol.pm {
		t.Errorf("%s: pmDec off by %.3e mas/yr, want under %.0e", what, d, tol.pm)
	}

	if d := math.Abs(got.parallax.Arcseconds() - want.parallax.Arcseconds()); d > tol.parallax {
		t.Errorf("%s: parallax off by %.3e arcsec, want under %.0e", what, d, tol.parallax)
	}

	if d := math.Abs(got.rv - want.rv); d > tol.rv {
		t.Errorf("%s: radial velocity off by %.3e km/s, want under %.0e", what, d, tol.rv)
	}
}

type elementTolerance struct {
	pos, pm, parallax, rv float64
}

var (
	tolInertial = elementTolerance{tolPosArcsec, tolPMMasPerYr, tolParallaxAs, tolRVKmPerS}
	tolFK4      = elementTolerance{tolPosFK4, tolPMFK4MasYr, tolParallaxFK4, tolRVFK4}
)

// TestICRSFK5RoundTripsAllSixElements covers the pair that is a pure frame
// rotation plus FK5's spin — no E-terms, so it should close tightest.
func TestICRSFK5RoundTripsAllSixElements(t *testing.T) {
	t.Parallel()

	for _, star := range kinematicGrid() {
		t.Run(star.name, func(t *testing.T) {
			t.Parallel()

			start := coord.NewICRSWithKinematics(
				star.ra, star.dec, star.pmRA, star.pmDec, star.parallax, star.rv)

			back := coord.FK5ToICRS(coord.ICRSToFK5(start, coord.J2000Epoch))

			assertClosed(t, "ICRS -> FK5 -> ICRS",
				icrsElements(back), icrsElements(start), tolInertial)
		})
	}
}

// TestFK5ICRSRoundTripsAllSixElements is the same pair started from the other
// side, because a conversion can be its own inverse in one direction and not
// the other when a term is applied rather than removed.
func TestFK5ICRSRoundTripsAllSixElements(t *testing.T) {
	t.Parallel()

	for _, star := range kinematicGrid() {
		t.Run(star.name, func(t *testing.T) {
			t.Parallel()

			start := coord.NewFK5WithProperMotion(
				star.ra, star.dec, star.pmRA, star.pmDec, star.parallax, star.rv)

			back := coord.ICRSToFK5(coord.FK5ToICRS(start), coord.J2000Epoch)

			assertClosed(t, "FK5 -> ICRS -> FK5",
				fk5Elements(back), fk5Elements(start), tolInertial)
		})
	}
}

// TestICRSFK4RoundTripsAllSixElements is the pair #278 was found in, now over
// the whole grid rather than one star.
func TestICRSFK4RoundTripsAllSixElements(t *testing.T) {
	t.Parallel()

	for _, star := range kinematicGrid() {
		t.Run(star.name, func(t *testing.T) {
			t.Parallel()

			start := coord.NewICRSWithKinematics(
				star.ra, star.dec, star.pmRA, star.pmDec, star.parallax, star.rv)

			back := coord.FK4ToICRS(coord.ICRSToFK4(start, coord.B1950))

			assertClosed(t, "ICRS -> FK4 -> ICRS",
				icrsElements(back), icrsElements(start), tolFK4)
		})
	}
}

// TestFK4ICRSRoundTripsAllSixElements starts from FK4, which is the direction
// a catalogue reader actually takes.
func TestFK4ICRSRoundTripsAllSixElements(t *testing.T) {
	t.Parallel()

	for _, star := range kinematicGrid() {
		t.Run(star.name, func(t *testing.T) {
			t.Parallel()

			start := coord.NewFK4WithProperMotion(
				star.ra, star.dec, star.pmRA, star.pmDec, star.parallax, star.rv)

			back := coord.ICRSToFK4(coord.FK4ToICRS(start), coord.B1950)

			assertClosed(t, "FK4 -> ICRS -> FK4",
				fk4Elements(back), fk4Elements(start), tolFK4)
		})
	}
}

// TestFK5FK4RoundTripsAllSixElements covers the pair that never goes through
// ICRS, which is the one with no test at all before this.
func TestFK5FK4RoundTripsAllSixElements(t *testing.T) {
	t.Parallel()

	for _, star := range kinematicGrid() {
		t.Run(star.name, func(t *testing.T) {
			t.Parallel()

			start := coord.NewFK5WithProperMotion(
				star.ra, star.dec, star.pmRA, star.pmDec, star.parallax, star.rv)

			back := coord.FK4ToFK5(coord.FK5ToFK4(start, coord.B1950))

			assertClosed(t, "FK5 -> FK4 -> FK5",
				fk5Elements(back), fk5Elements(start), tolFK4)
		})
	}
}

// TestFK4FK5RoundTripsAllSixElements is that pair from the other side.
func TestFK4FK5RoundTripsAllSixElements(t *testing.T) {
	t.Parallel()

	for _, star := range kinematicGrid() {
		t.Run(star.name, func(t *testing.T) {
			t.Parallel()

			start := coord.NewFK4WithProperMotion(
				star.ra, star.dec, star.pmRA, star.pmDec, star.parallax, star.rv)

			back := coord.FK5ToFK4(coord.FK4ToFK5(start), coord.B1950)

			assertClosed(t, "FK4 -> FK5 -> FK4",
				fk4Elements(back), fk4Elements(start), tolFK4)
		})
	}
}

// TestSixElementConversionsNeedAParallax pins a limitation rather than a
// contract, because it is a trap and silence about it is worse than the trap.
//
// SOFA's six-element conversions go through a space-motion pv-vector, which
// needs a distance. Starpv clamps any parallax below PXMIN = 1e-7 arcsec to
// that value — about 10 Mpc — and then caps the resulting speed at VMAX = 0.5c.
// A star with a real proper motion and no recorded parallax exceeds that cap by
// orders of magnitude: 150 mas/yr at 10 Mpc is some 24c. The velocity is
// clamped, and the proper motion does not survive.
//
// SOFA reports this: Starpv returns a status. iauH2fk5 is void and discards it,
// so nothing downstream can see it either, which is why the loss is silent.
//
// This matters because catalogues of exactly this shape exist — proper motion
// measured, parallax not — and a caller converting one frame to another would
// find the motion gone with nothing said. Measured here so the threshold is
// known: see #331.
func TestSixElementConversionsNeedAParallax(t *testing.T) {
	t.Parallel()

	const (
		pmRAIn  = 150.0 // mas/yr
		pmDecIn = 220.0
	)

	for _, tc := range []struct {
		parallax  float64 // arcsec
		preserved bool
	}{
		{0, false},
		{1e-9, false},
		{1e-7, false}, // exactly PXMIN, still far too distant for this motion
		{1e-6, false},
		{1e-4, true},
		{1e-2, true},
	} {
		start := coord.NewICRSWithKinematics(
			angle.Deg(123.4), angle.Deg(0),
			angle.Arcsec(pmRAIn/1000), angle.Arcsec(pmDecIn/1000),
			angle.Arcsec(tc.parallax), 0)

		back := coord.FK5ToICRS(coord.ICRSToFK5(start, coord.J2000Epoch))

		gotRA := back.PmRA().Arcseconds() * 1000
		gotDec := back.PmDec().Arcseconds() * 1000

		survived := math.Abs(gotRA-pmRAIn) < 1 && math.Abs(gotDec-pmDecIn) < 1

		t.Logf("parallax %-8g -> proper motion (%7.1f, %7.1f) mas/yr, survived=%v",
			tc.parallax, gotRA, gotDec, survived)

		if survived != tc.preserved {
			t.Errorf("parallax %g: proper motion survived=%v, want %v — the threshold "+
				"this test pins has moved, which means SOFA's clamp changed or a guard "+
				"was added; update the documentation with it",
				tc.parallax, survived, tc.preserved)
		}
	}
}

// TestAtRestWithoutParallaxGainsMotion is the other half of the same
// limitation, and the more surprising one.
//
// A star declared at rest in FK4 with no parallax is not merely *left* without
// motion by the six-element round trip — it comes back with several
// milliarcseconds a year it never had. The clamp that makes the pv-vector
// representable puts the star at 10 Mpc, where FK4's fictitious proper motion
// no longer inverts, so the motion removed on the way out is not the motion
// restored on the way back.
//
// A caller reading an old proper-motion catalogue — measured motion, no
// parallax, which describes a great many of them — is exactly who hits this.
// See #331.
func TestAtRestWithoutParallaxGainsMotion(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		parallax float64 // arcsec
		clean    bool
	}{
		{0, false},
		{1e-7, false},
		{1e-3, true},
		{2e-2, true},
	} {
		start := coord.NewFK4WithProperMotion(
			angle.Deg(123.4), angle.Deg(0),
			angle.Zero(), angle.Zero(),
			angle.Arcsec(tc.parallax), 0)

		back := coord.ICRSToFK4(coord.FK4ToICRS(start), coord.B1950)

		pmRA, pmDec, _ := back.ProperMotion()
		gotRA := pmRA.Arcseconds() * 1000
		gotDec := pmDec.Arcseconds() * 1000

		// A hundredth of a mas/yr: far above the round trip's own residual for
		// a representable parallax, far below the whole mas/yr the clamp
		// produces.
		clean := math.Abs(gotRA) < 1e-2 && math.Abs(gotDec) < 1e-2

		t.Logf("parallax %-8g -> a star at rest comes back at (%+7.3f, %+7.3f) mas/yr, clean=%v",
			tc.parallax, gotRA, gotDec, clean)

		if clean != tc.clean {
			t.Errorf("parallax %g: round trip clean=%v, want %v — the threshold this test "+
				"pins has moved", tc.parallax, clean, tc.clean)
		}
	}
}
