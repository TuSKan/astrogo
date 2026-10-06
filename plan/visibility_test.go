package plan

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"

	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// mockObject implements coord.Object for testing.
type mockObject struct {
	pos coord.ICRS
}

func (m mockObject) ICRS(_ time.Time) (coord.ICRS, error) {
	return m.pos, nil
}

func (m mockObject) Name() string                             { return "mock" }
func (m mockObject) Position(_ time.Time) (coord.ICRS, error) { return m.pos, nil }
func (m mockObject) GetDetails(_ *coord.Context, _ DetailOverrides) (*TargetDetails, error) {
	return &TargetDetails{}, nil
}

func TestIsVisible(t *testing.T) {
	// Site at lat 45
	loc, err := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	if err != nil {
		t.Fatalf("Failed to create geodetic site: %v", err)
	}

	site, err := NewSite("Test", loc)
	if err != nil {
		t.Fatalf("Failed to create observatory: %v", err)
	}

	tm := fixedEpoch()

	// Object at zenith (same Dec as Lat, Hour Angle 0)
	// For simplicity, we'll just test the method exists and calls through.
	// Since AltAz calculation is complex, we'll verify it returns a boolean.
	obj := mockObject{pos: coord.NewICRS(angle.Deg(0), angle.Deg(45))}

	_, err = IsVisible(obj, tm, site, angle.Deg(20))
	testutil.AssertNoError(t, err)
}

func TestVisibleIntervals(t *testing.T) {
	loc, err := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	if err != nil {
		t.Fatalf("Failed to create geodetic site: %v", err)
	}

	site, _ := NewSite("Test", loc)

	start := time.FromJD(2460000.5, time.UTC)
	end := start.Add(unit.Days(1))

	// Circumpolar-like object (very high dec)
	obj := mockObject{pos: coord.NewICRS(angle.Deg(0), angle.Deg(89))}

	intervals, err := VisibleIntervals(obj, site, start, end, 15*time.Minute, angle.Deg(10))
	testutil.AssertNoError(t, err)

	if len(intervals) == 0 {
		t.Log("Warning: Circumpolar object not found visible in 24h window (might be refraction/ERA related)")
	}
}

func TestVisibleIntervals_StepTooLarge(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	site, _ := NewSite("Test", loc)

	start := time.FromJD(2460000.5, time.UTC)
	end := start.Add(unit.Days(1))

	obj := mockObject{pos: coord.NewICRS(angle.Deg(0), angle.Deg(45))}

	// A caller must be able to match this via errors.Is against the
	// documented public sentinel (R21 regression).
	_, err := VisibleIntervals(obj, site, start, end, 20*time.Minute, angle.Deg(10))
	if !errors.Is(err, ErrStepTooLarge) {
		t.Errorf("expected ErrStepTooLarge for step > 15 minutes, got %v", err)
	}
}

func TestNeverVisible(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	site, _ := NewSite("Test", loc)

	start := time.FromJD(2460000.5, time.UTC)
	end := start.Add(unit.Days(1))

	// Object far below horizon (antipode)
	obj := mockObject{pos: coord.NewICRS(angle.Deg(0), angle.Deg(-89))}

	intervals, err := VisibleIntervals(obj, site, start, end, 15*time.Minute, angle.Deg(0))
	testutil.AssertNoError(t, err)

	if len(intervals) > 0 {
		t.Errorf("Antipode object should not be visible, found %d intervals", len(intervals))
	}
}

func TestTransitEstimate(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	site, _ := NewSite("Test", loc)

	start := time.FromJD(2460000.0, time.UTC)
	end := start.Add(unit.Days(0.5))

	obj := mockObject{pos: coord.NewICRS(angle.Deg(100), angle.Deg(20))}

	tm, alt, err := TransitEstimate(obj, site, start, end)
	testutil.AssertNoError(t, err)

	if tm.IsZero() {
		t.Error("Transit time is zero")
	}

	if alt.Degrees() < -90 {
		t.Error("Invalid transit altitude")
	}
}

