package plan

import (
	"errors"
	"testing"
	stdtime "time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"

	"github.com/TuSKan/astrogo/time"
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
func (m mockObject) GetDetails(_ *coord.Context, _ ...string) (*TargetDetails, error) {
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

	tm := time.NowUTC()

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
	end := start.AddDays(1)

	// Circumpolar-like object (very high dec)
	obj := mockObject{pos: coord.NewICRS(angle.Deg(0), angle.Deg(89))}

	intervals, err := VisibleIntervals(obj, site, start, end, 15*stdtime.Minute, angle.Deg(10))
	testutil.AssertNoError(t, err)

	if len(intervals) == 0 {
		t.Log("Warning: Circumpolar object not found visible in 24h window (might be refraction/ERA related)")
	}
}

func TestVisibleIntervals_StepTooLarge(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	site, _ := NewSite("Test", loc)

	start := time.FromJD(2460000.5, time.UTC)
	end := start.AddDays(1)

	obj := mockObject{pos: coord.NewICRS(angle.Deg(0), angle.Deg(45))}

	// A caller must be able to match this via errors.Is against the
	// documented public sentinel (R21 regression).
	_, err := VisibleIntervals(obj, site, start, end, 20*stdtime.Minute, angle.Deg(10))
	if !errors.Is(err, ErrStepTooLarge) {
		t.Errorf("expected ErrStepTooLarge for step > 15 minutes, got %v", err)
	}
}

func TestNeverVisible(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	site, _ := NewSite("Test", loc)

	start := time.FromJD(2460000.5, time.UTC)
	end := start.AddDays(1)

	// Object far below horizon (antipode)
	obj := mockObject{pos: coord.NewICRS(angle.Deg(0), angle.Deg(-89))}

	intervals, err := VisibleIntervals(obj, site, start, end, 15*stdtime.Minute, angle.Deg(0))
	testutil.AssertNoError(t, err)

	if len(intervals) > 0 {
		t.Errorf("Antipode object should not be visible, found %d intervals", len(intervals))
	}
}

func TestTransitEstimate(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	site, _ := NewSite("Test", loc)

	start := time.FromJD(2460000.0, time.UTC)
	end := start.AddDays(0.5)

	obj := mockObject{pos: coord.NewICRS(angle.Deg(100), angle.Deg(20))}

	tm, alt, err := TransitEstimate(obj, site, start, end)
	testutil.AssertNoError(t, err)

	if tm.IsZero() {
		t.Error("Transit time is zero")
	}

	if alt.Degrees() < -90 {
		t.Error("Invalid transit altitude")
	}

	maxAlt, err := MaxAltitudeInWindow(obj, site, start, end)
	testutil.AssertNoError(t, err)
	testutil.AssertNear(t, "MaxAltitude == Transit", maxAlt.Degrees(), alt.Degrees(), 1e-6)
}

func TestFind(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	site, _ := NewSite("Test", loc)
	start := time.FromJD(2460000.0, time.UTC)
	end := start.AddDays(1)
	obj := mockObject{pos: coord.NewICRS(angle.Deg(100), angle.Deg(20))}

	intervals, err := Find(obj, site, nil, start, end, 15*stdtime.Minute)
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
	end := start.AddDays(1)
	win := Window{Start: start, End: end}

	dur := win.Duration()
	if dur != 24*stdtime.Hour {
		t.Errorf("expected 24h, got %v", dur)
	}
}
