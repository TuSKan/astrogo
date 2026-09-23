package time

import (
	"math"
	"testing"
	stdtime "time"

	"github.com/TuSKan/astrogo/unit"

	// gofa directly, not through gofaext, and deliberately: these tests use it
	// as the oracle the restated routines in utcday.go must agree with, and an
	// oracle routed through the package it is checking is not independent of it.
	"github.com/hebl/gofa"
)

// leapDays are the UTC days ending in the positive leap seconds gofa's table
// records, read back out of it rather than transcribed.
func leapDays(t *testing.T) [][3]int {
	t.Helper()

	var (
		out  [][3]int
		prev float64
	)

	gofa.Dat(1972, 1, 1, 0, &prev)

	for y := 1972; y <= 2030; y++ {
		for _, m := range []int{1, 7} {
			var got float64
			if status := gofa.Dat(y, m, 1, 0, &got); status < 0 {
				continue
			}

			if math.Abs(got-prev-1) < 1e-9 {
				py, pm, pd := dayBefore(y, m, 1)
				out = append(out, [3]int{py, pm, pd})
			}

			prev = got
		}
	}

	if len(out) < 20 {
		t.Fatalf("found %d leap seconds in gofa's table; the scan is wrong", len(out))
	}

	return out
}

// sofaUTC is gofa's own UTC two-part Julian Date for a calendar instant,
// including a second of 60 on a day that allows one.
func sofaUTC(t *testing.T, y, m, d, hh, mm int, sec float64) (float64, float64) {
	t.Helper()

	var d1, d2 float64
	if s := gofa.Dtf2d("UTC", y, m, d, hh, mm, sec, &d1, &d2); s < 0 {
		t.Fatalf("gofa.Dtf2d(%d-%02d-%02d %02d:%02d:%g) status %d", y, m, d, hh, mm, sec, s)
	}

	return d1, d2
}

// TestUTCToTAIAgreesWithSOFA checks the restatement of iauUtctai against the
// real one, at every leap second in gofa's table and at the instants around
// it — which is where the two could differ, and where #144 lived.
//
// With no table registered, deltaAT answers from gofa's own, so the two
// implementations share their data and must agree to the rounding of the
// arithmetic.
func TestUTCToTAIAgreesWithSOFA(t *testing.T) {
	t.Parallel()

	for _, day := range leapDays(t) {
		for _, c := range []struct {
			hh, mm int
			sec    float64
		}{
			{0, 0, 0}, {12, 0, 0}, {23, 59, 59}, {23, 59, 59.5},
			{23, 59, 60}, {23, 59, 60.5}, {23, 59, 60.999},
		} {
			u1, u2 := sofaUTC(t, day[0], day[1], day[2], c.hh, c.mm, c.sec)

			var w1, w2 float64
			gofa.Utctai(u1, u2, &w1, &w2)

			g1, g2 := utcToTAI(u1, u2)

			if diff := ((g1 - w1) + (g2 - w2)) * daySeconds; math.Abs(diff) > 1e-9 {
				t.Errorf("%04d-%02d-%02d %02d:%02d:%g: utcToTAI differs from iauUtctai by %.3g s",
					day[0], day[1], day[2], c.hh, c.mm, c.sec, diff)
			}
		}
	}
}

// TestTAIToUTCAgreesWithSOFA is the inverse, and the one that has to land an
// instant inside a leap second on the leap second's own label rather than on
// the following midnight.
func TestTAIToUTCAgreesWithSOFA(t *testing.T) {
	t.Parallel()

	for _, day := range leapDays(t) {
		for _, sec := range []float64{58.5, 59.5, 60, 60.25, 60.5, 60.999} {
			u1, u2 := sofaUTC(t, day[0], day[1], day[2], 23, 59, sec)

			var a1, a2 float64
			gofa.Utctai(u1, u2, &a1, &a2)

			var w1, w2 float64
			gofa.Taiutc(a1, a2, &w1, &w2)

			g1, g2 := taiToUTC(a1, a2)

			if diff := ((g1 - w1) + (g2 - w2)) * daySeconds; math.Abs(diff) > 1e-9 {
				t.Errorf("%04d-%02d-%02d 23:59:%g: taiToUTC differs from iauTaiutc by %.3g s",
					day[0], day[1], day[2], sec, diff)
			}
		}
	}
}