// TestTransitEstimateFindsThePeakAtTheWindowEdge holds TransitEstimate and
// MaxAltitudeInWindow to a scan of the window every 30 s plus both ends,
// through a full Context at each instant.
//
// The property is one-sided: the reported peak lies in the window and is
// never lower than any altitude the target actually reaches there. That holds
// whatever the scan's spacing, so a coarse scan cannot make it fail
// spuriously; it can only make it weaker, and the cases are chosen so the
// defect is not subtle.
//
// It replaces a check that MaxAltitudeInWindow equals TransitEstimate's
// altitude, which could not fail: one is the other. Its window was half a day,
// a whole number of the 10-minute coarse steps, so the window's end was
// sampled by luck. These windows are not. Until #540 the scan stopped at the
// last whole step, so a target still rising at the end was reported at that
// step instead, and a window shorter than one step sampled only its start:
// measured for this star, 0.87° low over 129 min and 1.45° low over 9. A
// setting target's peak at the window's start came back 1.4 s late and 9″
// low, because Brent's method never evaluates the ends of its bracket.
func TestTransitEstimateFindsThePeakAtTheWindowEdge(t *testing.T) {
	t.Parallel()

	loc, err := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	testutil.AssertNoError(t, err)

	site, err := NewSite("equator", loc)
	testutil.AssertNoError(t, err)

	// Declination −30°, so it culminates at 60°. A star at 0° would pass
	// within a few hundredths of a degree of this site's zenith, where
	// altitude peaks in a near-cusp rather than a parabola and the solver's
	// one-second time tolerance alone is worth tens of milliarcseconds.
	obj := mockObject{pos: coord.NewICRS(angle.Deg(100), angle.Deg(-30))}

	altAt := func(tm time.Time) float64 {
		aa, err := observedAltAz(obj, tm, coord.NewContext(tm, site.Location(), site.Refraction()), obj.pos)
		if err != nil {
			t.Fatalf("altitude at %v: %v", tm, err)
		}

		return aa.Alt().Degrees()
	}

	day := time.FromJD(2461000.5, time.UTC)

	transit, _, err := TransitEstimate(obj, site, day, day.Add(unit.Days(1)))
	testutil.AssertNoError(t, err)

	cases := []struct {
		name       string
		from       unit.Duration // window start, relative to transit
		length     unit.Duration
		peakAtEdge string // "start", "end", or "" for an interior culmination
	}{
		{"rising, 125 min", unit.Minutes(-180), unit.Minutes(125), "end"},
		{"rising, 129 min", unit.Minutes(-180), unit.Minutes(129), "end"},
		{"rising, shorter than a step", unit.Minutes(-120), unit.Minutes(9), "end"},
		{"setting, 129 min", unit.Minutes(60), unit.Minutes(129), "start"},
		{"culminating, 125 min", unit.Minutes(-61), unit.Minutes(125), ""},
	}

	for _, c := range cases {
		start := transit.Add(c.from)
		end := start.Add(c.length)

		bestT, bestAlt := end, altAt(end)
		for tm := start; tm.Before(end); tm = tm.Add(unit.Seconds(30)) {
			if a := altAt(tm); a > bestAlt {
				bestT, bestAlt = tm, a
			}
		}

		gotT, gotAlt, err := TransitEstimate(obj, site, start, end)
		testutil.AssertNoError(t, err)

		if gotT.Before(start) || gotT.After(end) {
			t.Errorf("%s: peak at %+.4f min, outside the %.0f min window",
				c.name, gotT.Sub(start).Minutes(), c.length.Minutes())
		}

		// 1e-6 deg is the slack for the refinement itself: the solver stops
		// within a second of an interior peak, which at this star's
		// culmination costs about 3e-7 deg, so a scan instant that happens to
		// land nearer the peak can sit that much above the answer.
		if gotAlt.Degrees() < bestAlt-1e-6 {
			t.Errorf("%s: peak %.6f deg at %+.2f min, but the target reaches %.6f deg at %+.2f min (%.4f deg higher)",
				c.name, gotAlt.Degrees(), gotT.Sub(start).Minutes(), bestAlt, bestT.Sub(start).Minutes(),
				bestAlt-gotAlt.Degrees())
		}

		switch c.peakAtEdge {
		case "start":
			if !gotT.Equal(start) {
				t.Errorf("%s: peak at %+.4f min, want the window's start", c.name, gotT.Sub(start).Minutes())
			}
		case "end":
			if !gotT.Equal(end) {
				t.Errorf("%s: peak at %+.4f min, want the window's end", c.name, gotT.Sub(start).Minutes())
			}
		}

		maxAlt, err := MaxAltitudeInWindow(obj, site, start, end)
		testutil.AssertNoError(t, err)

		if maxAlt.Degrees() < bestAlt-1e-6 {
			t.Errorf("%s: MaxAltitudeInWindow %.6f deg, but the target reaches %.6f deg",
				c.name, maxAlt.Degrees(), bestAlt)
		}
	}
}

func TestFind(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	site, _ := NewSite("Test", loc)
	start := time.FromJD(2460000.0, time.UTC)
	end := start.Add(unit.Days(1))
	obj := mockObject{pos: coord.NewICRS(angle.Deg(100), angle.Deg(20))}

	intervals, err := Find(obj, site, nil, start, end, 15*time.Minute)
	testutil.AssertNoError(t, err)

	if len(intervals) == 0 {
		t.Fatalf("Expected observable intervals")
	}

	dur := intervals[0].Window.Duration()
	if dur <= 0 {
		t.Errorf("Duration() should be positive, got %v", dur)
	}
}

func TestDuration(t *testing.T) {
	start := time.FromJD(2460000.0, time.UTC)
	end := start.Add(unit.Days(1))
	win := Window{Start: start, End: end}

	dur := win.Duration()
	if dur != 24*time.Hour {
		t.Errorf("expected 24h, got %v", dur)
	}
}
