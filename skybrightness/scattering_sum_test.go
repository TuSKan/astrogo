package skybrightness

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/unit"
)

// scatteredInPerSample is ScatteredIn's integral as it was written before the
// sum over a ring was factored out: one accumulate call per sample, each
// applying the path factor and both phase weights.
//
// It is the definition the factored form is held to, kept here rather than in
// the package for the reason scatterIntoPerSample is: nothing but this
// comparison needs it. Validation is left to ScatteredIn, which the factored
// form shares.
func scatteredInPerSample(
	ctx context.Context, dst SpectralRadiance, above SkyRadiance, scene *Scene,
	view coord.AltAz, grid unit.SpectralGrid, rings int,
) error {
	if rings <= 0 {
		rings = DefaultScatteringRings
	}

	viewAirmass, err := atmosphere.Airmass(view.Alt())
	if err != nil {
		return fmt.Errorf("view airmass: %w", err)
	}

	kernel, err := newScatterKernel(scene, grid)
	if err != nil {
		return err
	}

	source := NewSpectralRadiance(grid)
	path := make([]float64, grid.Len())
	ring := -1

	for _, p := range hemisphereQuadrature(rings) {
		if p.ring != ring {
			sourceAirmass, err := atmosphere.Airmass(p.alt)
			if err != nil {
				return fmt.Errorf("source airmass: %w", err)
			}

			kernel.pathFactor(path, sourceAirmass, viewAirmass)

			ring = p.ring
		}

		dir := coord.NewAltAz(p.alt, p.az)

		clear(source)

		if err := above(ctx, source, dir); err != nil {
			return fmt.Errorf("incoming field: %w", err)
		}

		phaseRayleigh, phaseAerosol, err := kernel.phaseAt(separation(view, dir))
		if err != nil {
			return err
		}

		kernel.accumulate(dst, source, path, p.dOmega, phaseRayleigh, phaseAerosol)
	}

	return nil
}

// patternSky is a deterministic incoming field that varies with both direction
// and wavelength. With sparse set, every third wavelength is left dark, which
// is what a narrow-band source looks like to the sums.
func patternSky(sparse bool) SkyRadiance {
	return func(_ context.Context, dst SpectralRadiance, dir coord.AltAz) error {
		alt, az := dir.Alt().Radians(), dir.Az().Radians()

		for i := range dst {
			if sparse && i%3 == 0 {
				continue
			}

			dst[i] += 1e-9 * (1.5 + math.Sin(3*az+0.07*float64(i))*math.Cos(alt))
		}

		return nil
	}
}

// ScatteredIn evaluates the same integral over the same quadrature as before
// the ring sums, so its answer may differ from the per-sample form only by
// reassociation, whatever the scene, resolution, view or field.
func TestScatteredInMatchesPerSampleAccumulation(t *testing.T) {
	t.Parallel()

	grid := DefaultOpticalGrid()

	views := map[string]coord.AltAz{
		"zenith":       coord.NewAltAz(angle.Deg(90), angle.Deg(0)),
		"mid sky":      coord.NewAltAz(angle.Deg(45), angle.Deg(120)),
		"near horizon": coord.NewAltAz(angle.Deg(5), angle.Deg(300)),
	}

	for _, sc := range []struct {
		name  string
		scene *Scene
	}{
		{"air only", kernelScene(t)},
		{"air and aerosol", aerosolScene(t)},
	} {
		for _, rings := range []int{0, 1, 3, 8} {
			for _, sparse := range []bool{false, true} {
				for name, view := range views {
					t.Run(fmt.Sprintf("%s/rings=%d/sparse=%t/%s", sc.name, rings, sparse, name),
						func(t *testing.T) {
							t.Parallel()

							want := intoBuffers([]ComponentID{Starlight}, grid.Len())[Starlight]
							got := intoBuffers([]ComponentID{Starlight}, grid.Len())[Starlight]
							above := patternSky(sparse)

							if err := scatteredInPerSample(t.Context(), want, above, sc.scene, view, grid, rings); err != nil {
								t.Fatalf("per-sample form: %v", err)
							}

							if err := ScatteredIn(t.Context(), got, above, sc.scene, view, grid, rings); err != nil {
								t.Fatalf("ScatteredIn: %v", err)
							}

							for i := range want {
								if d := relativeDiff(got[i], want[i]); d > scatterIntoTolerance {
									t.Fatalf("wavelength %d: got %.17g, want %.17g (rel diff %.2e)",
										i, got[i], want[i], d)
								}
							}
						})
				}
			}
		}
	}
}

// The incoming field is the expensive part of the integral, so it is asked for
// once per quadrature sample, in order, exactly as before: grouping the sums by
// ring must not change how often or in what sequence the caller is called.
func TestScatteredInEvaluatesTheFieldOncePerSampleInOrder(t *testing.T) {
	t.Parallel()

	const rings = 5

	grid := DefaultOpticalGrid()
	points := hemisphereQuadrature(rings)

	var seen []coord.AltAz

	record := func(_ context.Context, dst SpectralRadiance, dir coord.AltAz) error {
		seen = append(seen, dir)

		for i := range dst {
			dst[i] += 1e-9
		}

		return nil
	}

	dst := NewSpectralRadiance(grid)
	if err := ScatteredIn(t.Context(), dst, record, aerosolScene(t),
		coord.NewAltAz(angle.Deg(45), angle.Deg(10)), grid, rings); err != nil {
		t.Fatalf("ScatteredIn: %v", err)
	}

	if len(seen) != len(points) {
		t.Fatalf("field evaluated %d times for %d samples", len(seen), len(points))
	}

	for i, p := range points {
		if want := coord.NewAltAz(p.alt, p.az); seen[i] != want {
			t.Fatalf("call %d was at %v, want %v", i, seen[i], want)
		}
	}
}

var errFieldFailed = errors.New("incoming field failed")

// A field that fails stops the integral and says so, wrapped in ErrScattering
// with the cause still reachable.
func TestScatteredInReportsAFailingField(t *testing.T) {
	t.Parallel()

	calls := 0

	failing := func(_ context.Context, _ SpectralRadiance, _ coord.AltAz) error {
		calls++
		if calls == 7 {
			return errFieldFailed
		}

		return nil
	}

	grid := DefaultOpticalGrid()
	dst := NewSpectralRadiance(grid)

	err := ScatteredIn(t.Context(), dst, failing, aerosolScene(t),
		coord.NewAltAz(angle.Deg(45), angle.Deg(10)), grid, 5)

	if !errors.Is(err, ErrScattering) || !errors.Is(err, errFieldFailed) {
		t.Fatalf("err = %v, want ErrScattering wrapping the field's own error", err)
	}

	if calls != 7 {
		t.Fatalf("field called %d times after failing on the 7th", calls)
	}
}
