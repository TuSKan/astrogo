package coord_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/unit"
)

// barnardsStar is the canonical test case for a space velocity: the largest
// proper motion known, a parallax large enough that the distance is not in
// doubt, and a total velocity that is quoted in textbooks.
//
// Hipparcos/Gaia values: μα* = −798.47 mas/yr, μδ = +10337.77 mas/yr,
// parallax 546.98 mas, radial velocity −110.6 km/s.
func barnardsStar() coord.ICRS {
	return coord.NewICRSWithKinematics(
		angle.Deg(269.452), angle.Deg(4.693),
		angle.Arcsec(-0.79847), angle.Arcsec(10.33777),
		angle.Arcsec(0.54698), unit.KmPerSec(-110.6),
	)
}

// TestSpaceSpeedOfBarnardsStar is the external check: an answer that can be
// compared against something outside this repository.
//
// Barnard's Star moves at about 142 km/s with respect to the Sun, a figure
// that follows from its own catalogue entry and appears in every account of
// it. Reproducing it is what shows the unit conversion — au/day out of SOFA,
// km/s out of here — is right, which no internal consistency check can.
func TestSpaceSpeedOfBarnardsStar(t *testing.T) {
	t.Parallel()

	speed, ok := coord.SpaceSpeed(barnardsStar())
	if !ok {
		t.Fatal("Barnard's Star has a parallax of 547 mas; there is no reason this cannot answer")
	}

	// The published figure is about 142.5 km/s. A per-cent tolerance is wide
	// enough for the small differences between catalogue versions and far too
	// tight for a factor-of-86400 or factor-of-au unit error, which is the
	// class of mistake this is guarding.
	testutil.AssertRelNear(t, "Barnard's Star space speed", speed.KmPerSec(), 142.5, 0.01)
}

// TestTheRadialComponentIsTheRadialVelocity is the strongest internal check
// available, and it ties the vector back to an input the caller supplied.
//
// Projecting the space velocity onto the line of sight must return the radial
// velocity that went in. It exercises the whole chain — the proper-motion
// convention, the parallax inversion, the unit conversion and the axes — and a
// sign error or a wrong cosine anywhere in it moves this number.
func TestTheRadialComponentIsTheRadialVelocity(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		ra, dec float64 // degrees
		pmRA    float64 // arcsec/yr, on-sky
		pmDec   float64
		px      float64 // arcsec
		rv      float64 // km/s
	}{
		{"Barnard's Star", 269.452, 4.693, -0.79847, 10.33777, 0.54698, -110.6},
		{"approaching, high declination", 45, 78, 0.020, -0.035, 0.012, -22.4},
		{"receding, southern", 201.3, -43.1, -0.310, -0.480, 0.089, +45.0},
		{"no proper motion at all", 123.4, 0, 0, 0, 0.1, +30.0},
		{"no radial velocity", 10, -10, 0.5, 0.5, 0.05, 0},
	} {
		star := coord.NewICRSWithKinematics(
			angle.Deg(tc.ra), angle.Deg(tc.dec),
			angle.Arcsec(tc.pmRA), angle.Arcsec(tc.pmDec),
			angle.Arcsec(tc.px), unit.KmPerSec(tc.rv),
		)

		v, ok := coord.SpaceVelocity(star)
		if !ok {
			t.Errorf("%s: could not compute a velocity", tc.name)
			continue
		}

		// Not an identity to the last bit, and it should not be. A catalogue's
		// radial velocity is a measured Doppler quantity; SOFA's pv-vector is
		// a geometric velocity computed relativistically and with the changing
		// light-time its own notes describe. The two differ by that treatment.
		//
		// Measured across these cases the gap runs from 1.5 mm/s to 7.5 m/s,
		// largest for the star with the biggest transverse velocity and no
		// radial one. 20 m/s is comfortably above all of them and four orders
		// below the km/s a sign error or a wrong cosine would produce.
		radial := v.Dot(star.ToUnitVector())
		if math.Abs(radial-tc.rv) > 0.02 {
			t.Errorf("%s: the radial component is %.6f km/s, want the %.1f that went in",
				tc.name, radial, tc.rv)
		}
	}
}

