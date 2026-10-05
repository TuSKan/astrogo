package time

import (
	"math"
	"sync"

	"github.com/TuSKan/astrogo/internal/gofaext"
)

// UTC days are not all 86400 seconds long, and this file is where [Time]
// accounts for the ones that are not.
//
// # The convention, which is SOFA's
//
// A UTC day that ends in a leap second lasts 86401 SI seconds, and its last
// minute runs to 23:59:60. A two-part Julian Date whose day is 86400 seconds
// long has nowhere to put that second: 23:59:60 lands on the following
// midnight, and the one second of real time between them is zero seconds wide
// in the type. That is #144.
//
// iauDtf2d's answer is to let the fraction in a UTC Julian Date be a fraction
// of *that day's* length. On an ordinary day the two readings coincide. On a
// leap-second day the fraction is seconds-since-midnight over 86401, so
// 23:59:60.5 is 86400.5/86401 of the way through the day — distinct from the
// following midnight, and still inside the day it belongs to. iauUtctai,
// iauTaiutc and iauD2dtf all read UTC this way, and so does astropy through
// ERFA. It is the representation, not an approximation to one.
//
// Everything here follows from that one statement: to turn a UTC fraction into
// seconds, multiply by the day's own length; to turn it into TAI, add ΔAT as it
// stood at 0h, because the inserted second carries the old value.
//
// # Why this is not a call into SOFA
//
// Because SOFA's routines consult SOFA's own leap-second table. astrogo lets a
// caller register a newer one — see [RegisterLeapSeconds] — and a conversion
// that quietly ignored it would be right until the first leap second gofa has
// not heard of and a second wrong after. So the routines are restated here
// against [deltaAT], which is registry-aware, and tested against gofa's own
// where the two tables agree.
//
// # Which days count
//
// A day whose ΔAT steps by a whole second at its end — a leap second, positive
// or negative. Before 1972 UTC was steered by rate offsets and fractional
// jumps of a few hundredths of a second, which iauDtf2d stretches the day for
// and iauD2dtf, in the other direction, does not. That inconsistency is SOFA's,
// and those days are left exactly as they were: a whole second is what makes a
// leap second one, which is the line [leapSecondEndsDay] and
// [validateLeapTable] already draw.

// daySeconds is the length of a day in every scale but UTC, and of an ordinary
// day in UTC.
const daySeconds = 86400.0

// mjdZero is the Julian Date of MJD 0. Subtracting it from a two-part Julian
// Date and flooring gives the day number the step index is keyed by.
const mjdZero = 2400000.5

// unixEpochMJD is the MJD of 1970-01-01, where a Unix day count starts.
const unixEpochMJD = 40587

// utcSteps indexes the UTC days that do not last 86400 SI seconds.
//
// Keyed by MJD, and bounded by the first and last such day, so that the
// question asked on every conversion — is this day special — costs two integer
// comparisons for any epoch after the last leap second, which is every "now"
// since 2017. The map is consulted only inside the range.
//
// leapsecond.go records what an extra 8.7 ns cost BenchmarkUTCToTAI when a
// mutex was tried on this path. This is built once per leap-second table and
// read without locking.
type utcSteps struct {
	first, last int
	leap        map[int]float64 // MJD of a day ending in a step -> the step, seconds

}

// builtinStepDays are the days of the month on which gofa's record ends a day
// with a step. Its leap seconds take effect on 1 January or 1 July, so end 30
// June and 31 December; its 1960s jumps (see addRubberSteps) take effect on
// the 1st of other months too, and the days before those that occur are the
// 28th (1965-02-28), the 30th and the 31st. TestStepDaysCoverTheIndex holds
// this to the index rather than to this comment.
const builtinStepDays uint32 = 1<<28 | 1<<30 | 1<<31

// stepDays returns the day-of-month mask for the leap-second table in force: a
// bit set for every day of the month that ends a day with a step, in any year.
//
// A superset is harmless — a false positive only sends the caller to the
// index — and an exact answer is not needed. What is needed is that it costs
// nothing, so it travels inside the registered table, published atomically
// with it, and is a constant for gofa's own.
func stepDays() uint32 {
	if t := leapRegistry.Load(); t != nil {
		return t.days
	}

	return builtinStepDays
}

