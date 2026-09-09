package plan

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/satellite"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// constantOfAberration is κ = 20.49552″, the IAU 2009 system of astronomical
// constants (Luzum et al. 2011, Celest. Mech. Dyn. Astr. 110, 293).
//
// It is not one of astrogo's own constants and is deliberately written out
// here rather than derived from v⊕/c, because a value derived from this
// library's own ephemeris would not be independent of the thing it is checking.
const constantOfAberration = 20.49552

// TestApparentSunIsDisplacedByTheConstantOfAberration is the independent check
// on this change: a published number, from outside this library, that the
// answer has to land on.
//
// The Sun is the one body whose apparent displacement has a textbook value.
// Light time and annual aberration both act along Earth's orbital motion and
// both amount to v⊕/c at one astronomical unit, so the apparent Sun lags the
// geometric Sun by the constant of aberration — 20.49552″ — varying only with
// Earth's distance across the year, by the eccentricity, ±1.7%.
//
// Nothing about that number comes from astrogo. If this passes, the light-time
// iteration and the retarded-Earth aberration are both present and both have
// the right sign; if either were missing the answer would be half of it, and if
// the sign were wrong it would be right in magnitude and 41″ from the truth.
func TestApparentSunIsDisplacedByTheConstantOfAberration(t *testing.T) {
	t.Parallel()

	prov := eph.Default()
	sun := NewSun(prov)

	var (
		minSep = math.Inf(1)
		maxSep float64
		n      int
	)

	// A full year, so the ±1.7% from Earth's eccentricity is exercised at both
	// ends rather than sampled at one phase of the orbit.
	for d := range 365 {
		tm := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC).AddDays(float64(d))

		apparent, err := sun.Position(tm)
		testutil.AssertNoError(t, err)

		geometric, err := eph.Position(prov, eph.Sun, tm)
		testutil.AssertNoError(t, err)

		geoICRS, err := eph.ToICRS(geometric)
		testutil.AssertNoError(t, err)

		sep := coord.Separation(apparent, geoICRS).Arcseconds()

		minSep = math.Min(minSep, sep)
		maxSep = math.Max(maxSep, sep)
		n++
	}

	t.Logf("apparent-minus-geometric Sun over %d days: %.3f\" to %.3f\" (kappa = %.5f\")",
		n, minSep, maxSep, constantOfAberration)

	// Earth's orbital eccentricity is 0.0167, so v varies by ±1.7% over the
	// year and so does the displacement. 3% each way covers that with room for
	// the difference between the mean and osculating orbit, and is still far
	// too tight for an answer with either half of the correction missing.
	const tol = 0.03 * constantOfAberration

	if minSep < constantOfAberration-tol || maxSep > constantOfAberration+tol {
		t.Errorf("displacement ranged over [%.3f\", %.3f\"], want every value within "+
			"%.3f\" of the constant of aberration %.5f\"; half of it (%.3f\") means one of "+
			"light time and aberration is missing",
			minSep, maxSep, tol, constantOfAberration, constantOfAberration/2)
	}
}

