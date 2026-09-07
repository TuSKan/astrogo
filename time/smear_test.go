package time

import (
	"math"
	"testing"
	stdtime "time"

	"github.com/TuSKan/astrogo/internal/gofaext"
)

// utcAt is a UTC instant, built without going through the leap-second warnings
// in Date's boundary branches.
func utcAt(y int, m stdtime.Month, d, hh, mm, ss int) Time {
	return Date(y, m, d, hh, mm, ss, 0, stdtime.UTC)
}

// TestLeapSmearWindowAroundARealLeapSecond walks the real 2016-12-31 step.
//
// The window is one day either side, which is the union of the four published
// smearing methods rather than any one of them, so the boundaries are what this
// pins: 24 h out is inside, a minute past that is not.
func TestLeapSmearWindowAroundARealLeapSecond(t *testing.T) {
	tests := []struct {
		name string
		when Time
		want bool
	}{
		{"the instant of the step", utcAt(2017, stdtime.January, 1, 0, 0, 0), true},
		{"an hour before", utcAt(2016, stdtime.December, 31, 23, 0, 0), true},
		{"an hour after", utcAt(2017, stdtime.January, 1, 1, 0, 0), true},
		{"24 h before, still inside", utcAt(2016, stdtime.December, 31, 0, 0, 0), true},
		{"24 h after, still inside", utcAt(2017, stdtime.January, 2, 0, 0, 0), true},
		{"a minute too early", utcAt(2016, stdtime.December, 30, 23, 59, 0), false},
		{"a minute too late", utcAt(2017, stdtime.January, 2, 0, 1, 0), false},
		{"a week before", utcAt(2016, stdtime.December, 24, 12, 0, 0), false},
		{"an ordinary summer day", utcAt(2016, stdtime.July, 15, 12, 0, 0), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.when.LeapSmearWindow()
			if ok != tt.want {
				t.Fatalf("LeapSmearWindow() = %v, want %v", ok, tt.want)
			}

			if !ok {
				return
			}

			if got.Year != 2017 || got.Month != 1 || got.Day != 1 {
				t.Errorf("named %d-%02d-%02d, want the 2017-01-01 step", got.Year, got.Month, got.Day)
			}

			if got.DeltaAT != 37 {
				t.Errorf("DeltaAT = %v, want 37", got.DeltaAT)
			}
		})
	}
}

// TestLeapSmearWindowIsQuietForRecentEpochs: the most recent leap second was
// 2016-12-31 and none is scheduled, so every instant a caller is likely to hold
// is outside a window. A helper that answered true routinely would be worth
// nothing.
func TestLeapSmearWindowIsQuietForRecentEpochs(t *testing.T) {
	for _, when := range []Time{
		utcAt(2026, stdtime.September, 6, 22, 0, 0),
		utcAt(2020, stdtime.January, 1, 0, 0, 0),
		utcAt(2024, stdtime.July, 1, 0, 0, 0),
		utcAt(2021, stdtime.December, 31, 23, 59, 30),
	} {
		if got, ok := when.LeapSmearWindow(); ok {
			t.Errorf("%v was reported inside a smear window for %+v", when, got)
		}
	}
}

// TestLeapSmearWindowIgnoresPre1972Drift: before 1972 UTC tracked UT2 by rate
// offsets and fractional steps, so ΔAT changes across every month boundary by
// microseconds. Reading that as a leap second would put most of the 1960s
// inside a smear window — and leap smearing postdates leap seconds, which
// postdate the drift.
func TestLeapSmearWindowIgnoresPre1972Drift(t *testing.T) {
	for _, when := range []Time{
		utcAt(1965, stdtime.January, 1, 0, 0, 0),
		utcAt(1968, stdtime.February, 1, 0, 0, 0),
		utcAt(1971, stdtime.December, 31, 23, 0, 0),
	} {
		if got, ok := when.LeapSmearWindow(); ok {
			t.Errorf("%v was reported inside a smear window for %+v", when, got)
		}
	}
}

// TestLeapSmearWindowAcceptsAnyScale: the window is a property of the instant,
// not of how it is labelled. A caller holding TAI or TT must get the same
// answer as one holding the same instant in UTC, or the helper would report on
// whichever scale the caller happened to convert to.
func TestLeapSmearWindowAcceptsAnyScale(t *testing.T) {
	utc := utcAt(2016, stdtime.December, 31, 23, 0, 0)

	for _, when := range []Time{utc, utc.TAI(), utc.TT(), utc.TDB()} {
		got, ok := when.LeapSmearWindow()
		if !ok {
			t.Errorf("scale %v: not reported inside the 2016-12-31 window", when.Scale())
			continue
		}

		if got.Year != 2017 {
			t.Errorf("scale %v: named the %d step, want 2017", when.Scale(), got.Year)
		}
	}

	// The discriminating case, and the reason the conversion is not decorative:
	// TT runs 69.184 s ahead of UTC, so an instant a minute outside the early
	// edge lands 9 s inside it if the JD is compared without converting first.
	// Nothing above catches that — an hour from the boundary, a 69-second shift
	// changes no answer.
	justOutside := utcAt(2016, stdtime.December, 30, 23, 59, 0)

	for _, when := range []Time{justOutside, justOutside.TAI(), justOutside.TT(), justOutside.TDB()} {
		if got, ok := when.LeapSmearWindow(); ok {
			t.Errorf("scale %v: an instant a minute outside the window was reported inside it, for %+v",
				when.Scale(), got)
		}
	}
}

