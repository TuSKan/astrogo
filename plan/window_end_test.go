package plan

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestWindowFindersSampleTheEnd is #550. VisibleIntervals, ObservableWindows
// and Find stepped while t <= end, so end itself was evaluated only when the
// steps landed on it. Each is asked here for a window whose end falls two
// minutes after the last five-minute step:
//
//   - with the star setting in between, the window used to be reported up to
//     end, two minutes after the star had set;
//   - with the star rising in between, there used to be no window at all.
//
// The true rise and set come from VisibleIntervals over a whole day, which is
// 288 steps exactly, so its end is sampled either way.
func TestWindowFindersSampleTheEnd(t *testing.T) {
	t.Parallel()

	loc, err := coord.NewGeodetic(angle.Zero(), angle.Deg(45), 0)
	if err != nil {
		t.Fatal(err)
	}

	site, err := NewSite("45N", loc)
	if err != nil {
		t.Fatal(err)
	}

	obj := mockObject{pos: coord.NewICRS(angle.Deg(100), angle.Deg(10))}
	// Midnight, so the star rises and sets once inside the reference day.
	day := time.Date(2024, time.June, 15, 0, 0, 0, 0, time.LocationUTC)

	whole, err := VisibleIntervals(obj, site, day, day.Add(unit.Days(1)), 5*time.Minute, angle.Zero())
	if err != nil || len(whole) != 1 {
		t.Fatalf("reference day: %d windows, %v; want one", len(whole), err)
	}

	rise, set := whole[0].Window.Start, whole[0].Window.End

	finders := []struct {
		name string
		find func(start, end time.Time) ([]Window, error)
	}{
		{"VisibleIntervals", func(start, end time.Time) ([]Window, error) {
			iv, err := VisibleIntervals(obj, site, start, end, 5*time.Minute, angle.Zero())

			return windowsOf(iv), err
		}},
		{"ObservableWindows", func(start, end time.Time) ([]Window, error) {
			return ObservableWindows(obj, start, end, unit.Minutes(5), site, Altitude{Threshold: angle.Zero()})
		}},
		{"Find", func(start, end time.Time) ([]Window, error) {
			iv, err := Find(obj, site, []Constraint{Altitude{Threshold: angle.Zero()}}, start, end, 5*time.Minute)

			return windowsOf(iv), err
		}},
	}

	// One second: each boundary is refined to well under that, and the
	// error this replaces is two minutes.
	const tolSeconds = 1.0

	for _, f := range finders {
		// Setting two minutes before end, after the last step.
		ws, err := f.find(set.Add(unit.Minutes(-62)), set.Add(unit.Minutes(2)))
		if err != nil {
			t.Fatalf("%s, set case: %v", f.name, err)
		}

		if len(ws) != 1 {
			t.Errorf("%s, set case: %d windows, want 1", f.name, len(ws))
		} else if off := ws[0].End.Sub(set).Seconds(); math.Abs(off) > tolSeconds {
			t.Errorf("%s, set case: window ends %+.1f s from the set", f.name, off)
		}

		// Rising two minutes before end, after the last step.
		end := rise.Add(unit.Minutes(2))

		ws, err = f.find(rise.Add(unit.Minutes(-62)), end)
		if err != nil {
			t.Fatalf("%s, rise case: %v", f.name, err)
		}

		if len(ws) != 1 {
			t.Errorf("%s, rise case: %d windows, want 1 — the star is up for the last two minutes", f.name, len(ws))

			continue
		}

		if off := ws[0].Start.Sub(rise).Seconds(); math.Abs(off) > tolSeconds {
			t.Errorf("%s, rise case: window starts %+.1f s from the rise", f.name, off)
		}

		if !ws[0].End.Equal(end) {
			t.Errorf("%s, rise case: window ends at %v, want the end of the range", f.name, ws[0].End)
		}
	}
}