// TestTheLeapSecondCarriesTheOldDeltaAT is the measurement #144 was filed on.
//
// IERS and gofa agree that the inserted second belongs to the day it ends, so
// 2016-12-31 23:59:60 has ΔAT 36, not the 37 that starts at midnight. Aliased
// onto midnight it got 37 — a full second of error in the one instant a time
// library is expected to be exact about.
func TestTheLeapSecondCarriesTheOldDeltaAT(t *testing.T) {
	t.Parallel()

	leap := Date(2016, 12, 31, 23, 59, 60, 500_000_000, stdtime.UTC)
	tai := leap.TAI()

	// 23:59:60.5 UTC is 86400.5 SI seconds after 0h on the 31st, which was
	// 00:00:36 TAI; so this is 2017-01-01 00:00:36.5 TAI.
	want := Date(2017, 1, 1, 0, 0, 36, 500_000_000, stdtime.UTC)
	want1, want2 := want.JDParts()

	got1, got2 := tai.JDParts()
	if diff := ((got1 - want1) + (got2 - want2)) * daySeconds; math.Abs(diff) > 1e-6 {
		t.Errorf("TAI of 23:59:60.5 is %.6f s from 2017-01-01 00:00:36.5, want 0 — "+
			"a one-second miss means the new ΔAT was applied", diff)
	}
}

// TestOrdinaryInstantsKeepTheirPhysicalTime is the regression guard for every
// timestamp that was representable before this change: none of them may move.
//
// The oracle is independent of both old and new code. A UTC label on any day
// is (seconds since midnight) after that midnight, and TAI is that plus ΔAT at
// midnight — true on an ordinary day, and on a leap-second day for every second
// before the inserted one. The labels are sampled densely across leap-second
// days because those are the days whose Julian Dates changed; their physical
// meaning must not have.
func TestOrdinaryInstantsKeepTheirPhysicalTime(t *testing.T) {
	t.Parallel()

	for _, day := range leapDays(t) {
		midnight := Date(day[0], stdtime.Month(day[1]), day[2], 0, 0, 0, 0, stdtime.UTC).TAI()
		m1, m2 := midnight.JDParts()

		for _, sec := range []int{0, 1, 3600, 43200, 86000, 86399} {
			got := Date(day[0], stdtime.Month(day[1]), day[2], 0, 0, sec, 0, stdtime.UTC).TAI()
			g1, g2 := got.JDParts()

			elapsed := ((g1 - m1) + (g2 - m2)) * daySeconds
			if math.Abs(elapsed-float64(sec)) > 1e-6 {
				t.Errorf("%04d-%02d-%02d +%d s: TAI is %.9f s after midnight, want %d",
					day[0], day[1], day[2], sec, elapsed, sec)
			}
		}
	}
}

// TestUTCRoundTripsThroughTAIAcrossEveryLeapSecond: the property a scale
// conversion must never lose, at the instants where it was previously lost —
// an instant inside a leap second used to come back one second later.
func TestUTCRoundTripsThroughTAIAcrossEveryLeapSecond(t *testing.T) {
	t.Parallel()

	for _, day := range leapDays(t) {
		for _, c := range []struct{ sec, nsec int }{
			{59, 500_000_000}, {60, 0}, {60, 500_000_000}, {60, 999_000_000},
		} {
			start := Date(day[0], stdtime.Month(day[1]), day[2], 23, 59, c.sec, c.nsec, stdtime.UTC)
			back := start.TAI().UTC()

			s1, s2 := start.JDParts()
			b1, b2 := back.JDParts()

			if diff := ((b1 - s1) + (b2 - s2)) * daySeconds; math.Abs(diff) > 1e-9 {
				t.Errorf("%04d-%02d-%02d 23:59:%02d.%09d: UTC->TAI->UTC moved it by %.3g s",
					day[0], day[1], day[2], c.sec, c.nsec, diff)
			}
		}
	}
}

