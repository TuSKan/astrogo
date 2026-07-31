package kepler_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/ephemeris/kepler"
	"github.com/TuSKan/astrogo/internal/testutil"
	atime "github.com/TuSKan/astrogo/time"
)

// wrappedDiff returns the smallest signed angular difference a-b, in
// radians, wrapped to (-pi, pi] — the right comparison for two angles
// that are only meaningful modulo a full turn.
func wrappedDiff(a, b angle.Angle) float64 {
	return a.Sub(b).WrapPi().Radians()
}

func TestSolveKepler_RoundTrip(t *testing.T) {
	eccentricities := []float64{0, 0.1, 0.5, 0.9, 0.97}

	// Includes values outside [0, 2pi) and negative values to exercise
	// SolveKepler's internal Wrap2Pi of the input mean anomaly.
	meanAnomaliesDeg := []float64{
		0, 1, 45, 90, 135, 179, 180, 181, 270, 359, 360,
		-45, -180, -359, 450, 720 + 30,
	}

	for _, e := range eccentricities {
		for _, mDeg := range meanAnomaliesDeg {
			m := angle.Deg(mDeg)

			ea, err := kepler.SolveKepler(m, e)
			testutil.AssertNoError(t, err)

			// Round-trip: E - e*sin(E) must equal M (mod 2pi).
			recoveredM := angle.Rad(ea.Radians() - e*ea.Sin())
			if diff := math.Abs(wrappedDiff(recoveredM, m)); diff > 1e-9 {
				t.Errorf("e=%v M=%v deg: round-trip diff = %v rad, want < 1e-9", e, mDeg, diff)
			}
		}
	}
}

func TestSolveKepler_CircularReduction(t *testing.T) {
	for _, mDeg := range []float64{0, 30, 90, 180, 270, 359} {
		m := angle.Deg(mDeg)

		ea, err := kepler.SolveKepler(m, 0)
		testutil.AssertNoError(t, err)

		if diff := math.Abs(wrappedDiff(ea, m)); diff > 1e-12 {
			t.Errorf("e=0 M=%v deg: E should equal M, diff = %v rad", mDeg, diff)
		}
	}
}

func TestSolveKepler_RejectsUnsupportedEccentricity(t *testing.T) {
	for _, e := range []float64{-0.1, 1.0, 1.5, math.NaN(), math.Inf(1)} {
		_, err := kepler.SolveKepler(angle.Deg(45), e)
		testutil.AssertErrorIs(t, err, kepler.ErrUnsupportedOrbit)
	}
}

func TestSolveKepler_RejectsNonFiniteMeanAnomaly(t *testing.T) {
	for _, m := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		_, err := kepler.SolveKepler(angle.Rad(m), 0.5)
		testutil.AssertError(t, err)
	}
}

// keplerPeriodDays returns the two-body orbital period, in days, for a
// semi-major axis a in AU around the Sun (via constants.IAU.SunGravitationalParameter).
func keplerPeriodDays(a float64) float64 {
	aM := a * constants.IAU.AstronomicalUnit.Value
	periodSeconds := 2 * math.Pi * math.Sqrt(aM*aM*aM/constants.IAU.SunGravitationalParameter.Value)

	return periodSeconds / constants.Derived.JulianDaySeconds.Value
}

func testElements(epoch atime.Time, a, e float64, incl, node, argp, m0 angle.Angle) kepler.Elements {
	return kepler.Elements{
		Epoch: epoch, SemiMajorAxis: a, Eccentricity: e,
		Inclination: incl, AscendingNode: node, ArgPeriapsis: argp, MeanAnomaly: m0,
	}
}

func TestElements_Validate_RejectsBadInputs(t *testing.T) {
	epoch := atime.Date(2026, atime.January, 1, 0, 0, 0, 0, atime.LocationUTC)

	cases := []struct {
		name string
		el   kepler.Elements
		want error
	}{
		{"e=1 (parabolic)", testElements(epoch, 2.5, 1.0, 0, 0, 0, 0), kepler.ErrUnsupportedOrbit},
		{"e=1.5 (hyperbolic)", testElements(epoch, 2.5, 1.5, 0, 0, 0, 0), kepler.ErrUnsupportedOrbit},
		{"e=-0.1", testElements(epoch, 2.5, -0.1, 0, 0, 0, 0), kepler.ErrUnsupportedOrbit},
		{"e=NaN", testElements(epoch, 2.5, math.NaN(), 0, 0, 0, 0), kepler.ErrUnsupportedOrbit},
		{"a=0", testElements(epoch, 0, 0.1, 0, 0, 0, 0), kepler.ErrInvalidElements},
		{"a=-1", testElements(epoch, -1, 0.1, 0, 0, 0, 0), kepler.ErrInvalidElements},
		{"a=Inf", testElements(epoch, math.Inf(1), 0.1, 0, 0, 0, 0), kepler.ErrInvalidElements},
		{"M0=NaN", testElements(epoch, 2.5, 0.1, 0, 0, 0, angle.Rad(math.NaN())), kepler.ErrInvalidElements},
		{"i=Inf", testElements(epoch, 2.5, 0.1, angle.Rad(math.Inf(1)), 0, 0, 0), kepler.ErrInvalidElements},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.el.Validate()
			testutil.AssertErrorIs(t, err, tt.want)

			_, _, err = tt.el.StateAt(epoch)
			testutil.AssertErrorIs(t, err, tt.want)
		})
	}
}

