package plan

import (
	"errors"
	"math"
	"sync"
	"testing"

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

	tm := fixedEpoch()

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
	end := start.Add(1 * time.Hour)
	step := 10 * time.Minute

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

	start := fixedEpoch()
	step := 1 * time.Minute
	end := start.Add(5 * time.Minute) // 6 samples: 0, 1, 2, 3, 4, 5

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

func TestScorer(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := NewSite("Test", loc)
	tm := time.FromJD(2451545.0, time.UTC) // J2000 Noon (LST ~18.69h)

	t.Run("AltitudeScoring", func(t *testing.T) {
		// Target 1: Near zenith (Alt ~90)
		obj1 := NewStar("T", angle.Hour(18.69), angle.Deg(0))
		// Target 2: Lower (Alt ~45)
		obj2 := NewStar("T", angle.Hour(18.69), angle.Deg(45))

		s1, _ := Scorer{Site: site}.Score(obj1, tm)
		s2, _ := Scorer{Site: site}.Score(obj2, tm)

		if s1 <= s2 {
			t.Errorf("Expected higher altitude to have higher score: %f <= %f", s1, s2)
		}
	})

	t.Run("FailingConstraint", func(t *testing.T) {
		obj := NewStar("T", angle.Hour(18.69), angle.Deg(0))
		// Force fail with extreme altitude threshold
		c := Altitude{Threshold: angle.Deg(95)}

		s, err := Scorer{Site: site, Constraints: []Constraint{c}}.Score(obj, tm)
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

		sAlt, _ := Scorer{Site: site, Config: *altOnly}.Score(obj, tm)
		sUrg, _ := Scorer{Site: site, Config: *urgOnly}.Score(obj, tm)

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
		s, err := Scorer{Site: site}.Score(obj, tm) // Default config
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

// TestPlannerRankObservable is a regression test for Planner.RankObservable
// (the peak-altitude-within-a-window method, distinct from the
// package-level RankObservables function TestRankObservables already
// covers): before the fix it returned ErrNotCoordObject for every real
// Observable in this package -- none of Star/Planet/Asteroid/... implement
// coord.Object directly, only Observable.Position -- so this method was
// unreachable dead code with zero production callers. A plain *Star must
// now rank successfully.
func TestPlannerRankObservable(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Deg(45), 0)
	site, _ := NewSite("Test", loc)

	p, err := NewPlanner(site, nil)
	testutil.AssertNoError(t, err)

	start := time.FromJD(2451545.0, time.UTC)
	end := start.AddDays(1)

	objs := []Observable{
		NewStar("High", angle.Hour(12), angle.Deg(80)),
		NewStar("Low", angle.Hour(12), angle.Deg(-80)),
	}

	ranked, err := p.RankObservable(objs, start, end)
	if err != nil {
		if errors.Is(err, ErrNotCoordObject) {
			t.Fatal("RankObservable should never return ErrNotCoordObject for a plain Observable")
		}

		t.Fatalf("RankObservable: %v", err)
	}

	if len(ranked) != 2 {
		t.Fatalf("expected 2 ranked objects (no constraints, so both pass), got %d", len(ranked))
	}
}

var errCoordObjectICRS = errors.New("erroringCoordObject: ICRS always fails")

// erroringCoordObject implements both Observable and coord.Object directly
// — unlike every real Observable in this package (Star, Planet, Asteroid,
// Satellite, ...), none of which implement coord.Object directly, per
// RankObservable's own doc comment. This exercises the "obj already
// satisfies coord.Object, no observableObject wrap needed" branch of the
// type assertion, and its always-failing ICRS exercises TransitEstimate's
// error propagation through RankObservable's per-item closure — neither
// path is reached by TestPlannerRankObservable's plain *Star objects.
type erroringCoordObject struct{}

func (erroringCoordObject) Name() string { return "erroring" }

func (erroringCoordObject) Position(time.Time) (coord.ICRS, error) {
	return coord.ICRS{}, errCoordObjectICRS
}

func (erroringCoordObject) GetDetails(*coord.Context, DetailOverrides) (*TargetDetails, error) {
	return nil, errCoordObjectICRS
}

func (erroringCoordObject) ICRS(time.Time) (coord.ICRS, error) {
	return coord.ICRS{}, errCoordObjectICRS
}

func TestPlannerRankObservable_DirectCoordObjectAndTransitError(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Deg(45), 0)
	site, _ := NewSite("Test", loc)

	p, err := NewPlanner(site, nil)
	testutil.AssertNoError(t, err)

	start := time.FromJD(2451545.0, time.UTC)
	end := start.AddDays(1)

	_, err = p.RankObservable([]Observable{erroringCoordObject{}}, start, end)
	if err == nil {
		t.Fatal("expected an error from an object whose ICRS always fails")
	}

	if !errors.Is(err, errCoordObjectICRS) {
		t.Errorf("RankObservable error = %v, want it to wrap errCoordObjectICRS", err)
	}
}

func TestObservableWindows_StepTooLarge(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := NewSite("Test", loc)
	obj := NewStar("T", angle.Zero(), angle.Zero())

	start := fixedEpoch()
	end := start.Add(6 * time.Hour)

	// Step > 15min should return an error a caller can match via errors.Is
	// against the documented public sentinel (R21 regression: these
	// sentinels were declared and wrapped but never verified reachable).
	_, err := ObservableWindows(obj, start, end, 30*time.Minute, site, Altitude{Threshold: angle.Deg(30)})
	if !errors.Is(err, ErrStepTooLarge) {
		t.Errorf("expected ErrStepTooLarge for step > 15 minutes, got %v", err)
	}

	// Step <= 15min should succeed.
	_, err = ObservableWindows(obj, start, end, 15*time.Minute, site, Altitude{Threshold: angle.Deg(30)})
	testutil.AssertNoError(t, err)
}

func TestObservableWindows_StepNotPositive(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := NewSite("Test", loc)
	obj := NewStar("T", angle.Zero(), angle.Zero())

	start := fixedEpoch()
	end := start.Add(6 * time.Hour)

	_, err := ObservableWindows(obj, start, end, 0, site, Altitude{Threshold: angle.Deg(30)})
	if !errors.Is(err, ErrStepNotPositive) {
		t.Errorf("expected ErrStepNotPositive for a zero step, got %v", err)
	}

	_, err = ObservableWindows(obj, start, end, -time.Minute, site, Altitude{Threshold: angle.Deg(30)})
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

// TestScorerConfigIsUsed pins that Scorer.Config actually reaches the merit
// weighting, rather than the default being applied unconditionally.
//
// The existing UrgencyBoost subtest passes a custom config but only asserts
// the result is positive, which a Scorer that ignored Config entirely would
// also satisfy. This asserts the exact number instead: with AltitudeWeight
// alone, normalize() makes wAlt 1.0, so the composite is altMerit — and the
// ×90 rescaling turns that back into the altitude in degrees. A weighting
// this test can predict end to end is the only kind that proves the weights
// were read.
func TestScorerConfigIsUsed(t *testing.T) {
	t.Parallel()

	loc, err := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	testutil.AssertNoError(t, err)

	site, err := NewSite("Test", loc)
	testutil.AssertNoError(t, err)

	tm := time.FromJD(2451545.0, time.UTC) // J2000 noon, LST ~18.69h
	obj := NewStar("T", angle.Hour(18.69), angle.Deg(0))

	eval, err := IsObservable(obj, tm, site)
	testutil.AssertNoError(t, err)

	altDeg := eval.AltAz.Alt().Degrees()

	altOnly, err := Scorer{Site: site, Config: ScoreConfig{AltitudeWeight: 1}}.Score(obj, tm)
	testutil.AssertNoError(t, err)
	testutil.AssertNear(t, "altitude-only score", altOnly, altDeg, 1e-9)

	// And it must not coincide with the default weighting, or the assertion
	// above would hold for a Scorer that ignored Config.
	dflt, err := Scorer{Site: site}.Score(obj, tm)
	testutil.AssertNoError(t, err)

	if math.Abs(dflt-altOnly) < 1.0 {
		t.Errorf("default score %.6f is indistinguishable from the "+
			"altitude-only score %.6f; Config may be ignored", dflt, altOnly)
	}
}

// TestScorerContextIsReusedAndFixesTheEpoch covers both halves of the Context
// field's contract.
//
// Reused: a Context built at the scoring epoch must give the same answer as
// the one Score builds itself — a caller passing one is asking to skip the
// ~91 µs Apco13 solve, not to change the result.
//
// Fixes the epoch: a Context built at a *different* instant is used as given,
// not silently rebuilt for t. That is the trap in the field, so it is worth a
// test rather than only a doc comment: six hours of Earth rotation moves this
// equatorial target from the zenith to roughly 45°, and a Scorer that quietly
// rebuilt the context would hide the mistake instead of reporting it.
func TestScorerContextIsReusedAndFixesTheEpoch(t *testing.T) {
	t.Parallel()

	loc, err := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	testutil.AssertNoError(t, err)

	site, err := NewSite("Test", loc)
	testutil.AssertNoError(t, err)

	tm := time.FromJD(2451545.0, time.UTC)
	obj := NewStar("T", angle.Hour(18.69), angle.Deg(0))

	fresh, err := Scorer{Site: site}.Score(obj, tm)
	testutil.AssertNoError(t, err)

	atEpoch, err := Scorer{
		Site:    site,
		Context: coord.NewContext(tm, loc, site.Refraction()),
	}.Score(obj, tm)
	testutil.AssertNoError(t, err)
	testutil.AssertNear(t, "score with a Context at the scoring epoch", atEpoch, fresh, 1e-9)

	sixHoursEarlier, err := Scorer{
		Site:    site,
		Context: coord.NewContext(tm.AddDays(-0.25), loc, site.Refraction()),
	}.Score(obj, tm)
	testutil.AssertNoError(t, err)

	if math.Abs(sixHoursEarlier-fresh) < 10.0 {
		t.Errorf("score with a six-hour-stale Context (%.6f) is within 10 of "+
			"the fresh score (%.6f); the supplied Context is not being used",
			sixHoursEarlier, fresh)
	}
}

// TestScorerMeritTerms pins the three parts of the composite that a caller can
// reach only indirectly, each with a number the test can predict from the
// documented formula rather than from a previous run.
//
// All three predate the Scorer struct and none was asserted before; they are
// covered here because the rewrite moved them, and a merit term nothing checks
// is one a refactor can drop silently.
func TestScorerMeritTerms(t *testing.T) {
	t.Parallel()

	loc, err := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	testutil.AssertNoError(t, err)

	site, err := NewSite("Test", loc)
	testutil.AssertNoError(t, err)

	tm := time.FromJD(2451545.0, time.UTC)

	t.Run("priority multiplies the composite", func(t *testing.T) {
		t.Parallel()

		base := NewStar("T", angle.Hour(18.69), angle.Deg(45))

		plain, err := Scorer{Site: site}.Score(base, tm)
		testutil.AssertNoError(t, err)

		ranked, err := Scorer{Site: site}.Score(
			prioritizedTarget{Observable: base, priority: 3.0}, tm)
		testutil.AssertNoError(t, err)

		testutil.AssertNear(t, "priority-3 score", ranked, 3.0*plain, 1e-9)
	})

	t.Run("a target below the horizon scores zero, not negative", func(t *testing.T) {
		t.Parallel()

		// No constraints, so nothing rejects this target: Evaluation.Observable
		// stays true and the altitude merit is what has to stay in range. A
		// negative score would sort *below* a rejected target, inverting the
		// meaning of the zero that a failed constraint returns.
		below := NewStar("below", angle.Hour(6.69), angle.Deg(0))

		eval, err := IsObservable(below, tm, site)
		testutil.AssertNoError(t, err)

		if eval.AltAz.Alt().Degrees() >= 0 {
			t.Fatalf("fixture is above the horizon at %.3f°; it no longer "+
				"tests the clamp", eval.AltAz.Alt().Degrees())
		}

		score, err := Scorer{Site: site, Config: ScoreConfig{AltitudeWeight: 1}}.Score(below, tm)
		testutil.AssertNoError(t, err)

		if score < 0 {
			t.Errorf("score = %.6f for a target %.3f° below the horizon; the "+
				"altitude merit must clamp at zero", score, eval.AltAz.Alt().Degrees())
		}
	})

	t.Run("a zero MoonFullPenaltyDeg falls back to 30 degrees", func(t *testing.T) {
		t.Parallel()

		// ScoreConfig{MoonWeight: 1} is a natural thing to write, and without
		// the fallback its threshold would be zero: sep/0 is +Inf, min(+Inf, 1)
		// is 1, and every target would score a perfect Moon merit — the Moon
		// itself included.
		moon, err := getMoonPosition(tm)
		testutil.AssertNoError(t, err)

		// 15° away along the meridian is half the default 30° threshold, so
		// the merit is 0.5 and the ×90 rescaling makes the score exactly 45.
		near := NewStar("near the Moon", moon.RA(), moon.Dec()+angle.Deg(15))

		const wantHalfMerit = 45.0

		explicit, err := Scorer{
			Site:   site,
			Config: ScoreConfig{MoonWeight: 1, MoonFullPenaltyDeg: 30},
		}.Score(near, tm)
		testutil.AssertNoError(t, err)
		testutil.AssertNear(t, "score with an explicit 30° threshold", explicit, wantHalfMerit, 1e-6)

		fallback, err := Scorer{Site: site, Config: ScoreConfig{MoonWeight: 1}}.Score(near, tm)
		testutil.AssertNoError(t, err)
		testutil.AssertNear(t, "score with a zero threshold", fallback, wantHalfMerit, 1e-6)
	})
}

// TestScorerReportsFailureRatherThanZero is the error-vs-absence check for
// Score: a target that cannot be evaluated must not come back as a score of
// zero with a nil error.
//
// Zero is already a meaningful answer here — FailingConstraint above asserts
// it is what a rejected target gets — so a swallowed error would place a
// broken target at the bottom of a ranking instead of stopping the run, and
// a scheduler would go on to plan a night around the remaining targets as
// though nothing had gone wrong.
func TestScorerReportsFailureRatherThanZero(t *testing.T) {
	t.Parallel()

	loc, err := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	testutil.AssertNoError(t, err)

	site, err := NewSite("Test", loc)
	testutil.AssertNoError(t, err)

	score, err := Scorer{Site: site}.Score(errObservable{}, fixedEpoch())
	if err == nil {
		t.Fatalf("Score returned (%v, nil) for a target whose Position fails; "+
			"a failure must not be indistinguishable from a rejected target", score)
	}

	if !errors.Is(err, errAlwaysFails) {
		t.Errorf("error = %v, want it to wrap the target's own failure", err)
	}

	if score != 0 {
		t.Errorf("score = %v alongside an error, want 0", score)
	}
}
