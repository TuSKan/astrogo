package time_test

import (
	"math"
	"testing"

	"github.com/hebl/gofa"

	"github.com/TuSKan/astrogo/time"
)

// rubberDays calls f for every UTC day from 1960-01-01 to 1971-12-31 with
// SOFA's jump at that day's end, dleap = dat24 − (dat0 + dlod): zero on all
// but the handful of days UTC was stepped by a fraction of a second.
func rubberDays(t *testing.T, f func(y, m, d int, dleap float64)) {
	t.Helper()

	dat := func(y, m, d int, fd float64) float64 {
		var v float64
		if status := gofa.Dat(y, m, d, fd, &v); status < 0 {
			t.Fatalf("gofa.Dat(%d-%02d-%02d) status %d", y, m, d, status)
		}

		return v
	}

	var jd0, jd1 float64
	if gofa.Cal2jd(1960, 1, 1, &jd0, &jd1) != 0 {
		t.Fatal("Cal2jd(1960-01-01)")
	}

	for mjd := jd1; ; mjd++ {
		var y, m, d int

		var fd float64
		if gofa.Jd2cal(jd0, mjd, &y, &m, &d, &fd) != 0 {
			t.Fatalf("Jd2cal(%v)", mjd)
		}

		if y >= 1972 {
			return
		}

		var ny, nm, nd int
		if gofa.Jd2cal(jd0, mjd+1, &ny, &nm, &nd, &fd) != 0 {
			t.Fatalf("Jd2cal(%v)", mjd+1)
		}

		dat0 := dat(y, m, d, 0)
		dlod := 2 * (dat(y, m, d, 0.5) - dat0)

		f(y, m, d, dat(ny, nm, nd, 0)-(dat0+dlod))
	}
}

// sofaLabel is SOFA's two-part UTC Julian Date for a wall-clock time.
func sofaLabel(t *testing.T, y, m, d, hh, mm int, sec float64) (float64, float64) {
	t.Helper()

	var d1, d2 float64
	if status := gofa.Dtf2d("UTC", y, m, d, hh, mm, sec, &d1, &d2); status < 0 {
		t.Fatalf("Dtf2d(%d-%02d-%02d %02d:%02d:%v) status %d", y, m, d, hh, mm, sec, status)
	}

	return d1, d2
}

// daysApart is the difference of two two-part Julian Dates, in days.
func daysApart(a1, a2, b1, b2 float64) float64 { return (a1 - b1) + (a2 - b2) }

// nsPerDay converts a difference in days to nanoseconds for a message.
const nsPerDay = 86400e9

// sameInstant is 1e-14 day, 0.9 ns: astrogo and SOFA form these sums by
// different arithmetic, which is no claim to the last bit.
const sameInstant = 1e-14

