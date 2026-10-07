package plan

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/time"
)

// TestStarRiseAndSetAgreeWithSkyfield holds a star's rise and set to
// Skyfield 1.55's almanac.find_risings and find_settings on DE440s (#568),
// whose horizon for a star is the almanac one, 34′ of refraction below the
// geometric horizon:
//
//	obs = earth + wgs84.latlon(lat, 0)
//	find_risings(obs, Star(ra_hours=ra/15, dec_degrees=dec), ts.utc(2026, 3, 20), ts.utc(2026, 3, 21))
//
// Until #568 a star's threshold was the dip alone, 0° at sea level, so every
// one of these came 2.4 to 7 minutes late, or as early at a set.
func TestStarRiseAndSetAgreeWithSkyfield(t *testing.T) {
	t.Parallel()

	stars := map[string][2]float64{
		"Sirius": {101.2872, -16.7161},
		"Vega":   {279.2347, 38.7837},
	}

	for _, c := range []struct {
		star                 string
		lat                  float64
		kind                 string
		hour, minute, second int
		millisecond          float64
	}{
		{"Sirius", 0, "rise", 12, 51, 40, 661.181},
		{"Sirius", 0, "set", 0, 58, 21, 894.607},
		{"Vega", 0, "rise", 0, 44, 36, 822.932},
		{"Vega", 0, "set", 12, 48, 26, 953.074},
		{"Sirius", 45, "rise", 14, 0, 24, 28.833},
		{"Sirius", 45, "set", 23, 45, 42, 600.447},
		{"Vega", 45, "rise", 21, 3, 3, 466.977},
		{"Vega", 45, "set", 16, 26, 4, 438.316},
		{"Sirius", 60, "rise", 14, 53, 50, 733.978},
		{"Sirius", 60, "set", 22, 52, 15, 894.058},
	} {
		site, err := NewSiteEarthLocation("sea level", c.lat, 0, 0)
		if err != nil {
			t.Fatalf("NewSiteEarthLocation: %v", err)
		}

		radec := stars[c.star]
		star := NewStar(c.star, angle.Deg(radec[0]), angle.Deg(radec[1]))

		start := time.Date(2026, 3, 20, 0, 0, 0, 0, time.LocationUTC)
		end := time.Date(2026, 3, 21, 0, 0, 0, 0, time.LocationUTC)

		events, err := VisibilityEvents(start, end, star, site)
		if err != nil {
			t.Fatalf("%s at %g°: VisibilityEvents: %v", c.star, c.lat, err)
		}

		kind := EventRise
		if c.kind == "set" {
			kind = EventSet
		}

		want := time.Date(2026, 3, 20, c.hour, c.minute, c.second, int(math.Round(c.millisecond*1e6)), time.LocationUTC)

		var found []time.Time

		for _, e := range events {
			if e.Kind == kind {
				found = append(found, e.Time)
			}
		}

		if len(found) != 1 {
			t.Errorf("%s at %g°: %d %ss on 2026-03-20, Skyfield finds one at %v", c.star, c.lat, len(found), c.kind, want)

			continue
		}

		t.Logf("%s at %g°: %s %.3f s from Skyfield", c.star, c.lat, c.kind, found[0].Sub(want).Seconds())

		// Measured within 0.19 s. DUT1, which the plan tests run without,
		// is 0.07 s this month, and a star's apparent place from a
		// different ephemeris differs by milliarcseconds.
		if d := math.Abs(found[0].Sub(want).Seconds()); d > 1 {
			t.Errorf("%s at %g°: %s %v, Skyfield %v, %.2f s apart", c.star, c.lat, c.kind, found[0], want, d)
		}
	}
}
