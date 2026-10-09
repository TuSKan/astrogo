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

// fullContexts is the Context source the sampling functions used before #480:
// a full coord.NewContext at every instant. Each is held to its own algorithm
// run on these.
func fullContexts(site *Site) func(time.Time) *coord.Context {
	return func(t time.Time) *coord.Context {
		return coord.NewContext(t, site.Location(), site.Refraction())
	}
}

// movingObject gives a Planet the coord.Object method the sampling functions
// take, while keeping its GeocentricVec, so the Moon goes through the
// topocentric path rather than being read as a direction at infinity.
type movingObject struct{ *Planet }

func (o movingObject) ICRS(t time.Time) (coord.ICRS, error) { return o.Position(t) }

// samplingCase is one target at one site over a day: obj is the target as
// the coord.Object functions take it, observable as the Observable ones do.
type samplingCase struct {
	name       string
	obj        coord.Object
	observable Observable
	site       *Site
	start      time.Time
	window     unit.Duration
}

// samplingCases are a star, the Moon and the Sun, at a mid-latitude site and a
// high-latitude one where crossings are shallow and a small altitude error
// costs the most time.
func samplingCases(t *testing.T) []samplingCase {
	t.Helper()

	prov := eph.Default()
	start := time.Date(2026, time.March, 3, 0, 0, 0, 0, time.LocationUTC)
	lats := []float64{-24.6, 60.2}
	cases := make([]samplingCase, 0, 3*len(lats))

	for _, lat := range lats {
		loc, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(lat), 2400)
		if err != nil {
			t.Fatalf("NewGeodetic: %v", err)
		}

		site, err := NewSite("s", loc)
		if err != nil {
			t.Fatalf("NewSite: %v", err)
		}

		star := NewStar("s", angle.Hour(5.6), angle.Deg(-1.2))
		moon, sun := NewMoon(prov), NewSun(prov)

		cases = append(cases,
			samplingCase{name: "star", obj: observableObject{star}, observable: star, site: site, start: start, window: unit.Days(1)},
			samplingCase{name: "moon", obj: movingObject{moon}, observable: moon, site: site, start: start, window: unit.Days(1)},
			samplingCase{name: "sun", obj: movingObject{sun}, observable: sun, site: site, start: start, window: unit.Days(1)},
		)
	}

	return cases
}

// TestVisibleIntervalsThroughTheContextCache holds VisibleIntervals, which
// samples through a Context cache since #480, to the intervals a full Context
// at every sample finds.
func TestVisibleIntervalsThroughTheContextCache(t *testing.T) {
	t.Parallel()

	for _, c := range samplingCases(t) {
		end := c.start.Add(c.window)

		got, err := VisibleIntervals(c.obj, c.site, c.start, end, 10*time.Minute, angle.Deg(15))
		if err != nil {
			t.Fatalf("%s: VisibleIntervals: %v", c.name, err)
		}

		want, err := visibleIntervals(c.obj, fullContexts(c.site), c.start, end, 10*time.Minute, angle.Deg(15))
		if err != nil {
			t.Fatalf("%s: reference: %v", c.name, err)
		}

		compareIntervals(t, c.name, windowsOf(got), windowsOf(want))
	}
}

// TestFindThroughTheContextCache is the same for Find, whose reference is also
// the old cost per sample: Check builds a Context per constraint, and
// CheckCtx on a full Context is that Check.
func TestFindThroughTheContextCache(t *testing.T) {
	t.Parallel()

	constraints := []Constraint{Altitude{Threshold: angle.Deg(20)}, Airmass{Threshold: 2.5}}

	for _, c := range samplingCases(t) {
		end := c.start.Add(c.window)

		got, err := Find(c.obj, c.site, constraints, c.start, end, 10*time.Minute)
		if err != nil {
			t.Fatalf("%s: Find: %v", c.name, err)
		}

		want, err := find(c.obj, c.site, fullContexts(c.site), constraints, c.start, end, 10*time.Minute)
		if err != nil {
			t.Fatalf("%s: reference: %v", c.name, err)
		}

		compareIntervals(t, c.name, windowsOf(got), windowsOf(want))
	}
}

