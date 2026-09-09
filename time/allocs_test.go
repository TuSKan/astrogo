package time

import "testing"

// TestScaleConversionsDoNotAllocate pins the zero-allocation claim on the
// conversions that carry no Earth Orientation Parameters.
//
// # Why this is a test and not a benchmark
//
// This repository runs every benchmark on every push to main and uploads the
// numbers as an artifact. That found nothing, because a number in an artifact
// cannot be wrong: CI recorded 149 GB/op for a scheduler benchmark on every
// merge for months while #246 was live. A benchmark-diffing gate would not
// have caught it either — the defect was present at the baseline, so every run
// agreed with the one before it. What catches that class is an absolute claim
// about a named path (#248).
//
// allocs/op is the right thing to assert because it is deterministic, unlike
// ns/op on a shared runner. Checked rather than assumed: these counts are
// unchanged under -race, so this needs no build tag and no short-mode skip.
//
// # Scope
//
// TAI, TT, TDB and MJD are pure arithmetic over the epoch: no table lookup, no
// EOP, no I/O. Zero is therefore the honest claim and not merely the current
// measurement.
//
// UT1 is deliberately absent. It consults the EOP model, so what it costs
// depends on data this test would have to install, and the path that actually
// went wrong there is guarded far more precisely in
// time/internal/iers by counting Loader.Cached calls (#247). A weaker
// duplicate here would add noise rather than coverage.
//
// Not parallel: testing.AllocsPerRun measures the whole process, so a sibling
// test allocating concurrently would be counted here.
func TestScaleConversionsDoNotAllocate(t *testing.T) {
	epoch := FromJD(2460000.5, UTC)

	for _, tc := range []struct {
		name string
		f    func()
	}{
		{"TAI", func() { _ = epoch.TAI() }},
		{"TT", func() { _ = epoch.TT() }},
		{"TDB", func() { _ = epoch.TDB() }},
		{"MJD", func() { _ = epoch.MJD() }},
		{"JD", func() { _ = epoch.JD() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := testing.AllocsPerRun(1000, tc.f); got != 0 {
				t.Errorf("%s allocates %v times per call, want 0; it is arithmetic over "+
					"the epoch and has nothing to put on the heap", tc.name, got)
			}
		})
	}
}

// TestEpochArithmeticDoesNotAllocate covers the comparison and difference
// operations, which a solver runs at every step of every bisection.
func TestEpochArithmeticDoesNotAllocate(t *testing.T) {
	a := FromJD(2460000.5, UTC)
	b := FromJD(2460001.5, UTC)
	tt := a.TT()

	for _, tc := range []struct {
		name string
		f    func()
	}{
		{"Sub same scale", func() { _ = b.Sub(a) }},
		{"Sub cross scale", func() { _ = b.Sub(tt) }},
		{"Equal same scale", func() { _ = a.Equal(b) }},
		{"Add", func() { _ = a.Add(Hour) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := testing.AllocsPerRun(1000, tc.f); got != 0 {
				t.Errorf("%s allocates %v times per call, want 0", tc.name, got)
			}
		})
	}
}
