package plan

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
)

// TestSubsolarPoint_SolsticeLatitude confirms the subsolar latitude at the
// exact June solstice instant (found via the already-tested Seasons
// solver, not a hardcoded calendar date) matches Earth's axial tilt —
// the IAU/SOFA mean obliquity of the ecliptic at J2000, 23.4392794444°
// (84381.406″). Using the solver's own refined solstice instant avoids
// any "how close to the real solstice is this hardcoded date" error.
func TestSubsolarPoint_SolsticeLatitude(t *testing.T) {
	prov := eph.Default()

	events, err := Seasons(2026, prov)
	if err != nil {
		t.Fatalf("Seasons: %v", err)
	}

	var solstice *SeasonEvent

	for i, e := range events {
		if e.Season == SeasonSummerSolstice {
			solstice = &events[i]
		}
	}

	if solstice == nil {
		t.Fatal("no SeasonSummerSolstice event found in 2026")
	}

	geo, err := SubsolarPoint(prov, solstice.Time)
	if err != nil {
		t.Fatalf("SubsolarPoint: %v", err)
	}

	const wantLat = 23.4392794444 // IAU/SOFA mean obliquity of the ecliptic at J2000

	if got := geo.Lat().Degrees(); math.Abs(got-wantLat) > 0.02 {
		t.Errorf("subsolar latitude at summer solstice = %v°, want %v° ± 0.02°", got, wantLat)
	}
}

// TestSubsolarPoint_LongitudeMatchesLocalSiderealTime cross-checks
// SubsolarPoint's longitude against plan.Site.LocalSiderealTime. At the
// point where the Sun is exactly at the zenith its hour angle is 0, so the
// local apparent sidereal time there equals the Sun's apparent right
// ascension.
//
// Referred to the true equator and equinox of date, which is what sidereal
// time is measured from. This test used to compare against the right
// ascension of the GCRS vector, and called the two sides "an exact algebraic
// identity" because SubsolarPoint rotated that vector by GAST alone: the
// identity held, and both sides were off by the precession and nutation since
// J2000.
//
// The two sides now take different routes through SOFA. SubsolarPoint is the
// CIO-based C2t06a; this side is the equinox-based Pnm06a for the right
// ascension and Gst06a for sidereal time. They are the same rotation stated
// two ways, and agree to float precision; polar motion, the one term only one
// side carries, is zero here because no EOP are loaded.
func TestSubsolarPoint_LongitudeMatchesLocalSiderealTime(t *testing.T) {
	prov := eph.Default()
	tm := time.FromJD(2461000.25, time.UTC) // arbitrary date, no special significance

	sun, err := apparentVec(prov, eph.Sun, tm)
	if err != nil {
		t.Fatalf("apparentVec: %v", err)
	}

	tt1, tt2 := tm.TT().JDParts()
	ofDate := gofaext.Rxp(gofaext.Pnm06a(tt1, tt2), [3]float64{sun.X, sun.Y, sun.Z})
	wantRA := math.Atan2(ofDate[1], ofDate[0])

	geo, err := SubsolarPoint(prov, tm)
	if err != nil {
		t.Fatalf("SubsolarPoint: %v", err)
	}

	site, err := NewSite("subsolar", geo)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	lst, err := site.LocalSiderealTime(tm)
	if err != nil {
		t.Fatalf("LocalSiderealTime: %v", err)
	}

	diff := math.Remainder(lst.Radians()-wantRA, 2*math.Pi)
	t.Logf("LAST at the subsolar point minus the Sun's apparent RA of date: %.3g rad (%.3g arcsec)",
		diff, diff*180/math.Pi*3600)

	if math.Abs(diff) > 1e-9 {
		t.Errorf("LST at subsolar point = %v rad, Sun's apparent RA of date = %v rad, diff = %v rad (want ~0)",
			lst.Radians(), wantRA, diff)
	}
}