// leapAtEnd returns the whole-second step in ΔAT at the end of the UTC day with
// the given MJD: +1 for a positive leap second, −1 for a negative one, 0 for an
// ordinary day. SOFA's dleap.
func (s *utcSteps) leapAtEnd(mjd int) float64 {
	if mjd < s.first || mjd > s.last {
		return 0
	}

	return s.leap[mjd]
}

// builtinUTCSteps is the index for gofa's own table, built on first use.
var builtinUTCSteps = sync.OnceValue(func() *utcSteps { return buildUTCSteps(nil) })

// currentUTCSteps returns the index for the leap-second table in force.
func currentUTCSteps() *utcSteps {
	t := leapRegistry.Load()
	if t == nil {
		return builtinUTCSteps()
	}

	if t.steps == nil {
		// A table installed without going through RegisterLeapSeconds. Correct
		// and slow; nothing in the package does it.
		return buildUTCSteps(t.entries)
	}

	return t.steps()
}

// buildUTCSteps indexes the leap seconds of the table formed by gofa's own
// record extended by entries, which is the table [deltaATIn] answers from.
//
// The candidates are every date either record says a value takes effect —
// gofa's on 1 January and 1 July, which is where all of them fall, and each
// registered entry wherever it falls, since [validateLeapTable] does not
// require a month boundary. Each is kept if ΔAT steps across it by a whole
// second.
func buildUTCSteps(entries []LeapSecond) *utcSteps {
	s := &utcSteps{first: math.MaxInt, last: math.MinInt, leap: map[int]float64{}}

	consider := func(y, m, d int) {
		py, pm, pd := dayBefore(y, m, d)

		step := deltaATIn(entries, y, m, d, 0) - deltaATIn(entries, py, pm, pd, 0)
		if math.Abs(math.Abs(step)-1) > 1e-9 {
			return
		}

		mjd := mjdOf(py, pm, pd)
		s.leap[mjd] = step
		s.first = min(s.first, mjd)
		s.last = max(s.last, mjd)
	}

	for y := 1972; y <= 2100; y++ {
		consider(y, 1, 1)
		consider(y, 7, 1)
	}

	for _, e := range entries {
		consider(e.Year, e.Month, e.Day)
	}

	addRubberSteps(s, entries)

	return s
}

// addRubberSteps indexes the 1960s days that end in a fractional step of
// TAI−UTC, which [Time] reads as SOFA does since #479.
//
// From 1960 to 1971 UTC was steered by rate offsets — "rubber seconds" — and
// by occasional jumps of 0.05 to 0.1 s. iauDtf2d stretches a day that ends in
// such a jump, and iauUtctai and iauUtcut1 read its fraction that way, exactly
// as for a leap second. The jump is SOFA's dleap: what TAI−UTC does across the
// midnight beyond the day's own drift, dleap = dat24 − (dat0 + dlod), with
// dlod = 2(dat12 − dat0). Every jump takes effect on the 1st of a month.
//
// 1959-12-31 is not one, though SOFA's table makes it look like one: TAI−UTC
// is undefined before 1960 and SOFA reads it as zero, so the first day of the
// table is a 1.42 s "jump" that UTC never made. astrogo reads UTC before 1960
// as UT through ΔT, so that day is not a UTC day at all here.
func addRubberSteps(s *utcSteps, entries []LeapSecond) {
	for y := 1960; y <= 1971; y++ {
		for m := 1; m <= 12; m++ {
			if y == 1960 && m == 1 {
				continue
			}

			py, pm, pd := dayBefore(y, m, 1)

			dat0 := deltaATIn(entries, py, pm, pd, 0)
			dlod := 2 * (deltaATIn(entries, py, pm, pd, 0.5) - dat0)

			dleap := deltaATIn(entries, y, m, 1, 0) - (dat0 + dlod)
			if math.Abs(dleap) < 1e-9 {
				continue
			}

			mjd := mjdOf(py, pm, pd)
			s.leap[mjd] = dleap
			s.first = min(s.first, mjd)
			s.last = max(s.last, mjd)
		}
	}

	// The last of them, 1971-12-31, is the jump to the leap-second era's first
	// whole count, 10 s. The loop stops at 1971-12-01, so it is taken here.
	py, pm, pd := dayBefore(1972, 1, 1)

	dat0 := deltaATIn(entries, py, pm, pd, 0)
	dlod := 2 * (deltaATIn(entries, py, pm, pd, 0.5) - dat0)

	if dleap := deltaATIn(entries, 1972, 1, 1, 0) - (dat0 + dlod); math.Abs(dleap) >= 1e-9 {
		mjd := mjdOf(py, pm, pd)
		s.leap[mjd] = dleap
		s.first = min(s.first, mjd)
	}
}

