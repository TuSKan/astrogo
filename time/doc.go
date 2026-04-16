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
package time
