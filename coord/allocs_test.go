package coord_test

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// Allocation contracts for coord's hot path.
//
// # Why these are tests and not benchmarks
//
// This repository already runs every benchmark on every push to main and
// uploads the numbers as an artifact. That found nothing: a number in an
// artifact cannot be wrong. #246 was a defect that made an EOP lookup
// allocate 15.6 MB, and CI recorded 149 GB/op for the scheduler benchmark on
// every merge for months without failing (#248).
//
// A benchmark-diffing gate would not have caught it either, because the
// defect was present at the baseline — every run agreed with the one before
// it. What catches that class is an *absolute* claim about a specific path.
//
// # Why allocations rather than time
//
// ns/op on a shared CI runner is noisy, and a noisy gate gets muted. allocs/op
// is deterministic: the figures below are identical on Windows and Linux and,
// checked rather than assumed, unchanged under -race — so these need no build
// tag and no short-mode skip.
//
// # Why zero, specifically
//
// coord.Context exists so that the expensive per-epoch setup happens once and
// each subsequent transform is cheap. "Cheap" here means it touches no heap at
// all, which is what lets a scheduler run one Context over thousands of
// targets without giving the collector anything to do. If a transform starts
// allocating, that property is gone whether or not the wall clock notices.
//
// None of these use t.Parallel: testing.AllocsPerRun measures the whole
// process, so a sibling test allocating concurrently would be counted here.

// TestCachedContextTransformsDoNotAllocate pins the zero-allocation claim on
// the transforms a hot loop repeats.
func TestCachedContextTransformsDoNotAllocate(t *testing.T) {
	loc, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	atm := atmosphere.AtAltitude(2635)
	epoch := time.FromJD(2460000.5, time.UTC)
	ctx := coord.NewContext(epoch, loc, atm)

	pos := coord.NewICRS(angle.Hour(5.5), angle.Deg(-5.4))
	vec := vector.Vec3{X: 0.5, Y: 0.5, Z: 0.7071}

	for _, tc := range []struct {
		name string
		f    func()
	}{
		{"ICRSToAltAz", func() { _, _ = ctx.ICRSToAltAz(pos) }},
		{"GeocentricToObserved", func() { _ = ctx.GeocentricToObserved(vec) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := testing.AllocsPerRun(1000, tc.f); got != 0 {
				t.Errorf("%s allocates %v times per call, want 0.\n"+
					"  A cached Context transform touching the heap costs a scheduler "+
					"one allocation per target per time step, which is the reason the "+
					"Context cache exists.", tc.name, got)
			}
		})
	}
}

// TestBatchTransformDoesNotAllocate covers the batch API's whole reason for
// existing: the caller supplies the output slice, so the transform itself has
// nothing to allocate however many targets it is given.
func TestBatchTransformDoesNotAllocate(t *testing.T) {
	loc, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	ctx := coord.NewContext(time.FromJD(2460000.5, time.UTC), loc, atmosphere.AtAltitude(2635))

	const n = 512

	stars := make([]coord.ICRS, n)
	for i := range stars {
		stars[i] = coord.NewICRS(angle.Hour(float64(i)*24.0/n), angle.Deg(float64(i%80)-40))
	}

	out := make([]coord.AltAz, n)

	if got := testing.AllocsPerRun(100, func() { ctx.ICRSBatchToAltAz(stars, out) }); got != 0 {
		t.Errorf("ICRSBatchToAltAz allocates %v times per call over %d targets, want 0; "+
			"the caller already supplied the destination", got, n)
	}
}

// TestNewContextAllocationIsBounded states a bound rather than zero, because
// zero would be a false claim: Context holds the SOFA astrometry parameters
// and one allocation is what materialises them.
//
// The bound is what matters. NewContext is the expensive call — measured
// ~137 us against ~293 ns for a cached transform — and the risk is not that it
// costs one allocation but that it quietly starts costing many, which is how
// a per-epoch cost turns into a per-target one.
func TestNewContextAllocationIsBounded(t *testing.T) {
	loc, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	atm := atmosphere.AtAltitude(2635)
	epoch := time.FromJD(2460000.5, time.UTC)

	// Measured: 1. The headroom is for an implementation that legitimately
	// splits that one structure, not for a regression — 8 is still far below
	// anything that would make per-epoch setup look like per-target work.
	const maxAllocs = 8

	if got := testing.AllocsPerRun(200, func() { _ = coord.NewContext(epoch, loc, atm) }); got > maxAllocs {
		t.Errorf("NewContext allocates %v times per call, want at most %d (measured 1)",
			got, maxAllocs)
	}
}