// TestALeapDayIsLongerByOneSecond: 0h to 0h across a leap-second day is 86401
// SI seconds, and across the day before it is 86400.
func TestALeapDayIsLongerByOneSecond(t *testing.T) {
	t.Parallel()

	d30 := Date(2016, 12, 30, 0, 0, 0, 0, stdtime.UTC)
	d31 := Date(2016, 12, 31, 0, 0, 0, 0, stdtime.UTC)
	d01 := Date(2017, 1, 1, 0, 0, 0, 0, stdtime.UTC)

	if got := d31.Sub(d30).Seconds(); math.Abs(got-86400) > 1e-6 {
		t.Errorf("2016-12-30 lasted %.6f s, want 86400", got)
	}

	if got := d01.Sub(d31).Seconds(); math.Abs(got-86401) > 1e-6 {
		t.Errorf("2016-12-31 lasted %.6f s, want 86401", got)
	}
}

// TestAddKeepsLabelsOnALeapDay: Add is label arithmetic, and on a leap-second
// day the Julian Date's fraction is of 86401 seconds. A whole day added to noon
// must land on noon, not on 12:00:00.5, which is what adding to the raw Julian
// Date would do.
func TestAddKeepsLabelsOnALeapDay(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name      string
		from, to  Time
		wantPhysS float64
	}{
		{
			"onto the leap day",
			Date(2016, 12, 30, 12, 0, 0, 0, stdtime.UTC),
			Date(2016, 12, 31, 12, 0, 0, 0, stdtime.UTC),
			86400,
		},
		{
			"off the leap day, across the leap second",
			Date(2016, 12, 31, 12, 0, 0, 0, stdtime.UTC),
			Date(2017, 1, 1, 12, 0, 0, 0, stdtime.UTC),
			86401,
		},
	} {
		got := c.from.Add(unit.Days(1))

		if !got.Equal(c.to) {
			t1, t2 := got.JDParts()
			w1, w2 := c.to.JDParts()
			t.Errorf("%s: landed %.6f s from the next day's same label",
				c.name, ((t1-w1)+(t2-w2))*daySeconds)
		}

		// And the label day is the physical length it really was.
		if phys := got.Sub(c.from).Seconds(); math.Abs(phys-c.wantPhysS) > 1e-6 {
			t.Errorf("%s: one label day spans %.6f SI s, want %v", c.name, phys, c.wantPhysS)
		}
	}
}

// TestToGoOfALeapSecondIsTheStandardLibrarysOwnReading: a standard-library time
// cannot hold 23:59:60, and ToGo gives what time.Date itself makes of it — the
// following midnight — while every other instant on the day converts exactly.
func TestToGoOfALeapSecondIsTheStandardLibrarysOwnReading(t *testing.T) {
	t.Parallel()

	leap := Date(2016, 12, 31, 23, 59, 60, 0, stdtime.UTC).ToGo()
	if want := stdtime.Date(2016, 12, 31, 23, 59, 60, 0, stdtime.UTC); !leap.Equal(want) {
		t.Errorf("ToGo(23:59:60) = %v, want the standard library's %v", leap, want)
	}

	noon := Date(2016, 12, 31, 12, 0, 0, 0, stdtime.UTC).ToGo()
	if want := stdtime.Date(2016, 12, 31, 12, 0, 0, 0, stdtime.UTC); !noon.Equal(want) {
		t.Errorf("ToGo(noon on the leap day) = %v, want %v — an ordinary instant moved", noon, want)
	}
}

// constantDUT1 is an EOP model whose UT1−UTC steps by exactly one second at
// the 2017 leap second, as the real series does, and is otherwise constant — so
// that UT1 continuity is a property of the conversion and not of how any
// particular EOP series is interpolated.
type constantDUT1 struct{}

