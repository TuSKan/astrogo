package time

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/logging"
)

// A leap second is one second wide in reality and zero seconds wide in this
// type. UTC labels the inserted second 23:59:60, and [Time] holds a two-part
// Julian Date whose day is 86400 seconds long, so that label has nowhere to
// land: [Date] normalises it onto the following midnight, where it is
// indistinguishable from an instant one second later.
//
// The consequence is measurable rather than theoretical. IERS and gofa agree
// that the inserted second carries the *old* ΔAT:
//
//	2016-12-31 23:59:59 -> 36
//	2016-12-31 23:59:60 -> 36     the leap second itself
//	2017-01-01 00:00:00 -> 37     the step instant
//
// An aliased instant therefore gets 37 where the authority says 36 — a full
// second of error, in the one place a time library is expected to be exact.
//
// SOFA does have a representation for this: iauDtf2d lets the UTC day run to
// 86401 seconds, so 23:59:60 gets a two-part JD of its own. Adopting it is not
// a local change — every conversion in this package divides by 86400 — so this
// file does the next thing, which is to make the loss audible. See #144 for
// the full argument and the options that were weighed.

var warnLeapSecondAliasedOnce sync.Once

// warnLeapSecondAliased reports, once per process, that a calendar instant
// with second ≥ 60 was normalised away.
//
// Once per process rather than per call, matching [warnEOPUnavailable]: a
// caller iterating a corpus that crosses a leap second would otherwise get one
// line per row, and the attributes on the first line already name the instant
// and the size of the error. The condition is a property of the type, not of
// the particular timestamp, so the second report would add nothing.
func warnLeapSecondAliased(year int, month time.Month, day, hour, minute, second int, loc *time.Location) {
	warnLeapSecondAliasedOnce.Do(func() {
		y, m, d, hh, mm := utcComponents(year, month, day, hour, minute, loc)
		logLeapSecondAliased(y, int(m), d, hh, mm, second)
	})
}

// utcComponents restates a wall-clock instant in UTC.
//
// Leap seconds are inserted at the end of a UTC day, so a caller in another
// zone writes the same instant with a different date and a different hour:
// classifying on the components as given would report the real 2016-12-31 leap
// second as a second that never existed. The second is deliberately not passed
// through the conversion — it is the value under discussion, and handing it to
// time.Date is exactly what normalises it away.
//
// Deep-historical years are returned unchanged, because [Date] ignores loc for
// them too.
func utcComponents(year int, month time.Month, day, hour, minute int, loc *time.Location) (int, time.Month, int, int, int) {
	if loc == nil || loc == time.UTC || year < 1 || year > 9999 {
		return year, month, day, hour, minute
	}

	u := time.Date(year, month, day, hour, minute, 0, 0, loc).UTC()

	return u.Year(), u.Month(), u.Day(), u.Hour(), u.Minute()
}

// logLeapSecondAliased writes the warning, separately from the [sync.Once]
// that rations it — split out for the same reason as [logEOPUnavailable], so a
// test can assert on the message and its level without depending on which test
// happened to spend the Once first.
func logLeapSecondAliased(y, m, d, hour, minute, second int) {
	// Warn, not Info, and so still emitted by the default logger. Date has no
	// error return, and this is the only notice a caller gets that the instant
	// they asked for is not the instant they received.
	if second == 60 && leapSecondEndsDay(y, m, d) {
		logging.Warn("leap second not representable, instant moved to the following midnight",
			"utc", isoSecond(y, m, d, hour, minute, second),
			"error", "1 s",
			"delta_at_applied", deltaAT(nextDay(y, m, d)),
			// The inserted second carries the ΔAT in force throughout the day
			// it ends, which is what a mid-day lookup returns. Asked at the
			// day's own boundary the answer depends on which side gofa rounds
			// to, and the boundary is the thing under discussion.
			"delta_at_correct", deltaAT(y, m, d, 0.5),
			"remedy", "hold instants inside a leap second in TAI; UTC cannot label them")

		return
	}

	// Second 60 on a day with no leap second, or second > 60 anywhere, is not
	// a representation limit — it is an instant that never existed. Go's
	// time.Date rolls it into the next minute, which is documented but silent,
	// and a wrong-by-a-second epoch looks exactly like a right one.
	logging.Warn("second out of range, instant normalised into the following minute",
		"utc", isoSecond(y, m, d, hour, minute, second),
		"reason", "no leap second was inserted at the end of this UTC day",
		"remedy", "pass a second in [0,59]; check the source that produced this timestamp")
}

