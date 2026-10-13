package plan

import (
	"math"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestRiseAndSetAtAltitudeAgreeWithSkyfield holds the Sun's and the Moon's
// rise and set on Everest's summit, 8849 m up, to Skyfield 1.55's
// almanac.find_risings and find_settings on DE440s, given the site's own
// threshold (#621):
//
//	obs = earth + wgs84.latlon(27.9881, 86.925, elevation_m=8849)
//	find_risings(obs, sun, ts.utc(2026, 3, 20), ts.utc(2026, 3, 21), horizon_degrees=-3.5927609566)
//	find_risings(obs, moon, ts.utc(2026, 3, 20), ts.utc(2026, 3, 21), horizon_degrees=-3.5843609566)
//
// The threshold is −(semi-diameter + 34′ + 1.76′√h), and 2.76° of it is the
// dip, the almanac's convention for a sea horizon. Together with the
// topocentric position 8849 m up, it puts each event here 12.5 to 15.1
// minutes from its sea-level time.
//
// USNO cannot check this: its rise/set service ignores height. Until #621
// the only check was usno_test.go's, that the events moved at least 3
// minutes from astrogo's own sea-level times.
//
// Skyfield was given a fixed horizon, so for the Moon this compares the
// center at MoonRiseSetThreshold's mean semi-diameter, through the solver
// with that threshold. MoonEvents itself puts the upper limb on the horizon
// with the semi-diameter of the instant (#693), which
// TestMoonEventsPutTheUpperLimbOnTheHorizon holds; what this test is for is
// the height.
func TestRiseAndSetAtAltitudeAgreeWithSkyfield(t *testing.T) {
	t.Parallel()

	site, err := NewSiteEarthLocation("Everest", 27.9881, 86.925, 8849)
	if err != nil {
		t.Fatalf("NewSiteEarthLocation: %v", err)
	}

	// The thresholds the reference times were computed for.
	for _, th := range []struct {
		body      string
		got, want float64
	}{
		{"Sun", site.SunRiseSetThreshold().Degrees(), -3.5927609566},
		{"Moon", site.MoonRiseSetThreshold().Degrees(), -3.5843609566},
	} {
		if math.Abs(th.got-th.want) > 1e-9 {
			t.Fatalf("%s threshold %.10f°, the references were computed for %.10f°", th.body, th.got, th.want)
		}
	}

	for _, c := range []struct {
		body                 string
		month, day           int
		kind                 string
		hour, minute, second int
		millisecond          float64
	}{
		{"Sun", 3, 20, "rise", 0, 4, 7, 861},
		{"Sun", 3, 20, "set", 12, 35, 55, 118},
		{"Sun", 6, 21, "set", 13, 25, 48, 533},
		{"Sun", 6, 21, "rise", 23, 2, 32, 444},
		{"Sun", 12, 21, "rise", 0, 45, 21, 6},
		{"Sun", 12, 21, "set", 11, 35, 7, 824},
		{"Moon", 3, 20, "rise", 0, 34, 7, 669},
		{"Moon", 3, 20, "set", 14, 4, 40, 830},
		{"Moon", 6, 21, "rise", 5, 25, 35, 281},
		{"Moon", 6, 21, "set", 18, 12, 15, 554},
		{"Moon", 12, 21, "rise", 8, 16, 56, 174},
		{"Moon", 12, 21, "set", 23, 9, 54, 914},
	} {
		start := time.Date(2026, time.Month(c.month), c.day, 0, 0, 0, 0, time.LocationUTC)
		end := time.Date(2026, time.Month(c.month), c.day+1, 0, 0, 0, 0, time.LocationUTC)

		events := SunEvents
		if c.body == "Moon" {
			events = moonCenterEvents
		}

		got, err := events(start, end, site, eph.Default())
		if err != nil {
			t.Fatalf("%s events on %v: %v", c.body, start, err)
		}

		kind := EventRise
		if c.kind == "set" {
			kind = EventSet
		}

		want := time.Date(2026, time.Month(c.month), c.day, c.hour, c.minute, c.second, int(math.Round(c.millisecond*1e6)), time.LocationUTC)

		var found []time.Time

		for _, e := range got {
			if e.Kind == kind {
				found = append(found, e.Time)
			}
		}

		if len(found) != 1 {
			t.Errorf("%s %s on 2026-%02d-%02d: found %d, Skyfield finds one at %v", c.body, c.kind, c.month, c.day, len(found), want)

			continue
		}

		t.Logf("%s %s on 2026-%02d-%02d: %.3f s from Skyfield", c.body, c.kind, c.month, c.day, found[0].Sub(want).Seconds())

		// Measured within 0.09 s for the Sun and 0.31 s for the Moon, on the
		// analytical ephemeris against DE440s and without DUT1.
		if d := math.Abs(found[0].Sub(want).Seconds()); d > 1 {
			t.Errorf("%s %s %v, Skyfield %v, %.2f s apart", c.body, c.kind, found[0], want, d)
		}
	}
}

// moonCenterEvents is MoonEvents with the Moon's center held to
// MoonRiseSetThreshold, the fixed horizon a reference computed that way was
// given, rather than its upper limb to RiseSetThreshold.
func moonCenterEvents(start, end time.Time, site *Site, prov eph.Provider) ([]Event, error) {
	return NewEventSolver(unit.Minutes(15), unit.Seconds(1)).Find(EventSpec{
		Family:    EventFamilyVisibility,
		Kind:      EventAnyVisibility,
		Target:    NewMoon(prov),
		Observer:  site,
		Threshold: site.MoonRiseSetThreshold(),
	}, start, end)
}
