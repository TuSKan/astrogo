package plan

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
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
// SubsolarPoint's longitude against plan.Site.LocalSiderealTime — an
// entirely separate code path (GAST + site longitude). At the point where
// the Sun is exactly at the zenith, the Sun's hour angle is 0 by
// definition, which means Local Sidereal Time there must equal the Sun's
// own right ascension. This is an exact algebraic identity (both sides
// reduce to the same GAST-based formula), so the tolerance here is purely
// float precision, not a physical approximation.
func TestSubsolarPoint_LongitudeMatchesLocalSiderealTime(t *testing.T) {
	prov := eph.Default()
	tm := time.FromJD(2461000.25, time.UTC) // arbitrary date, no special significance

	sunVec, err := eph.Position(prov, eph.Sun, tm)
	if err != nil {
		t.Fatalf("eph.Position: %v", err)
	}

	wantRA := math.Atan2(sunVec.Y, sunVec.X)

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

	diff := lst.Radians() - wantRA
	for diff > math.Pi {
		diff -= 2 * math.Pi
	}

	for diff < -math.Pi {
		diff += 2 * math.Pi
	}

	if math.Abs(diff) > 1e-9 {
		t.Errorf("LST at subsolar point = %v rad, Sun RA = %v rad, diff = %v rad (want ~0)", lst.Radians(), wantRA, diff)
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
// date) passes close to both poles — the subsolar point sits close to the
// equator at that instant, so its 90°-radius small circle should closely
// approach lat=±90°.
//
// The tolerance here (1.5°, not the sub-arcsecond precision coord's own
// synthetic-exactly-equatorial-center test uses) is set by a real,
// already-documented frame convention gap, not sloppy math: Seasons finds
// the instant the Sun's TRUE-OF-DATE ecliptic longitude crosses 0°, while
// SubsolarPoint/eph.Position report the Sun's direction in the fixed
// ICRS/J2000 equatorial frame (no precession-nutation applied) — the same
// convention difference behind the ~1142″ RA gap this codebase's own
// Horizons-comparison tests document elsewhere (session history: "CIRS-
// vs-True-Equinox frame convention difference"). Left as a real,
// documented limitation rather than "corrected" here — fixing it would
// mean picking a side of a genuine of-date-vs-fixed-frame design question
// that spans more than this one feature.
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

	if maxAbsLat < 88.5 {
		t.Errorf("max |lat| among equinox terminator points = %v°, want >= 88.5° (close to 90°)", maxAbsLat)
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
