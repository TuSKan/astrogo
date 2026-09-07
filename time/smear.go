package time

import (
	"math"

	"github.com/TuSKan/astrogo/internal/gofaext"
)

// smearWindowDays is how far either side of a leap second a host clock may be
// deliberately wrong.
//
// One day, which is the envelope of the four published methods rather than any
// one of them (Levine, Tavella & Milton 2023, Metrologia 60 014001, table 2):
// Google adjusts for the 24 h before, Facebook for the 18 h after, Alibaba
// symmetrically 12 h either side, Microsoft for the second before. Providers
// "generally do not indicate which method is being used", so a caller cannot
// narrow it by asking, and the union is the only honest answer.
const smearWindowDays = 1.0

// LeapSmearWindow reports whether t falls close enough to a leap second that a
// host clock disciplined by NTP may have been deliberately wrong when it was
// read, and returns the leap second responsible.
//
// # What the window means
//
// Rather than repeat or skip a second, an NTP provider spreads the step over
// hours by running the clock slightly fast or slow. All the published methods
// "have an error on the order of ±0.5 s during the adjustment period", and for
// this library 0.5 s is 0.3 arcsec of lunar motion, 7.5 arcsec of Earth
// rotation and 3.8 km of ISS ground track — far above the accuracy the rest of
// this package works to, and far below the threshold at which anything looks
// wrong.
//
// # What to do with a true
//
// Nothing automatic, which is why this reports rather than corrects. No library
// can tell whether a given host smeared, by how much, or in which direction:
// the answer depends on whose NTP server it happened to be using, and the host
// cannot say. What a caller can do is treat such an epoch as good to 0.5 s
// rather than to the microsecond, and say so in whatever the results feed.
//
// # When it is worth asking
//
// Only for an epoch that came from a host clock — [Now], [NowUTC], or a
// timestamp recorded by some other machine. An epoch built with [Date] or
// [FromJD] never touched a clock and is never smeared, so asking about one
// answers a question nobody has.
//
// The answer is false for every instant since 2016-12-31, the most recent leap
// second, and none is currently scheduled. It is historical data, and the
// negative leap second projected for about 2030, that this is for.
//
// The record consulted is the one in force — see [RegisterLeapSeconds] — so a
// caller who registered an announced step gets it before gofa's table carries
// it.
func (t Time) LeapSmearWindow() (LeapSecond, bool) {
	utc := t.UTC()
	y, m, _, _ := utc.Calendar()

	// A leap second takes effect at midnight beginning the first of a month,
	// and months are at least 28 days, so the 48-hour window around t can
	// reach at most one such boundary: this month's start or the next one's.
	for _, b := range [2]struct{ y, m int }{{y, m}, nextMonth(y, m)} {
		step, ok := leapStepAt(b.y, b.m)
		if !ok {
			continue
		}

		if math.Abs(utc.JD()-monthStartJD(b.y, b.m)) <= smearWindowDays {
			return step, true
		}
	}

	return LeapSecond{}, false
}

// leapStepAt reports the leap second taking effect at midnight beginning
// (y, m, 1), if one does.
//
// A whole second, in either direction: pre-1972 ΔAT drifts by microseconds
// across every boundary, and calling that a leap second would put half the
// 1960s inside a smear window that did not exist — leap smearing postdates
// leap seconds, which postdate that drift.
func leapStepAt(y, m int) (LeapSecond, bool) {
	after := deltaAT(y, m, 1, 0.0)
	before := deltaAT(prevDay(y, m, 1))

	if math.Abs(math.Abs(after-before)-1.0) > 1e-9 {
		return LeapSecond{}, false
	}

	return LeapSecond{Year: y, Month: m, Day: 1, DeltaAT: after}, true
}

// monthStartJD is the Julian Date of midnight beginning (y, m, 1) UTC.
func monthStartJD(y, m int) float64 {
	jd1, jd2, _ := gofaext.Dtf2d("UTC", y, m, 1, 0, 0, 0)
	return jd1 + jd2
}

// nextMonth returns the month following (y, m).
func nextMonth(y, m int) struct{ y, m int } {
	if m == 12 {
		return struct{ y, m int }{y + 1, 1}
	}

	return struct{ y, m int }{y, m + 1}
}

// prevDay returns the calendar date before (y, m, d), with a trailing 0.0 day
// fraction so the result feeds [deltaAT] directly — the mirror of [nextDay],
// and via the Julian Date for the same reason.
func prevDay(y, m, d int) (int, int, int, float64) {
	jd1, jd2, _ := gofaext.Dtf2d("UTC", y, m, d, 0, 0, 0)

	py, pm, pd, _, _ := gofaext.JdToDate(jd1, jd2-1.0)

	return py, pm, pd, 0.0
}
