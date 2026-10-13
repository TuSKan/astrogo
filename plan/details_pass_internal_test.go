package plan

import (
	"testing"

	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestPassAtPicksThePassAtTheInstant holds passAt to its rule on hand-made
// event lists: the pass in progress when the target is up, the next one when
// it is down, and the next culmination alone when it neither rises nor sets.
// GetDetails used to report the first rise after twelve hours before the
// Context's time, which was often a pass already over (#704).
func TestPassAtPicksThePassAtTheInstant(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.LocationUTC)
	at := func(h float64) time.Time { return t0.Add(unit.Hours(h)) }
	ev := func(k EventKind, h float64) Event { return Event{Kind: k, Time: at(h)} }

	// A day of a target that rises at 6, culminates at 12, sets at 18, and
	// does the same the next day.
	day := []Event{
		ev(EventRise, -18), ev(EventTransit, -12), ev(EventSet, -6),
		ev(EventRise, 6), ev(EventTransit, 12), ev(EventSet, 18),
		ev(EventRise, 30), ev(EventTransit, 36), ev(EventSet, 42),
	}

	for _, c := range []struct {
		name               string
		events             []Event
		now                float64
		rise, transit, set float64 // hours; -999 for none
	}{
		{"up, before the culmination", day, 9, 6, 12, 18},
		{"up, after the culmination", day, 15, 6, 12, 18},
		{"down before tonight's rise", day, 2, 6, 12, 18},
		{"down after the set: the next pass, not the one just over", day, 20, 30, 36, 42},
		{"at the rise itself", day, 6, 6, 12, 18},
		{"a set first: up longer than the search reaches back",
			[]Event{ev(EventTransit, 3), ev(EventSet, 5), ev(EventRise, 20), ev(EventTransit, 26)}, 1, -999, 3, 5},
		{"no rise or set: the next culmination alone",
			[]Event{ev(EventTransit, -10), ev(EventTransit, 14), ev(EventTransit, 38)}, 0, -999, 14, -999},
		{"a culmination below the horizon before the target starts rising",
			[]Event{ev(EventTransit, 2), ev(EventRise, 20), ev(EventTransit, 25), ev(EventSet, 30)}, 0, 20, 25, 30},
	} {
		rise, transit, set := passAt(c.events, at(c.now))

		for _, f := range []struct {
			what string
			got  *Event
			want float64
		}{{"rise", rise, c.rise}, {"transit", transit, c.transit}, {"set", set, c.set}} {
			switch {
			case f.want == -999 && f.got != nil:
				t.Errorf("%s: %s at %v, want none", c.name, f.what, f.got.Time)
			case f.want != -999 && f.got == nil:
				t.Errorf("%s: no %s, want hour %g", c.name, f.what, f.want)
			case f.want != -999 && !f.got.Time.Equal(at(f.want)):
				t.Errorf("%s: %s at %v, want hour %g", c.name, f.what, f.got.Time, f.want)
			}
		}
	}
}