// TestSubsolarPoint_EquinoxLatitude holds the subsolar latitude to zero at
// both equinoxes of 2026, which Seasons finds independently of anything here.
//
// An equinox is the instant the Sun's apparent ecliptic longitude of date is
// 0° or 180°, so its apparent declination is its ecliptic latitude times
// cos ε, and the Sun's ecliptic latitude stays within about an arcsecond.
// That is the whole tolerance: the solver's one-second convergence moves the
// declination by 0.02 arcsec.
//
// This is where the old rotation showed. A solstice is not: there the Sun's
// right ascension is 90° or 270°, where precession does not move declination,
// which is how TestSubsolarPoint_SolsticeLatitude passed throughout.
func TestSubsolarPoint_EquinoxLatitude(t *testing.T) {
	prov := eph.Default()

	events, err := Seasons(2026, prov)
	if err != nil {
		t.Fatalf("Seasons: %v", err)
	}

	var n int

	for _, e := range events {
		if e.Season != SeasonVernalEquinox && e.Season != SeasonAutumnalEquinox {
			continue
		}

		n++

		geo, err := SubsolarPoint(prov, e.Time)
		if err != nil {
			t.Fatalf("SubsolarPoint: %v", err)
		}

		lat := geo.Lat().Degrees() * 3600
		t.Logf("%v at %v: subsolar latitude %.3f arcsec", e.Season, e.Time, lat)

		if math.Abs(lat) > 1.5 {
			t.Errorf("%v at %v: subsolar latitude = %.3f arcsec, want within 1.5 of 0", e.Season, e.Time, lat)
		}
	}

	if n != 2 {
		t.Fatalf("Seasons(2026) gave %d equinoxes, want 2", n)
	}
}

// TestSubsolarAndSublunarPointsAreOverhead observes the Sun and the Moon from
// the points SubsolarPoint and SublunarPoint return, through the path every
// other moving-body altitude in this package takes: the apparent place into
// [coord.Context.GeocentricToObserved], with refraction off.
//
// The two tolerances are the two things a direction-only sub-point cannot
// remove. For the Sun it is diurnal aberration, 0.32 arcsec at the equator;
// measured worst 0.319. For the Moon it is parallax: an observer at the
// geodetic sub-point stands up to 21 km off the geocentric line, since
// geodetic and geocentric latitude differ by up to 11.5′, and from the Moon
// that is up to 12 arcsec at perigee; measured worst 9.1. With the old
// rotation both were off by the precession since J2000, 1355 arcsec for the
// Sun in 2026.
func TestSubsolarAndSublunarPointsAreOverhead(t *testing.T) {
	prov := eph.Default()

	for _, body := range []struct {
		name      string
		id        eph.ID
		point     func(eph.Provider, time.Time) (*coord.Geodetic, error)
		tolArcsec float64
	}{
		{"Sun", eph.Sun, SubsolarPoint, 1},
		{"Moon", eph.Moon, SublunarPoint, 15},
	} {
		var worst float64

		for _, jd := range []float64{2451545.0, 2455197.5, 2461318.0, 2469807.5} {
			tm := time.FromJD(jd, time.UTC)

			geo, err := body.point(prov, tm)
			if err != nil {
				t.Fatalf("%s at JD %.1f: %v", body.name, jd, err)
			}

			vec, err := apparentVec(prov, body.id, tm)
			if err != nil {
				t.Fatalf("%s at JD %.1f: apparentVec: %v", body.name, jd, err)
			}

			ctx := coord.NewContext(tm, geo, atmosphere.Refraction{Pressure: 0})
			zd := 90*3600 - ctx.GeocentricToObserved(vec).Alt().Degrees()*3600
			worst = max(worst, zd)

			if zd > body.tolArcsec {
				t.Errorf("%s at JD %.1f: %.3f arcsec from the zenith at its own sub-point, want under %g",
					body.name, jd, zd, body.tolArcsec)
			}
		}

		t.Logf("%s: worst zenith distance at its sub-point %.3f arcsec", body.name, worst)
	}
}

// TestTerminator_PointsAtCorrectSeparationFromSubsolarPoint checks every
// TwilightKind's Terminator output is exactly kind.zenithAngle() away
// from the subsolar point, via coord.Separation — a code path independent
// of both SubsolarPoint and coord.SmallCircle's own construction.
func TestTerminator_PointsAtCorrectSeparationFromSubsolarPoint(t *testing.T) {
	prov := eph.Default()
	tm := time.FromJD(2461000.25, time.UTC)

	sub, err := SubsolarPoint(prov, tm)
	if err != nil {
		t.Fatalf("SubsolarPoint: %v", err)
	}

	subICRS := coord.NewICRS(sub.Lon(), sub.Lat())

	kinds := []TwilightKind{GeometricTwilight, ApparentTwilight, CivilTwilight, NauticalTwilight, AstronomicalTwilight}

	for _, kind := range kinds {
		pts, err := Terminator(prov, tm, kind, 24)
		if err != nil {
			t.Fatalf("Terminator(%v): %v", kind, err)
		}

		wantSep := kind.zenithAngle().Degrees()

		for i, p := range pts {
			sep := coord.Separation(subICRS, coord.NewICRS(p.Lon(), p.Lat()))
			if math.Abs(sep.Degrees()-wantSep) > 1e-6 {
				t.Errorf("%v point %d: separation from subsolar point = %v°, want %v°", kind, i, sep.Degrees(), wantSep)
			}
		}
	}
}

