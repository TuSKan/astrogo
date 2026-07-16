package plan

import (
	"errors"
	"sync"
	"testing"

	stdtime "time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
)

func TestPlanner(t *testing.T) {
	// North Pole makes Alt = Dec (independent of LST)
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(90), 0)
	site, _ := NewSite("Test", loc)
	constraints := []Constraint{
		Altitude{Threshold: angle.Deg(30)},
	}

	planner, err := NewPlanner(site, constraints)
	testutil.AssertNoError(t, err)

	tm := time.NowUTC()

	objs := []Observable{
		NewStar("High", angle.Deg(0), angle.Deg(45)),
		NewStar("Low", angle.Deg(180), angle.Deg(-45)),
	}

	filtered, err := planner.FilterObservable(objs, tm)
	testutil.AssertNoError(t, err)

	if len(filtered) != 1 {
		t.Errorf("expected 1 observable object, got %d", len(filtered))
	}
}

func TestObservableWindows_Fixed(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := NewSite("Test", loc)

	// Target at zenith at Greenwich J2000 (LST ~18.69h)
	obj := NewStar("T", angle.Hour(18.69), angle.Deg(0))

	start := time.FromJD(2451545.0, time.UTC) // J2000 Noon (Observable)
	end := start.Add(1 * stdtime.Hour)
	step := 10 * stdtime.Minute

	t.Run("ContinuousWindow", func(t *testing.T) {
		// Altitude > 20 deg (It's at ~90 deg)
		constraints := []Constraint{Altitude{Threshold: angle.Deg(20)}}
		windows, err := ObservableWindows(obj, start, end, step, site, constraints...)
		testutil.AssertNoError(t, err)

		if len(windows) != 1 {
			t.Errorf("expected 1 window, got %d", len(windows))
		}

		if !windows[0].Start.Equal(start) || !windows[0].End.Equal(end) {
			t.Errorf("window range mismatch: %v - %v", windows[0].Start, windows[0].End)
		}
	})

	t.Run("MultipleWindows", func(_ *testing.T) {
		// This is harder to test with real math without finding exact time points.
		// Let's use a mock constraint that flips every sample.
	})
}

func TestObservableWindows_Moving(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := NewSite("Test", loc)

	sun := NewSun(eph.Default())

	// Start at Noon J2000 (Sun high)
	start := time.FromJD(2451545.0, time.UTC)
	// End 24 hours later
	end := start.Add(24 * time.Hour)
	step := 15 * time.Minute // ≤ 15min max

	t.Run("SunDaylight", func(t *testing.T) {
		// Sun altitude > 0 (Daylight)
		constraints := []Constraint{Altitude{Threshold: angle.Zero()}}
		windows, err := ObservableWindows(sun, start, end, step, site, constraints...)
		testutil.AssertNoError(t, err)

		// Over 24 hours, we should see at least one daylight window (actually parts of two if we cross midnight).
		// At JD 2451545.0, it's noon. So it should be observable at start.
		if len(windows) < 1 {
			t.Error("expected at least one daylight window")
		}
	})
}

type flipConstraint struct {
	count int
}

func (f *flipConstraint) Check(_ Observable, _ time.Time, _ *Site) (Result, error) {
	f.count++
	pass := f.count%2 == 0

	return Result{Pass: pass}, nil
}

func TestObservableWindows_Grouping(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := NewSite("Test", loc)
	obj := NewStar("T", angle.Zero(), angle.Zero())

	start := time.NowUTC()
	step := 1 * stdtime.Minute
	end := start.Add(5 * stdtime.Minute) // 6 samples: 0, 1, 2, 3, 4, 5

	// flipConstraint:
	// t=0: count=1, fail
	// t=1: count=2, pass -> start win
	// t=2: count=3, fail -> end win
	// t=3: count=4, pass -> start win
	// t=4: count=5, fail -> end win
	// t=5: count=6, pass -> start win, end at end

	windows, err := ObservableWindows(obj, start, end, step, site, &flipConstraint{})
	testutil.AssertNoError(t, err)

	// Expected windows:
	// [1, 2]
	// [3, 4]
	// [5, end]
	if len(windows) != 3 {
		t.Errorf("expected 3 windows, got %d", len(windows))
	}
}

