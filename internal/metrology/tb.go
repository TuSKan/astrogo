package metrology

// TB is the part of [testing.TB] this package uses.
//
// Declared here rather than taking testing.TB directly for two reasons. The
// first is testability: testing.TB carries an unexported method precisely so
// that nothing outside the standard library can implement it, which would
// leave this package's own failure paths — contract violations, non-finite
// samples, regressions — impossible to exercise without a test that is
// supposed to fail. The second is that a four-method surface says plainly
// what a Suite does to a test: it logs, it fails, and it skips. It does not
// call Fatal, so a suite never aborts a test mid-way and leaves the rest of
// the comparison unmeasured.
//
// *testing.T, *testing.B and *testing.F all satisfy it.
type TB interface {
	Helper()
	Log(args ...any)
	Errorf(format string, args ...any)
	Skipf(format string, args ...any)
}
