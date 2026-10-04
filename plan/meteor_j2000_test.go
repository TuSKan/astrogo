package plan

import (
	"math"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// TestSunLongitudeJ2000MatchesSkyfield checks the solar longitude meteor
// showers are compared with against Skyfield 1.54's geometric longitude on
// ecliptic_J2000_frame (DE421), at the issue's four instants (#415):
// 0.0004° at worst on the analytical ephemeris, about half a minute of time.
func TestSunLongitudeJ2000MatchesSkyfield(t *testing.T) {
	for _, c := range []struct {
		at   time.Time
		want float64
	}{
		{time.Date(2026, 7, 16, 21, 50, 0, 0, time.LocationUTC), 114.009},
		{time.Date(2026, 8, 24, 23, 54, 0, 0, time.LocationUTC), 151.461},
		{time.Date(2026, 8, 12, 16, 52, 0, 0, time.LocationUTC), 139.634},
		{time.Date(2026, 8, 13, 2, 0, 33, 0, time.LocationUTC), 140.000},
	} {
		got, err := sunLongitudeJ2000(c.at, eph.Default())
		if err != nil {
			t.Fatal(err)
		}

		if math.Abs(got-c.want) > 0.002 {
			t.Errorf("%v: λ☉ (J2000) %.4f°, Skyfield gives %.3f°", c.at, got, c.want)
		}
	}
}

// TestPerseidsFollowIMOsJ2000Longitudes: IMO's 2026 calendar has the
// Perseids active from July 17, with the maximum on August 13 at 02h–04h UT
// (node at λ☉ = 140.0°–140.1°, "for the equinox 2000.0"). Compared with the
// Sun's longitude of date, as before #415, the window opened at 21:50 on July
// 16 and the peak came at 16:52 on August 12, about nine hours early.
func TestPerseidsFollowIMOsJ2000Longitudes(t *testing.T) {
	prov := eph.Default()
	per := MeteorShowers["perseids"]

	for _, c := range []struct {
		at   time.Time
		want bool
	}{
		{time.Date(2026, 7, 17, 3, 0, 0, 0, time.LocationUTC), false},
		{time.Date(2026, 7, 17, 9, 0, 0, 0, time.LocationUTC), true},
	} {
		active, err := per.IsActive(c.at, prov)
		if err != nil {
			t.Fatal(err)
		}

		if active != c.want {
			t.Errorf("Perseids active at %v: %v, want %v", c.at, active, c.want)
		}
	}

	// At λ☉ (J2000) = 140.0°, 2026-08-13 02:00:33 UT by Skyfield, the radiant
	// is the tabulated peak radiant.
	ra, dec, err := per.RadiantAt(time.Date(2026, 8, 13, 2, 0, 33, 0, time.LocationUTC), prov)
	if err != nil {
		t.Fatal(err)
	}

	if math.Abs(ra.Degrees()-per.RadiantRA.Degrees()) > 0.01 || math.Abs(dec.Degrees()-per.RadiantDec.Degrees()) > 0.01 {
		t.Errorf("Perseids radiant at IMO's peak: (%.3f°, %.3f°), want the peak radiant (%.3f°, %.3f°)",
			ra.Degrees(), dec.Degrees(), per.RadiantRA.Degrees(), per.RadiantDec.Degrees())
	}
}