func (constantDUT1) EOP(mjd float64) (EOP, error) {
	if mjd < 57754 { // 2017-01-01
		return EOP{DUT1: -0.4}, nil
	}

	return EOP{DUT1: 0.6}, nil
}

// TestUT1IsContinuousAcrossTheLeapSecond: UT1 is Earth's rotation angle and has
// no leap seconds, so three UTC instants one SI second apart straddling the
// inserted one are three UT1 instants one second apart. UT1 − UTC jumps by a
// whole second at 23:59:60; the conversion has to absorb that exactly, which it
// does by going through TAI with ΔAT at 0h — iauUtcut1.
//
// Not parallel: it installs a process-wide EOP model.
func TestUT1IsContinuousAcrossTheLeapSecond(t *testing.T) {
	RegisterModel(constantDUT1{})
	t.Cleanup(ResetEOP)

	var prev Time

	for i, sec := range []int{59, 60, 0} {
		day, s := 31, sec
		if i == 2 {
			day = 0
		}

		var utc Time
		if day == 0 {
			utc = Date(2017, 1, 1, 0, 0, s, 500_000_000, stdtime.UTC)
		} else {
			utc = Date(2016, 12, day, 23, 59, s, 500_000_000, stdtime.UTC)
		}

		ut1, err := utc.UT1()
		if err != nil {
			t.Fatalf("UT1: %v", err)
		}

		back := ut1.UTC()
		if d := back.Sub(utc).Seconds(); math.Abs(d) > 1e-9 {
			t.Errorf("sample %d: UTC->UT1->UTC moved it by %.3g s", i, d)
		}

		if i > 0 {
			p1, p2 := prev.JDParts()
			c1, c2 := ut1.JDParts()

			if step := ((c1 - p1) + (c2 - p2)) * daySeconds; math.Abs(step-1) > 1e-6 {
				t.Errorf("UT1 advanced %.6f s between samples %d and %d, want 1", step, i-1, i)
			}
		}

		prev = ut1
	}
}

// TestStepIndexAgreesWithTheDirectPredicates cross-checks two independent
// implementations: the index utcday.go consults on every conversion, and the
// predicates Date and the warnings use, which ask deltaAT directly. Over every
// day from 1972 through 2030 they must name the same leap seconds, with the
// same sign.
func TestStepIndexAgreesWithTheDirectPredicates(t *testing.T) {
	t.Parallel()

	steps := currentUTCSteps()
	first := mjdOf(1972, 1, 1)
	last := mjdOf(2030, 12, 31)

	var positive int

	for mjd := first; mjd <= last; mjd++ {
		var (
			y, m, d int
			fd      float64
		)

		gofa.Jd2cal(mjdZero, float64(mjd), &y, &m, &d, &fd)

		want := 0.0

		switch {
		case leapSecondEndsDay(y, m, d):
			want = 1
			positive++
		case negativeLeapSecondEndsDay(y, m, d):
			want = -1
		}

		if got := steps.leapAtEnd(mjd); got != want {
			t.Errorf("%04d-%02d-%02d: index says %v, direct predicate says %v", y, m, d, got, want)
		}
	}

	if positive < 20 {
		t.Errorf("only %d leap seconds found between 1972 and 2030; the scan is wrong", positive)
	}
}