// mjdOf returns the MJD of 0h on a Gregorian calendar date.
func mjdOf(y, m, d int) int {
	jd1, jd2, _ := gofaext.Dtf2d("UTC", y, m, d, 0, 0, 0)

	return int(math.Round(jd1 - mjdZero + jd2))
}

// dayBefore returns the calendar date preceding (y, m, d).
func dayBefore(y, m, d int) (int, int, int) {
	jd1, jd2, _ := gofaext.Dtf2d("UTC", y, m, d, 0, 0, 0)
	py, pm, pd, _, _ := gofaext.JdToDate(jd1, jd2-1.0)

	return py, pm, pd
}

// utcDayOf splits a UTC two-part Julian Date into the MJD of its day and the
// fraction of that day elapsed, the fraction being of the day's own length.
//
// Computed from the parts rather than their sum so the fraction keeps the
// precision a two-part date exists to hold: jd1 − mjdZero − mjd is exact, and
// only jd2 carries the fine part.
func utcDayOf(jd1, jd2 float64) (mjd int, frac float64) {
	day := math.Floor(jd1 - mjdZero + jd2)

	return int(day), (jd1 - mjdZero - day) + jd2
}

// stepDayOf is the MJD of the UTC day a label belongs to, as gofaext.JdToDate
// names it, for the conversions that read a label's date from JdToDate and its
// step from the index.
//
// utcDayOf floors the sum of the parts, and a label within an ulp below
// midnight sums to the integer: it names the next day, with a fraction a hair
// below zero, where JdToDate names the earlier day with a fraction a hair below
// one. Reading the date from one and the step from the other cost a whole leap
// second at 23:59:60.99999997 (#499). utcDayOf itself is right for what label
// arithmetic asks of it, where a uniform label on midnight means midnight.
func stepDayOf(jd1, jd2 float64) int {
	mjd, frac := utcDayOf(jd1, jd2)
	if frac < 0 {
		mjd--
	}

	return mjd
}

// utcToTAI converts a UTC two-part Julian Date to TAI. iauUtctai, against
// [deltaAT].
//
// On an ordinary day this is exactly the arithmetic [Time.TAI] did before
// leap seconds could be represented, so no ordinary epoch moves by even a bit.
// On a leap-second day the fraction is of that day's longer length, so it is
// rescaled to SI seconds before ΔAT is added, and ΔAT is the value at 0h: the
// inserted second carries the old count.
func utcToTAI(jd1, jd2 float64) (float64, float64) {
	y, m, d, fd, _ := gofaext.JdToDate(jd1, jd2)

	// The day of the month is already in hand, and for 29 days in 31 it rules
	// a leap second out before the index is so much as fetched — which is
	// the part that costs: measured, fetching it on every call added 6.5 ns,
	// 16%, to BenchmarkUTCToTAI.
	if stepDays()&(1<<uint(d)) != 0 {
		if leap := currentUTCSteps().leapAtEnd(stepDayOf(jd1, jd2)); leap != 0 {
			// iauUtctai: the fraction is of the stretched day, so it is
			// rescaled to SI seconds, si, and then by the day's own drift in
			// TAI−UTC, dlod, before ΔAT at 0h is added. dlod is the 1960s rate
			// offset and exactly zero from 1972, where this reduces to the
			// leap-second arithmetic it always was, bit for bit.
			dat0 := deltaAT(y, m, d, 0)
			dlod := 2 * (deltaAT(y, m, d, 0.5) - dat0)
			si := fd * (daySeconds + leap) / daySeconds

			return jd1, jd2 + (fd*leap+si*dlod+dat0)/daySeconds
		}
	}

	return jd1, jd2 + deltaAT(y, m, d, fd)/daySeconds
}