// TestTerminator_EquinoxGeometricPassesNearPoles confirms the geometric
// terminator at the equinox instant (found via Seasons, not a hardcoded
// date) passes through both poles — the subsolar point sits on the equator
// at that instant, so its 90°-radius small circle reaches lat=±90°, less the
// subsolar latitude TestSubsolarPoint_EquinoxLatitude bounds by 1.5 arcsec.
//
// The bound used to be 1.5°, explained as a frame convention gap: Seasons
// finds the Sun's true-of-date longitude crossing 0°, while SubsolarPoint
// placed the Sun by its fixed-frame direction with no precession-nutation
// applied. That was not a convention to choose between but the defect in
// coord.SubPoint, which rotated a GCRS vector by GAST alone: the terminator
// stopped 0.146° short of the pole in 2026, and now stops 0.3 arcsec short.
func TestTerminator_EquinoxGeometricPassesNearPoles(t *testing.T) {
	prov := eph.Default()

	events, err := Seasons(2026, prov)
	if err != nil {
		t.Fatalf("Seasons: %v", err)
	}

	var equinox *SeasonEvent

	for i, e := range events {
		if e.Season == SeasonVernalEquinox {
			equinox = &events[i]
		}
	}

	if equinox == nil {
		t.Fatal("no SeasonVernalEquinox event found in 2026")
	}

	pts, err := Terminator(prov, equinox.Time, GeometricTwilight, 36)
	if err != nil {
		t.Fatalf("Terminator: %v", err)
	}

	var maxAbsLat float64

	for _, p := range pts {
		if abs := math.Abs(p.Lat().Degrees()); abs > maxAbsLat {
			maxAbsLat = abs
		}
	}

	const wantArcsec = 1.5

	if short := (90 - maxAbsLat) * 3600; short > wantArcsec {
		t.Errorf("max |lat| among equinox terminator points = %v°, %.3f arcsec short of the pole, want under %g",
			maxAbsLat, short, wantArcsec)
	}
}

// TestTerminator_TooFewPoints confirms coord.SmallCircle's ErrTooFewPoints
// propagates through Terminator unwrapped-but-reachable via errors.Is.
func TestTerminator_TooFewPoints(t *testing.T) {
	prov := eph.Default()
	tm := time.FromJD(2461000.25, time.UTC)

	if _, err := Terminator(prov, tm, GeometricTwilight, 2); !errors.Is(err, coord.ErrTooFewPoints) {
		t.Errorf("Terminator(n=2) error = %v, want ErrTooFewPoints", err)
	}
}

// TestSublunarPoint_Basic is a light sanity check that SublunarPoint
// returns a plausible geodetic point (finite, in range) — the Moon's
// declination swings roughly ±28° over its 18.6-year nodal cycle, so no
// tight known-value check is attempted here, only structural validity.
func TestSublunarPoint_Basic(t *testing.T) {
	prov := eph.Default()
	tm := time.FromJD(2461000.25, time.UTC)

	geo, err := SublunarPoint(prov, tm)
	if err != nil {
		t.Fatalf("SublunarPoint: %v", err)
	}

	if lat := geo.Lat().Degrees(); lat < -30 || lat > 30 {
		t.Errorf("sublunar latitude = %v°, outside the Moon's plausible ±28° declination range", lat)
	}
}

// TestTerminatorFunctions_NilProviderDefaults is a regression test: all
// three exported functions in this file call eph.Position(p, ...)
// directly (bypassing plan.NewSun/NewMoon), so each needs its own
// nil-provider guard.
func TestTerminatorFunctions_NilProviderDefaults(t *testing.T) {
	tm := time.FromJD(2461000.25, time.UTC)

	if _, err := SubsolarPoint(nil, tm); err != nil {
		t.Errorf("SubsolarPoint(nil): unexpected error: %v", err)
	}

	if _, err := SublunarPoint(nil, tm); err != nil {
		t.Errorf("SublunarPoint(nil): unexpected error: %v", err)
	}

	if _, err := Terminator(nil, tm, GeometricTwilight, 12); err != nil {
		t.Errorf("Terminator(nil): unexpected error: %v", err)
	}
}