// TestApparentPositionReachesTheAltAzPath pins that the fix arrives where it
// was needed, rather than only in Position.
//
// A MovingBody's altitude does not come from Position at all: observedAltAz
// sends it down GeocentricVec into coord.Context.GeocentricToObserved, which
// deliberately applies no aberration because its input is meant to arrive
// apparent. Fixing Position alone would leave the scheduler exactly as wrong
// as before while every direct test of Position passed.
func TestApparentPositionReachesTheAltAzPath(t *testing.T) {
	t.Parallel()

	loc, err := coord.NewGeodetic(angle.Deg(-70.40417), angle.Deg(-24.62722), 2635)
	testutil.AssertNoError(t, err)

	site, err := NewSite("Paranal", loc)
	testutil.AssertNoError(t, err)

	prov := eph.Default()
	mars := NewMars(prov)

	var (
		maxSep float64
		n      int
	)

	for step := range 365 * 4 {
		tm := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC).
			AddDays(float64(step) * 0.25)

		ctx := coord.NewContext(tm, loc, site.Refraction())

		eval, err := IsObservable(mars, tm, site)
		testutil.AssertNoError(t, err)

		if eval.AltAz.Alt().Degrees() < 10 {
			continue // near the horizon refraction dominates and swamps the point
		}

		geometric, err := eph.Position(prov, eph.Mars, tm)
		testutil.AssertNoError(t, err)

		wasAA := ctx.GeocentricToObserved(geometric)

		maxSep = math.Max(maxSep, altAzSeparationArcsec(eval.AltAz, wasAA))
		n++
	}

	t.Logf("Mars above 10 deg at Paranal, %d samples: alt/az moved by up to %.2f\"", n, maxSep)

	if n < 100 {
		t.Fatalf("only %d samples above the horizon; the fixture no longer exercises the path", n)
	}

	// Measured at 38.7" max over 2026. The floor is what makes this a test of
	// the alt/az path rather than of Position: a fix applied only to Position
	// leaves this at zero.
	const floor = 20.0

	if maxSep < floor {
		t.Errorf("alt/az moved by at most %.2f\" against the geometric vector, want more "+
			"than %.2f\" — the apparent place is not reaching GeocentricToObserved",
			maxSep, floor)
	}
}

// TestPositionAndGeocentricVecAgree pins the invariant that made this worth
// fixing in one place rather than two.
//
// The two answers feed different consumers — separations and details read
// Position, altitude reads GeocentricVec — and a target whose altitude is
// apparent while its Moon separation is geometric is wrong in a way no single
// test would catch, because each answer is defensible on its own.
func TestPositionAndGeocentricVecAgree(t *testing.T) {
	t.Parallel()

	prov := eph.Default()

	for _, tc := range []struct {
		name string
		obj  Observable
	}{
		{"Mars", NewMars(prov)},
		{"Sun", NewSun(prov)},
		{"Moon", NewMoon(prov)},
		{"Jupiter", NewJupiter(prov)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mb, ok := tc.obj.(MovingBody)
			if !ok {
				t.Fatalf("%s is not a MovingBody; it would not reach the path this fixes", tc.name)
			}

			for d := 0; d < 365; d += 11 {
				tm := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC).AddDays(float64(d))

				pos, err := tc.obj.Position(tm)
				testutil.AssertNoError(t, err)

				vec, err := mb.GeocentricVec(tm)
				testutil.AssertNoError(t, err)

				fromVec, err := eph.ToICRS(vec)
				testutil.AssertNoError(t, err)

				// Same quantity by two routes, so this is exact to rounding,
				// not to a physical tolerance.
				if sep := coord.Separation(pos, fromVec).Arcseconds(); sep > 1e-6 {
					t.Fatalf("%s at %v: Position and GeocentricVec disagree by %g\"",
						tc.name, tm, sep)
				}
			}
		})
	}
}

// TestSatellitePositionsAreNotRetarded covers the body this change must not
// touch, and the reason is not that the effect is small.
//
// eph.ApparentState returns the *apparent* place, and it gets there by asking
// the provider for a geocentric state at the retarded epoch — which moves the
// observer back along with the target and thereby folds in annual aberration
// (see its doc comment for the derivation). That is right for a body orbiting
// the Sun. It is wrong for one orbiting the Earth: a satellite shares Earth's
// orbital motion, so the 20" annual-aberration term cancels between observer
// and target and adding it would put the pass 20" off in a quantity that
// should not be there at all.
//
// Light time to a satellite is real but is a different correction — 1.3 ms to
// low Earth orbit, from the observer rather than from the geocentre — and it
// is not what ApparentState computes.
func TestSatellitePositionsAreNotRetarded(t *testing.T) {
	t.Parallel()

	prov, err := satellite.NewFromTLE("ISS (ZARYA)", issLine1, issLine2)
	testutil.AssertNoError(t, err)

	sat := NewSatellite("ISS", eph.ID(0), prov)

	tm := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.LocationUTC)

	got, err := sat.GeocentricVec(tm)
	testutil.AssertNoError(t, err)

	want, err := eph.Position(prov, eph.ID(0), tm)
	testutil.AssertNoError(t, err)

	if got != want {
		t.Errorf("Satellite.GeocentricVec = %v, want the unretarded %v — a satellite "+
			"shares Earth's orbital motion, so the annual aberration ApparentState "+
			"folds in must not be applied to it", got, want)
	}
}