// TestUTCFrom1960IsSOFAs is #479: from 1960, a UTC epoch means what SOFA says
// it means, on every day, at the start, middle and end of the day.
//
//   - The label a calendar time becomes is iauDtf2d's, so a day that ends in
//     a fractional jump is stretched by it.
//   - TAI and TT are iauUtctai's (+32.184 s), and TAI back to UTC is
//     iauTaiutc's.
//   - UT1 for a given UT1−UTC is iauUtcut1's.
func TestUTCFrom1960IsSOFAs(t *testing.T) {
	t.Parallel()

	steps := 0

	rubberDays(t, func(y, m, d int, dleap float64) {
		if dleap != 0 {
			steps++
		}

		for _, hms := range []struct {
			hh, mm int
			sec    float64
		}{{0, 0, 0}, {12, 0, 0}, {23, 59, 59.5}} {
			s1, s2 := sofaLabel(t, y, m, d, hms.hh, hms.mm, hms.sec)

			whole := int(hms.sec)
			utc := time.Date(y, time.Month(m), d, hms.hh, hms.mm, whole, int((hms.sec-float64(whole))*1e9), time.LocationUTC)

			if u1, u2 := utc.JDParts(); math.Abs(daysApart(u1, u2, s1, s2)) > sameInstant {
				t.Fatalf("%d-%02d-%02d %02d:%02d:%v: label %.3f ns from iauDtf2d's", y, m, d, hms.hh, hms.mm, hms.sec, daysApart(u1, u2, s1, s2)*nsPerDay)
			}

			var a1, a2 float64
			if gofa.Utctai(s1, s2, &a1, &a2) < 0 {
				t.Fatal("Utctai")
			}

			fromSOFA := time.FromJDParts(s1, s2, time.UTC)

			if g1, g2 := fromSOFA.TAI().JDParts(); math.Abs(daysApart(g1, g2, a1, a2)) > sameInstant {
				t.Fatalf("%d-%02d-%02d %02d:%02d:%v: TAI %.3f ns from iauUtctai's", y, m, d, hms.hh, hms.mm, hms.sec, daysApart(g1, g2, a1, a2)*nsPerDay)
			}

			if g1, g2 := fromSOFA.TT().JDParts(); math.Abs(daysApart(g1, g2, a1, a2+32.184/86400)) > sameInstant {
				t.Fatalf("%d-%02d-%02d %02d:%02d:%v: TT %.3f ns from iauUtctai's + 32.184 s", y, m, d, hms.hh, hms.mm, hms.sec, daysApart(g1, g2, a1, a2+32.184/86400)*nsPerDay)
			}

			var b1, b2 float64
			if gofa.Taiutc(a1, a2, &b1, &b2) < 0 {
				t.Fatal("Taiutc")
			}

			// Except at the very first instant of UTC. iauTaiutc iterates
			// through iauUtctai, and an iterate a hair before 1960-01-01 0h
			// lands on 1959-12-31, which SOFA stretches by its table's 1.42 s
			// edge (see TestUTCBefore1960IsNotStretched) and astrogo does not;
			// SOFA's answer there is 0.94 s away. Every other instant, and
			// every forward conversion at this one, agrees.
			firstInstant := y == 1960 && m == 1 && d == 1 && hms.hh == 0 && hms.mm == 0 && hms.sec == 0

			if g1, g2 := time.FromJDParts(a1, a2, time.TAI).UTC().JDParts(); !firstInstant && math.Abs(daysApart(g1, g2, b1, b2)) > sameInstant {
				t.Fatalf("%d-%02d-%02d %02d:%02d:%v: TAI→UTC %.3f ns from iauTaiutc's", y, m, d, hms.hh, hms.mm, hms.sec, daysApart(g1, g2, b1, b2)*nsPerDay)
			}

			const dut1 = 0.1234

			var v1, v2 float64
			if gofa.Utcut1(s1, s2, dut1, &v1, &v2) < 0 {
				t.Fatal("Utcut1")
			}

			if g1, g2 := fromSOFA.UT1Using(dut1).JDParts(); math.Abs(daysApart(g1, g2, v1, v2)) > sameInstant {
				t.Fatalf("%d-%02d-%02d %02d:%02d:%v: UT1 %.3f ns from iauUtcut1's", y, m, d, hms.hh, hms.mm, hms.sec, daysApart(g1, g2, v1, v2)*nsPerDay)
			}
		}
	})

	// A guard on the guard: the 1960s jumps are what makes this period
	// different, and a loop that found none tested nothing about them.
	if steps < 10 {
		t.Fatalf("only %d days with a fractional jump; SOFA's table has more", steps)
	}
}

// TestRubberDayLastMinuteRunsPast60: on a day that gained a fraction of a
// second, iauDtf2d gives its last minute 60 + the jump, and so does Date.
func TestRubberDayLastMinuteRunsPast60(t *testing.T) {
	t.Parallel()

	found := false

	rubberDays(t, func(y, m, d int, dleap float64) {
		if dleap <= 0 || found {
			return
		}

		found = true
		sec := 60 + dleap/2

		s1, s2 := sofaLabel(t, y, m, d, 23, 59, sec)

		got := time.Date(y, time.Month(m), d, 23, 59, 60, int(dleap/2*1e9), time.LocationUTC)
		if g1, g2 := got.JDParts(); math.Abs(daysApart(g1, g2, s1, s2)) > sameInstant {
			t.Errorf("%d-%02d-%02d 23:59:%v: %.3f ns from iauDtf2d's, so the second the day gained was normalized away",
				y, m, d, sec, daysApart(g1, g2, s1, s2)*nsPerDay)
		}
	})

	if !found {
		t.Fatal("no positive fractional jump in 1960–1971; SOFA's table has several")
	}
}

// TestUTCBefore1960IsNotStretched: SOFA's table starts in 1960 and reads
// TAI−UTC before it as zero, so iauDtf2d stretches 1959-12-31 by the whole
// 1.42 s of the table's first value. That is the table's edge, not a jump UTC
// made, and astrogo reads 1959 as UT through ΔT: noon is half the day.
func TestUTCBefore1960IsNotStretched(t *testing.T) {
	t.Parallel()

	noon := time.Date(1959, time.December, 31, 12, 0, 0, 0, time.LocationUTC)

	jd1, jd2 := noon.JDParts()
	if frac := math.Mod(jd1+jd2+0.5, 1); math.Abs(frac-0.5) > 1e-12 {
		t.Errorf("1959-12-31 12:00 is %.15f of the day; want 0.5, unstretched", frac)
	}
}
