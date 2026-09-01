package skybrightness

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// kernelScene is the scene the equivalence below is measured against.
//
// Internal to this file rather than shared with the external tests, because
// scatterKernel is unexported and this has to be an in-package test.
func kernelScene(t *testing.T) *Scene {
	t.Helper()

	loc, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635) // Paranal
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	return &Scene{
		Observer:   loc,
		Time:       time.GoDate(2026, 8, 14, 3, 0, 0, 0, time.LocationUTC),
		Atmosphere: atmosphere.StandardDefault(2635),
	}
}

// TestScatterKernelMatchesTheReferenceFunctions is the test scatterKernel's
// doc comment has cited since the type was written.
//
// # Why it is worth having
//
// scatterKernel is a rearrangement of atmosphere.SingleScatteredRadiance and
// atmosphere.CombinedPhaseFunction, hoisting everything that does not vary
// around a ring of the hemispheric quadrature out of the innermost loop. The
// doc comment states plainly that this "is a rearrangement, not an
// approximation, and it is not allowed to drift from the functions it
// rearranges", and names this test as what holds it to that.
//
// The test did not exist. The rearranged form — the innermost loop of the
// whole sky-brightness model, and the only place in it where a transcendental
// was traded for a table lookup — had no test of any kind, while its comment
// said otherwise. That is worse than an untested optimisation, because a
// reader checking whether the claim is guarded finds a name and stops.
//
// # What equivalence means here
//
// Exactly, not approximately. Both forms compute
//
//	L = source * phase * (tau_sca/tau_ext) * M_v * P(tau, M_s, M_v)
//
// from the same inputs in a different order, so they may differ only by
// floating-point reassociation. The tolerance is relative and set at 1e-12 —
// a few ulp of headroom over the handful of operations reordered, and many
// orders below any physical effect. An approximation would blow through it.
func TestScatterKernelMatchesTheReferenceFunctions(t *testing.T) {
	t.Parallel()

	scene := kernelScene(t)
	grid := DefaultOpticalGrid()

	k, err := newScatterKernel(scene, grid)
	if err != nil {
		t.Fatalf("newScatterKernel: %v", err)
	}

	aerosol := scene.Atmosphere.Aerosol()
	pressure, _ := scene.Atmosphere.Surface()

	// A spread of geometries and optical depths, as the doc comment promises:
	// both airmasses near the zenith, both near the horizon, the source above
	// the view and below it, and the degenerate case where they coincide —
	// which is the branch the path integral special-cases.
	geometries := []struct {
		name                       string
		airmassSource, airmassView float64
		thetaDeg                   float64
	}{
		{"both near zenith", 1.0, 1.0, 30},
		{"source below view", 1.2, 3.5, 90},
		{"view below source", 4.0, 1.1, 150},
		{"both near horizon", 5.5, 6.0, 5},
		{"airmasses coincide", 2.5, 2.5, 60},
		{"forward scattering", 1.5, 2.0, 0.5},
		{"back scattering", 1.5, 2.0, 179.5},
	}

	const (
		dOmega       = 0.017 // a plausible quadrature cell, sr
		relTolerance = 1e-12
	)

	for _, g := range geometries {
		t.Run(g.name, func(t *testing.T) {
			t.Parallel()

			// Per subtest, not hoisted: these run in parallel, and sharing
			// the scratch slices across them is a data race. Hoisting them
			// was the first version of this test, and it failed by reporting
			// a zero where another subtest had just cleared the buffer —
			// which reads exactly like the kernel drifting.
			path := make([]float64, grid.Len())
			dst := make([]float64, grid.Len())

			theta := g.thetaDeg * math.Pi / 180

			phaseRayleigh, phaseAerosol, perr := k.phaseAt(theta)
			if perr != nil {
				t.Fatalf("phaseAt: %v", perr)
			}

			k.pathFactor(path, g.airmassSource, g.airmassView)

			// A source spectrum that is neither flat nor zero, so a
			// wavelength-indexing slip cannot pass by symmetry.
			source := make([]float64, grid.Len())
			for i := range source {
				source[i] = 1e-9 * (1 + float64(i%7))
			}

			k.accumulate(dst, source, path, dOmega, phaseRayleigh, phaseAerosol)

			for i := range dst {
				lambda := grid.At(i)

				rayleigh, rerr := atmosphere.RayleighOpticalDepth(lambda, float64(pressure))
				if rerr != nil {
					t.Fatalf("RayleighOpticalDepth: %v", rerr)
				}

				aer := unit.OpticalDepth(aerosol.TauAt(lambda))
				ext := rayleigh + aer
				sca := rayleigh + unit.OpticalDepth(
					float64(aer)*float64(aerosol.SingleScatteringAlbedo))

				// The reference phase function, combined by the same weights
				// the kernel splits apart.
				phase, perr := atmosphere.CombinedPhaseFunction(
					theta, rayleigh, aer, k.asymmetry, k.depolarisation)
				if perr != nil {
					t.Fatalf("CombinedPhaseFunction: %v", perr)
				}

				want, werr := atmosphere.SingleScatteredRadiance(
					source[i], phase, sca, ext, g.airmassSource, g.airmassView)
				if werr != nil {
					t.Fatalf("SingleScatteredRadiance: %v", werr)
				}

				want *= dOmega

				if rel := relativeDiff(dst[i], want); rel > relTolerance {
					t.Fatalf("at %v nm the kernel gives %.17g and the reference functions %.17g, "+
						"a relative %.3e — the rearrangement has drifted from what it rearranges",
						lambda, dst[i], want, rel)
				}
			}
		})
	}
}

// TestScatterKernelSkipsZeroSourceWithoutChangingTheAnswer covers the one
// shortcut accumulate takes that is not pure arithmetic: it skips a
// wavelength whose source radiance is zero.
//
// That is only sound if the contribution would have been zero anyway, which
// it is — every remaining factor is finite. Worth a test because the branch
// exists for speed and a future edit could make it skip something that
// matters.
func TestScatterKernelSkipsZeroSourceWithoutChangingTheAnswer(t *testing.T) {
	t.Parallel()

	scene := kernelScene(t)
	grid := DefaultOpticalGrid()

	k, err := newScatterKernel(scene, grid)
	if err != nil {
		t.Fatalf("newScatterKernel: %v", err)
	}

	phaseRayleigh, phaseAerosol, err := k.phaseAt(math.Pi / 3)
	if err != nil {
		t.Fatalf("phaseAt: %v", err)
	}

	path := make([]float64, grid.Len())
	k.pathFactor(path, 1.4, 2.2)

	// Every other wavelength carries no light.
	sparse := make([]float64, grid.Len())
	for i := range sparse {
		if i%2 == 0 {
			sparse[i] = 3e-9
		}
	}

	got := make([]float64, grid.Len())
	k.accumulate(got, sparse, path, 0.02, phaseRayleigh, phaseAerosol)

	for i := range got {
		if i%2 == 1 && got[i] != 0 {
			t.Errorf("wavelength %d had no source but accumulated %g", i, got[i])
		}

		if i%2 == 0 && got[i] == 0 {
			t.Errorf("wavelength %d had a source and accumulated nothing", i)
		}
	}
}

// relativeDiff compares two values that should agree to within reassociation,
// falling back to an absolute comparison when both are effectively zero.
func relativeDiff(got, want float64) float64 {
	if want == 0 {
		return math.Abs(got)
	}

	return math.Abs(got-want) / math.Abs(want)
}
