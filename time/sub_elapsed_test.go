package time_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestSubBetweenTwoUTCEpochsIsElapsedSITime is the headline defect.
//
// UTC is not a uniform scale: it absorbs a leap second twice a decade, so the
// difference of two UTC *labels* is short by every step between them. Across
// 1972-2026 that is 27 s — 207 km of ISS track at 7.66 km/s, and about 16
// arcsec of lunar motion.
//
// Sub used to unify scales only when they *differed*, so being careless gave
// the right answer and being consistent gave the wrong one:
//
//	b.Sub(a)        // 1704153600 s — two UTC labels, 27 s short
//	b.Sub(a.TAI())  // 1704153627 s — mixed, so unified via TT
//
// That is the opposite of the incentive the type system should create, and it
// is invisible: both are plausible and neither errors.
func TestSubBetweenTwoUTCEpochsIsElapsedSITime(t *testing.T) {
	a := time.Date(1972, time.January, 1, 0, 0, 0, 0, time.LocationUTC)
	b := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	// 27 leap seconds were inserted between these two dates: ΔAT went from 10
	// to 37. The label difference is a whole number of 86400-second days.
	const (
		labelSeconds   = 1704153600.0
		elapsedSeconds = labelSeconds + 27
	)

	if got := b.Sub(a).Seconds(); got != elapsedSeconds {
		t.Errorf("UTC−UTC Sub = %.3f s, want %.3f s (a difference of %.0f s).\n"+
			"  Two UTC labels differ by less than the time between them, by every "+
			"leap second in the interval.", got, elapsedSeconds, elapsedSeconds-got)
	}

	// The three ways of asking must now agree. Before the fix the first
	// disagreed with the other two.
	for _, tc := range []struct {
		name string
		got  float64
	}{
		{"both UTC", b.Sub(a).Seconds()},
		{"both TAI", b.TAI().Sub(a.TAI()).Seconds()},
		{"both TT", b.TT().Sub(a.TT()).Seconds()},
		{"mixed UTC and TAI", b.Sub(a.TAI()).Seconds()},
		{"mixed TT and UTC", b.TT().Sub(a).Seconds()},
	} {
		if tc.got != elapsedSeconds {
			t.Errorf("%s: %.3f s, want %.3f s", tc.name, tc.got, elapsedSeconds)
		}
	}
}

// TestSubAcrossASingleLeapSecondIs86401 pins the smallest visible case.
//
// One leap second was inserted at the end of 2016. A day spanning it contains
// 86401 SI seconds, however the calendar labels its ends. One second is 7.66 km
// of ISS track, so this is not a rounding concern.
func TestSubAcrossASingleLeapSecondIs86401(t *testing.T) {
	before := time.Date(2016, time.December, 31, 12, 0, 0, 0, time.LocationUTC)
	after := time.Date(2017, time.January, 1, 12, 0, 0, 0, time.LocationUTC)

	if got := after.Sub(before).Seconds(); got != 86401 {
		t.Errorf("a UTC day spanning the 2016 leap second measured %.6f s, want 86401 s", got)
	}

	// And a day that spans no leap second is exactly 86400, so the test above
	// is detecting the step rather than a constant offset.
	q1 := time.Date(2020, time.June, 1, 12, 0, 0, 0, time.LocationUTC)
	q2 := time.Date(2020, time.June, 2, 12, 0, 0, 0, time.LocationUTC)

	if got := q2.Sub(q1).Seconds(); got != 86400 {
		t.Errorf("an ordinary UTC day measured %.6f s, want 86400 s", got)
	}
}

// TestSubInAUniformScaleIsUnchanged pins what this fix must *not* touch.
//
// TAI, TT and TDB run at a constant rate, so the difference of two labels is
// already elapsed time and no conversion is needed. Routing them through TT
// anyway would be wasted work for TAI and TT, and wrong for TDB: TDB and TT
// differ by a periodic term of ±1.7 ms, so a TDB interval converted to TT and
// back would move by up to a microsecond per hour — and an ephemeris
// interpolating in TDB days wants the TDB interval it asked for.
func TestSubInAUniformScaleIsUnchanged(t *testing.T) {
	base := time.Date(2026, time.March, 15, 6, 0, 0, 0, time.LocationUTC)

	for _, tc := range []struct {
		name  string
		start time.Time
	}{
		{"TAI", base.TAI()},
		{"TT", base.TT()},
		{"TDB", base.TDB()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Half a year, so a TDB round-trip through TT would show up: the
			// periodic term is near its extremes at this separation.
			const days = 182.5

			end := tc.start.Add(unit.Days(days))

			j1, j2 := tc.start.JDParts()
			k1, k2 := end.JDParts()
			label := ((k1 - j1) + (k2 - j2)) * 86400.0

			if got := end.Sub(tc.start).Seconds(); math.Abs(got-label) > 1e-9 {
				t.Errorf("Sub = %.9f s but the label difference is %.9f s.\n"+
					"  %s is a uniform scale: the two must be identical, and no "+
					"conversion should happen at all.", got, label, tc.name)
			}
		})
	}
}

