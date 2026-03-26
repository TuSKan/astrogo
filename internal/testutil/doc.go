// Package testutil provides shared test helpers for astrogo packages.
//
// It is an internal-only package. Code outside the astrogo module must not
// import it. All public symbols are intended for use inside _test.go files.
//
// # Float comparisons
//
// [InAbsTol], [InRelTol], and [InAngleTol] are pure predicates that return a
// bool — handy inside table-driven tests where you want to compute the result
// before deciding how to fail.
//
// [AssertNear], [AssertRelNear], and [AssertAngleNear] are assertion wrappers
// that call t.Errorf with a structured diagnostic message when the predicate
// fails. All accept [testing.TB] so they work in *testing.T, *testing.B and
// *testing.F contexts.
//
// # Error helpers
//
// [AssertNoError] and [AssertError] cover the common "must succeed / must fail"
// pair. [AssertErrorContains] and [AssertErrorIs] add substring and sentinel
// matching.
//
// # Table diagnostics
//
// [CaseLabel] and [FailCase] make table-driven test failure messages
// consistent across the repository: every failure identifies the row index and
// optional name so failures are immediately actionable.
package testutil
