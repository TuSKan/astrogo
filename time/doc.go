// Package time (atime) provides high-precision astronomical time handling.
//
// # Why not stdlib time?
//
// Go's standard `time.Time` is designed for civil time and recent history/future.
// In contrast, `atime` is designed for:
//   - Precision across millennia: Uses a two-part Julian Date representation
//     to maintain sub-millisecond precision over long time scales.
//   - Multiple Time Scales: Supports UTC, TAI, TT, UT1, and TDB with a
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
//	 ↕
//	UT1
//
// Conversion status:
//   - UTC ↔ TAI: Complete (via SOFA leap-second table)
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
// methods ([Time.Sub], [Time.SubDays]) automatically convert operands to a
// common scale (TT) when they differ. Same-scale operations have zero overhead.
//
// # Status
//
// The current implementation supports:
//   - Construction from JD, Go time, and current UTC.
//   - Basic arithmetic (AddDays/SubDays).
//   - Full bidirectional scale conversion graph (UTC, TAI, TT, TDB, UT1).
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
//     say so in whatever the results feed.
//
// This is not a defect this package can fix — the host clock is the host's —
// and it is worth knowing rather than discovering. The window is also
// shrinking: the most recent leap second was 2016-12-31, none is currently
// scheduled, and the 2022 CGPM resolution abandons them by 2035.
package time
