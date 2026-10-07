//go:build integration

package plan_test

import (
	"math"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestGreatestElongationsAgreeWithSkyfield holds GreatestElongations to
// Skyfield 1.55 on the same DE440s kernel: the maxima, found with
// find_maxima, of the separation between the planet's and the Sun's apparent
// geocentric places (`observe(...).apparent().separation_from(...)`). The
// instants and elongations below are Skyfield's for 2026, computed on
// 2026-10-06.
//
// Each search spans ten days either side of Skyfield's instant, so the test
// costs a few hundred samples rather than a year's.
//
// Measured, astrogo's instants are within 0.58 s and its elongations agree to
// the fourth decimal (#557). A greatest elongation is a flat maximum, so the
// instant is the poorly determined half of the answer: 1 s is the bound for it,
// 1e-4 degrees for the elongation itself.
func TestGreatestElongationsAgreeWithSkyfield(t *testing.T) {
	prov, err := eph.NewProvider(kernelContext(t), eph.Planets, "de440s")
	if err != nil {
		t.Fatalf("DE440s provider: %v", err)
	}

	t.Cleanup(func() { _ = prov.Close() })

	sun := plan.NewSun(prov)

	at := func(y, mo, d, h, mi, s int, frac float64) time.Time {
		return time.Date(y, time.Month(mo), d, h, mi, s, int(frac*1e9), time.LocationUTC)
	}

	for _, c := range []struct {
		name     string
		body     eph.ID
		east     bool
		skyfield time.Time
		elongDeg float64
	}{
		{"Mercury E Feb", eph.Mercury, true, at(2026, 2, 19, 17, 41, 8, 0.58), 18.1227},
		{"Mercury W Apr", eph.Mercury, false, at(2026, 4, 3, 22, 33, 34, 0.82), 27.8195},
		{"Mercury E Jun", eph.Mercury, true, at(2026, 6, 15, 19, 59, 51, 0.16), 24.5169},
		{"Mercury W Aug", eph.Mercury, false, at(2026, 8, 2, 8, 7, 28, 0.32), 19.4680},
		{"Mercury E Oct", eph.Mercury, true, at(2026, 10, 12, 10, 3, 18, 0.38), 25.1607},
		{"Mercury W Nov", eph.Mercury, false, at(2026, 11, 20, 23, 31, 18, 0.16), 19.6217},
		{"Venus E Aug", eph.Venus, true, at(2026, 8, 15, 6, 31, 33, 0.44), 45.8923},
	} {
		events, err := plan.GreatestElongations(c.skyfield.Add(unit.Days(-10)), c.skyfield.Add(unit.Days(10)),
			plan.NewPlanet(c.name, c.body, prov), sun)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}

		if len(events) != 1 {
			t.Errorf("%s: %d greatest elongations within ten days, want 1", c.name, len(events))

			continue
		}

		e := events[0]

		wantKind := plan.EventGreatestElongationWest
		if c.east {
			wantKind = plan.EventGreatestElongationEast
		}

		if e.Kind != wantKind {
			t.Errorf("%s: %v, want %v", c.name, e.Kind, wantKind)
		}

		if off := e.Time.Sub(c.skyfield).Seconds(); math.Abs(off) > 1 {
			t.Errorf("%s: at %v, %+.2f s from Skyfield's", c.name, e.Time, off)
		}

		// Skyfield's elongation is printed to four decimals.
		if d := math.Abs(e.Value - c.elongDeg); d > 1e-4+5e-5 {
			t.Errorf("%s: elongation %.5f°, Skyfield %.4f°", c.name, e.Value, c.elongDeg)
		}
	}
}