// TestANegativeLeapSecondDayIsShorter covers the step that has never happened,
// and through it the step index built for a registered table rather than for
// gofa's own — the path a real announcement would take.
//
// With a hypothetical negative leap second at the end of 2029, that day lasts
// 86399 SI seconds: 23:59:58 is followed directly by the next midnight, so
// 23:59:58.5 is half a second from it. Not parallel: it registers a table
// process-wide.
func TestANegativeLeapSecondDayIsShorter(t *testing.T) {
	withNegativeLeapSecondAt2030(t)

	d31 := Date(2029, 12, 31, 0, 0, 0, 0, stdtime.UTC)
	d01 := Date(2030, 1, 1, 0, 0, 0, 0, stdtime.UTC)

	if got := d01.Sub(d31).Seconds(); math.Abs(got-86399) > 1e-6 {
		t.Errorf("2029-12-31 lasted %.6f s, want 86399", got)
	}

	last := Date(2029, 12, 31, 23, 59, 58, 500_000_000, stdtime.UTC)

	if got := d01.Sub(last).Seconds(); math.Abs(got-0.5) > 1e-6 {
		t.Errorf("23:59:58.5 is %.6f s before midnight, want 0.5 — there is no 23:59:59", got)
	}

	back := last.TAI().UTC()

	l1, l2 := last.JDParts()
	b1, b2 := back.JDParts()

	if diff := ((b1 - l1) + (b2 - l2)) * daySeconds; math.Abs(diff) > 1e-9 {
		t.Errorf("UTC->TAI->UTC beside the removed second moved it by %.3g s", diff)
	}
}

// TestUT1UsingIsUT1WithTheLookupDone pins the relationship the doc comment
// states: UT1 is UT1Using with DUT1 looked up, so a caller holding the same
// DUT1 gets the same instant.
//
// Not parallel: it installs a process-wide EOP model.
func TestUT1UsingIsUT1WithTheLookupDone(t *testing.T) {
	RegisterModel(constantDUT1{})
	t.Cleanup(ResetEOP)

	for _, utc := range []Time{
		Date(2016, 6, 15, 8, 30, 0, 0, stdtime.UTC),              // an ordinary day
		Date(2016, 12, 31, 12, 0, 0, 0, stdtime.UTC),             // the leap day, mid-afternoon
		Date(2016, 12, 31, 23, 59, 60, 250_000_000, stdtime.UTC), // inside the leap second
		Date(2017, 1, 1, 0, 0, 0, 750_000_000, stdtime.UTC),      // just after it
	} {
		want, err := utc.UT1()
		if err != nil {
			t.Fatalf("UT1: %v", err)
		}

		eop, err := constantDUT1{}.EOP(utc.MJD())
		if err != nil {
			t.Fatalf("EOP: %v", err)
		}

		got := utc.UT1Using(eop.DUT1)

		w1, w2 := want.JDParts()
		g1, g2 := got.JDParts()

		if diff := ((g1 - w1) + (g2 - w2)) * daySeconds; diff != 0 {
			t.Errorf("JD %.9f: UT1Using differs from UT1 by %.3g s", utc.JD(), diff)
		}

		if got.Scale() != UT1 {
			t.Errorf("UT1Using returned scale %v", got.Scale())
		}
	}
}

// TestUT1UsingAddsDUT1ToTheLabel: on an ordinary day UT1 − UTC is DUT1, read
// as labels. And on the day of a leap second the same holds for every second
// but the inserted one — which is the case adding DUT1 to the Julian Date
// directly got wrong, by the day's fraction of a second.
func TestUT1UsingAddsDUT1ToTheLabel(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name string
		utc  Time
	}{
		{"an ordinary day", Date(2016, 6, 15, 8, 30, 0, 0, stdtime.UTC)},
		{"noon on a leap-second day", Date(2016, 12, 31, 12, 0, 0, 0, stdtime.UTC)},
		{"late on a leap-second day", Date(2016, 12, 31, 23, 0, 0, 0, stdtime.UTC)},
	} {
		const dut1 = -0.4

		ut1 := c.utc.UT1Using(dut1)

		// The UTC label as the standard library counts it, uniform, against
		// UT1, which has no leap seconds and so is uniform too.
		g := c.utc.ToGo()
		labelDays := float64(g.Unix())/daySeconds + float64(g.Nanosecond())/1e9/daySeconds

		u1, u2 := ut1.JDParts()
		ut1Days := (u1 - 2440587.5) + u2

		if got := (ut1Days - labelDays) * daySeconds; math.Abs(got-dut1) > 1e-6 {
			t.Errorf("%s: UT1 − UTC = %.6f s, want %v", c.name, got, dut1)
		}
	}
}
