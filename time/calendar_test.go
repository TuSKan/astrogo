package time_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/time"
)

// TestWeekdayOfKnownDates holds Weekday to dates whose day of the week is a
// matter of record, across the Gregorian reform and both sides of noon.
//
// Weekday is computed from the Julian Date as floor(JD + 1.5) mod 7, so the
// day changes at midnight of the time's own scale, not at the Julian Date's
// noon: the 23:59 / 00:01 pair below holds it to that.
func TestWeekdayOfKnownDates(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		at   time.Time
		want time.Weekday
	}{
		{"J2000.0, 2000-01-01 12:00", time.FromJD(2451545.0, time.UTC), time.Saturday},
		{"Apollo 11 landing, 1969-07-20", time.Date(1969, 7, 20, 20, 17, 0, 0, time.LocationUTC), time.Sunday},
		// The first day of the Gregorian calendar, and the day before it, which
		// the Julian calendar called Thursday 4 October.
		{"Gregorian reform, 1582-10-15", time.FromJD(2299160.5, time.UTC), time.Friday},
		{"1582-10-14 noon (Julian 4 October)", time.FromJD(2299160.0, time.UTC), time.Thursday},
		{"just before midnight", time.Date(2026, 10, 6, 23, 59, 0, 0, time.LocationUTC), time.Tuesday},
		{"just after midnight", time.Date(2026, 10, 7, 0, 1, 0, 0, time.LocationUTC), time.Wednesday},
		// JD 0 is noon on Monday 1 January 4713 BC, the origin the formula
		// counts from; a negative day number must still land in 0..6.
		{"JD 0", time.FromJD(0, time.UTC), time.Monday},
		{"JD -1", time.FromJD(-1, time.UTC), time.Sunday},
	} {
		if got := tc.at.Weekday(); got != tc.want {
			t.Errorf("%s: Weekday = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestJulianCalendarAcrossTheReform: the Julian calendar runs thirteen days
// behind the Gregorian today and ten at the reform, where Thursday 4 October
// (Julian) was followed by Friday 15 October (Gregorian).
func TestJulianCalendarAcrossTheReform(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		jd           float64
		y, m, d      int
		wantFraction float64
	}{
		{"Gregorian 1582-10-15 00:00", 2299160.5, 1582, 10, 5, 0},
		{"Gregorian 1582-10-14 12:00", 2299160.0, 1582, 10, 4, 0.5},
		{"Gregorian 2000-01-01 12:00", 2451545.0, 1999, 12, 19, 0.5},
	} {
		y, m, d, f := time.FromJD(tc.jd, time.TT).JulianCalendar()
		if y != tc.y || m != tc.m || d != tc.d || f != tc.wantFraction {
			t.Errorf("%s: JulianCalendar = %d-%02d-%02d +%g, want %d-%02d-%02d +%g",
				tc.name, y, m, d, f, tc.y, tc.m, tc.d, tc.wantFraction)
		}
	}
}

// TestAddDateNormalizesLikeTheStandardLibrary: a day of the month past the end
// of the target month carries into the next, as the standard library's
// AddDate does — 31 January plus one month is 2 March in a leap year — and the
// time of day survives.
func TestAddDateNormalizesLikeTheStandardLibrary(t *testing.T) {
	t.Parallel()

	base := time.Date(2024, 1, 31, 18, 30, 0, 0, time.LocationUTC)

	for _, tc := range []struct {
		name                string
		years, months, days int
		wantY, wantM, wantD int
	}{
		{"one month into a leap February", 0, 1, 0, 2024, 3, 2},
		{"a year and a month, common February", 1, 1, 0, 2025, 3, 3},
		{"back across the year", 0, -1, 0, 2023, 12, 31},
		{"thirteen months", 0, 13, 0, 2025, 3, 3},
		{"days only", 0, 0, 1, 2024, 2, 1},
		{"a year back, then days", -1, 0, 29, 2023, 3, 1},
	} {
		got := base.AddDate(tc.years, tc.months, tc.days)

		if y, m, d := got.Year(), int(got.Month()), got.Day(); y != tc.wantY || m != tc.wantM || d != tc.wantD {
			t.Errorf("%s: AddDate(%d, %d, %d) = %d-%02d-%02d, want %d-%02d-%02d",
				tc.name, tc.years, tc.months, tc.days, y, m, d, tc.wantY, tc.wantM, tc.wantD)
		}

		if h, mi := got.ToGo().Hour(), got.ToGo().Minute(); h != 18 || mi != 30 {
			t.Errorf("%s: AddDate moved the time of day to %02d:%02d", tc.name, h, mi)
		}
	}

	// 29 February plus a year has no 29th to land on.
	leap := time.Date(2024, 2, 29, 0, 0, 0, 0, time.LocationUTC).AddDate(1, 0, 0)
	if y, m, d := leap.Year(), int(leap.Month()), leap.Day(); y != 2025 || m != 3 || d != 1 {
		t.Errorf("2024-02-29 + 1 year = %d-%02d-%02d, want 2025-03-01", y, m, d)
	}
}

// TestFromGoDurationKeepsTheNanosecondFor48Days pins the limit
// FromGoDuration's doc states. unit.Duration is float64 seconds, whose spacing
// grows with the value: below 2^22 s, about 48.5 days, it is under half a
// nanosecond and every nanosecond count comes back exactly; past it the count
// rounds, by up to 4 ns at a year.
func TestFromGoDurationKeepsTheNanosecondFor48Days(t *testing.T) {
	t.Parallel()

	for _, ns := range []int64{
		1,
		1_500_000_001,                  // a second and a half, plus a nanosecond
		86_400_000_000_001,             // a day and a nanosecond
		(1<<22)*1_000_000_000 - 1,      // the last nanosecond under 2^22 s
		41*86_400_000_000_000 + 77_777, // 41 days and change
	} {
		s := float64(time.FromGoDuration(time.Duration(ns)))
		if back := int64(math.Round(s * 1e9)); back != ns {
			t.Errorf("FromGoDuration(%d ns) = %.12f s, which comes back as %d ns", ns, s, back)
		}
	}

	const yearNS = int64(365.25 * 86400 * 1e9)

	s := float64(time.FromGoDuration(time.Duration(yearNS)))
	if diff := math.Abs(s*1e9 - float64(yearNS)); diff > 4 {
		t.Errorf("FromGoDuration(a year) is %.3g ns off, want within 4", diff)
	}
}
