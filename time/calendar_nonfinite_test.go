package time_test

import (
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/time"
)

// A time whose Julian Date is NaN or infinite has no day of the year, no Julian
// calendar date, no instant on the standard library's timeline, and nothing to
// format. Each used to get a definite answer from converting the NaN to an
// int: -1721059 for DayOfYear, -4712-01-01 from JulianCalendar, and the year
// 292277019622 from ToGo, GoTime and Format on amd64 (#986).
func TestCalendarOfANonFiniteTimeHasNoValue(t *testing.T) {
	t.Parallel()

	tokyo := time.FixedZone("JST", 9*3600)

	for _, jd := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		tm := time.FromJD(jd, time.UTC)

		if d := tm.DayOfYear(); !math.IsNaN(d) {
			t.Errorf("FromJD(%v).DayOfYear() = %v, want NaN", jd, d)
		}

		if y, m, d, f := tm.JulianCalendar(); y != 0 || m != 0 || d != 0 || f != 0 {
			t.Errorf("FromJD(%v).JulianCalendar() = %d-%d-%d %v, want all zeros as Calendar gives", jd, y, m, d, f)
		}

		if g := tm.ToGo(); !g.IsZero() {
			t.Errorf("FromJD(%v).ToGo() = %v, want the zero time.Time", jd, g)
		}

		if g := tm.GoTime(); !g.IsZero() {
			t.Errorf("FromJD(%v).GoTime() = %v, want the zero time.Time", jd, g)
		}

		for _, s := range []string{tm.Format("2006-01-02 15:04:05"), tm.In(tokyo).String()} {
			if !strings.Contains(s, "NaN") {
				t.Errorf("FromJD(%v) formats as %q, want its NaN shown, not a date", jd, s)
			}
		}
	}

	// J2000.0, JD 2451545.0, is noon on 1 January 2000 UTC: a finite time
	// keeps its values. Julian calendar: 19 December 1999.
	tm := time.FromJD(2451545.0, time.UTC)

	if d := tm.DayOfYear(); math.Abs(d-1.5) > 1e-9 {
		t.Errorf("DayOfYear() = %v, want 1.5", d)
	}

	if y, m, d, f := tm.JulianCalendar(); y != 1999 || m != 12 || d != 19 || math.Abs(f-0.5) > 1e-9 {
		t.Errorf("JulianCalendar() = %d-%d-%d %v, want 1999-12-19 0.5", y, m, d, f)
	}

	if g := tm.ToGo(); g.Year() != 2000 || g.YearDay() != 1 || g.Hour() != 12 {
		t.Errorf("ToGo() = %v, want 2000-01-01 12:00 UTC", g)
	}

	if s := tm.Format("2006-01-02 15:04"); s != "2000-01-01 12:00" {
		t.Errorf("Format = %q, want 2000-01-01 12:00", s)
	}
}