// taiToUTC converts a TAI two-part Julian Date to UTC. iauTaiutc, against
// [deltaAT].
//
// The ordinary answer is the one [Time.UTC] always gave: subtract ΔAT, and
// look it up again on the far side in case a step lay between. Only when that
// answer lands on a leap-second day, or on the day after one, is it replaced —
// by inverting [utcToTAI] with SOFA's own method, a fixed-point iteration that
// is exact after two steps and is given three.
func taiToUTC(jd1, jd2 float64) (float64, float64) {
	y, m, d, fd, _ := gofaext.JdToDate(jd1, jd2)
	dat := deltaAT(y, m, d, fd)
	u2 := jd2 - dat/daySeconds

	uy, um, ud, ufd, _ := gofaext.JdToDate(jd1, u2)
	if dat2 := deltaAT(uy, um, ud, ufd); dat2 != dat {
		u2 = jd2 - dat2/daySeconds
	}

	steps := currentUTCSteps()

	mjd, _ := utcDayOf(jd1, u2)
	if steps.leapAtEnd(mjd) == 0 && steps.leapAtEnd(mjd-1) == 0 {
		return jd1, u2
	}

	r1, r2 := jd1, jd2
	for range 3 {
		g1, g2 := utcToTAI(r1, r2)
		r2 += jd1 - g1
		r2 += jd2 - g2
	}

	return r1, r2
}

// utcToLabel re-expresses a UTC two-part Julian Date with an 86400-second day,
// which is the form label arithmetic works in: the uniform count of civil
// seconds that [Time.Add] advances.
//
// On an ordinary day it is the identity. On a leap-second day it stretches the
// fraction back to 86400 seconds, which is lossless for every instant but the
// leap second itself — that one has no uniform label, and lands on the
// following midnight exactly as it always did.
func utcToLabel(jd1, jd2 float64) (float64, float64) {
	mjd, frac := utcDayOf(jd1, jd2)

	leap := currentUTCSteps().leapAtEnd(mjd)
	if leap == 0 {
		return jd1, jd2
	}

	return jd1, jd2 + frac*leap/daySeconds
}

// utcFromLabel is the inverse of [utcToLabel].
func utcFromLabel(jd1, jd2 float64) (float64, float64) {
	mjd, frac := utcDayOf(jd1, jd2)

	leap := currentUTCSteps().leapAtEnd(mjd)
	if leap == 0 {
		return jd1, jd2
	}

	return jd1, jd2 - frac*leap/(daySeconds+leap)
}

// ut1FromUTC applies UT1−UTC to a UTC two-part Julian Date. iauUtcut1.
//
// On an ordinary day that is the addition [Time.UT1] always did. On a
// leap-second day the UTC fraction is of an 86401-second day, so the sum is
// formed in TAI instead, with ΔAT as it stood at 0h: UT1 = TAI + (DUT1 − ΔAT).
//
// Every UTC day from 1960 to 1971 goes the TAI way too, as it does in
// iauUtcut1: TAI−UTC drifted through those days, and SOFA holds UT1−TAI at its
// 0h value across each, which puts UT1 up to the day's drift (~1.3 ms) from
// UTC + DUT1 by evening. astrogo follows SOFA there since #479.
func ut1FromUTC(jd1, jd2, dut1 float64) (float64, float64) {
	if currentUTCSteps().leapAtEnd(stepDayOf(jd1, jd2)) == 0 && !inRubberEra(jd1, jd2) {
		return jd1, jd2 + dut1/daySeconds
	}

	y, m, d, _, _ := gofaext.JdToDate(jd1, jd2)
	tai1, tai2 := utcToTAI(jd1, jd2)

	return tai1, tai2 + (dut1-deltaAT(y, m, d, 0))/daySeconds
}