// TestTheTransverseComponentMatchesTheClassicalIdentity checks the other half
// against arithmetic that predates SOFA and can be done on paper.
//
// A proper motion of μ arcsec/yr at a distance of d parsecs is a transverse
// velocity of 4.74047 μ d km/s — the constant being one au per Julian year
// expressed in km/s. Recovering it from the vector confirms the distance and
// the time unit independently of the radial check above, which is blind to
// both.
func TestTheTransverseComponentMatchesTheClassicalIdentity(t *testing.T) {
	t.Parallel()

	// One au per Julian year in km/s: 149597870700 m over 365.25 x 86400 s,
	// both exact by definition. Written as the decimal rather than the
	// division on purpose, so that it is an independent check on the constant
	// coord computes rather than the same arithmetic run twice.
	//
	// This test previously held 4.740470446, the same wrong decimal the
	// constant did, and so could not have caught it.
	const auPerYearInKmPerSec = 4.740470463533348

	for _, tc := range []struct {
		name    string
		ra, dec float64
		pmRA    float64 // arcsec/yr, on-sky
		pmDec   float64
		px      float64 // arcsec
		rv      float64
	}{
		{"Barnard's Star", 269.452, 4.693, -0.79847, 10.33777, 0.54698, -110.6},
		{"a slow nearby star", 120, 30, 0.010, 0.010, 0.2, 0},
		{"at rest apart from its radial motion", 300, -20, 0, 0, 0.05, 55},
	} {
		star := coord.NewICRSWithKinematics(
			angle.Deg(tc.ra), angle.Deg(tc.dec),
			angle.Arcsec(tc.pmRA), angle.Arcsec(tc.pmDec),
			angle.Arcsec(tc.px), unit.KmPerSec(tc.rv),
		)

		v, ok := coord.SpaceVelocity(star)
		if !ok {
			t.Errorf("%s: could not compute a velocity", tc.name)
			continue
		}

		// Remove the radial part and measure what is left. The local is not
		// called "unit" because this file now imports the package of that name.
		los := star.ToUnitVector()
		transverse := v.Sub(los.MulScalar(v.Dot(los))).Norm()

		totalPM := math.Hypot(tc.pmRA, tc.pmDec)
		distancePc := coord.ParallaxDistance(angle.Arcsec(tc.px)).Pc()
		want := auPerYearInKmPerSec * totalPM * distancePc

		// A part in a thousand. The residual is the relativistic and
		// light-time content of SOFA's pv-vector, which the classical identity
		// does not have and which grows with the radial velocity — visibly so
		// for Barnard's Star at −110 km/s.
		testutil.AssertRelNear(t, tc.name+" transverse velocity", transverse, want, 1e-3)
	}
}

// TestSpaceVelocityRefusesWhatItCannotAnswer covers the two ways there is no
// velocity to report, and is the contrast with #331 that the doc comment
// draws.
//
// A frame conversion can rotate a proper motion without knowing the distance,
// because the distance divides out. Turning an angular rate into km/s cannot:
// the same 150 mas/yr is 7 km/s at 10 pc and 700 at a kiloparsec. So where
// #331's fix found another route, this refuses — and refusing is the whole
// point, because the alternative is a plausible number computed from a
// distance nobody supplied.
func TestSpaceVelocityRefusesWhatItCannotAnswer(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		star coord.ICRS
	}{
		{
			name: "no kinematics were ever recorded",
			star: coord.NewICRS(angle.Deg(123.4), angle.Deg(-35.6)),
		},
		{
			name: "kinematics recorded, but no parallax to scale them",
			star: coord.NewICRSWithKinematics(
				angle.Deg(123.4), angle.Deg(-35.6),
				angle.Arcsec(0.150), angle.Arcsec(0.220), 0, unit.KmPerSec(-22.4)),
		},
		{
			name: "a parallax below SOFA's floor is the same as none",
			star: coord.NewICRSWithKinematics(
				angle.Deg(123.4), angle.Deg(-35.6),
				angle.Arcsec(0.150), angle.Arcsec(0.220), angle.Arcsec(1e-9), unit.KmPerSec(-22.4)),
		},
	} {
		if v, ok := coord.SpaceVelocity(tc.star); ok {
			t.Errorf("%s: reported a velocity of %v km/s, want none", tc.name, v)
		}

		if speed, ok := coord.SpaceSpeed(tc.star); ok {
			t.Errorf("%s: SpaceSpeed reported %g km/s, want none", tc.name, speed)
		}
	}

	// A star at rest with a parallax is a different thing entirely: the answer
	// exists and is zero, and must not be confused with having no answer.
	atRest := coord.NewICRSWithKinematics(
		angle.Deg(123.4), angle.Deg(-35.6), 0, 0, angle.Arcsec(0.020), 0)

	v, ok := coord.SpaceVelocity(atRest)
	if !ok {
		t.Fatal("a star recorded at rest has a space velocity, and it is zero")
	}

	if v.Norm() > 1e-9 {
		t.Errorf("a star recorded at rest moves at %g km/s", v.Norm())
	}
}

// TestSpaceSpeedAgreesWithTheVectorItSummarises guards the one thing that
// could drift between the two entry points.
func TestSpaceSpeedAgreesWithTheVectorItSummarises(t *testing.T) {
	t.Parallel()

	star := barnardsStar()

	v, okV := coord.SpaceVelocity(star)
	speed, okS := coord.SpaceSpeed(star)

	if okV != okS {
		t.Fatalf("SpaceVelocity ok=%v but SpaceSpeed ok=%v", okV, okS)
	}

	testutil.AssertExact(t, "SpaceSpeed against the vector's norm", speed.KmPerSec(), v.Norm())
}