func TestElements_Validate_AcceptsGoodInputs(t *testing.T) {
	epoch := atime.Date(2026, atime.January, 1, 0, 0, 0, 0, atime.LocationUTC)
	el := testElements(epoch, 2.7, 0.15, angle.Deg(10), angle.Deg(80), angle.Deg(73), angle.Deg(20))
	testutil.AssertNoError(t, el.Validate())
}

// TestElements_StateAt_KnownGeometry_Inclination0 locks in the
// perifocal-to-ecliptic rotation's sign convention with a hand-derivable
// case: a circular (e=0), uncrowded (i=0, so AscendingNode is
// physically irrelevant) orbit with the argument of periapsis rotated
// 90 degrees from the reference X axis. A quarter period after epoch
// (mean anomaly = eccentric anomaly = true anomaly = 90 degrees for a
// circular orbit), the body should be 180 degrees from the reference X
// axis (90 deg to reach periapsis direction + 90 deg of travel), i.e.
// at (-a, 0, 0) AU — and since that vector has zero Y/Z components, the
// final ecliptic-to-equatorial obliquity rotation (which only mixes
// Y/Z) leaves it unchanged, so this same expectation holds after
// StateAt's full transform chain, not just the ecliptic intermediate.
func TestElements_StateAt_KnownGeometry_Inclination0(t *testing.T) {
	const a = 2.0

	epoch := atime.Date(2026, atime.January, 1, 0, 0, 0, 0, atime.LocationUTC)
	el := testElements(epoch, a, 0, angle.Zero(), angle.Zero(), angle.Deg(90), angle.Deg(0))

	quarterPeriod := epoch.AddDays(keplerPeriodDays(a) / 4)

	pos, _, err := el.StateAt(quarterPeriod)
	testutil.AssertNoError(t, err)

	testutil.AssertNear(t, "x", pos.X, -a, 1e-6)
	testutil.AssertNear(t, "y", pos.Y, 0, 1e-6)
	testutil.AssertNear(t, "z", pos.Z, 0, 1e-6)
}

// TestElements_StateAt_KnownGeometry_Inclination90 locks in both the
// perifocal-to-ecliptic rotation AND the ecliptic-to-equatorial obliquity
// rotation together: a circular orbit inclined 90 degrees, periapsis at
// the ascending node (argp=0), ascending node along the reference X
// axis. A quarter period after epoch the body is at the orbit's pole
// direction, ecliptic (0, 0, a) — independently derivable as the
// rotation-matrix chain's self-consistent result — which the fixed
// J2000-obliquity rotation then carries to equatorial
// (0, -a*sin(eps), a*cos(eps)), the same known relation as the ecliptic
// north pole's real equatorial position (RA 18h, Dec 90-eps).
func TestElements_StateAt_KnownGeometry_Inclination90(t *testing.T) {
	const a = 2.0

	epoch := atime.Date(2026, atime.January, 1, 0, 0, 0, 0, atime.LocationUTC)
	el := testElements(epoch, a, 0, angle.Deg(90), angle.Zero(), angle.Zero(), angle.Deg(0))

	quarterPeriod := epoch.AddDays(keplerPeriodDays(a) / 4)

	pos, _, err := el.StateAt(quarterPeriod)
	testutil.AssertNoError(t, err)

	eps := constants.IAU.ObliquityJ2000.Value

	testutil.AssertNear(t, "x", pos.X, 0, 1e-6)
	testutil.AssertNear(t, "y", pos.Y, -a*math.Sin(eps), 1e-6)
	testutil.AssertNear(t, "z", pos.Z, a*math.Cos(eps), 1e-6)
}