// utcFromUT1 is the inverse of [ut1FromUTC], with DUT1 looked up as it goes.
//
// The ordinary answer is the subtraction [Time.UTC] always did. Near a leap
// second it is replaced by a fixed-point inversion of ut1FromUTC, for the same
// reason and by the same method as [taiToUTC]: that is what makes the two
// directions agree across the step, and SOFA's own iauUt1utc, which ramps
// DUT1 by hand to the same end, would be a second implementation of it.
func utcFromUT1(jd1, jd2 float64) (float64, float64) {
	u1, u2 := jd1, jd2-dut1OrFallback(jd1, jd2)/daySeconds

	steps := currentUTCSteps()

	mjd, _ := utcDayOf(u1, u2)
	if steps.leapAtEnd(mjd) == 0 && steps.leapAtEnd(mjd-1) == 0 && !inRubberEra(u1, u2) {
		return u1, u2
	}

	for range 3 {
		g1, g2 := ut1FromUTC(u1, u2, dut1OrFallback(u1, u2))
		u2 += jd1 - g1
		u2 += jd2 - g2
	}

	return u1, u2
}

// dateInLeapSecond builds the instant a caller named with a second of 60 or
// more, when that instant is a real one: the last minute of a UTC day that
// ends in a positive leap second, within the second the day gained.
//
// Anything else reports false, and [Date] falls back to what it always did —
// normalizing, and saying so. That covers a second of 60 on a day with no leap
// second, a second of 61, and every second of 60 on the day of a negative leap
// second, when the minute is shorter rather than longer.
//
// The day is decided in UTC, because a leap second is inserted at the end of a
// UTC day: in UTC+1 the same instant is 00:59:60 on the following date.
// [utcComponents] converts without passing the second through, since handing
// 60 to the standard library is exactly what normalizes it away.
//
// The same holds for a 1960s day that ends in a positive fractional jump
// (#479): its last minute runs to 60 + the jump, as iauDtf2d allows, so
// 23:59:60.04 is a real instant on a day that gained 0.05 s.
func dateInLeapSecond(year int, month Month, day, hour, minute, second, nanosecond int, loc *Location) (Time, bool) {
	y, m, d, hh, mm := utcComponents(year, month, day, hour, minute, loc)
	if hh != 23 || mm != 59 {
		return Time{}, false
	}

	leap := currentUTCSteps().leapAtEnd(mjdOf(y, int(m), d))
	if leap <= 0 {
		return Time{}, false
	}

	sec := float64(second) + float64(nanosecond)/1e9
	if sec >= 60+leap {
		return Time{}, false
	}

	// 23:59 is 86340 seconds into the day, and the fraction is of the day's
	// own length, 86400 + the step — iauDtf2d exactly.
	jd1, jd2, _ := gofaext.Dtf2d("UTC", y, int(m), d, 0, 0, 0)

	result := FromJDParts(jd1, jd2+(86340+sec)/(daySeconds+leap), UTC)
	result.loc = loc

	return result, true
}

// jd1972 is the Julian Date of 1972-01-01 00:00 UTC, where the leap-second era
// begins.
const jd1972 = 2441317.5

// jd1960 is the Julian Date of 1960-01-01 00:00 UTC, where UTC begins and
// [Time.TT] stops using the ΔT polynomial for UTC (#479). Before it, a UTC
// label is read as UT; from it, through SOFA's TAI−UTC.
const jd1960 = 2436934.5

// inRubberEra reports whether a UTC two-part Julian Date falls from 1960 to
// 1971, when TAI−UTC drifted ("rubber seconds") rather than stepping by whole
// seconds.
func inRubberEra(jd1, jd2 float64) bool {
	jd := jd1 + jd2

	return jd >= jd1960 && jd < jd1972
}

// fromUTCOnStepDay converts a UTC epoch on a day of the month that can end in a
// leap second, adding offset seconds on top of TAI — 0 for TAI, 32.184 for TT.
//
// Kept out of [Time.TAI] and [Time.TT] on purpose. They handle the ordinary
// day inline, and carrying this branch's locals in their own frames cost every
// ordinary conversion a few nanoseconds, measured, for a path taken on two
// days of the month.
func (t Time) fromUTCOnStepDay(offset float64, s Scale) Time {
	jd1, jd2 := utcToTAI(t.jd1, t.jd2)

	return fromPartsPreserveLoc(t, jd1, jd2+offset/daySeconds, s)
}
