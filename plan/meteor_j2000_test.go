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

// TestPerseidsFollowIMOsJ2000Longitudes: IMO's calendar has the Perseids
// active from July 17, which in 2027, the year it is for, begins at
// λ☉ = 113.85° "for the equinox 2000.0". In 2026 the Sun reaches that at
// 17:50 UT on July 16 by Skyfield (DE440s, ecliptic_J2000_frame). Compared
// with the Sun's longitude of date, as before #415, the window would open at
// 08:27, about nine hours early; the maximum, at λ☉ = 140.0°, likewise came
// at 16:52 on August 12 instead of 02:00 on August 13.
func TestPerseidsFollowIMOsJ2000Longitudes(t *testing.T) {
	prov := eph.Default()
	per := meteorShowers["perseids"]

	for _, c := range []struct {
		at   time.Time
		want bool
	}{
		{time.Date(2026, 7, 16, 13, 0, 0, 0, time.LocationUTC), false},
		{time.Date(2026, 7, 16, 22, 0, 0, 0, time.LocationUTC), true},
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

// TestMeteorShowersAreIMOs2027WorkingList holds every shower to Table 5 of
// the IMO Meteor Shower Calendar 2027 (#573), and every activity window to
// that table's dates: the Sun's J2000 longitude at 0h UT on the first date
// of activity and at 24h UT on the last, in 2027.
//
// The table had drifted from IMO's: the Southern δ-Aquariids peaked at
// λ☉ = 125° rather than 128°, three days early, and every maximum was
// rounded to the whole degree, 17 hours for the Ursids.
func TestMeteorShowersAreIMOs2027WorkingList(t *testing.T) {
	t.Parallel()

	prov := eph.Default()

	for _, c := range []struct {
		key              string
		peak, ra, dec    float64
		velocity, r, zhr float64
		first, last      [3]int // IMO's dates of activity, inclusive
	}{
		{"quadrantids", 283.15, 230, 49, 41, 2.1, 80, [3]int{2026, 12, 28}, [3]int{2027, 1, 12}},
		{"lyrids", 32.32, 271, 34, 49, 2.1, 18, [3]int{2027, 4, 14}, [3]int{2027, 4, 30}},
		{"eta_aquariids", 45.5, 338, -1, 66, 2.4, 50, [3]int{2027, 4, 19}, [3]int{2027, 5, 28}},
		{"southern_delta_aquariids", 128, 340, -16, 41, 2.5, 25, [3]int{2027, 7, 12}, [3]int{2027, 8, 23}},
		{"perseids", 140.0, 48, 58, 59, 2.2, 110, [3]int{2027, 7, 17}, [3]int{2027, 8, 24}},
		{"orionids", 208, 95, 16, 66, 2.5, 20, [3]int{2027, 10, 2}, [3]int{2027, 11, 7}},
		{"leonids", 235.27, 152, 22, 71, 2.5, 15, [3]int{2027, 11, 6}, [3]int{2027, 11, 30}},
		{"geminids", 262.2, 112, 33, 35, 2.6, 150, [3]int{2027, 12, 4}, [3]int{2027, 12, 20}},
		{"ursids", 270.7, 217, 76, 33, 2.8, 10, [3]int{2027, 12, 17}, [3]int{2027, 12, 26}},
	} {
		m, ok := meteorShowers[c.key]
		if !ok {
			t.Errorf("%s: not in the table", c.key)

			continue
		}

		// Angles and velocities are stored in radians and m/s, so they come
		// back with a rounding error in the last place.
		near := func(got, want float64) bool { return math.Abs(got-want) <= 1e-9 }

		if !near(m.PeakSolarLongitude, c.peak) || !near(m.RadiantRA.Degrees(), c.ra) || !near(m.RadiantDec.Degrees(), c.dec) ||
			!near(m.Velocity.KmPerSec(), c.velocity) || !near(m.PopulationIndex, c.r) || !near(m.ZHR, c.zhr) {
			t.Errorf("%s: λ☉ %g, α %g, δ %g, V∞ %g, r %g, ZHR %g; IMO 2027 gives %g, %g, %g, %g, %g, %g",
				c.key, m.PeakSolarLongitude, m.RadiantRA.Degrees(), m.RadiantDec.Degrees(),
				m.Velocity.KmPerSec(), m.PopulationIndex, m.ZHR,
				c.peak, c.ra, c.dec, c.velocity, c.r, c.zhr)
		}

		// The day after the last date, at 0h UT, is the end of that date.
		for _, w := range []struct {
			name string
			date [3]int
			day  int
			got  float64
		}{
			{"start", c.first, 0, m.ActiveStartSolarLon},
			{"end", c.last, 1, m.ActiveEndSolarLon},
		} {
			at := time.Date(w.date[0], time.Month(w.date[1]), w.date[2]+w.day, 0, 0, 0, 0, time.LocationUTC)

			want, err := sunLongitudeJ2000(at, prov)
			if err != nil {
				t.Fatalf("%s: %v", c.key, err)
			}

			if math.Abs(w.got-want) > 0.01 {
				t.Errorf("%s: activity %s at λ☉ %.2f°; the Sun is at %.3f° at %v", c.key, w.name, w.got, want, at)
			}
		}
	}
}
