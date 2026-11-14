package plan_test

import (
	"testing"

	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// separationAt is the geocentric angular separation of a and b at t, in
// degrees: the quantity GreatestElongations maximizes and Appulses minimizes.
func separationAt(t *testing.T, a, b plan.Observable, at time.Time) float64 {
	t.Helper()

	pa, err := a.Position(at)
	if err != nil {
		t.Fatalf("%s at %v: %v", a.Name(), at, err)
	}

	pb, err := b.Position(at)
	if err != nil {
		t.Fatalf("%s at %v: %v", b.Name(), at, err)
	}

	return coord.Separation(pa, pb).Degrees()
}

// windowEndStarts are the window starts that put a sweep's last regular
// sample nanoseconds before the end of a 365-day window, so that the end is
// sampled a second time and the two samples match by rounding (#1140).
func windowEndStarts() []time.Time {
	return []time.Time{
		time.Date(2026, 11, 13, 11, 18, 0, 0, time.LocationUTC),
		time.Date(2026, 11, 13, 11, 18, 28, 0, time.LocationUTC),
		time.Date(2026, 11, 13, 11, 18, 48, 0, time.LocationUTC),
	}
}

// TestGreatestElongationsIgnoreTheWindowEnd: from these starts, Venus's
// elongation is still growing when the window closes, and the end of the
// window was reported as a greatest elongation east, 24.26°, two seconds
// before it (#1140). The real one comes on 2028-03-22.
//
// Every event must be a maximum, larger than the elongation a day either side
// of it, and the western elongation of 2027-01-03 must still be found.
func TestGreatestElongationsIgnoreTheWindowEnd(t *testing.T) {
	venus, sun := plan.NewVenus(eph.Default()), plan.NewSun(eph.Default())

	for _, start := range windowEndStarts() {
		end := start.Add(unit.Days(365))

		events, err := plan.GreatestElongations(start, end, venus, sun)
		if err != nil {
			t.Fatalf("start %v: %v", start, err)
		}

		var west int

		for _, e := range events {
			for _, d := range []float64{-1, 1} {
				if near := separationAt(t, venus, sun, e.Time.Add(unit.Days(d))); near > e.Value {
					t.Errorf("start %v: %v at %v, %.4f°, is no maximum: %.4f° a day %s",
						start, e.Kind, e.Time, e.Value, near, map[float64]string{-1: "before", 1: "after"}[d])
				}
			}

			if e.Kind == plan.EventGreatestElongationWest {
				west++
			}
		}

		if west != 1 {
			t.Errorf("start %v: %d western elongations, want the one of 2027-01-03 (events %v)", start, west, events)
		}
	}
}

// TestAppulsesIgnoreTheWindowEnd: the same window end was reported as an
// appulse wherever a separation was still falling there, for eight of the
// fifteen pairs of Venus, Mars, Jupiter, Saturn, Mercury and the Moon. One,
// Saturn and Mercury, was 167° wide (#1140). Every appulse must be a minimum,
// smaller than the separation a day either side of it.
func TestAppulsesIgnoreTheWindowEnd(t *testing.T) {
	p := eph.Default()

	for _, pair := range [][2]plan.Observable{
		{plan.NewVenus(p), plan.NewMars(p)},
		{plan.NewSaturn(p), plan.NewMercury(p)},
		{plan.NewJupiter(p), plan.NewMoon(p)},
	} {
		a, b := pair[0], pair[1]
		start := windowEndStarts()[0]

		events, err := plan.Appulses(start, start.Add(unit.Days(365)), a, b)
		if err != nil {
			t.Fatalf("%s-%s: %v", a.Name(), b.Name(), err)
		}

		for _, e := range events {
			for _, d := range []float64{-1, 1} {
				if near := separationAt(t, a, b, e.Time.Add(unit.Days(d))); near < e.Value {
					t.Errorf("%s-%s: appulse at %v, %.4f°, is no minimum: %.4f° a day %s",
						a.Name(), b.Name(), e.Time, e.Value, near, map[float64]string{-1: "before", 1: "after"}[d])
				}
			}
		}
	}
}