// altAzSeparationArcsec is the angle between two horizontal directions.
func altAzSeparationArcsec(a, b coord.AltAz) float64 {
	a1, z1 := a.Alt().Radians(), a.Az().Radians()
	a2, z2 := b.Alt().Radians(), b.Az().Radians()

	cosSep := math.Sin(a1)*math.Sin(a2) + math.Cos(a1)*math.Cos(a2)*math.Cos(z1-z2)

	return angle.Rad(math.Acos(math.Min(1, math.Max(-1, cosSep)))).Arcseconds()
}

// linearProvider is a body moving in a straight line at constant velocity,
// geocentric, so the retardation this package applies has a closed form:
// asking for the state at t - tau returns pos0 + vel*(t - tau), displaced from
// the geometric answer by exactly -vel*tau.
//
// A stub rather than a real ephemeris because the assertion is then against
// arithmetic rather than against another astrogo call. It also reaches the
// three target types the default provider cannot: Asteroid, Comet and
// GenericBody have no SOFA body to stand in for them.
type linearProvider struct {
	epoch time.Time
	pos0  vector.Vec3 // AU at epoch
	vel   vector.Vec3 // AU/day
}

func (p linearProvider) State(_ eph.ID, t time.Time) (eph.State, error) {
	dt := t.SubDays(p.epoch)

	return eph.State{
		Pos:    p.pos0.Add(p.vel.MulScalar(dt)),
		Vel:    p.vel,
		Frame:  eph.FrameICRS,
		Center: eph.CenterGeocentre,
	}, nil
}

func (linearProvider) Close() error { return nil }

// TestEveryEphemerisBackedTargetIsApparent covers the four types that share
// this fix, against a displacement computed in closed form.
//
// Planet is the one anybody notices, and it was the only one the first round
// of tests reached — mutating Asteroid, Comet and GenericBody back to the
// geometric call left the suite green. They are the same defect in the same
// shape, and a table is what stops the next one being added without it.
func TestEveryEphemerisBackedTargetIsApparent(t *testing.T) {
	t.Parallel()

	epoch := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	// Two astronomical units out along x, crossing the line of sight at
	// 0.01 AU/day (~17 km/s, a plausible relative speed for a minor body).
	prov := linearProvider{
		epoch: epoch,
		pos0:  vector.V3(2, 0, 0),
		vel:   vector.V3(0, 0.01, 0),
	}

	// tau = r/c with r = 2 AU: light takes 2/173.1446 days to arrive, and in
	// that time the body moves vel*tau across the line of sight.
	const (
		lightAUPerDay = 173.1446326847695
		rangeAU       = 2.0
		speedAUPerDay = 0.01
	)

	tau := rangeAU / lightAUPerDay
	wantOffsetAU := speedAUPerDay * tau

	for _, tc := range []struct {
		name string
		obj  Observable
	}{
		{"Planet", NewPlanet("stub", eph.Mars, prov)},
		{"Asteroid", NewAsteroid("stub", eph.ID(2000001), prov)},
		{"Comet", NewComet("stub", eph.ID(1000001), prov, 10, 4)},
		{"GenericBody", NewGenericBody("stub", eph.ID(2000002), prov)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mb, ok := tc.obj.(MovingBody)
			if !ok {
				t.Fatalf("%s is not a MovingBody", tc.name)
			}

			got, err := mb.GeocentricVec(epoch)
			testutil.AssertNoError(t, err)

			// The geometric answer at the epoch is pos0 exactly, so the whole
			// of y is the retardation.
			testutil.AssertNear(t, "x (unchanged, motion is transverse)", got.X, 2.0, 1e-9)
			testutil.AssertNear(t, "y (the retardation)", got.Y, -wantOffsetAU, 1e-12)

			if got.Y >= 0 {
				t.Errorf("y = %g, want negative: the apparent place is where the body "+
					"*was* when the light left, which is behind its motion", got.Y)
			}

			// And Position must be the same place seen as a direction.
			pos, err := tc.obj.Position(epoch)
			testutil.AssertNoError(t, err)

			fromVec, err := eph.ToICRS(got)
			testutil.AssertNoError(t, err)

			if sep := coord.Separation(pos, fromVec).Arcseconds(); sep > 1e-6 {
				t.Errorf("Position and GeocentricVec disagree by %g\"", sep)
			}
		})
	}
}

