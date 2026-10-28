package skybrightness

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/unit"
)

// scatterIntoPerSample is scatterInto as it was written before the sum over a
// ring was factored out: one accumulate call per sample per component, each
// applying the path factor and both phase weights.
//
// It is the definition the factored form is held to. Kept in the test file
// rather than the package because nothing but this comparison needs it, and a
// second implementation in the package would be a second thing to keep right.
func scatterIntoPerSample(
	f *hemisphereField, kernel *scatterKernel, view coord.AltAz, into map[ComponentID][]float64,
) error {
	if len(f.samples) == 0 {
		return nil
	}

	viewAirmass, err := atmosphere.Airmass(view.Alt())
	if err != nil {
		return fmt.Errorf("view airmass: %w", err)
	}

	width := f.grid.Len()
	path := make([]float64, width)
	ring := -1

	for _, sample := range f.samples {
		if sample.ring != ring {
			kernel.pathFactor(path, f.ringAirmass[sample.ring], viewAirmass)
			ring = sample.ring
		}

		phaseRayleigh, phaseAerosol, err := kernel.phaseAt(separation(view, sample.dir))
		if err != nil {
			return err
		}

		for c, id := range f.components {
			dst, ok := into[id]
			if !ok {
				continue
			}

			kernel.accumulate(dst, sample.spectrumOf(c, width), path,
				sample.dOmega, phaseRayleigh, phaseAerosol)
		}
	}

	return nil
}

// syntheticField builds a hemisphereField the way sampleHemisphere does, with
// structured radiance in place of evaluated components so the comparison needs
// no scene, no ephemeris and no datasets.
//
// Every zeroEvery-th value is exactly zero (none when zeroEvery is 0), because
// the sums must cope with the sparse spectra a narrow-band source produces.
func syntheticField(
	tb testing.TB, grid unit.SpectralGrid, rings int, ids []ComponentID, zeroEvery int,
) *hemisphereField {
	tb.Helper()

	r := rand.New(rand.NewPCG(7, 11))
	width := grid.Len()

	f := &hemisphereField{
		grid:        grid,
		rings:       rings,
		ringAirmass: make([]float64, rings),
		components:  ids,
	}

	for _, p := range hemisphereQuadrature(rings) {
		if f.ringAirmass[p.ring] == 0 {
			airmass, err := atmosphere.Airmass(p.alt)
			if err != nil {
				tb.Fatalf("Airmass(%v): %v", p.alt, err)
			}

			f.ringAirmass[p.ring] = airmass
		}

		sample := hemisphereSample{
			dir:      coord.NewAltAz(p.alt, p.az),
			ring:     p.ring,
			dOmega:   p.dOmega,
			radiance: make([]float64, len(ids)*width),
		}

		for i := range sample.radiance {
			if zeroEvery > 0 && i%zeroEvery == 0 {
				continue
			}

			sample.radiance[i] = 1e-9 * (0.1 + r.Float64())
		}

		f.samples = append(f.samples, sample)
	}

	return f
}

// airOnlyKernel is the kernel under kernelScene's atmosphere, which scatters off
// air alone: every aerosol weight is zero.
func airOnlyKernel(tb testing.TB) *scatterKernel {
	tb.Helper()

	kernel, err := newScatterKernel(kernelScene(tb), DefaultOpticalGrid())
	if err != nil {
		tb.Fatalf("newScatterKernel: %v", err)
	}

	return kernel
}

// aerosolScene is kernelScene's observer and time under an atmosphere that
// scatters off aerosol as well as air.
//
// It exists because kernelScene's atmosphere has no aerosol, which leaves the
// aerosol weight at zero at every wavelength and the whole aerosol sum
// unexercised: a mutation that dropped its clearing, or fed it the wrong phase
// function, passed this file's first version.
func aerosolScene(tb testing.TB) *Scene {
	tb.Helper()

	atm, err := atmosphere.NewBuilder().
		Surface(743, 284).
		Aerosol(0.15, 550, 1.3, 0.9, 0.7).
		AerosolScaleHeight(1500).
		Build()
	if err != nil {
		tb.Fatalf("atmosphere Build: %v", err)
	}

	scene := kernelScene(tb)
	scene.Atmosphere = atm

	return scene
}

// aerosolKernel is the kernel for aerosolScene. The check below keeps a future
// change to the scene from quietly putting the zero-aerosol gap back.
func aerosolKernel(tb testing.TB) *scatterKernel {
	tb.Helper()

	kernel, err := newScatterKernel(aerosolScene(tb), DefaultOpticalGrid())
	if err != nil {
		tb.Fatalf("newScatterKernel: %v", err)
	}

	for i, w := range kernel.aerosolWeight {
		if w < 0.01 {
			tb.Fatalf("aerosol weight at wavelength %d is %g: the aerosol sum would go untested", i, w)
		}
	}

	return kernel
}

// intoBuffers returns one destination per id, each starting from the same
// nonzero values, because the sum must accumulate into what is already there
// rather than overwrite it.
func intoBuffers(ids []ComponentID, width int) map[ComponentID][]float64 {
	into := make(map[ComponentID][]float64, len(ids))

	for k, id := range ids {
		buf := make([]float64, width)
		for i := range buf {
			buf[i] = 1e-10 * float64(1+(i+k)%7)
		}

		into[id] = buf
	}

	return into
}

// scatterIntoTolerance is the bound the kernel's own equivalence test uses: a
// few ulp of headroom over the operations reordered, and many orders below any
// physical effect. A wrong weight, a missed ring or a stale sum is not
// rounding and would pass straight through it.
const scatterIntoTolerance = 1e-12

