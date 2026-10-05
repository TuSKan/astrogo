package plan

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestAsObjectKeepsMovingBody is #483's defect stated structurally: wrapping an
// Observable as a coord.Object must not change whether it is a MovingBody,
// since that decides whether observedAltAz applies the observer's offset from
// the geocenter.
func TestAsObjectKeepsMovingBody(t *testing.T) {
	t.Parallel()

	sat, _ := testISS(t)

	for _, obj := range []Observable{
		NewMoon(eph.Default()),
		NewSatellite("ISS", 0, sat),
		NewStar("s", angle.Hour(5.6), angle.Deg(-1.2)),
	} {
		_, want := obj.(MovingBody)
		if _, got := asObject(obj).(MovingBody); got != want {
			t.Errorf("%s: MovingBody %v, wrapped %v", obj.Name(), want, got)
		}
	}
}

// TestRankObservableScoresTheMoonFromTheObserver holds RankObservable's score,
// the Moon's peak altitude, to the maximum of its topocentric altitude over a
// one-minute grid, computed from GeocentricVec directly.
//
// The two windows are the ones with the most parallax of those measured for
// #483, where the score was 0.53° and 0.78° too high. A one-minute grid sits
// at most 2e-4° below the true maximum, from the Moon's curvature in
// altitude at culmination; the bound is 1e-3°.
func TestRankObservableScoresTheMoonFromTheObserver(t *testing.T) {
	t.Parallel()

	loc, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2400)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	site, err := NewSite("s", loc)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	planner, err := NewPlanner(site, nil)
	if err != nil {
		t.Fatalf("NewPlanner: %v", err)
	}

	moon := NewMoon(eph.Default())

	for _, start := range []time.Time{
		time.Date(2026, time.March, 3, 0, 0, 0, 0, time.LocationUTC),
		time.Date(2026, time.March, 23, 0, 0, 0, 0, time.LocationUTC),
	} {
		end := start.Add(unit.Days(1))

		ranked, err := planner.RankObservable([]Observable{moon}, start, end)
		if err != nil {
			t.Fatalf("RankObservable: %v", err)
		}

		if len(ranked) != 1 {
			t.Fatalf("%v: %d ranked objects, want the Moon", start, len(ranked))
		}

		peak := math.Inf(-1)

		for tt := start; !tt.After(end); tt = tt.Add(unit.Minutes(1)) {
			vec, err := moon.GeocentricVec(tt)
			if err != nil {
				t.Fatalf("GeocentricVec: %v", err)
			}

			aa := coord.NewContext(tt, site.Location(), site.Refraction()).GeocentricToObserved(vec)
			peak = math.Max(peak, aa.Alt().Degrees())
		}

		if d := ranked[0].Score - peak; math.Abs(d) > 1e-3 {
			t.Errorf("%v: the Moon scored %.4f°, but its topocentric altitude peaks at %.4f° (%.4f° apart)",
				start, ranked[0].Score, peak, d)
		}
	}
}

// TestRankObservableDoesNotRaiseASatelliteAboveTheHorizon: in a window where
// the pass search finds the ISS never rising, RankObservable must not score
// it above the horizon. Read as a star at infinity, its geocentric direction
// scored 18°–64° in these windows (#483).
func TestRankObservableDoesNotRaiseASatelliteAboveTheHorizon(t *testing.T) {
	t.Parallel()

	prov, loc := testISS(t)

	site, err := NewSite("s", loc)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	planner, err := NewPlanner(site, nil)
	if err != nil {
		t.Fatalf("NewPlanner: %v", err)
	}

	sat := NewSatellite("ISS", 0, prov)
	day := time.Date(2026, time.April, 17, 0, 0, 0, 0, time.LocationUTC)
	checked := 0

	for h := 0; h < 24; h += 3 {
		start := day.Add(unit.Hours(float64(h)))
		end := start.Add(unit.Hours(3))

		// Widened by a pass's length each side, so a pass already in progress
		// at the window's start, or still in progress at its end, is found.
		passes, err := SatellitePasses(prov, "ISS", start.Add(unit.Minutes(-15)), end.Add(unit.Minutes(15)), loc, angle.Deg(0))
		if err != nil {
			t.Fatalf("SatellitePasses: %v", err)
		}

		rises := false

		for _, p := range passes {
			if p.Rise.Time.Before(end) && p.Set.Time.After(start) {
				rises = true
			}
		}

		if rises {
			continue
		}

		checked++

		ranked, err := planner.RankObservable([]Observable{sat}, start, end)
		if err != nil {
			t.Fatalf("RankObservable: %v", err)
		}

		for _, r := range ranked {
			if r.Score > 0 {
				t.Errorf("%v: the ISS never rises, but scored %.2f°", start, r.Score)
			}
		}
	}

	if checked < 2 {
		t.Fatalf("only %d windows without a pass; the fixture has changed", checked)
	}
}
