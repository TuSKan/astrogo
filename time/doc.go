// Package time (atime) provides high-precision astronomical time handling.
//
// # Why not stdlib time?
//
// Go's standard `time.Time` is designed for civil time and recent history/future.
// In contrast, `atime` is designed for:
//   - Precision across millenia: Uses a two-part Julian Date representation
//     to maintain sub-millisecond precision over long time scales.
//   - Multiple Time Scales: Supports UTC, TAI, TT, UT1, and TDB.
//   - Numerical correctness: Facilitates precise propagation of planet
//     positions and telescope pointing.
//
// # Design
//
// The core type is [Time], which stores a Julian Date as two `float64` values
// (`jd1` and `jd2`). By convention, `jd1` is the large "integer" part of the
// day, and `jd2` is the fractional part (e.g., [0, 1) or [-0.5, 0.5]).
//
// # Status (Initial Version)
//
// The current implementation is a foundational skeleton.
//   - Supported: Construction from JD, Go time, and current UTC. Basic
//     arithmetic (AddDays/SubDays).
//   - Not yet supported: Automatic scale conversions (e.g., UTC to TT),
//     leap second tables, or DUT1 offsets. These will be added in subsequent
//     phases.
package time