// leapSecondEndsDay reports whether a leap second is inserted between the end
// of the UTC day (y, m, d) and the start of the next one — that is, whether
// 23:59:60 is a real instant on that date.
//
// Asked of the same ΔAT lookup every conversion in this package uses, so a
// caller who registered their own leap-second table (see [RegisterLeapSeconds])
// is answered from that table rather than from gofa's built-in one.
//
// The test is that ΔAT steps by exactly one second, not merely that it rises.
// Before 1972 UTC tracked UT2 by rate offsets and fractional jumps, so ΔAT
// there increases across *every* day by a few microseconds; a bare inequality
// would report 23:59:60 as a real instant on any date in the 1960s, which it
// never was. A whole second is what makes a leap second one, and it is the
// same property validateLeapTable enforces on a registered table.
func leapSecondEndsDay(y, m, d int) bool {
	return math.Abs(deltaAT(nextDay(y, m, d))-deltaAT(y, m, d, 0.0)-1.0) < 1e-9
}

// nextDay returns the calendar date following (y, m, d), with a trailing 0.0
// day fraction so the result feeds [deltaAT] directly.
//
// Via the Julian Date rather than by incrementing d, so month lengths, leap
// years and the proleptic Gregorian calendar are gofa's problem and not this
// file's — and leap seconds are only ever inserted at the end of a month, so
// the carry is the case that matters.
func nextDay(y, m, d int) (int, int, int, float64) {
	jd1, jd2, _ := gofaext.Dtf2d("UTC", y, m, d, 0, 0, 0)

	ny, nm, nd, _, _ := gofaext.JdToDate(jd1, jd2+1.0)

	return ny, nm, nd, 0.0
}

// isoSecond formats a calendar instant for the warning's utc attribute. The
// second is printed verbatim, 60 and above included: which value was rejected
// is the whole point of the message, so this must not normalise it the way the
// constructor being reported on just did.
func isoSecond(y, m, d, hour, minute, second int) string {
	return fmt.Sprintf("%04d-%02d-%02dT%02d:%02d:%02dZ", y, m, d, hour, minute, second)
}

// The mirror image, which has never happened and is now expected to.
//
// A negative leap second removes the last second of a UTC day: 23:59:58 is
// followed directly by 00:00:00, and 23:59:59 is an instant that did not occur.
// The ITU-R has permitted one since 1972 and none has ever been announced, so
// fifty years of one-sided history sit behind every assumption here — Levine,
// Tavella & Milton (2023) project one "by about the year 2030" and warn that
// because they "have never happened ... it is almost a certainty that there
// will be widespread errors in realizing the event".
//
// [Time] cannot say a civil second is absent any more than it can say one is
// present, so the answer is the same as for the positive case: report it. See
// #147, and #144 for the representation question both share.

var warnSecondRemovedOnce sync.Once

// warnIfSecondRemoved reports, once per process, that an instant named the
// second a negative leap second took out of the record. Called for any second
// of 59; it decides the rest.
//
// The decision runs on the UTC components, not on the ones the caller wrote.
// The removal happens at the end of a UTC day, so in UTC+13 the same instant is
// 12:59:59 on the following date — a gate on the caller's own hour and minute
// would never fire there, and would then look at the wrong day anyway. Found by
// the test for it, after a first version gated in Date on the local clock.
//
// The order is deliberate, and the cost is worth stating rather than calling
// cheap. Measured on Date, i9-11980HK, ns/op:
//
//	ordinary second                    19.6
//	second 59, UTC                     22.1   the branch and a no-op conversion
//	second 59, UTC+13                  54.0   a real zone conversion
//	23:59:59 UTC                      209.7   plus the two ΔAT lookups
//
// So one timestamp in sixty pays a few nanoseconds, one in 86400 pays the
// lookups, and today none reaches the classifier's true branch at all, because
// no negative leap second has ever been announced.
func warnIfSecondRemoved(year int, month time.Month, day, hour, minute int, loc *time.Location) {
	y, m, d, hh, mm := utcComponents(year, month, day, hour, minute, loc)

	if hh != 23 || mm != 59 {
		return
	}

	if !negativeLeapSecondEndsDay(y, int(m), d) {
		return
	}

	warnSecondRemovedOnce.Do(func() { logSecondRemoved(y, int(m), d, hh, mm) })
}

// logSecondRemoved writes the warning, split from its [sync.Once] for the same
// reason as [logLeapSecondAliased].
func logSecondRemoved(y, m, d, hour, minute int) {
	logging.Warn("second removed by a negative leap second, instant did not occur",
		"utc", isoSecond(y, m, d, hour, minute, 59),
		"reason", "this UTC day ended at 23:59:58; the next instant was the following midnight",
		"delta_at_before", deltaAT(y, m, d, 0.5),
		"delta_at_after", deltaAT(nextDay(y, m, d)),
		"remedy", "check the source of this timestamp; UTC never labelled this second")
}

// negativeLeapSecondEndsDay reports whether the last second of the UTC day
// (y, m, d) was removed — that is, whether 23:59:59 never happened on it.
//
// The mirror of [leapSecondEndsDay], and a whole second in the other
// direction for the same reason: pre-1972 ΔAT drifts by microseconds across
// every day, so only a step of exactly −1 is a leap second at all.
func negativeLeapSecondEndsDay(y, m, d int) bool {
	return math.Abs(deltaAT(nextDay(y, m, d))-deltaAT(y, m, d, 0.0)+1.0) < 1e-9
}