// TestSubHasNoCeiling is the third version of this test, and the first that
// asks for the right thing.
//
// The history is the argument for the type. Sub returned an int64 nanosecond
// count, which runs out just past ±292 years — well inside the range this
// library supports, which reaches year 1. It *wrapped*: year 1 to 2026 came
// back as −9223372037 s, a negative span for an interval that plainly runs
// forwards. The fix was to saturate, which this test then pinned, and a second
// method — SubDays — existed alongside solely because a saturated maximum is
// not an answer.
//
// Sub now returns a [unit.Duration], which is float64 seconds. There is no
// ceiling to saturate at, so there is one method and the span is simply
// reported.
func TestSubHasNoCeiling(t *testing.T) {
	year1 := time.Date(1, time.January, 1, 0, 0, 0, 0, time.LocationUTC)
	modern := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	// 2025 Julian years is a little over 739,000 days.
	forward := modern.Sub(year1)
	if got := forward.Days(); got < 739_000 || got > 740_000 {
		t.Errorf("a 2025-year span = %.1f days, want about 739,616", got)
	}

	if forward <= 0 {
		t.Errorf("a forward interval produced %v — the old int64 count wrapped here", forward)
	}

	// Symmetric, which the wrapping version was not.
	if backward := year1.Sub(modern); backward != -forward {
		t.Errorf("the reversed span is %v, want exactly %v", backward, -forward)
	}

	// And the standard library's type is still what cannot hold it, which is
	// the whole reason Sub does not return one.
	if _, ok := time.ToGoDuration(forward); ok {
		t.Error("ToGoDuration reported that a 2025-year span fits in an int64 " +
			"nanosecond count; it does not, and saying so is the point of the bool")
	}
}

// TestAddThenSubReturnsTheInterval pins the round trip, and the size of the
// residue it is allowed to leave.
//
// This used to assert *exact* equality, and got it, because Sub returned an
// integer nanosecond count that quantised the residue away. A scale round trip
// adds and removes an offset of about 69 s, which lands the result a part in
// 1e13 below a whole second; rounding to the nearest nanosecond hid that, and
// truncating instead — which an earlier version did — reported a ten-minute
// interval as 9m59.999999999s and biased every duration downward.
//
// A float64 second count neither hides nor biases it. The residue is what two
// Julian-date sums actually leave, a few picoseconds at these magnitudes, so
// the assertion is a bound rather than an equality.
func TestAddThenSubReturnsTheInterval(t *testing.T) {
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	for _, want := range []unit.Duration{
		unit.Minutes(10),
		unit.Hours(1),
		unit.Seconds(1),
		unit.Hours(24),
	} {
		got := start.Add(want).Sub(start)
		if off := (got - want).Abs(); off > unit.Seconds(1e-9) {
			t.Errorf("Add(%v) then Sub = %v, off by %v — more than a nanosecond",
				want, got, got-want)
		}
	}
}

// TestAddAdvancesTheLabelAndSubMeasuresTheClock pins the one asymmetry this
// change deliberately leaves in place, so that it is a decision on the record
// rather than something a later reader has to rediscover.
//
// Add is label arithmetic and Sub is physical, so across a leap second
// t.Add(24h).Sub(t) is 86401 s rather than 86400 s. Both cannot be true at
// once while UTC has no label for 23:59:60: physical addition into a leap
// second has nowhere to land, aliases to its neighbour, and the trip back
// arrives a second away — which is why Add stays reversible instead.
//
// Doing the arithmetic in a uniform scale gives the physical answer.
func TestAddAdvancesTheLabelAndSubMeasuresTheClock(t *testing.T) {
	start := time.Date(2016, time.December, 31, 12, 0, 0, 0, time.LocationUTC)

	day := unit.Hours(24)

	// The label lands on the same clock time the next day.
	if got := start.Add(day).Format(time.RFC3339); got != "2017-01-01T12:00:00Z" {
		t.Errorf("Add(24h) gave the label %s, want 2017-01-01T12:00:00Z", got)
	}

	// And 86401 SI seconds really did pass, which is what Sub reports.
	if got := start.Add(day).Sub(start).Seconds(); got != 86401 {
		t.Errorf("Add(24h) then Sub = %.6f s, want 86401 s across the leap second", got)
	}

	// Add remains exactly reversible, which physical addition could not be.
	if back := start.Add(day).Add(-day); !back.Equal(start) {
		t.Errorf("Add(24h) then Add(-24h) did not return to the start: %v vs %v",
			back.Format(time.RFC3339), start.Format(time.RFC3339))
	}

	// Advancing by physical time is available by going through a uniform scale.
	if got := start.TAI().Add(day).UTC().Format(time.RFC3339); got != "2017-01-01T11:59:59Z" {
		t.Errorf("TAI().Add(24h).UTC() gave %s, want 2017-01-01T11:59:59Z — "+
			"86400 SI seconds after the start lands one second short of the "+
			"same clock time, because a leap second intervened", got)
	}
}
