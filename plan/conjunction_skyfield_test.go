//go:build integration

package plan_test

import (
	"math"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

// TestConjunctionsAgreeWithSkyfield holds Conjunctions to Skyfield 1.55's
// conjunctions in apparent right ascension of date — `radec(epoch='date')`,
// equal for the two bodies — found with find_discrete on the same DE440s
// kernel astrogo reads. Jupiter, Saturn and Mars are their system
// barycenters on both sides, as DE440s carries them.
//
// Until #545 Conjunctions equated right ascension on the J2000 equator, and
// matched Skyfield's ICRS right ascension to the second instead: 21 s from
// these for Venus and Jupiter, 2.5 min for Jupiter and Saturn, 39 s for the
// Moon and Venus, 59 s for Mars and Saturn.
func TestConjunctionsAgreeWithSkyfield(t *testing.T) {
	prov, err := eph.NewProvider(kernelContext(t), eph.Planets, "de440s")
	if err != nil {
		t.Fatalf("DE440s provider: %v", err)
	}

	t.Cleanup(func() { _ = prov.Close() })

	// Two seconds: astrogo's apparent place and Skyfield's agree to about a
	// second of conjunction time on these pairs, measured on the ICRS
	// definition both share, and the frame error this replaces is 21 s at
	// its smallest.
	const tolSeconds = 2.0

	day := func(y, m, d int) time.Time { return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.LocationUTC) }

	for _, c := range []struct {
		name       string
		a, b       eph.ID
		start, end time.Time
		skyfield   time.Time
	}{
		{"Venus-Jupiter", eph.Venus, eph.Jupiter, day(2023, 2, 25), day(2023, 3, 8),
			time.Date(2023, 3, 2, 10, 40, 47, 60_000_000, time.LocationUTC)},
		{"Jupiter-Saturn", eph.Jupiter, eph.Saturn, day(2020, 12, 15), day(2020, 12, 28),
			time.Date(2020, 12, 21, 13, 32, 4, 470_000_000, time.LocationUTC)},
		{"Moon-Venus", eph.Moon, eph.Venus, day(2026, 1, 10), day(2026, 1, 25),
			time.Date(2026, 1, 19, 1, 1, 0, 0, time.LocationUTC)},
		{"Mars-Saturn", eph.Mars, eph.Saturn, day(2026, 4, 1), day(2026, 5, 15),
			time.Date(2026, 4, 20, 17, 36, 7, 520_000_000, time.LocationUTC)},
	} {
		events, err := plan.Conjunctions(c.start, c.end, plan.NewPlanet(c.name, c.a, prov), plan.NewPlanet(c.name, c.b, prov))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}

		if len(events) != 1 {
			t.Errorf("%s: %d conjunctions, want 1", c.name, len(events))

			continue
		}

		off := events[0].Time.Sub(c.skyfield).Seconds()
		t.Logf("%s: %v, %+.2f s from Skyfield", c.name, events[0].Time, off)

		if math.Abs(off) > tolSeconds {
			t.Errorf("%s: conjunction %+.2f s from Skyfield's, want within %.0f s", c.name, off, tolSeconds)
		}
	}
}
