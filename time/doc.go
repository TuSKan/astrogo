// Package time (atime) provides high-precision astronomical time handling.
//
// # Why not stdlib time?
//
// Go's standard `time.Time` is designed for civil time and recent history/future.
// In contrast, `atime` is designed for:
//   - Precision across millennia: Uses a two-part Julian Date representation
//     to maintain sub-millisecond precision over long time scales.
//   - Multiple Time Scales: Supports UTC, TAI, TT, UT1, TDB, the GNSS
//     system times GPST and BDT, and the coordinate times TCG and TCB, with a
//     complete bidirectional conversion graph.
//   - Numerical correctness: Facilitates precise propagation of planet
//     positions and telescope pointing.
//
// # Design
//
// The core type is [Time], which stores a Julian Date as two `float64` values
// (`jd1` and `jd2`). By convention, `jd1` is the large "integer" part of the
// day, and `jd2` is the fractional part (e.g., [0, 1) or [-0.5, 0.5)).
//
// # Time Scale Conversions
//
// The conversion graph is:
//
//	UTC ←→ TAI ←→ TT ←→ TDB
//	 ↕     ↕
//	UT1   GPST, BDT
//
// Conversion status:
//   - UTC ↔ TAI: Complete (via SOFA leap-second table)
//   - TAI ↔ GPST: Complete (GPST = TAI − 19 s, exact by definition)
//   - TAI ↔ BDT:  Complete (BDT = TAI − 33 s, exact by definition)
//   - TAI ↔ TT:  Complete (TT = TAI + 32.184s, exact by definition)
//   - TT  ↔ TDB: Complete (Fairhead & Bretagnon 1990 single-term, amplitude 1.657 ms)
//   - UTC ↔ UT1: Complete when IERS EOP data is loaded; returns error when unavailable.
//
// [UT1] is the only conversion that can fail because it depends on observed
// Earth rotation data (DUT1 = UT1 − UTC). All other conversions are
// deterministic and always succeed.
//
// # Cross-Scale Operations
//
// Comparison methods ([Time.Before], [Time.After], [Time.Equal]) and arithmetic
// methods ([Time.Add], [Time.Sub]) automatically convert operands to a
// common scale (TT) when they differ. Same-scale operations have zero overhead.
//
// # Status
//
// The current implementation supports:
//   - Construction from JD, Go time, and current UTC.
//   - Basic arithmetic ([Time.Add]/[Time.Sub], over a [unit.Duration]).
//   - Full bidirectional scale conversion graph (UTC, TAI, TT, TDB, UT1,
//     GPST, BDT, TCG, TCB), every pair round-tripped by
//     TestScaleRoundTripMatrix.
//   - Scale-safe cross-scale comparison and arithmetic.
//   - Dynamic UT1/UTC derivations via the IERS EOP data model.
//
// # Concurrency
//
// The scale conversions are pure functions on a value type and are safe to
// call from any goroutine. Earth-orientation data is process-wide state
// behind a sync.RWMutex: [RegisterModel] and the lazy load that [Time.EOP],
// [Time.UTC] and [Time.UT1] trigger are safe to race, and a caller that wants
// determinism registers a model up front rather than relying on whatever the
// first lookup happens to load.
//
// # Failure and degradation
//
// The EOP path degrades rather than failing: a pre-seeded cache object, then
// a network fetch if consent was granted, then zero EOP with a one-time
// warning. Zero EOP is a real answer with real error — up to about 0.9 s of
// UT1 — so a caller who needs to know which of the three it got asks
// [EOPSource]. [Time.UT1] is the exception and reports the failure instead of
// degrading.
//
// That 0.9 s is borrowed, not intrinsic. It is the bound leap seconds keep
// |UT1−UTC| inside, and CGPM Resolution 4 (2022) commits to abandoning leap
// seconds by 2035; after that UT1−UTC grows without bound, and the zero-EOP
// degradation grows with it. Levine, Tavella & Milton (2023) estimate that a
// tolerance of one minute would mean an adjustment roughly once a century.
//
// The data path is unaffected: this package reads full UT1−UTC from
// finals2000A, not the 0.1 s-resolution DUT1 a time signal broadcasts, so it
// has no 0.9 s assumption of its own to unlearn. Only the stated bound goes
// stale, and only for the caller who has no EOP data at all.
//
// Where those bytes come from is supplied by a registered [EOPLoader], not
// reached for by this package. Importing astrogo/remote registers one — as
// any program granting download consent necessarily does — so nothing
// changes for a caller that wants EOP data. A program that imports neither
// degrades to zero EOP and links no storage backend at all, which is the
// difference between a 2.5 MB binary and a 19.4 MB one.
//
// # The clock you are given
//
// [NowUTC] and [Now] read the host's clock, and around a leap second that clock
// may be deliberately wrong by up to 0.5 s for up to 24 hours. Nothing in this
// package, or in any library, can detect it.
//
// The cause is leap smearing: rather than repeat or skip a second, an NTP
// provider spreads the step over hours by running the clock slightly fast or
// slow. Every major provider does it differently (Levine, Tavella & Milton
// 2023, Metrologia 60 014001, table 2):
//
//	Google      frequency adjustment for the 24 h before the leap second
//	Facebook    frequency adjustment for the 18 h after
//	Alibaba     symmetric, 12 h either side
//	Microsoft   frequency halved for the second before
//
// All of them, in that paper's words, "have an error on the order of ±0.5 s
// during the adjustment period", and providers "generally do not indicate which
// method is being used". So a smeared timestamp is not merely offset — it is
// offset by an amount that depends on whose NTP server the host happened to be
// using, and the host cannot say which.
//
// For this library 0.5 s is not a rounding error. It is 0.3 arcsec of lunar
// motion, 7.5 arcsec of Earth rotation, and 3.8 km of ISS ground track: far
// above the accuracy the rest of this package works to, and far below the
// threshold at which anything looks wrong. The result is a plausible epoch,
// silently off, on the days around a leap second.
//
// What to do about it, in order of preference:
//
//   - Pass an explicit epoch. A [Time] built from [Date] or [FromJD] never
//     touches the host clock and so is never smeared. Anything meant to be
//     reproducible should be doing this regardless.
//   - Take time from PTP (IEEE 1588) rather than NTP. PTP distributes TAI plus
//     the current UTC offset, so there is no step to smear.
//   - If neither is possible and the work spans a leap second, treat epochs
//     from that window as good to 0.5 s rather than to the microsecond, and
//     say so in whatever the results feed. [Time.LeapSmearWindow] is how a
//     caller finds out which epochs those are: it reports whether an instant
//     falls within a day of a leap second, and names the step. It cannot say
//     whether a particular host smeared — nothing can — only that this is an
//     epoch where the question arises.
//
// This is not a defect this package can fix — the host clock is the host's —
// and it is worth knowing rather than discovering. The window is also
// shrinking: the most recent leap second was 2016-12-31, none is currently
// scheduled, and the 2022 CGPM resolution abandons them by 2035.
//
// # 23:59:60
//
// [Time] represents a leap second. [Date] builds 2016-12-31 23:59:60 as an
// instant of its own, one SI second after 23:59:59 and one before the
// following midnight, and it converts with the ΔAT that IERS and gofa both
// give the inserted second — the old value, 36, not the 37 that starts at
// midnight.
//
// It does so by SOFA's convention. The fraction in a UTC Julian Date is a
// fraction of *that day's* length, which is 86401 seconds on a day ending in a
// leap second, so 23:59:60 sits at 86400/86401 of the day — iauDtf2d exactly,
// and what astropy does through ERFA. On every ordinary day the two readings
// coincide and nothing changed. utcday.go has the details, and #144 the
// argument; before it, 23:59:60 landed on the following midnight and was
// converted a full second wrong.
//
// Three consequences worth knowing:
//
//   - Do not subtract two UTC Julian Dates across a leap-second day. Their
//     fractions are of days of different lengths, so the difference is neither
//     the labels nor the elapsed time. [Time.Sub] measures elapsed time, and
//     does it correctly; SOFA's documentation gives the same warning.
//   - The standard library's time.Time cannot hold 23:59:60. [Time.ToGo] gives
//     what time.Date itself makes of one — the following midnight — and every
//     other instant converts exactly.
//   - [Time.Calendar]'s day fraction is of the day's own length, so it stays in
//     [0, 1) through the leap second.
//
// A second UTC never labelled is still reported rather than built: 23:59:60 on
// a day that gained no leap second, 23:59:61 anywhere, and 23:59:59 on the day
// of a negative leap second. [Date] has no error to return, so it does what the
// standard library does — rolls the second into the following minute — and
// emits a [logging] warning, because a timestamp one second wrong looks exactly
// like one that is right.
//
// That last case has never happened and is now expected to. A negative leap
// second removes the last second of a day — 23:59:58 is followed directly by
// 00:00:00. The ITU-R has permitted one since 1972 and none has been announced;
// Levine, Tavella & Milton (2023) project one by about 2030 and warn that
// because they "have never happened ... it is almost a certainty that there
// will be widespread errors in realizing the event". The representation above
// handles it on the same terms, the day running to 86399 seconds, and a caller
// who registers the announcement through [RegisterLeapSeconds] gets it before
// gofa's table does.
package time
