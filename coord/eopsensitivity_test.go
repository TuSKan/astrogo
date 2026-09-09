package coord_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// siderealArcsecPerSecond is how far the Earth turns, in arcseconds of hour
// angle, per second of UT1.
//
// 15.041069 arcsec/s: the sidereal rate, 360 degrees per sidereal day rather
// than per solar day. The distinction is not pedantic here — using the solar
// rate of 15.0 would be wrong by 0.041, which is 0.3% and larger than the
// tolerance below.
const siderealArcsecPerSecond = 15.041069

// dut1Override serves real polar motion with a substituted UT1-UTC.
//
// Varying one Earth Orientation Parameter at a time is the whole point.
// ZeroModel would answer the question badly: it zeroes polar motion too, so
// the measured shift would mix Earth rotation with a tilted local zenith and
// could not attribute either.
type dut1Override struct {
	base time.Model
	dut1 float64
}

func (m dut1Override) EOP(mjd float64) (time.EOP, error) {
	e, err := m.base.EOP(mjd)
	if err != nil {
		return e, fmt.Errorf("dut1Override: underlying model: %w", err)
	}

	e.DUT1 = m.dut1

	return e, nil
}

// TestHourAngleTracksUT1AtTheSiderealRate pins the coupling between Earth
// orientation data and the observed hour angle.
//
// # Why this tier exists
//
// Two questions get confused whenever a position is compared against a
// reference: *is the reduction chain right*, and *is the Earth-orientation
// data current*. A single end-to-end residual answers neither, because either
// can produce it.
//
// Skyfield's suite separates them by pinning the inputs — its NOVAS
// comparison runs with `delta_t=0.0` and `xp = yp = 0.0`, holding Earth
// orientation identical on both sides so that what remains is the arithmetic.
// That is why its agreement figure is quoted in microarcseconds while real
// sky positions carry the tenths of an arcsecond that polar motion alone is
// worth. It is a good design and astrogo had no equivalent.
//
// This is that tier, done without needing a second implementation to compare
// against. Instead of holding UT1 fixed and comparing two libraries, it varies
// UT1 by a known amount and checks astrogo moves by the amount physics
// requires. A reference implementation can be unavailable, versioned, or
// wrong; the Earth's rotation rate cannot.
//
// # What a failure would mean
//
// The hour angle must move 15.041069 arcseconds per second of UT1, and by
// exactly that — no more, from double-counting the correction, and no less,
// from dropping it. Getting this wrong is invisible in ordinary use: every
// position stays plausible and merely sits at the wrong time, which is
// precisely how a sub-arcsecond bias survives review.
//
// It also bounds the interpretation of a residual measured elsewhere. astrogo
// differs from USNO by about 0.66 arcseconds in Greenwich Hour Angle, which is
// 44 milliseconds of UT1; this test establishes that the 44 ms is a statement
// about the *data*, since the coupling that turns data into an angle is exact.
func TestHourAngleTracksUT1AtTheSiderealRate(t *testing.T) {
	// Not parallel: the EOP model is process-wide.
	base := time.GetModel()

	t.Cleanup(time.ResetEOP)

	site, err := coord.NewGeodetic(angle.Deg(-70.4042), angle.Deg(-24.6272), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	atm := atmosphere.StandardRefraction
	atm.Model = atmosphere.RefractionNone{}

	// A fixed catalogue direction rather than a body: no ephemeris, no light
	// time, nothing that could move for a reason other than the one under
	// test.
	star := coord.NewICRS(angle.Hour(5.5), angle.Deg(-5.4))
	epoch := time.Date(2026, time.March, 20, 12, 0, 0, 0, time.LocationUTC)

	hourAngleWith := func(dut1 float64) float64 {
		t.Helper()

		time.RegisterModel(dut1Override{base: base, dut1: dut1})

		ha, err := coord.NewContext(epoch, site, atm).ICRSToHourAngle(star)
		if err != nil {
			t.Fatalf("ICRSToHourAngle at DUT1=%v: %v", dut1, err)
		}

		return ha.Arcseconds()
	}

	for _, step := range []float64{0.05, 0.2, -0.3, 0.9} {
		reference := hourAngleWith(0)
		shifted := hourAngleWith(step)

		got := shifted - reference
		want := step * siderealArcsecPerSecond

		// A tenth of a milliarcsecond over shifts of up to 13.5 arcseconds.
		// Loose enough for the difference between the rate used here and the
		// one SOFA derives internally, tight enough that a missing or
		// doubled correction cannot hide.
		const tolArcsec = 1e-4

		if math.Abs(got-want) > tolArcsec {
			t.Errorf("DUT1 %+.2f s moved the hour angle by %+.6f arcsec, want %+.6f "+
				"(%.6f arcsec/s of UT1); the coupling between Earth orientation and "+
				"Earth rotation is wrong, which shifts every observed position in time "+
				"while leaving it entirely plausible",
				step, got, want, siderealArcsecPerSecond)
		}
	}
}

// TestPolarMotionMovesTheObservedPlace covers the other Earth-orientation
// term, the one Skyfield's NOVAS comparison sets to zero.
//
// Polar motion reaches roughly 0.3 arcseconds in x and 0.6 in y. It tilts the
// local zenith rather than turning the Earth, so it moves an observed place
// without appearing in the hour angle at the sidereal rate. A comparison that
// zeroes it on both sides — as Skyfield's does — cannot see it at all, which
// is how a suite stays honest at microarcseconds while real positions carry
// tenths of an arcsecond.
//
// # Why the pole is injected rather than read
//
// Because coord's test binary has no Earth-orientation data at all. It does
// not import remote, so no loader is registered and time.GetModel returns
// ZeroModel — a first version of this test compared real EOP against zeroed
// EOP and measured exactly 0.000000 arcsec, because both sides were zero.
//
// That is worth knowing beyond this test: nothing in coord's own suite
// exercises real Earth orientation, so every other test in this package runs
// at a pole that does not wander. Injecting the values makes this one
// deterministic as well as honest — it asserts astrogo's response to a known
// pole rather than to whatever a bulletin happens to say today.
func TestPolarMotionMovesTheObservedPlace(t *testing.T) {
	t.Cleanup(time.ResetEOP)

	site, err := coord.NewGeodetic(angle.Deg(-70.4042), angle.Deg(-24.6272), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	atm := atmosphere.StandardRefraction
	atm.Model = atmosphere.RefractionNone{}

	star := coord.NewICRS(angle.Hour(5.5), angle.Deg(-5.4))
	epoch := time.Date(2026, time.March, 20, 12, 0, 0, 0, time.LocationUTC)

	observed := func(xpArcsec, ypArcsec float64) coord.AltAz {
		t.Helper()

		time.RegisterModel(fixedEOP{
			xp: angle.Arcsec(xpArcsec).Radians(),
			yp: angle.Arcsec(ypArcsec).Radians(),
		})

		aa, err := coord.NewContext(epoch, site, atm).ICRSToAltAz(star)
		if err != nil {
			t.Fatalf("ICRSToAltAz: %v", err)
		}

		return aa
	}

	// Representative IERS values — the pole was near here in 2026 — against
	// a pole pinned at the origin. UT1 is zero in both, so only polar motion
	// differs.
	const (
		xpArcsec = 0.32548
		ypArcsec = 0.596732
	)

	sep := altAzSeparationArcsec(observed(xpArcsec, ypArcsec), observed(0, 0))

	// The pole displacement is 0.68 arcsec in magnitude, and its effect on an
	// observed direction is at most that, reduced by geometry. Anything near
	// zero means the term never reached the transform; anything above the
	// displacement itself means it was applied more than once or in the wrong
	// units.
	displacement := math.Hypot(xpArcsec, ypArcsec)

	if sep < 0.01 {
		t.Errorf("a pole displaced by %.3f arcsec moved the observed place by only "+
			"%.6f arcsec; polar motion is not reaching the transform",
			displacement, sep)
	}

	if sep > displacement {
		t.Errorf("a pole displaced by %.3f arcsec moved the observed place by %.4f "+
			"arcsec, which is more than the displacement itself — the term is being "+
			"applied twice, or in the wrong units", displacement, sep)
	}

	t.Logf("a pole at (%.5f, %.5f) arcsec moves this observed place by %.4f arcsec",
		xpArcsec, ypArcsec, sep)
}

// fixedEOP serves constant Earth-orientation values, so a test asserts
// astrogo's response to a known pole rather than to today's bulletin.
type fixedEOP struct {
	dut1   float64
	xp, yp float64
}

func (m fixedEOP) EOP(_ float64) (time.EOP, error) {
	return time.EOP{DUT1: m.dut1, XP: m.xp, YP: m.yp}, nil
}

// altAzSeparationArcsec is the angle between two horizontal directions.
//
// Written out rather than reaching for a helper so this file depends on
// nothing but coord itself: a test about whether a term reaches the transform
// should not be able to fail because a shared helper changed.
func altAzSeparationArcsec(a, b coord.AltAz) float64 {
	const rad = math.Pi / 180

	alt1, az1 := a.Alt().Degrees()*rad, a.Az().Degrees()*rad
	alt2, az2 := b.Alt().Degrees()*rad, b.Az().Degrees()*rad

	cosSep := math.Sin(alt1)*math.Sin(alt2) +
		math.Cos(alt1)*math.Cos(alt2)*math.Cos(az1-az2)

	// acos is undefined a hair outside [-1,1], which rounding reaches for two
	// nearly identical directions — which is every case here.
	cosSep = math.Min(1, math.Max(-1, cosSep))

	return math.Acos(cosSep) / rad * 3600
}

// TestObserverVectorRotatesWithUT1 covers the second place UT1 enters a
// Context, which the hour-angle test above cannot see.
//
// [coord.Context] applies UT1 twice, to two different things: SOFA's astrom
// structure, which the apparent-place and observed transforms use, and the
// celestial-to-terrestrial matrix that places the observer, which
// [coord.Context.ObsVec] exposes and diurnal parallax depends on. They are
// computed from the same DUT1 by separate code, so one can drift from the
// other.
//
// A mutation confirmed the gap: doubling DUT1 in the observer-vector path
// alone left every other test in this file passing, because the hour angle
// comes from astrom and never consults that matrix. The error it represents
// is a mis-placed observer — the whole Earth turned to the wrong moment
// underneath a correct sky — which shows up only as parallax, so it is worth
// tens of milliarcseconds for the Moon and nothing at all for a star.
//
// # Why a rate rather than an angle
//
// The observer turns about the Celestial Intermediate Pole, not about the
// ICRS z-axis, and by 2026 precession has moved those apart by about 0.14
// degrees. So the ICRS longitude of the observer vector does *not* change by
// exactly the Earth rotation angle: measured, it changes by 1.000326 times
// it, consistently at every step. That factor is the pole offset and the
// site's geometry, not an error — a first version of this test asserted the
// exact identity and failed by 4 milliarcseconds on a 13.5 arcsecond shift.
//
// What is exactly true is that the response is *linear* in UT1 with a
// constant of proportionality near the sidereal rate. That is what is
// asserted: every step must yield the same rate, and that rate must sit
// within a tenth of a percent of 15.041069 arcsec/s. A dropped correction
// gives zero, a doubled one gives twice — both are 100% away from a bound of
// 0.1%.
func TestObserverVectorRotatesWithUT1(t *testing.T) {
	t.Cleanup(time.ResetEOP)

	site, err := coord.NewGeodetic(angle.Deg(-70.4042), angle.Deg(-24.6272), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	atm := atmosphere.StandardRefraction
	atm.Model = atmosphere.RefractionNone{}

	epoch := time.Date(2026, time.March, 20, 12, 0, 0, 0, time.LocationUTC)

	longitudeWith := func(dut1 float64) float64 {
		t.Helper()

		time.RegisterModel(fixedEOP{dut1: dut1})

		v := coord.NewContext(epoch, site, atm).ObsVec()

		return math.Atan2(v.Y, v.X) / math.Pi * 180 * 3600
	}

	reference := longitudeWith(0)

	steps := []float64{0.05, 0.2, -0.3, 0.9}
	rates := make([]float64, 0, len(steps))

	for _, step := range steps {
		rate := wrapArcsec(longitudeWith(step)-reference) / step
		rates = append(rates, rate)

		// A tenth of a percent: far tighter than a dropped or doubled
		// correction, and loose enough for the pole-offset factor above.
		if math.Abs(rate-siderealArcsecPerSecond)/siderealArcsecPerSecond > 1e-3 {
			t.Errorf("DUT1 %+.2f s rotated the observer vector at %.6f arcsec/s, want "+
				"%.6f within 0.1%%; the observer is being placed at the wrong moment of "+
				"the Earth's rotation, which misplaces every parallax",
				step, rate, siderealArcsecPerSecond)
		}
	}

	// Linearity is the sharper claim: a scale error common to every step
	// could sit inside the bound above, but the steps disagreeing with each
	// other cannot be geometry.
	for i, r := range rates[1:] {
		if math.Abs(r-rates[0]) > 1e-6*siderealArcsecPerSecond {
			t.Errorf("step %d rotated at %.9f arcsec/s against the first step's %.9f; "+
				"the response to UT1 is not linear, so it is not a rotation",
				i+1, r, rates[0])
		}
	}

	t.Logf("observer vector rotates at %.6f arcsec/s of UT1 (sidereal rate %.6f, "+
		"ratio %.6f — the CIP/ICRS pole offset)",
		rates[0], siderealArcsecPerSecond, rates[0]/siderealArcsecPerSecond)
}

// wrapArcsec folds an angle difference into (-648000, 648000] arcseconds,
// which is (-180, 180] degrees.
func wrapArcsec(d float64) float64 {
	const fullTurn = 360 * 3600

	for d > fullTurn/2 {
		d -= fullTurn
	}

	for d <= -fullTurn/2 {
		d += fullTurn
	}

	return d
}

// TestObserverVectorRespondsToPolarMotion closes the last gap in this file.
//
// Polar motion reaches the observer through two independent routes: SOFA's
// astrom, which the alt-az test above exercises, and the
// celestial-to-terrestrial matrix behind [coord.Context.ObsVec]. A mutation
// dropping it from the second alone left every other test here passing.
//
// What that mutation represents is an observer placed on an untilted Earth —
// wrong by the pole's own displacement, about 0.68 arcseconds of latitude and
// longitude, or roughly 20 metres of position. It never shows in a star's
// direction and does show in diurnal parallax, so it is worth around 0.01
// arcseconds for the Moon and nothing at all for anything further away. Small,
// and silent, which is the combination worth a test.
func TestObserverVectorRespondsToPolarMotion(t *testing.T) {
	t.Cleanup(time.ResetEOP)

	site, err := coord.NewGeodetic(angle.Deg(-70.4042), angle.Deg(-24.6272), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	atm := atmosphere.StandardRefraction
	atm.Model = atmosphere.RefractionNone{}

	epoch := time.Date(2026, time.March, 20, 12, 0, 0, 0, time.LocationUTC)

	obsVecWith := func(xpArcsec, ypArcsec float64) vector.Vec3 {
		t.Helper()

		time.RegisterModel(fixedEOP{
			xp: angle.Arcsec(xpArcsec).Radians(),
			yp: angle.Arcsec(ypArcsec).Radians(),
		})

		return coord.NewContext(epoch, site, atm).ObsVec()
	}

	const (
		xpArcsec = 0.32548
		ypArcsec = 0.596732
	)

	withPole := obsVecWith(xpArcsec, ypArcsec)
	withoutPole := obsVecWith(0, 0)

	// The angle between the two observer positions, seen from the geocentre.
	// A pole displaced by d moves a surface point by at most d, so the
	// separation is bounded by the displacement and must not be zero.
	cosSep := withPole.Unit().Dot(withoutPole.Unit())
	cosSep = math.Min(1, math.Max(-1, cosSep))
	sep := math.Acos(cosSep) / math.Pi * 180 * 3600

	displacement := math.Hypot(xpArcsec, ypArcsec)

	if sep < 0.01 {
		t.Errorf("a pole displaced by %.3f arcsec moved the observer vector by only "+
			"%.6f arcsec; polar motion is not reaching the observer's position, so "+
			"every diurnal parallax is computed from the wrong place",
			displacement, sep)
	}

	if sep > displacement*1.01 {
		t.Errorf("a pole displaced by %.3f arcsec moved the observer vector by %.4f "+
			"arcsec, more than the displacement itself — applied twice, or in the "+
			"wrong units", displacement, sep)
	}

	t.Logf("polar motion moves the observer vector by %.4f arcsec of geocentric angle",
		sep)
}