// withLeapSecondAt registers gofa's own record extended by one hypothetical
// step, and restores the built-in table afterwards.
//
// The base record is read back out of gofa rather than transcribed, because
// RegisterLeapSeconds refuses a table that contradicts the built-in one: a
// hand-copied fixture would need updating by hand on a gofa upgrade, and the
// failure would read as a registry bug rather than a stale test.
func withLeapSecondAt(t *testing.T, year, month int, deltaAT float64) {
	t.Helper()

	const (
		firstYear = 1972
		lastYear  = 2100
	)

	var (
		record   []LeapSecond
		previous float64
	)

	for y := firstYear; y <= lastYear; y++ {
		for _, m := range [2]int{1, 7} {
			got, status := gofaext.Dat(y, m, 1, 0)
			if status < 0 {
				continue
			}

			if len(record) == 0 || math.Abs(got-previous) > 1e-9 {
				record = append(record, LeapSecond{Year: y, Month: m, Day: 1, DeltaAT: got})
			}

			previous = got
		}
	}

	if len(record) == 0 {
		t.Fatal("gofa reported no ΔAT steps; the scan is wrong")
	}

	err := RegisterLeapSeconds(append(record,
		LeapSecond{Year: year, Month: month, Day: 1, DeltaAT: deltaAT},
	), "test: hypothetical leap second")
	if err != nil {
		t.Fatalf("RegisterLeapSeconds: %v", err)
	}

	t.Cleanup(ResetLeapSeconds)
}

// TestLeapSmearWindowSeesARegisteredStep: the record consulted is the one in
// force, so a caller who registered an announced leap second is answered from
// it before gofa's table carries it. That is the whole reason the registry
// exists, and it is how the next leap second will arrive.
func TestLeapSmearWindowSeesARegisteredStep(t *testing.T) {
	withLeapSecondAt(t, 2030, 1, 38)

	got, ok := utcAt(2029, stdtime.December, 31, 20, 0, 0).LeapSmearWindow()
	if !ok {
		t.Fatal("a registered leap second did not open a smear window")
	}

	if got.Year != 2030 || got.Month != 1 {
		t.Errorf("named %d-%02d, want the 2030-01 step", got.Year, got.Month)
	}

	if got.DeltaAT != 38 {
		t.Errorf("DeltaAT = %v, want 38", got.DeltaAT)
	}

	if _, ok := utcAt(2029, stdtime.December, 20, 12, 0, 0).LeapSmearWindow(); ok {
		t.Error("an instant eleven days before the step was reported inside its window")
	}
}

// TestLeapSmearWindowSeesARemovedSecond: a negative step opens a window too.
// None has ever been announced, and the smearing literature is explicit that
// the published methods say nothing about how one would be handled — so a
// caller has more reason to ask, not less.
func TestLeapSmearWindowSeesARemovedSecond(t *testing.T) {
	withLeapSecondAt(t, 2030, 1, 36)

	got, ok := utcAt(2029, stdtime.December, 31, 20, 0, 0).LeapSmearWindow()
	if !ok {
		t.Fatal("a registered negative leap second did not open a smear window")
	}

	if got.DeltaAT != 36 {
		t.Errorf("DeltaAT = %v, want 36 — a negative step lowers it", got.DeltaAT)
	}
}

// TestPrevDayCarries mirrors TestNextDayCarries: leap seconds land on the first
// of a month, so every real call crosses a month boundary and most cross a year
// boundary too.
func TestPrevDayCarries(t *testing.T) {
	tests := []struct {
		y, m, d    int
		wy, wm, wd int
	}{
		{2017, 1, 1, 2016, 12, 31},
		{2015, 7, 1, 2015, 6, 30},
		{2016, 3, 1, 2016, 2, 29}, // leap year
		{1900, 3, 1, 1900, 2, 28}, // not a leap year in the Gregorian calendar
		{2000, 3, 1, 2000, 2, 29}, // but 2000 is
	}

	for _, tt := range tests {
		y, m, d, fd := prevDay(tt.y, tt.m, tt.d)
		if y != tt.wy || m != tt.wm || d != tt.wd || fd != 0.0 {
			t.Errorf("prevDay(%d-%02d-%02d) = %d-%02d-%02d+%v, want %d-%02d-%02d+0",
				tt.y, tt.m, tt.d, y, m, d, fd, tt.wy, tt.wm, tt.wd)
		}
	}
}

// TestNextMonthCarries: the year boundary is the case that matters, since
// December is where leap seconds most often land.
func TestNextMonthCarries(t *testing.T) {
	for _, tt := range []struct{ y, m, wy, wm int }{
		{2016, 12, 2017, 1},
		{2015, 6, 2015, 7},
		{2020, 1, 2020, 2},
	} {
		got := nextMonth(tt.y, tt.m)
		if got.y != tt.wy || got.m != tt.wm {
			t.Errorf("nextMonth(%d-%02d) = %d-%02d, want %d-%02d",
				tt.y, tt.m, got.y, got.m, tt.wy, tt.wm)
		}
	}
}
