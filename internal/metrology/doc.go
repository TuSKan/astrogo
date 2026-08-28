// Package metrology measures how far astrogo is from a reference, and says so
// in a form that survives being read a year later.
//
// It is an internal package. Nothing outside the astrogo module imports it,
// and every symbol here exists to be used from _test.go files.
//
// # Why this is not just a tolerance check
//
// A validation test that asserts a bound and prints nothing has recorded one
// bit of information: that the bound held. It cannot say whether the bound
// held with a factor of a thousand to spare or by a hair, so it cannot detect
// a result getting ten times worse, and nobody reading it can tell whether
// the bound was chosen from physics or from whatever the code happened to
// produce on the day it was written.
//
// This package separates the two numbers that get conflated there:
//
//   - The [Contract] is the scientific bound. It is a claim about what the
//     software must achieve, it carries the reason it has the value it has,
//     and it changes only when that reason changes. [NewContract] refuses one
//     without a rationale and a source.
//   - The [Stats] are what was measured. They change whenever the code, the
//     reference data or the sampling changes, and they are recorded rather
//     than asserted.
//
// Keeping them apart is the whole point. Encoding the observed maximum as the
// tolerance produces a test that can never fail for the reason it exists —
// astrogo had exactly that in its topocentric-pointing suite, where a 3.0
// arcsecond tolerance was documented as having been chosen because a live run
// measured 2.66. In the other direction, a tolerance set far looser than the
// reference's own published accuracy cannot detect a real regression either,
// and without the measured value printed beside it nobody can see which case
// they are in.
//
// # Distribution, not maximum
//
// [Stats] reports percentiles because a maximum over a few dozen samples is
// the least stable statistic available: it is determined by a single point
// and moves whenever that point moves. p50 and p95 describe the behaviour,
// the maximum bounds it, and [Stats.Worst] names the sample that produced it
// so the report says *where* rather than only *how much*.
//
// # A skipped suite is not a passing suite
//
// Every network-tagged suite in this repository skips when its endpoint is
// unreachable, which is the right thing to do to a build and the wrong thing
// to do to a record. A suite that could not run is [StatusNotVerified] and
// says so in the report, because the alternative — being absent — reads
// identically to never having existed, and a permanently dead endpoint would
// then look like a permanently green one.
//
// # Shared ancestry
//
// Agreement between two implementations means less when they descend from the
// same source. astrogo reaches SOFA through gofa; Astropy reaches it through
// ERFA. Comparing the two is a consistency check on the plumbing, not
// independent validation of the physics. [Reference.SharedAncestor] records
// that where it applies, so a generated report can label the row instead of
// leaving a reader to know it.
//
// # Usage
//
//	s := metrology.NewSuite("ephemeris.observer_precision", ref, contract)
//	for _, point := range corpus {
//		s.Add(metrology.Sample{Error: sep, Label: point.Name, Context: point.Epoch})
//	}
//	s.Report(t)
//
// [Suite.Report] logs the summary, fails tb for any sample outside the
// contract, and — when ASTROGO_METROLOGY_OUT names a directory — writes the
// result as JSON for a CI job to collect.
package metrology
