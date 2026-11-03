package gofaext_test

import (
	"math"
	"testing"

	"github.com/hebl/gofa"

	"github.com/TuSKan/astrogo/internal/gofaext"
)

// JdToDate gives the proleptic Gregorian date for every finite Julian Date,
// where SOFA's iauJd2cal stops at JD −68569.5 (−4900 March 1) and JD 1e9 and
// returned zeros (#961). Outside that range a date is carried into it by
// whole 400-year cycles, so these tests hold the two things that rests on:
// SOFA's own answer inside the range, and the cycle beyond it.

const cycleDays = 146097

type calendarDate struct {
	y, m, d int
	f       float64
}

func jdToDate(t *testing.T, jd1, jd2 float64) calendarDate {
	t.Helper()

	y, m, d, f, status := gofaext.JdToDate(jd1, jd2)
	if status != 0 {
		t.Fatalf("JdToDate(%v, %v): status %d, want 0", jd1, jd2, status)
	}

	return calendarDate{y, m, d, f}
}

func sameDate(a, b calendarDate) bool {
	return a.y == b.y && a.m == b.m && a.d == b.d && math.Float64bits(a.f) == math.Float64bits(b.f)
}

// TestJdToDateIsSOFAsWithinItsRange: wherever iauJd2cal gives a date,
// JdToDate gives that date, by its bits, its two ends included.
func TestJdToDateIsSOFAsWithinItsRange(t *testing.T) {
	for _, jd := range [][2]float64{
		{-68569.5, 0}, {-68569, 0.25}, {0, 0}, {2451545, 0.3}, {2460000.5, 0.999}, {1e9, 0}, {1e9 - 0.5, 0.25},
	} {
		var y, m, d int

		var f float64

		if status := gofa.Jd2cal(jd[0], jd[1], &y, &m, &d, &f); status != 0 {
			t.Fatalf("iauJd2cal(%v): status %d, inside its range", jd, status)
		}

		if got := jdToDate(t, jd[0], jd[1]); !sameDate(got, calendarDate{y, m, d, f}) {
			t.Errorf("JdToDate(%v) = %+v, SOFA gives %d-%d-%d %v", jd, got, y, m, d, f)
		}
	}
}

// TestJdToDateRepeatsEvery400Years: a date k cycles of 146097 days away,
// either side of SOFA's range and far beyond it, is the same month, day and
// fraction, 400k years away. This holds of the proleptic Gregorian calendar
// whatever the algorithm, so it is the reference outside SOFA's range.
func TestJdToDateRepeatsEvery400Years(t *testing.T) {
	for _, base := range [][2]float64{{-68569.5, 0}, {-60000, 0.25}, {2451545, 0.3}, {2415020.5, 0.75}} {
		want := jdToDate(t, base[0], base[1])

		for _, k := range []float64{1, 2, 37, 1e3, 6.8e6} {
			got := jdToDate(t, base[0]-k*cycleDays, base[1])

			if !sameDate(got, calendarDate{want.y - 400*int(k), want.m, want.d, want.f}) {
				t.Errorf("%v minus %v cycles = %+v, want %+v 400×%v years earlier", base, k, got, want, k)
			}
		}
	}

	for _, base := range [][2]float64{{1e9, 0}, {1e9 - 1000, 0.5}} {
		want := jdToDate(t, base[0], base[1])

		for _, k := range []float64{1, 3, 1e3} {
			got := jdToDate(t, base[0]+k*cycleDays, base[1])

			if !sameDate(got, calendarDate{want.y + 400*int(k), want.m, want.d, want.f}) {
				t.Errorf("%v plus %v cycles = %+v, want %+v 400×%v years later", base, k, got, want, k)
			}
		}
	}
}

// TestJdToDateIsContinuousAtSOFAsFirstDay: the day before SOFA's first is
// the calendar's. −4900 is a century year not divisible by 400, so it has no
// February 29: JD −68569.5 is March 1 at 0h, and half a day earlier is
// February 28 at noon.
func TestJdToDateIsContinuousAtSOFAsFirstDay(t *testing.T) {
	for _, tc := range []struct {
		jd   float64
		want calendarDate
	}{
		{-68569.5, calendarDate{-4900, 3, 1, 0}},
		{-68570, calendarDate{-4900, 2, 28, 0.5}},
		{-68570.5, calendarDate{-4900, 2, 28, 0}},
		{-68628.5, calendarDate{-4900, 1, 1, 0}},
		{-68629, calendarDate{-4901, 12, 31, 0.5}},
	} {
		if got := jdToDate(t, tc.jd, 0); !sameDate(got, tc.want) {
			t.Errorf("JdToDate(%v) = %+v, want %+v", tc.jd, got, tc.want)
		}
	}
}

// TestJdToDateRefusesWhatHasNoDate: a Julian Date that is not finite has no
// calendar date, and stays refused.
func TestJdToDateRefusesWhatHasNoDate(t *testing.T) {
	for _, jd := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if y, m, d, f, status := gofaext.JdToDate(jd, 0); status == 0 || y != 0 || m != 0 || d != 0 || f != 0 {
			t.Errorf("JdToDate(%v) = %d-%d-%d %v, status %d: want zeros and a refusal", jd, y, m, d, f, status)
		}
	}
}