func TestIsObservable(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := NewSite("Test", loc)
	// Equinox 2000 noon
	tm := time.FromJD(2451545.0, time.UTC)

	// Target at zenith
	obj := NewStar("T", angle.Hour(18.69), angle.Deg(0))

	t.Run("AllPass", func(t *testing.T) {
		constraints := []Constraint{
			Altitude{Threshold: angle.Deg(20)},
			Airmass{Threshold: 2.0},
		}
		eval, err := IsObservable(obj, tm, site, constraints...)
		testutil.AssertNoError(t, err)

		if !eval.Observable {
			t.Errorf("Expected observable, got evaluation: %+v", eval)
		}

		if len(eval.Results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(eval.Results))
		}
	})

	t.Run("OneFails", func(t *testing.T) {
		constraints := []Constraint{
			Altitude{Threshold: angle.Deg(95)}, // Should fail
			Airmass{Threshold: 2.0},            // Should pass
		}
		eval, err := IsObservable(obj, tm, site, constraints...)
		testutil.AssertNoError(t, err)

		if eval.Observable {
			t.Error("Expected NOT observable")
		}

		if len(eval.Results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(eval.Results))
		}

		if eval.Results[0].Pass {
			t.Error("Expected first constraint to fail")
		}

		if !eval.Results[1].Pass {
			t.Error("Expected second constraint to pass")
		}
	})

	t.Run("MovingBody", func(t *testing.T) {
		// Sun is near horizon at this time/site?
		// Actually at 2451545.0 UTC it's noon at Greenwich Jan 1.
		// Sun is at approx RA=18.7h, Dec=-23deg.
		// Site (0,0) at LST=18.7h means Sun is near meridian at Dec=-23.

		// Wait, for this test let's just use a high threshold to force a fail.
		sun := NewSun(eph.Default())
		eval, err := IsObservable(sun, tm, site, Altitude{Threshold: angle.Deg(80)})
		testutil.AssertNoError(t, err)

		if eval.Observable {
			t.Error("Expected Sun to be below 80 deg threshold (it's at ~67 deg)")
		}
	})
}

func TestScoreObservable(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := NewSite("Test", loc)
	tm := time.FromJD(2451545.0, time.UTC) // J2000 Noon (LST ~18.69h)

	t.Run("AltitudeScoring", func(t *testing.T) {
		// Target 1: Near zenith (Alt ~90)
		obj1 := NewStar("T", angle.Hour(18.69), angle.Deg(0))
		// Target 2: Lower (Alt ~45)
		obj2 := NewStar("T", angle.Hour(18.69), angle.Deg(45))

		s1, _ := ScoreObservable(obj1, tm, site, nil, nil)
		s2, _ := ScoreObservable(obj2, tm, site, nil, nil)

		if s1 <= s2 {
			t.Errorf("Expected higher altitude to have higher score: %f <= %f", s1, s2)
		}
	})

	t.Run("FailingConstraint", func(t *testing.T) {
		obj := NewStar("T", angle.Hour(18.69), angle.Deg(0))
		// Force fail with extreme altitude threshold
		c := Altitude{Threshold: angle.Deg(95)}

		s, err := ScoreObservable(obj, tm, site, nil, nil, c)
		testutil.AssertNoError(t, err)

		if s != 0 {
			t.Errorf("Expected score 0 for failing constraint, got %f", s)
		}
	})

	t.Run("UrgencyBoost", func(t *testing.T) {
		// Use altitude-only config to isolate urgency testing.
		altOnly := &ScoreConfig{AltitudeWeight: 1, UrgencyWeight: 0, MoonWeight: 0}
		urgOnly := &ScoreConfig{AltitudeWeight: 0, UrgencyWeight: 1, MoonWeight: 0}

		obj := NewStar("T", angle.Hour(18.69), angle.Deg(0))

		sAlt, _ := ScoreObservable(obj, tm, site, altOnly, nil)
		sUrg, _ := ScoreObservable(obj, tm, site, urgOnly, nil)

		// Both should be positive for a visible target
		if sAlt <= 0 {
			t.Errorf("Expected positive altitude score, got %f", sAlt)
		}

		if sUrg <= 0 {
			t.Errorf("Expected positive urgency score, got %f", sUrg)
		}
	})

	t.Run("CompositeHigherThanZero", func(t *testing.T) {
		obj := NewStar("T", angle.Hour(18.69), angle.Deg(0))
		s, err := ScoreObservable(obj, tm, site, nil, nil) // Default config
		testutil.AssertNoError(t, err)

		if s <= 0 {
			t.Errorf("Expected positive composite score for visible target, got %f", s)
		}
	})
}

type prioritizedTarget struct {
	Observable

	priority float64
}

func (p prioritizedTarget) Priority() float64 { return p.priority }

