package plan_test

import (
	"testing"

	stdtime "time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/body"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/constraint"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/earth"
	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/observatory"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/target"
	"github.com/TuSKan/astrogo/time"
)

func TestPlanner(t *testing.T) {
	// North Pole makes Alt = Dec (independent of LST)
	loc, _ := earth.NewGeodetic(angle.Deg(0), angle.Deg(90), 0)
	site, _ := observatory.NewSite("Test", loc, angle.Deg(0), nil)
	constraints := []constraint.Constraint{
		constraint.Altitude{Threshold: angle.Deg(30)},
	}

	planner, err := plan.NewPlanner(site, constraints)
	testutil.AssertNoError(t, err)
	tm := time.NowUTC()

	objs := []target.Observable{
		target.Fixed{Object: catalog.Target{Name: "High", Coord: coord.ICRS{RA: angle.Deg(0), Dec: angle.Deg(45)}}},
		target.Fixed{Object: catalog.Target{Name: "Low", Coord: coord.ICRS{RA: angle.Deg(180), Dec: angle.Deg(-45)}}},
	}

	filtered, err := planner.FilterObservable(objs, tm)
	testutil.AssertNoError(t, err)

	if len(filtered) != 1 {
		t.Errorf("expected 1 observable object, got %d", len(filtered))
	}
}

func TestObservableWindows_Fixed(t *testing.T) {
	loc, _ := earth.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := observatory.NewSite("Test", loc, angle.Zero(), nil)

	// Target at zenith at Greenwich J2000 (LST ~18.69h)
	obj := target.Custom{Coord: coord.ICRS{RA: angle.Hour(18.69), Dec: angle.Deg(0)}}

	start := time.FromJD(2451545.0, time.UTC) // J2000 Noon (Observable)
	end := start.Add(1 * stdtime.Hour)
	step := 10 * stdtime.Minute

	t.Run("ContinuousWindow", func(t *testing.T) {
		// Altitude > 20 deg (It's at ~90 deg)
		constraints := []constraint.Constraint{constraint.Altitude{Threshold: angle.Deg(20)}}
		windows, err := plan.ObservableWindows(obj, start, end, step, site, constraints...)
		testutil.AssertNoError(t, err)

		if len(windows) != 1 {
			t.Errorf("expected 1 window, got %d", len(windows))
		}
		if !windows[0].Start.Equal(start) || !windows[0].End.Equal(end) {
			t.Errorf("window range mismatch: %v - %v", windows[0].Start, windows[0].End)
		}
	})

	t.Run("MultipleWindows", func(t *testing.T) {
		// This is harder to test with real math without finding exact time points.
		// Let's use a mock constraint that flips every sample.
	})
}

func TestObservableWindows_Moving(t *testing.T) {
	loc, _ := earth.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := observatory.NewSite("Test", loc, angle.Zero(), nil)

	sun := target.Body{ID: body.Sun, Provider: ephemeris.Default()}

	// Start at Noon J2000 (Sun high)
	start := time.FromJD(2451545.0, time.UTC)
	// End 24 hours later
	end := start.Add(24 * time.Hour)
	step := 30 * time.Minute

	t.Run("SunDaylight", func(t *testing.T) {
		// Sun altitude > 0 (Daylight)
		constraints := []constraint.Constraint{constraint.Altitude{Threshold: angle.Zero()}}
		windows, err := plan.ObservableWindows(sun, start, end, step, site, constraints...)
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

func (f *flipConstraint) Check(_ target.Observable, _ time.Time, _ observatory.Site) (constraint.Result, error) {
	f.count++
	pass := f.count%2 == 0
	return constraint.Result{Pass: pass}, nil
}

func TestObservableWindows_Grouping(t *testing.T) {
	loc, _ := earth.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := observatory.NewSite("Test", loc, angle.Zero(), nil)
	obj := target.Custom{Coord: coord.ICRS{}}

	start := time.Now()
	step := 1 * stdtime.Minute
	end := start.Add(5 * stdtime.Minute) // 6 samples: 0, 1, 2, 3, 4, 5

	// flipConstraint:
	// t=0: count=1, fail
	// t=1: count=2, pass -> start win
	// t=2: count=3, fail -> end win
	// t=3: count=4, pass -> start win
	// t=4: count=5, fail -> end win
	// t=5: count=6, pass -> start win, end at end

	windows, err := plan.ObservableWindows(obj, start, end, step, site, &flipConstraint{})
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
	loc, _ := earth.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := observatory.NewSite("Test", loc, angle.Zero(), nil)
	// Equinox 2000 noon
	tm := time.FromJD(2451545.0, time.UTC)

	// Target at zenith
	obj := target.Custom{Coord: coord.ICRS{RA: angle.Hour(18.69), Dec: angle.Deg(0)}}

	t.Run("AllPass", func(t *testing.T) {
		constraints := []constraint.Constraint{
			constraint.Altitude{Threshold: angle.Deg(20)},
			constraint.Airmass{Threshold: 2.0},
		}
		eval, err := plan.IsObservable(obj, tm, site, constraints...)
		testutil.AssertNoError(t, err)
		if !eval.Observable {
			t.Errorf("Expected observable, got evaluation: %+v", eval)
		}
		if len(eval.Results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(eval.Results))
		}
	})

	t.Run("OneFails", func(t *testing.T) {
		constraints := []constraint.Constraint{
			constraint.Altitude{Threshold: angle.Deg(95)}, // Should fail
			constraint.Airmass{Threshold: 2.0},            // Should pass
		}
		eval, err := plan.IsObservable(obj, tm, site, constraints...)
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
		sun := target.Body{ID: body.Sun, Provider: ephemeris.Default()}
		eval, err := plan.IsObservable(sun, tm, site, constraint.Altitude{Threshold: angle.Deg(80)})
		testutil.AssertNoError(t, err)
		if eval.Observable {
			t.Error("Expected Sun to be below 80 deg threshold (it's at ~67 deg)")
		}
	})
}
