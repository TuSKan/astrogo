package plan

import (
	"errors"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/unit"
)

// TestARefinementStartsFromTheSampledBracket: an evaluator whose second
// answer at an instant differs from its first, as one served by the event
// solvers' context cache can by a few times 1e-7, near the threshold. The
// sweep saw +1e-7 at a and -1 at b, a crossing; re-evaluated, a reads -1e-7,
// and FindRoot, which evaluates both ends again, refuses the bracket.
// findRootFrom starts from what the sweep saw and finds the root (#425).
func TestARefinementStartsFromTheSampledBracket(t *testing.T) {
	a := epoch.Date(2026, 6, 11, 22, 15, 0, 0, time.UTC)
	b := a.Add(unit.Minutes(15))

	// What a re-evaluation reads: -1e-7 at a, falling to -1 at b. The sweep
	// read +1e-7 at a.
	f := func(at epoch.Time) (float64, error) {
		return -1e-7 - at.Sub(a).Seconds()/900, nil
	}

	if _, _, err := DefaultSolver().FindRoot(f, Window{Start: a, End: b}); !errors.Is(err, ErrBracketingViolated) {
		t.Fatalf("FindRoot re-evaluating the ends: %v, want ErrBracketingViolated", err)
	}

	root, _, err := DefaultSolver().findRootFrom(f, a, b, 1e-7, -1)
	if err != nil {
		t.Fatalf("findRootFrom with the sampled ends: %v", err)
	}

	if d := root.Sub(a).Seconds(); d < 0 || d > 2 {
		t.Errorf("root %v s after a, want within the first 2 s", d)
	}
}

// TestTwilightNearASampleOnTheThreshold is #425's scan. It finds the latitude
// at which the Sun is exactly 18° down at 22:15 UTC on 2026-06-11, a sample of
// TwilightEvents' 15-minute sweep from 20:00, and calls TwilightEvents for
// 201 latitudes within ±2e-6° of it. Each must find both events: astronomical
// night there runs from 22:15 to 01:44 UTC, and the Sun goes 22° down, so
// nothing about the twilight is marginal. Before the fix the sweep's sample,
// served from a context cache based an hour earlier, could land on one side
// of -18° and the refinement's re-evaluation on the other, and the whole call
// failed with ErrBracketingViolated — for 144 to 456 of 4,001 latitudes at
// each of three samples in the issue's measurement. The latitude is computed
// here rather than written down so the test does not depend on the
// ephemeris to the tenth decimal.
func TestTwilightNearASampleOnTheThreshold(t *testing.T) {
	t.Parallel()

	prov := eph.Default()
	sample := epoch.Date(2026, 6, 11, 22, 15, 0, 0, time.UTC)
	start := epoch.Date(2026, 6, 11, 20, 0, 0, 0, time.UTC)
	end := epoch.Date(2026, 6, 12, 8, 0, 0, 0, time.UTC)

	// The Sun's geometric altitude plus 18° at the sample, from a context
	// based at the sample itself.
	above := func(lat float64) float64 {
		site, err := NewSite("s", angle.LatDeg(lat), angle.LonDeg(0), unit.Meters(0))
		if err != nil {
			t.Fatal(err)
		}

		vec, err := NewSun(prov).GeocentricVec(sample)
		if err != nil {
			t.Fatal(err)
		}

		ctx, err := coord.NewContext(sample, site.Location(), atmosphere.Refraction{Pressure: 0})
		if err != nil {
			t.Fatal(err)
		}

		return ctx.GeocentricToObserved(vec).Alt().Degrees() + 18
	}

	lo, hi := 44.0, 45.5
	if (above(lo) < 0) == (above(hi) < 0) {
		t.Fatalf("the Sun is not 18° down at 22:15 between %v° and %v°N", lo, hi)
	}

	for range 60 {
		mid := (lo + hi) / 2
		if (above(mid) < 0) == (above(lo) < 0) {
			lo = mid
		} else {
			hi = mid
		}
	}

	failed := 0

	for k := -100; k <= 100; k++ {
		lat := lo + float64(k)*2e-8

		site, err := NewSite("s", angle.LatDeg(lat), angle.LonDeg(0), unit.Meters(0))
		if err != nil {
			t.Fatal(err)
		}

		nights, err := TwilightEvents(site, Window{Start: start, End: end}, prov, AstronomicalTwilight)
		if err != nil || len(nights) != 1 || nights[0].Dusk == nil || nights[0].Dawn == nil {
			if failed++; failed <= 3 {
				t.Errorf("latitude %.10f: %d nights %v, err %v, want one with a dusk and a dawn", lat, len(nights), nights, err)
			}
		}
	}

	if failed > 0 {
		t.Errorf("%d of 201 latitudes within ±2e-6° of the threshold failed", failed)
	}
}