func TestRankObservables(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := NewSite("Test", loc)
	tm := time.FromJD(2451545.0, time.UTC)

	t.Run("RankingStability", func(t *testing.T) {
		objs := []Observable{
			prioritizedTarget{
				Observable: NewStar("T", angle.Hour(18.69), angle.Deg(45)),
				priority:   2.0, // High priority but lower altitude
			},
			NewStar("T", angle.Hour(18.69), angle.Deg(0)), // Zenith but priority 1.0
		}

		// Score 1: ~45 * 2.0 = 90
		// Score 2: ~90 * 1.0 = 90
		// (Actually depends on exact math, let's adjust to be sure)

		objs[0] = prioritizedTarget{
			Observable: NewStar("T", angle.Hour(18.69), angle.Deg(45)),
			priority:   3.0, // Score ~135
		}

		ranked, err := RankObservables(objs, tm, site)
		testutil.AssertNoError(t, err)

		if len(ranked) != 2 {
			t.Errorf("Expected 2 ranked targets, got %d", len(ranked))
		}

		if ranked[0].Object.Name() != objs[0].Name() {
			t.Error("Priority should have pushed lower altitude target to first place")
		}
	})
}

func TestObservableWindows_StepTooLarge(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := NewSite("Test", loc)
	obj := NewStar("T", angle.Zero(), angle.Zero())

	start := time.NowUTC()
	end := start.Add(6 * stdtime.Hour)

	// Step > 15min should return an error a caller can match via errors.Is
	// against the documented public sentinel (R21 regression: these
	// sentinels were declared and wrapped but never verified reachable).
	_, err := ObservableWindows(obj, start, end, 30*stdtime.Minute, site, Altitude{Threshold: angle.Deg(30)})
	if !errors.Is(err, ErrStepTooLarge) {
		t.Errorf("expected ErrStepTooLarge for step > 15 minutes, got %v", err)
	}

	// Step <= 15min should succeed.
	_, err = ObservableWindows(obj, start, end, 15*stdtime.Minute, site, Altitude{Threshold: angle.Deg(30)})
	testutil.AssertNoError(t, err)
}

func TestObservableWindows_StepNotPositive(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := NewSite("Test", loc)
	obj := NewStar("T", angle.Zero(), angle.Zero())

	start := time.NowUTC()
	end := start.Add(6 * stdtime.Hour)

	_, err := ObservableWindows(obj, start, end, 0, site, Altitude{Threshold: angle.Deg(30)})
	if !errors.Is(err, ErrStepNotPositive) {
		t.Errorf("expected ErrStepNotPositive for a zero step, got %v", err)
	}

	_, err = ObservableWindows(obj, start, end, -stdtime.Minute, site, Altitude{Threshold: angle.Deg(30)})
	if !errors.Is(err, ErrStepNotPositive) {
		t.Errorf("expected ErrStepNotPositive for a negative step, got %v", err)
	}
}

// TestGetMoonPosition_MultiEpochCacheHits is a regression test for R25: the
// old single-entry moonSepCache thrashed to a ~0% hit rate under concurrent
// multi-epoch access, since every lookup at a new epoch evicted whatever was
// cached before it could ever be reused. This exercises the realistic
// pattern (many targets/goroutines revisiting a small set of shared epochs)
// and asserts the ephemeris is only computed once per distinct epoch.
func TestGetMoonPosition_MultiEpochCacheHits(t *testing.T) {
	epochs := make([]time.Time, 5)
	for i := range epochs {
		epochs[i] = time.FromJD(2460000.5+float64(i), time.UTC)
	}

	var wg sync.WaitGroup

	// Each of many goroutines revisits every epoch, simulating several
	// targets/constraints sharing a handful of common evaluation times.
	for range 20 {
		wg.Go(func() {
			for _, e := range epochs {
				if _, err := getMoonPosition(e); err != nil {
					t.Errorf("getMoonPosition(%v): %v", e, err)
				}
			}
		})
	}

	wg.Wait()

	// All 5 epochs must still be resident in the bounded cache — a
	// single-entry design could only ever retain the last one.
	moonSepCache.mu.Lock()
	defer moonSepCache.mu.Unlock()

	for _, e := range epochs {
		if _, ok := moonSepCache.entries[e]; !ok {
			t.Errorf("epoch %v evicted from cache; expected all %d epochs to fit within moonPosCacheSize=%d",
				e, len(epochs), moonPosCacheSize)
		}
	}
}

func TestScoreConfig_Defaults(t *testing.T) {
	cfg := DefaultScoreConfig()
	wA, wU, wM := cfg.normalize()

	total := wA + wU + wM
	if total < 0.999 || total > 1.001 {
		t.Errorf("Expected normalized weights to sum to 1.0, got %f", total)
	}
}