// assertScatterIntoMatches runs both forms from identical starting buffers and
// requires every component to agree. The omitted component is deliberately
// absent from into, so a form that ignored that and wrote to it would index a
// nil slice and panic rather than quietly pass.
func assertScatterIntoMatches(
	t *testing.T, f *hemisphereField, kernel *scatterKernel, view coord.AltAz, ids []ComponentID,
) {
	t.Helper()

	want := intoBuffers(ids[:len(ids)-1], f.grid.Len())
	got := intoBuffers(ids[:len(ids)-1], f.grid.Len())

	if err := scatterIntoPerSample(f, kernel, view, want); err != nil {
		t.Fatalf("per-sample form: %v", err)
	}

	if err := f.scatterInto(nil, kernel, view, got); err != nil {
		t.Fatalf("scatterInto: %v", err)
	}

	for id, w := range want {
		for i := range w {
			if d := relativeDiff(got[id][i], w[i]); d > scatterIntoTolerance {
				t.Fatalf("%s wavelength %d: got %.17g, want %.17g (rel diff %.2e)",
					id, i, got[id][i], w[i], d)
			}
		}
	}
}

var scatterIDs = []ComponentID{Starlight, Zodiacal, DiffuseGalactic, AirglowContinuum, Twilight}

// The ring-factored sum must be the per-sample sum, reassociated and nothing
// else, over the views a sky map actually asks for: the zenith, the horizon,
// somewhere between, and a view pointing straight at a sample — the forward
// peak of both phase functions, where a wrong weight shows most.
func TestScatterIntoMatchesPerSampleAccumulation(t *testing.T) {
	t.Parallel()

	grid := DefaultOpticalGrid()

	for _, kc := range []struct {
		name   string
		kernel *scatterKernel
	}{
		{"air only", airOnlyKernel(t)},
		{"air and aerosol", aerosolKernel(t)},
	} {
		for _, tc := range []struct {
			name      string
			rings     int
			zeroEvery int
		}{
			{"default rings, dense", DefaultScatteringRings, 0},
			{"default rings, sparse spectra", DefaultScatteringRings, 3},
			{"one ring", 1, 0},
			{"coarse rings", 3, 0},
		} {
			t.Run(kc.name+"/"+tc.name, func(t *testing.T) {
				t.Parallel()

				f := syntheticField(t, grid, tc.rings, scatterIDs, tc.zeroEvery)

				views := map[string]coord.AltAz{
					"zenith":          coord.NewAltAz(angle.Deg(90), angle.Deg(0)),
					"mid sky":         coord.NewAltAz(angle.Deg(45), angle.Deg(120)),
					"near horizon":    coord.NewAltAz(angle.Deg(5), angle.Deg(300)),
					"toward a sample": f.samples[len(f.samples)/2].dir,
				}

				for name, view := range views {
					t.Run(name, func(t *testing.T) {
						assertScatterIntoMatches(t, f, kc.kernel, view, scatterIDs)
					})
				}
			})
		}
	}
}

// Samples arrive ring-major from sampleHemisphere, but nothing in the form
// should depend on it. Shuffling puts samples of different rings next to each
// other and splits every ring into runs of one, which is the harshest test of
// where a ring's sum is closed and its path factor swapped.
func TestScatterIntoDoesNotDependOnSampleOrder(t *testing.T) {
	t.Parallel()

	grid := DefaultOpticalGrid()
	kernel := aerosolKernel(t)

	f := syntheticField(t, grid, DefaultScatteringRings, scatterIDs, 0)

	r := rand.New(rand.NewPCG(3, 5))
	r.Shuffle(len(f.samples), func(i, j int) { f.samples[i], f.samples[j] = f.samples[j], f.samples[i] })

	assertScatterIntoMatches(t, f, kernel, coord.NewAltAz(angle.Deg(40), angle.Deg(75)), scatterIDs)
}

// A field with nothing sampled has nothing to scatter, and must leave the
// destinations exactly as it found them.
func TestScatterIntoEmptyFieldChangesNothing(t *testing.T) {
	t.Parallel()

	grid := DefaultOpticalGrid()
	kernel := aerosolKernel(t)

	f := &hemisphereField{grid: grid, components: scatterIDs}
	into := intoBuffers(scatterIDs, grid.Len())
	before := intoBuffers(scatterIDs, grid.Len())

	if err := f.scatterInto(nil, kernel, coord.NewAltAz(angle.Deg(45), angle.Deg(0)), into); err != nil {
		t.Fatalf("scatterInto: %v", err)
	}

	for id, b := range before {
		for i := range b {
			if into[id][i] != b[i] {
				t.Fatalf("%s wavelength %d changed: %g -> %g", id, i, b[i], into[id][i])
			}
		}
	}
}

// BenchmarkScatterInto is the hemispheric sum for one view direction over a
// default-resolution field, which is the unit a sky map repeats once per
// direction. Unlike the preset sky-map benchmark it is serial and free of
// scheduler and GC noise, so a change to this function shows up here alone.
func BenchmarkScatterInto(b *testing.B) {
	grid := DefaultOpticalGrid()
	kernel := aerosolKernel(b)

	ids := []ComponentID{Starlight, Zodiacal, DiffuseGalactic, AirglowContinuum, AirglowLines, Twilight}
	f := syntheticField(b, grid, DefaultScatteringRings, ids, 0)
	into := intoBuffers(ids, grid.Len())
	view := coord.NewAltAz(angle.Deg(45), angle.Deg(120))

	b.ReportAllocs()

	for b.Loop() {
		if err := f.scatterInto(nil, kernel, view, into); err != nil {
			b.Fatalf("scatterInto: %v", err)
		}
	}
}
