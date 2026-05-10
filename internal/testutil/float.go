package testutil

import (
	"math"
	"testing"
)

// ── Pure predicates ─────────────────────────────────────────────────────────
// These return a bool and have no dependency on testing.TB, making them safe
// to use inside custom error messages or nested conditions.

// InAbsTol reports whether |got - want| <= tol.
// tol must be >= 0; a negative tolerance always returns false.
func InAbsTol(got, want, tol float64) bool {
	if tol < 0 {
		return false
	}

	return math.Abs(got-want) <= tol
}

// InRelTol reports whether the relative error |got - want| / |want| <= relTol.
// When want == 0 the comparison falls back to an absolute comparison against
// relTol (i.e. |got| <= relTol).
// relTol must be >= 0.
func InRelTol(got, want, relTol float64) bool {
	if relTol < 0 {
		return false
	}

	if want == 0 {
		return math.Abs(got) <= relTol
	}

	return math.Abs(got-want)/math.Abs(want) <= relTol
}

// InAngleTol reports whether the shortest angular distance between got and
// want is <= tol. Both got and want are in radians. The comparison is modulo
// 2π so that, for example, 0 and 2π are considered equal.
// tol must be >= 0 and is in radians.
func InAngleTol(got, want, tol float64) bool {
	if tol < 0 {
		return false
	}

	diff := math.Abs(got - want)
	// Reduce to [0, π] by folding over 2π and then π.
	diff = math.Mod(diff, 2*math.Pi)
	if diff > math.Pi {
		diff = 2*math.Pi - diff
	}

	return diff <= tol
}

// ── Assertion wrappers ───────────────────────────────────────────────────────
// These call t.Helper() and t.Errorf when the predicate fails.
// They accept testing.TB so they work in *testing.T, *testing.B, and
// *testing.F contexts without modification.

// AssertNear fails t if |got - want| > tol.
// label is included verbatim in the failure message.
func AssertNear(tb testing.TB, label string, got, want, tol float64) {
	tb.Helper()

	if !InAbsTol(got, want, tol) {
		tb.Errorf("%s: got %.15g, want %.15g (abs tol %.3g, diff %.3g)",
			label, got, want, tol, math.Abs(got-want))
	}
}

// AssertRelNear fails t if |got - want| / |want| > relTol.
// When want == 0 the comparison is |got| <= relTol (see InRelTol).
func AssertRelNear(tb testing.TB, label string, got, want, relTol float64) {
	tb.Helper()

	if !InRelTol(got, want, relTol) {
		var rel float64
		if want != 0 {
			rel = math.Abs(got-want) / math.Abs(want)
		} else {
			rel = math.Abs(got)
		}

		tb.Errorf("%s: got %.15g, want %.15g (rel tol %.3g, rel err %.3g)",
			label, got, want, relTol, rel)
	}
}

// AssertAngleNear fails t if the shortest angular distance between got and
// want (both in radians) is > tol (in radians).
func AssertAngleNear(tb testing.TB, label string, got, want, tol float64) {
	tb.Helper()

	if !InAngleTol(got, want, tol) {
		diff := math.Abs(got - want)

		diff = math.Mod(diff, 2*math.Pi)
		if diff > math.Pi {
			diff = 2*math.Pi - diff
		}

		tb.Errorf("%s: got %.15g rad, want %.15g rad (angle tol %.3g rad, shortest diff %.3g rad)",
			label, got, want, tol, diff)
	}
}

// AssertExact fails t if got != want.
// Use only when exact bit-level equality is mathematically guaranteed
// (e.g. integer Julian day round-trips).
func AssertExact(tb testing.TB, label string, got, want float64) {
	tb.Helper()

	if got != want {
		tb.Errorf("%s: got %.15g, want %.15g (expected exact equality)", label, got, want)
	}
}

// ── Unit conversion helpers ──────────────────────────────────────────────────
// Provided here for test-setup convenience; not astronomy logic.

// DegToRad converts degrees to radians.
func DegToRad(d float64) float64 { return d * math.Pi / 180 }

// RadToDeg converts radians to degrees.
func RadToDeg(r float64) float64 { return r * 180 / math.Pi }
