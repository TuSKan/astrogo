package plan

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
)

func moreSite(t *testing.T) *Site {
	t.Helper()

	loc, err := coord.NewGeodetic(angle.Deg(-70.4028), angle.Deg(-24.6251), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	site, err := NewSite("Paranal", loc)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	return site
}

// TestSunSep uses the one geometry that needs no ephemeris table to predict:
// at a solstice the Sun is near one of the tropics, so a star placed on its
// coordinates is separated from it by nearly nothing, and its antipode by
// nearly 180 degrees.
func TestSunSep(t *testing.T) {
	site := moreSite(t)
	when := time.Date(2026, time.June, 21, 12, 0, 0, 0, time.LocationUTC)

	sun := NewSun(nil)

	sunPos, err := sun.Position(when)
	if err != nil {
		t.Fatalf("sun position: %v", err)
	}

	near := NewStar("beside the sun", sunPos.RA(), sunPos.Dec())
	far := NewStar("opposite the sun",
		sunPos.RA().Add(angle.Deg(180)), sunPos.Dec().Neg())

	c := SunSep{Threshold: angle.Deg(30)}

	res, err := c.Check(near, when, site)
	testutil.AssertNoError(t, err)

	if res.Pass {
		t.Errorf("a target on the Sun's own coordinates passed a 30° separation constraint: %v", res)
	}

	if res.Value > 1 {
		t.Errorf("separation from the Sun's own position = %.3f°, want ~0", res.Value)
	}

	res, err = c.Check(far, when, site)
	testutil.AssertNoError(t, err)

	if !res.Pass {
		t.Errorf("a target opposite the Sun failed a 30° separation constraint: %v", res)
	}

	if res.Value < 170 {
		t.Errorf("separation from the Sun's antipode = %.3f°, want ~180", res.Value)
	}
}

// TestSunSepPassesTheSunItself mirrors MoonSep's treatment of the Moon: an
// object's separation from itself is not a constraint on observing it, and
// zero would otherwise fail every threshold.
func TestSunSepPassesTheSunItself(t *testing.T) {
	site := moreSite(t)
	when := time.Date(2026, time.June, 21, 12, 0, 0, 0, time.LocationUTC)

	res, err := SunSep{Threshold: angle.Deg(30)}.Check(NewSun(nil), when, site)
	testutil.AssertNoError(t, err)

	if !res.Pass {
		t.Errorf("the Sun failed its own separation constraint: %v", res)
	}
}

// TestGalacticLatitude is checked against the two directions the Galactic
// frame is defined by, so the expected values are the definition rather than
// a computation of it: the North Galactic Pole is b = +90 and the Galactic
// centre b = 0.
func TestGalacticLatitude(t *testing.T) {
	site := moreSite(t)
	when := time.FromJD(2451545.0, time.UTC)

	// IAU 1958 Galactic frame, on ICRS: the pole and the centre.
	pole := NewStar("north galactic pole", angle.Deg(192.85948), angle.Deg(27.12825))
	centre := NewStar("galactic centre", angle.Deg(266.40510), angle.Deg(-28.93617))

	c := GalacticLatitude{Threshold: angle.Deg(20)}

	res, err := c.Check(pole, when, site)
	testutil.AssertNoError(t, err)

	if !res.Pass {
		t.Errorf("the north galactic pole failed a |b| >= 20° constraint: %v", res)
	}

	if math.Abs(res.Value-90) > 0.01 {
		t.Errorf("|b| at the pole = %.4f, want 90", res.Value)
	}

	res, err = c.Check(centre, when, site)
	testutil.AssertNoError(t, err)

	if res.Pass {
		t.Errorf("the galactic centre passed a |b| >= 20° constraint: %v", res)
	}

	if math.Abs(res.Value) > 0.01 {
		t.Errorf("|b| at the centre = %.4f, want 0", res.Value)
	}
}

// TestGalacticLatitudeIsUnsigned: the constraint is a distance from the plane,
// so a target the same way below it has to score the same as one above. A
// signed comparison would pass everything in the northern galactic hemisphere
// and nothing in the southern, which is a plausible-looking half-sky bug.
func TestGalacticLatitudeIsUnsigned(t *testing.T) {
	site := moreSite(t)
	when := time.FromJD(2451545.0, time.UTC)

	// The south galactic pole is the north one's antipode.
	south := NewStar("south galactic pole", angle.Deg(12.85948), angle.Deg(-27.12825))

	res, err := GalacticLatitude{Threshold: angle.Deg(20)}.Check(south, when, site)
	testutil.AssertNoError(t, err)

	if !res.Pass {
		t.Errorf("the south galactic pole failed a |b| >= 20° constraint: %v", res)
	}

	if math.Abs(res.Value-90) > 0.01 {
		t.Errorf("|b| at the south pole = %.4f, want 90", res.Value)
	}
}

func TestTimeWindow(t *testing.T) {
	site := moreSite(t)
	obj := NewStar("anything", angle.Deg(0), angle.Deg(0))

	open := time.Date(2026, time.April, 20, 2, 0, 0, 0, time.LocationUTC)
	shut := time.Date(2026, time.April, 20, 6, 0, 0, 0, time.LocationUTC)

	tests := []struct {
		name string
		when time.Time
		win  TimeWindow
		pass bool
	}{
		{"inside", open.Add(2 * time.Hour), TimeWindow{From: open, To: shut}, true},
		{"on the opening edge", open, TimeWindow{From: open, To: shut}, true},
		{"on the closing edge", shut, TimeWindow{From: open, To: shut}, true},
		{"before it opens", open.Add(-time.Hour), TimeWindow{From: open, To: shut}, false},
		{"after it closes", shut.Add(time.Hour), TimeWindow{From: open, To: shut}, false},

		// One-sided: a deadline with no start, and a start with no deadline.
		{"deadline, in time", open, TimeWindow{To: shut}, true},
		{"deadline, too late", shut.Add(time.Minute), TimeWindow{To: shut}, false},
		{"not before, and it is", shut, TimeWindow{From: open}, true},
		{"not before, and it is not", open.Add(-time.Minute), TimeWindow{From: open}, false},

		// Unbounded both ways constrains nothing, which is what a zero value
		// of this type has to mean: a constraint nobody configured must not
		// silently reject everything.
		{"unbounded", open, TimeWindow{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := tt.win.Check(obj, tt.when, site)
			testutil.AssertNoError(t, err)

			if res.Pass != tt.pass {
				t.Errorf("Pass = %v, want %v (%v)", res.Pass, tt.pass, res)
			}
		})
	}
}

// TestTimeWindowValueRanksByHeadroom: the value is what a scorer ranks on, so
// it has to separate "two hours of window left" from "two minutes left" and
// both from "closed an hour ago".
func TestTimeWindowValueRanksByHeadroom(t *testing.T) {
	site := moreSite(t)
	obj := NewStar("anything", angle.Deg(0), angle.Deg(0))

	open := time.Date(2026, time.April, 20, 2, 0, 0, 0, time.LocationUTC)
	shut := time.Date(2026, time.April, 20, 6, 0, 0, 0, time.LocationUTC)
	win := TimeWindow{From: open, To: shut}

	value := func(when time.Time) float64 {
		t.Helper()

		res, err := win.Check(obj, when, site)
		testutil.AssertNoError(t, err)

		return res.Value
	}

	middle := value(open.Add(2 * time.Hour))
	nearlyShut := value(shut.Add(-2 * time.Minute))
	closed := value(shut.Add(time.Hour))

	if !(middle > nearlyShut) {
		t.Errorf("mid-window scores %.3f and nearly-closed %.3f; more headroom must score higher",
			middle, nearlyShut)
	}

	if !(nearlyShut > closed) {
		t.Errorf("nearly-closed scores %.3f and closed %.3f; inside must beat outside",
			nearlyShut, closed)
	}

	if closed >= 0 {
		t.Errorf("a closed window scores %.3f; outside the window must be negative", closed)
	}

	if math.Abs(middle-2) > 1e-6 {
		t.Errorf("two hours into a four-hour window scores %.6f, want 2", middle)
	}
}

// ridgeProfile is a horizon with one obstruction: flat everywhere except a
// sector to the east, which is what a real site looks like in miniature.
func ridgeProfile(az angle.Angle) angle.Angle {
	d := math.Mod(az.Degrees()+360, 360)
	if d >= 60 && d <= 120 {
		return angle.Deg(25)
	}

	return angle.Deg(0)
}

func ridgeSite(t *testing.T, opts ...SiteOption) *Site {
	t.Helper()

	loc, err := coord.NewGeodetic(angle.Deg(-70.4028), angle.Deg(-24.6251), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	site, err := NewSite("Paranal with a ridge", loc, opts...)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	return site
}

// TestHorizon is the constraint Altitude cannot express: the same altitude is
// observable in one azimuth and behind rock in another, so a single threshold
// cannot describe the site.
//
// This is also the first production consumer of WithHorizonProfile, whose own
// doc comment records that it had none — "purely additive data plumbing today"
// — and names this constraint as the one it was waiting for.
func TestHorizon(t *testing.T) {
	site := ridgeSite(t, WithHorizonProfile(ridgeProfile))
	when := time.Date(2026, time.April, 20, 3, 0, 0, 0, time.LocationUTC)

	c := Horizon{}

	for _, tc := range []struct {
		name string
		ra   angle.Angle
		dec  angle.Angle
	}{
		{"somewhere", angle.Deg(120), angle.Deg(-20)},
		{"elsewhere", angle.Deg(300), angle.Deg(-60)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obj := NewStar(tc.name, tc.ra, tc.dec)

			ctx := coord.NewContext(when, site.Location(), site.Refraction())

			aa, err := skyAltAzCtx(obj, when, ctx)
			testutil.AssertNoError(t, err)

			res, err := c.Check(obj, when, site)
			testutil.AssertNoError(t, err)

			want := aa.Alt().Degrees() - ridgeProfile(aa.Az()).Degrees()
			if math.Abs(res.Value-want) > 1e-9 {
				t.Errorf("clearance = %.6f, want %.6f (alt %.3f, terrain %.3f at az %.3f)",
					res.Value, want, aa.Alt().Degrees(),
					ridgeProfile(aa.Az()).Degrees(), aa.Az().Degrees())
			}

			if res.Pass != (res.Value >= 0) {
				t.Errorf("Pass = %v with clearance %.3f", res.Pass, res.Value)
			}
		})
	}
}

// TestHorizonVariesWithAzimuth is the property, stated directly: one profile,
// one instant, two pointings, and the terrain the constraint compares against
// differs between them. A constraint reading a scalar would report the same
// horizon for both, and that is exactly the bug this type exists to prevent.
func TestHorizonVariesWithAzimuth(t *testing.T) {
	site := ridgeSite(t, WithHorizonProfile(ridgeProfile))

	behind := site.HorizonAt(angle.Deg(90)).Degrees()
	open := site.HorizonAt(angle.Deg(270)).Degrees()

	if behind == open {
		t.Fatalf("the profile reports %.2f in both azimuths; the fixture is not a ridge", behind)
	}

	if behind != 25 || open != 0 {
		t.Errorf("profile gives %.2f east and %.2f west, want 25 and 0", behind, open)
	}
}

// TestHorizonFallsBackToTheScalarLimit: a site with no profile is not a site
// with no horizon. Site.HorizonAt answers with the scalar Horizon() at every
// azimuth, so the constraint degrades to Altitude rather than to nothing —
// which is what keeps it usable at the many sites nobody has surveyed.
func TestHorizonFallsBackToTheScalarLimit(t *testing.T) {
	site := ridgeSite(t) // no profile
	when := time.Date(2026, time.April, 20, 3, 0, 0, 0, time.LocationUTC)
	obj := NewStar("target", angle.Deg(120), angle.Deg(-20))

	ctx := coord.NewContext(when, site.Location(), site.Refraction())

	aa, err := skyAltAzCtx(obj, when, ctx)
	testutil.AssertNoError(t, err)

	res, err := Horizon{}.Check(obj, when, site)
	testutil.AssertNoError(t, err)

	want := aa.Alt().Degrees() - site.Horizon().Degrees()
	if math.Abs(res.Value-want) > 1e-9 {
		t.Errorf("clearance = %.6f, want %.6f against the scalar horizon", res.Value, want)
	}
}

// TestHorizonMarginRaisesTheBar: the margin is clearance above the ridge line
// rather than on it, so it can only make a constraint stricter.
func TestHorizonMarginRaisesTheBar(t *testing.T) {
	site := ridgeSite(t, WithHorizonProfile(ridgeProfile))
	when := time.Date(2026, time.April, 20, 3, 0, 0, 0, time.LocationUTC)
	obj := NewStar("target", angle.Deg(120), angle.Deg(-20))

	bare, err := Horizon{}.Check(obj, when, site)
	testutil.AssertNoError(t, err)

	withMargin, err := Horizon{Margin: angle.Deg(5)}.Check(obj, when, site)
	testutil.AssertNoError(t, err)

	if math.Abs((bare.Value-withMargin.Value)-5) > 1e-9 {
		t.Errorf("a 5° margin moved the clearance by %.6f°, want exactly 5",
			bare.Value-withMargin.Value)
	}
}

// TestHorizonReadsTheSiteItIsGiven: two sites can share a constraint set, so
// the horizon has to come from the site passed to Check rather than from
// anything the constraint captured.
//
// The blocking profile is built around the azimuth the target actually reaches,
// found rather than assumed — the first version of this test picked a fixed
// ridge sector, the target rose outside it, and both sites agreed for a reason
// that had nothing to do with the property under test.
func TestHorizonReadsTheSiteItIsGiven(t *testing.T) {
	when := time.Date(2026, time.April, 20, 3, 0, 0, 0, time.LocationUTC)
	obj := NewStar("target", angle.Deg(120), angle.Deg(-20))

	flat := ridgeSite(t, WithHorizonProfile(func(angle.Angle) angle.Angle { return angle.Deg(0) }))

	ctx := coord.NewContext(when, flat.Location(), flat.Refraction())

	aa, err := skyAltAzCtx(obj, when, ctx)
	testutil.AssertNoError(t, err)

	// A wall exactly where this target is.
	blocking := ridgeSite(t, WithHorizonProfile(func(az angle.Angle) angle.Angle {
		if math.Abs(az.Sub(aa.Az()).WrapPi().Degrees()) < 5 {
			return angle.Deg(89)
		}

		return angle.Deg(0)
	}))

	c := Horizon{}

	open, err := c.Check(obj, when, flat)
	testutil.AssertNoError(t, err)

	blocked, err := c.Check(obj, when, blocking)
	testutil.AssertNoError(t, err)

	if !open.Pass {
		t.Errorf("the target failed against a flat horizon at altitude %.2f: %v",
			aa.Alt().Degrees(), open)
	}

	if blocked.Pass {
		t.Errorf("the target passed a wall at its own azimuth: %v", blocked)
	}

	if math.Abs((open.Value-blocked.Value)-89) > 1e-9 {
		t.Errorf("the two sites differ by %.6f°, want 89 — the horizon is not being read "+
			"from the site passed to Check", open.Value-blocked.Value)
	}
}

// TestSunSepDoesNotExemptOtherPlanets is the boundary on the exemption above,
// and it needs no chosen epoch: Mercury is an inferior planet, so its greatest
// elongation is about 28 degrees and it can never be 30 from the Sun. A
// constraint that waved through every *Planet rather than the Sun itself would
// report a pass here at any instant.
//
// Found by mutation — relaxing the check to any *Planet left every other test
// in this file passing.
func TestSunSepDoesNotExemptOtherPlanets(t *testing.T) {
	site := moreSite(t)

	for _, when := range []time.Time{
		time.Date(2026, time.January, 15, 0, 0, 0, 0, time.LocationUTC),
		time.Date(2026, time.June, 21, 12, 0, 0, 0, time.LocationUTC),
		time.Date(2026, time.November, 3, 6, 0, 0, 0, time.LocationUTC),
	} {
		res, err := SunSep{Threshold: angle.Deg(30)}.Check(NewMercury(nil), when, site)
		testutil.AssertNoError(t, err)

		if res.Pass {
			t.Errorf("at %v Mercury passed a 30° solar-separation constraint at %.2f°; "+
				"an inferior planet never reaches that elongation", when, res.Value)
		}

		if res.Value == 180 {
			t.Errorf("at %v Mercury was reported at 180° from the Sun, which is the "+
				"exemption value — the check is matching any planet, not the Sun", when)
		}
	}
}