// zeroProvider reports a body at the geocentre, which is not a direction.
//
// It is the one input eph.ToICRS refuses: a zero vector has no right ascension
// and no declination, and returning RA 0 Dec 0 for it would put a target in
// Pisces with complete confidence.
type zeroProvider struct{}

func (zeroProvider) State(eph.ID, time.Time) (eph.State, error) {
	return eph.State{Frame: eph.FrameICRS, Center: eph.CenterGeocentre}, nil
}

func (zeroProvider) Close() error { return nil }

// TestApparentFailuresNameTheTargetThatFailed covers the error path of all four
// types, which is the half of this change a caller only meets when something
// has gone wrong.
//
// Each wrapper exists to add the body's own name to the error, and that is the
// whole reason they are not one shared function: a scheduler evaluating two
// hundred targets and reporting "ephemeris: apparent state: ..." tells whoever
// reads the log nothing about which target to look at. An error that loses the
// name is as good as no error for that purpose, and nothing else would catch
// it — the message is not what any other assertion reads.
func TestApparentFailuresNameTheTargetThatFailed(t *testing.T) {
	t.Parallel()

	prov := failingProvider{}
	when := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	for _, tc := range []struct {
		name string
		obj  Observable
	}{
		{"Vesta", NewAsteroid("Vesta", eph.ID(2000004), prov)},
		{"Halley", NewComet("Halley", eph.ID(1000036), prov, 5.5, 8)},
		{"Chiron", NewGenericBody("Chiron", eph.ID(2002060), prov)},
		{"Neptune", NewPlanet("Neptune", eph.Neptune, prov)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mb, ok := tc.obj.(MovingBody)
			if !ok {
				t.Fatalf("%s is not a MovingBody", tc.name)
			}

			_, posErr := tc.obj.Position(when)
			_, vecErr := mb.GeocentricVec(when)

			for _, got := range []struct {
				label string
				err   error
			}{
				{"Position", posErr},
				{"GeocentricVec", vecErr},
			} {
				switch {
				case got.err == nil:
					t.Errorf("%s: a provider that cannot supply a state reported a position", got.label)
				case !errors.Is(got.err, errFailingProvider):
					t.Errorf("%s: err = %v, want the provider's own failure to survive wrapping",
						got.label, got.err)
				case !strings.Contains(got.err.Error(), tc.name):
					t.Errorf("%s: err = %v, want the target's name in it — a scheduler running "+
						"two hundred targets cannot act on a failure that does not say which one",
						got.label, got.err)
				}
			}
		})
	}
}

// TestApparentDirectionRefusesTheGeocentre covers apparentICRS's second
// failure, which is not a provider failure at all.
//
// A body reported at the geocentre has a state and no direction. eph.ToICRS
// refuses it rather than answering RA 0 Dec 0 — a real point in Pisces that a
// target would rise, transit and set from — and this keeps that refusal from
// being flattened into a success on the way through.
func TestApparentDirectionRefusesTheGeocentre(t *testing.T) {
	t.Parallel()

	body := NewPlanet("AtTheCentre", eph.Mars, zeroProvider{})
	when := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	// The vector itself is fine: zero is a state, just not a direction.
	vec, err := body.GeocentricVec(when)
	testutil.AssertNoError(t, err)

	if vec != (vector.Vec3{}) {
		t.Fatalf("GeocentricVec = %v, want the zero vector the provider reported", vec)
	}

	_, err = body.Position(when)
	if err == nil {
		t.Fatal("Position answered a direction for a body at the geocentre")
	}

	if !errors.Is(err, eph.ErrZeroVector) {
		t.Errorf("err = %v, want eph.ErrZeroVector", err)
	}

	if !strings.Contains(err.Error(), "AtTheCentre") {
		t.Errorf("err = %v, want the target's name in it", err)
	}
}