// TestObservableWindowsThroughTheContextCache is the same for
// ObservableWindows.
func TestObservableWindowsThroughTheContextCache(t *testing.T) {
	t.Parallel()

	constraints := []Constraint{Altitude{Threshold: angle.Deg(20)}, Airmass{Threshold: 2.5}}

	for _, c := range samplingCases(t) {
		end := c.start.Add(c.window)

		got, err := ObservableWindows(c.observable, c.start, end, unit.Minutes(10), c.site, constraints...)
		if err != nil {
			t.Fatalf("%s: ObservableWindows: %v", c.name, err)
		}

		want, err := observableWindows(c.observable, c.start, end, unit.Minutes(10), c.site, fullContexts(c.site), constraints...)
		if err != nil {
			t.Fatalf("%s: reference: %v", c.name, err)
		}

		compareIntervals(t, c.name, got, want)
	}
}

// TestTransitEstimateThroughTheContextCache is the same for TransitEstimate,
// once per day of each window.
//
// The altitude is what a culmination is, and it is held tightly: it is read
// through a full Context at the refined instant on both sides and the maximum
// is flat, so the two agree to nanoarcseconds, and 1 mas bounds anything
// physical rather than rounding.
//
// The instant is held only to the 1 s the solver refines to, unlike a window
// boundary. A crossing is steep, so its time is well conditioned; a maximum is
// flat, so its time is not, and where Brent's search stops inside its
// tolerance is decided by the last bits. amd64 put both sides within 1 ms;
// macOS arm64, where a multiply and an add can fuse, put the Moon's 30 ms
// apart with the altitude still agreeing.
func TestTransitEstimateThroughTheContextCache(t *testing.T) {
	t.Parallel()

	for _, c := range samplingCases(t) {
		for day := c.start; day.Before(c.start.Add(c.window)); day = day.Add(unit.Days(1)) {
			gotT, gotAlt, err := TransitEstimate(c.obj, c.site, day, day.Add(unit.Days(1)))
			if err != nil {
				t.Fatalf("%s: TransitEstimate: %v", c.name, err)
			}

			wantT, wantAlt, err := transitEstimate(c.obj, c.site, fullContexts(c.site), day, day.Add(unit.Days(1)))
			if err != nil {
				t.Fatalf("%s: reference: %v", c.name, err)
			}

			if dt := math.Abs(gotT.Sub(wantT).Seconds()); dt > DefaultSolver().Tolerance.Seconds() {
				t.Errorf("%s %v: transit at %v, %.3f s from the full-Context %v", c.name, day, gotT, dt, wantT)
			}

			if d := math.Abs(gotAlt.Degrees()-wantAlt.Degrees()) * 3600e3; d > 1 {
				t.Errorf("%s %v: transit altitude %.3f mas from the full-Context one", c.name, day, d)
			}
		}
	}
}

func windowsOf(intervals []Interval) []Window {
	out := make([]Window, len(intervals))
	for i, iv := range intervals {
		out[i] = iv.Window
	}

	return out
}

// maxBoundaryShift is how far a cached-Context window boundary may sit from
// the full-Context one.
//
// Measured over samplingCases, the largest shift is 3.1 ms, on a star at the
// high-latitude site; the Moon and Sun move by microseconds, since what SetTime
// holds fixed that matters most here is the direction of annual aberration,
// which their geocentric vectors do not pass through. The bound is the 0.01 s
// of rise/set bias SetTime's own doc comment promises, three times the
// measurement. It is also a test of SetTime's rebuild hour: stretched to a
// day, the same cases shift by up to 32 ms and fail.
const maxBoundaryShift = 0.01

func compareIntervals(t *testing.T, name string, got, want []Window) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("%s: %d windows, a full Context per sample finds %d", name, len(got), len(want))
	}

	for i := range got {
		if d := math.Abs(got[i].Start.Sub(want[i].Start).Seconds()); d > maxBoundaryShift {
			t.Errorf("%s window %d: starts at %v, %.3f s from the full-Context %v", name, i, got[i].Start, d, want[i].Start)
		}

		if d := math.Abs(got[i].End.Sub(want[i].End).Seconds()); d > maxBoundaryShift {
			t.Errorf("%s window %d: ends at %v, %.3f s from the full-Context %v", name, i, got[i].End, d, want[i].End)
		}
	}
}
