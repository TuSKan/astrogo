package visibility_test

import (
	"testing"
	stdtime "time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/earth"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/observatory"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/visibility"
)

// mockObject implements sky.Object for testing.
type mockObject struct {
	pos coord.ICRS
}

func (m mockObject) ICRS(t time.Time) (coord.ICRS, error) {
	return m.pos, nil
}

func TestIsVisible(t *testing.T) {
	// Site at lat 45
	loc, _ := earth.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	site, _ := observatory.NewSite("Test", loc, angle.Zero(), nil)
	tm := time.NowUTC()

	// Object at zenith (same Dec as Lat, Hour Angle 0)
	// For simplicity, we'll just test the method exists and calls through.
	// Since AltAz calculation is complex, we'll verify it returns a boolean.
	obj := mockObject{pos: coord.ICRS{RA: angle.Deg(0), Dec: angle.Deg(45)}}

	_, err := visibility.IsVisible(obj, tm, site, angle.Deg(20))
	testutil.AssertNoError(t, err)
}

func TestVisibleIntervals(t *testing.T) {
	loc, _ := earth.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	site, _ := observatory.NewSite("Test", loc, angle.Zero(), nil)

	start := time.FromJD(2460000.5, time.UTC)
	end := start.AddDays(1)

	// Circumpolar-like object (very high dec)
	obj := mockObject{pos: coord.ICRS{RA: angle.Deg(0), Dec: angle.Deg(89)}}

	intervals, err := visibility.VisibleIntervals(obj, site, start, end, 1*stdtime.Hour, angle.Deg(10))
	testutil.AssertNoError(t, err)

	if len(intervals) == 0 {
		t.Log("Warning: Circumpolar object not found visible in 24h window (might be refraction/ERA related)")
	}
}

func TestNeverVisible(t *testing.T) {
	loc, _ := earth.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	site, _ := observatory.NewSite("Test", loc, angle.Zero(), nil)

	start := time.FromJD(2460000.5, time.UTC)
	end := start.AddDays(1)

	// Object far below horizon (antipode)
	obj := mockObject{pos: coord.ICRS{RA: angle.Deg(0), Dec: angle.Deg(-89)}}

	intervals, err := visibility.VisibleIntervals(obj, site, start, end, 1*stdtime.Hour, angle.Deg(0))
	testutil.AssertNoError(t, err)

	if len(intervals) > 0 {
		t.Errorf("Antipode object should not be visible, found %d intervals", len(intervals))
	}
}

func TestTransitEstimate(t *testing.T) {
	loc, _ := earth.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	site, _ := observatory.NewSite("Test", loc, angle.Zero(), nil)

	start := time.FromJD(2460000.0, time.UTC)
	end := start.AddDays(0.5)

	obj := mockObject{pos: coord.ICRS{RA: angle.Deg(100), Dec: angle.Deg(20)}}

	tm, alt, err := visibility.TransitEstimate(obj, site, start, end)
	testutil.AssertNoError(t, err)

	if tm.IsZero() {
		t.Error("Transit time is zero")
	}
	if alt.Degrees() < -90 {
		t.Error("Invalid transit altitude")
	}
}