func TestElements_StateAt_OnePeriodClosure(t *testing.T) {
	epoch := atime.Date(2026, atime.January, 1, 0, 0, 0, 0, atime.LocationUTC)
	el := testElements(epoch, 2.7, 0.2, angle.Deg(12), angle.Deg(50), angle.Deg(200), angle.Deg(30))

	pos0, vel0, err := el.StateAt(epoch)
	testutil.AssertNoError(t, err)

	pos1, vel1, err := el.StateAt(epoch.AddDays(keplerPeriodDays(el.SemiMajorAxis)))
	testutil.AssertNoError(t, err)

	testutil.AssertNear(t, "x", pos1.X, pos0.X, 1e-6)
	testutil.AssertNear(t, "y", pos1.Y, pos0.Y, 1e-6)
	testutil.AssertNear(t, "z", pos1.Z, pos0.Z, 1e-6)
	testutil.AssertNear(t, "vx", vel1.X, vel0.X, 1e-6)
	testutil.AssertNear(t, "vy", vel1.Y, vel0.Y, 1e-6)
	testutil.AssertNear(t, "vz", vel1.Z, vel0.Z, 1e-6)
}

// TestElements_StateAt_EnergyConservation checks the vis-viva relation
// v^2 = GM*(2/r - 1/a) (specific orbital energy conservation) holds at
// several points around the orbit, in SI units.
func TestElements_StateAt_EnergyConservation(t *testing.T) {
	epoch := atime.Date(2026, atime.January, 1, 0, 0, 0, 0, atime.LocationUTC)
	el := testElements(epoch, 1.8, 0.35, angle.Deg(7), angle.Deg(120), angle.Deg(300), angle.Deg(10))

	gm := constants.IAU.SunGravitationalParameter.Value
	auM := constants.IAU.AstronomicalUnit.Value
	dayS := constants.Derived.JulianDaySeconds.Value
	aM := el.SemiMajorAxis * auM

	for _, dtDays := range []float64{0, 10, 50, 123.4, 400} {
		pos, vel, err := el.StateAt(epoch.AddDays(dtDays))
		testutil.AssertNoError(t, err)

		rM := pos.Norm() * auM
		vMS := vel.Norm() * auM / dayS

		energy := vMS*vMS - gm*(2/rM-1/aM)
		testutil.AssertNear(t, "vis-viva residual", energy, 0, 1e-6*gm/aM)
	}
}

// TestElements_StateAt_AngularMomentumConservation checks |r x v| stays
// constant (equal to sqrt(GM*a*(1-e^2))) at several points around the
// orbit, in SI units.
func TestElements_StateAt_AngularMomentumConservation(t *testing.T) {
	epoch := atime.Date(2026, atime.January, 1, 0, 0, 0, 0, atime.LocationUTC)
	el := testElements(epoch, 3.1, 0.5, angle.Deg(15), angle.Deg(200), angle.Deg(80), angle.Deg(-40))

	gm := constants.IAU.SunGravitationalParameter.Value
	auM := constants.IAU.AstronomicalUnit.Value
	dayS := constants.Derived.JulianDaySeconds.Value
	aM := el.SemiMajorAxis * auM

	want := math.Sqrt(gm * aM * (1 - el.Eccentricity*el.Eccentricity))

	for _, dtDays := range []float64{0, 5, 77, 300} {
		pos, vel, err := el.StateAt(epoch.AddDays(dtDays))
		testutil.AssertNoError(t, err)

		rM := pos.MulScalar(auM)
		vMS := vel.MulScalar(auM / dayS)

		h := rM.Cross(vMS).Norm()
		testutil.AssertRelNear(t, "angular momentum", h, want, 1e-6)
	}
}

// TestElements_StateAt_VelocityMatchesFiniteDifference cross-checks the
// analytic velocity against a central finite difference of position, at
// several points around the orbit.
func TestElements_StateAt_VelocityMatchesFiniteDifference(t *testing.T) {
	epoch := atime.Date(2026, atime.January, 1, 0, 0, 0, 0, atime.LocationUTC)
	el := testElements(epoch, 2.2, 0.4, angle.Deg(20), angle.Deg(60), angle.Deg(150), angle.Deg(90))

	const h = 1e-3 // days

	for _, dtDays := range []float64{0, 30, 150, 500} {
		mid := epoch.AddDays(dtDays)

		_, vel, err := el.StateAt(mid)
		testutil.AssertNoError(t, err)

		posBefore, _, err := el.StateAt(mid.AddDays(-h))
		testutil.AssertNoError(t, err)

		posAfter, _, err := el.StateAt(mid.AddDays(h))
		testutil.AssertNoError(t, err)

		fd := posAfter.Sub(posBefore).DivScalar(2 * h)

		testutil.AssertRelNear(t, "vx finite-difference", vel.X, fd.X, 1e-6)
		testutil.AssertRelNear(t, "vy finite-difference", vel.Y, fd.Y, 1e-6)
		testutil.AssertRelNear(t, "vz finite-difference", vel.Z, fd.Z, 1e-6)
	}
}
