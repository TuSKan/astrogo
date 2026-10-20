// Package keplerlab holds the numerical experiments behind docs/kepler.md: a
// multiprecision oracle for Kepler's equation and the two-body state, the
// candidate solvers the document compares, and the measurements that decide
// between them. It has no API; everything is in its _test.go files.
//
// The oracle is math/big at 256 bits, with π computed two independent ways
// that must agree, so a candidate's error is measured against the exact root
// of the equation it was given — not against another float64 solver, and not
// against a mean anomaly re-derived from a float64 eccentric anomaly, which
// would build the candidate's own rounding into the reference.
package keplerlab
