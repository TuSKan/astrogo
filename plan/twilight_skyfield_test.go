package plan

import (
	"math"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// skyfieldTwilightCrossing is one instant at which the Sun's apparent
// altitude, without refraction, crossed a twilight threshold in Skyfield
// 1.55 with DE440s. dusk is true when the Sun went below. Found by
//
//	obs = earth + wgs84.latlon(lat, lon, elevation)
//	f = lambda t: obs.at(t).observe(sun).apparent().altaz()[0].degrees < threshold
//	f.step_days = 1/1440
//	find_discrete(start, end, f, epsilon=1e-4/86400)
//
// at a 1-minute step rather than almanac.dark_twilight_day's 0.04 days:
// at that step Skyfield misses every solstice night's darkness at 48.4°N,
// 43 minutes of it.
type skyfieldTwilightCrossing struct {
	month, day, hour, minute, second int
	millisecond                      float64
	dusk                             bool
}

// TestTwilightAgreesWithSkyfield holds nautical and astronomical twilight to
// Skyfield. USNO, which checks civil twilight in usno_test.go, publishes no
// other kind, so until #565 these two were checked for their thresholds and
// ordering alone.
//
// The grazing case is the reason this is more than a repeat of civil: at
// 48.562°N around the June solstice the Sun only just passes −18°, and on the
// night of 20–21 June the darkness lasts 5.6 minutes, shorter than the
// solver's 15-minute search step. At 1.45°W the four shortest nights fall
// wholly between the samples at 00:00 and 00:15 UTC, so only the solver's
// search for hidden crossings (#426) can find them; at 0° the 00:00 sample
// lands inside each of them and they would be found without it.
func TestTwilightAgreesWithSkyfield(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		lat, lon, elev float64
		kind           TwilightKind
		from, to       [2]int // month and day in 2026; to is the day after the last
		want           []skyfieldTwilightCrossing
	}{
		{
			name: "La Silla nautical", lat: -29.2567, lon: -70.7346, elev: 2400,
			kind: NauticalTwilight, from: [2]int{3, 19}, to: [2]int{3, 22},
			want: []skyfieldTwilightCrossing{
				{3, 19, 9, 54, 27, 389.011, false},
				{3, 19, 23, 46, 13, 835.676, true},
				{3, 20, 9, 55, 4, 731.476, false},
				{3, 20, 23, 45, 1, 472.762, true},
				{3, 21, 9, 55, 41, 696.232, false},
				{3, 21, 23, 43, 49, 236.098, true},
			},
		},
		{
			name: "La Silla astronomical", lat: -29.2567, lon: -70.7346, elev: 2400,
			kind: AstronomicalTwilight, from: [2]int{3, 19}, to: [2]int{3, 22},
			want: []skyfieldTwilightCrossing{
				{3, 19, 0, 15, 18, 247.096, true},
				{3, 19, 9, 26, 34, 181.993, false},
				{3, 20, 0, 14, 3, 541.848, true},
				{3, 20, 9, 27, 13, 731.487, false},
				{3, 21, 0, 12, 49, 63.716, true},
				{3, 21, 9, 27, 52, 778.549, false},
			},
		},
		{
			name: "grazing astronomical at 48.562N 1.45W", lat: 48.562, lon: -1.45, elev: 0,
			kind: AstronomicalTwilight, from: [2]int{6, 17}, to: [2]int{6, 26},
			want: []skyfieldTwilightCrossing{
				{6, 17, 0, 20, 30, 531.903, false},
				{6, 17, 23, 55, 59, 741.692, true},
				{6, 18, 0, 17, 39, 552.424, false},
				{6, 18, 23, 59, 14, 492.074, true},
				{6, 19, 0, 14, 51, 951.241, false},
				{6, 20, 0, 2, 19, 535.648, true},
				{6, 20, 0, 12, 14, 65.998, false},
				{6, 21, 0, 4, 41, 846.444, true},
				{6, 21, 0, 10, 18, 871.170, false},
				{6, 22, 0, 4, 24, 428.473, true},
				{6, 22, 0, 11, 3, 313.455, false},
				{6, 23, 0, 2, 7, 408.698, true},
				{6, 23, 0, 13, 47, 218.863, false},
				{6, 23, 23, 59, 25, 79.544, true},
				{6, 24, 0, 16, 56, 249.421, false},
				{6, 24, 23, 56, 36, 70.080, true},
				{6, 25, 0, 20, 11, 732.327, false},
				{6, 25, 23, 53, 44, 489.880, true},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			site, err := NewSiteEarthLocation(c.name, c.lat, c.lon, c.elev)
			if err != nil {
				t.Fatalf("NewSiteEarthLocation: %v", err)
			}

			start := time.Date(2026, time.Month(c.from[0]), c.from[1], 0, 0, 0, 0, time.LocationUTC)
			end := time.Date(2026, time.Month(c.to[0]), c.to[1], 0, 0, 0, 0, time.LocationUTC)

			groups, err := TwilightEvents(start, end, site, eph.Default(), c.kind)
			if err != nil {
				t.Fatalf("TwilightEvents: %v", err)
			}

			// Each group is a dusk and the dawn after it, in order, so
			// flattening them keeps the crossings in time order.
			var got []Event

			for _, g := range groups {
				if g.Dusk != nil {
					got = append(got, *g.Dusk)
				}

				if g.Dawn != nil {
					got = append(got, *g.Dawn)
				}
			}

			if len(got) != len(c.want) {
				t.Fatalf("found %d crossings, Skyfield finds %d", len(got), len(c.want))
			}

			worst := 0.0

			for i, w := range c.want {
				wantAt := time.Date(2026, time.Month(w.month), w.day, w.hour, w.minute, w.second,
					int(math.Round(w.millisecond*1e6)), time.LocationUTC)

				if dusk := got[i].Kind == EventSet; dusk != w.dusk {
					t.Errorf("crossing %d at %v: dusk=%v, Skyfield says %v", i, wantAt, dusk, w.dusk)
				}

				diff := math.Abs(got[i].Time.Sub(wantAt).Seconds())
				worst = math.Max(worst, diff)

				if diff > 1 {
					t.Errorf("crossing %d: %v, Skyfield %v, %.3f s apart", i, got[i].Time, wantAt, diff)
				}
			}

			t.Logf("worst disagreement with Skyfield: %.3f s over %d crossings", worst, len(got))
		})
	}
}
