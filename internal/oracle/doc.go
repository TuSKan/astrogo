// Package oracle holds exact answers for two-body motion, in math/big at 256
// bits: the root of Kepler's equation, the universal anomaly with its Stumpff
// functions, and the state propagated from a position and velocity.
// ephemeris/kepler's contract tests hold the shipped solvers to them:
// TestSolveKeplerContract, TestUniversalContract and TestFromStateContract.
//
// π is computed two independent ways that must agree, so a solver's error is
// measured against the exact root of the equation it was given — not against
// another float64 solver, and not against a mean anomaly re-derived from a
// float64 eccentric anomaly, which would build the solver's own rounding into
// the reference.
//
// The package began as internal/keplerlab, which also held the candidate
// solvers and the measurements behind docs/kepler.md. Those were removed once
// every decision they informed had shipped (#874); commit ec5be0d5 has them.
package oracle
