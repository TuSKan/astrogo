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
	requireKernel(t, "DE440s provider", err)

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

// TestAppulsesAgreeWithSkyfield holds the instant of minimum separation and
// the separation there to Skyfield 1.55 on DE440s, on the same four pairs
// (#578). Skyfield's minimum is the separation of the two apparent places,
// scanned every 10 s within a day of the conjunction and then every 0.01 s.
//
// A scan rather than an optimizer, because the minimum is flat: on
// Venus-Jupiter the separation changes by 0.8" over 21 minutes, and a
// bounded optimizer with a tolerance in time stopped that far from it.
func TestAppulsesAgreeWithSkyfield(t *testing.T) {
	prov, err := eph.NewProvider(kernelContext(t), eph.Planets, "de440s")
	requireKernel(t, "DE440s provider", err)

	t.Cleanup(func() { _ = prov.Close() })

	day := func(y, m, d int) time.Time { return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.LocationUTC) }

	for _, c := range []struct {
		name       string
		a, b       eph.ID
		start, end time.Time
		skyfield   time.Time
		sepDeg     float64
	}{
		{"Venus-Jupiter", eph.Venus, eph.Jupiter, day(2023, 2, 25), day(2023, 3, 8),
			time.Date(2023, 3, 2, 5, 5, 18, 370_000_000, time.LocationUTC), 0.489177},
		{"Jupiter-Saturn", eph.Jupiter, eph.Saturn, day(2020, 12, 15), day(2020, 12, 28),
			time.Date(2020, 12, 21, 18, 21, 0, 40_000_000, time.LocationUTC), 0.101773},
		{"Moon-Venus", eph.Moon, eph.Venus, day(2026, 1, 10), day(2026, 1, 25),
			time.Date(2026, 1, 19, 2, 23, 50, 760_000_000, time.LocationUTC), 2.042813},
		{"Mars-Saturn", eph.Mars, eph.Saturn, day(2026, 4, 1), day(2026, 5, 15),
			time.Date(2026, 4, 19, 22, 14, 27, 20_000_000, time.LocationUTC), 1.194358},
	} {
		events, err := plan.Appulses(c.start, c.end, plan.NewPlanet(c.name, c.a, prov), plan.NewPlanet(c.name, c.b, prov))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}

		if len(events) != 1 {
			t.Errorf("%s: %d appulses, want 1", c.name, len(events))

			continue
		}

		off := events[0].Time.Sub(c.skyfield).Seconds()
		t.Logf("%s: %v, %+.2f s from Skyfield, separation %.6f°", c.name, events[0].Time, off, events[0].Value)

		// Two seconds, as for the conjunctions; the separation to a tenth
		// of an arcsecond, which the flat minimum makes the sharper test.
		if math.Abs(off) > 2 {
			t.Errorf("%s: appulse %+.2f s from Skyfield's", c.name, off)
		}

		if d := math.Abs(events[0].Value - c.sepDeg); d > 3e-5 {
			t.Errorf("%s: separation %.6f°, Skyfield %.6f°", c.name, events[0].Value, c.sepDeg)
		}
	}
}
