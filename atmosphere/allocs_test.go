package atmosphere

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
)

// TestRefractionAndAirmassDoNotAllocate pins the zero-allocation claim on the
// per-direction calls a sky map repeats.
//
// # Why this is a test and not a benchmark
//
// Every benchmark in this repository already runs on every push to main and
// its numbers are uploaded as an artifact. That caught nothing, because a
// number in an artifact cannot be wrong — CI recorded 149 GB/op for a
// scheduler benchmark on every merge for months while #246 was live, and a
// benchmark-diffing gate would have stayed green too, since the defect was
// there at the baseline. An absolute claim about a named path is what catches
// that class (#248).
//
// allocs/op is what to assert: it is deterministic where ns/op on a shared
// runner is not, and — measured, not assumed — unchanged under -race.
//
// # Why zero
//
// These are closed-form evaluations over scalars. skybrightness walks a whole
// hemisphere through them (129,600 points in the largest map here), so an
// allocation per call is an allocation per pixel, which is the difference
// between a sky map that fits in cache and one that keeps the collector busy.
//
// Not parallel: testing.AllocsPerRun measures the whole process, so a sibling
// test allocating concurrently would be counted here.
func TestRefractionAndAirmassDoNotAllocate(t *testing.T) {
	var (
		rigorous    = RefractionRigorous{}
		approximate = RefractionApproximate{}
		env         = StandardRefraction
		alt         = angle.Deg(30)
		horizon     = angle.Deg(0.5)
	)

	for _, tc := range []struct {
		name string
		f    func()
	}{
		{"RefractionRigorous.RefractFromTrue", func() { _ = rigorous.RefractFromTrue(alt, env) }},
		{"RefractionRigorous.RefractFromApparent", func() { _ = rigorous.RefractFromApparent(alt, env) }},
		{"RefractionRigorous near the horizon", func() { _ = rigorous.RefractFromTrue(horizon, env) }},
		{"RefractionApproximate.RefractFromTrue", func() { _ = approximate.RefractFromTrue(alt, env) }},
		{"Airmass", func() { _, _ = Airmass(alt) }},
		{"AtAltitude", func() { _ = AtAltitude(2635) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := testing.AllocsPerRun(1000, tc.f); got != 0 {
				t.Errorf("%s allocates %v times per call, want 0.\n"+
					"  A hemisphere walk evaluates this once per direction, so an "+
					"allocation here is an allocation per pixel.", tc.name, got)
			}
		})
	}
}
