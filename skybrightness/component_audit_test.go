package skybrightness_test

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// auditScene is an ordinary dark-site scene the whole battery runs against.
func auditScene(t *testing.T) *skybrightness.Scene {
	t.Helper()

	loc, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	atm, err := atmosphere.NewBuilder().
		Surface(743, 284).
		Aerosol(0.02, 550, 1.3, 0.95, 0.65).
		AerosolScaleHeight(1500).
		Build()
	if err != nil {
		t.Fatalf("atmosphere Build: %v", err)
	}

	return &skybrightness.Scene{
		Observer:   loc,
		Time:       time.GoDate(2026, 3, 20, 5, 0, 0, 0, time.LocationUTC),
		Atmosphere: atm,
		Ephemeris:  eph.Default(),
	}
}

// auditComponents builds every Component this module has, over offline
// fixtures, so the battery below can run in CI without a network.
func auditComponents(t *testing.T, grid unit.SpectralGrid) map[string]skybrightness.Component {
	t.Helper()

	out := map[string]skybrightness.Component{}

	moon, err := skybrightness.NewScatteredMoonlight(solarSpectrumFixture(t))
	if err != nil {
		t.Fatalf("NewScatteredMoonlight: %v", err)
	}

	out["moonlight"] = moon

	sky := uniformSky{value: 8.05e-10, galactic: true}

	dgl, err := skybrightness.NewDiffuseGalacticLight(uniformDust(3.0), sky, testBand())
	if err != nil {
		t.Fatalf("NewDiffuseGalacticLight: %v", err)
	}

	out["diffuse-galactic"] = dgl

	zenith := skybrightness.NewSpectralRadiance(grid)
	for i := range zenith {
		zenith[i] = 1.5e-9
	}

	glow, err := skybrightness.NewAirglow(zenith, grid, 87_000, false)
	if err != nil {
		t.Fatalf("NewAirglow: %v", err)
	}

	out["airglow"] = glow

	stars, err := skybrightness.NewIntegratedStarlight(sky, solarShape(grid), grid, testBand())
	if err != nil {
		t.Fatalf("NewIntegratedStarlight: %v", err)
	}

	out["starlight"] = stars
	out["zodiacal"] = skybrightness.NewZodiacalLight()
	out["extragalactic"] = skybrightness.NewExtragalacticBackground()

	city, err := coord.NewGeodetic(angle.Deg(-70.1), angle.Deg(-24.4), 2000)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	artificial, err := skybrightness.NewArtificialSkyglow([]skybrightness.GroundEmitter{
		&skybrightness.UniformEmitter{
			At:           city,
			Name:         "audit",
			WavelengthNM: []unit.WavelengthNM{300, 600, 1100},
			Radiance:     []float64{1e-5, 1e-5, 1e-5},
			Emission:     skybrightness.UpwardEmission{Cosine: 1, HorizontalFraction: 0.3},
		},
	})
	if err != nil {
		t.Fatalf("NewArtificialSkyglow: %v", err)
	}

	out["artificial"] = artificial

	return out
}

// evaluate runs one component in one direction and returns what it wrote.
func evaluate(
	t *testing.T,
	c skybrightness.Component,
	scene *skybrightness.Scene,
	grid unit.SpectralGrid,
	dir coord.AltAz,
) (skybrightness.SpectralRadiance, error) {
	t.Helper()

	dst := skybrightness.NewSpectralRadiance(grid)

	if _, err := c.AddRadiance(context.Background(), dst, grid, dir, scene); err != nil {
		return dst, fmt.Errorf("audit: AddRadiance: %w", err)
	}

	return dst, nil
}

// Nothing below the horizon may contribute. The ground is not the sky.
//
// This is the audit that found airglow returning light from underground.
// atmosphere.VanRhijn is a function of sin(z), and sin is symmetric about 90
// degrees, so a direction ten degrees below the horizon returned exactly what
// ten degrees above it would — a mirrored sky, positive and plausible, in a
// direction where the answer is zero. Every other component guards the horizon;
// that one did not, and nothing else in the module compares them.
func TestNoComponentEmitsBelowTheHorizon(t *testing.T) {
	t.Parallel()

	grid := skybrightness.DefaultOpticalGrid()
	scene := auditScene(t)

	for name, c := range auditComponents(t, grid) {
		for _, alt := range []float64{-90, -45, -10, -1, -0.001} {
			dir := coord.NewAltAz(angle.Deg(alt), angle.Deg(137))

			dst, err := evaluate(t, c, scene, grid, dir)
			if err != nil {
				t.Errorf("%s at %.3f deg: %v", name, alt, err)

				continue
			}

			for i, v := range dst {
				if v != 0 {
					t.Errorf("%s wrote %.4e at %.0f nm looking %.3f degrees below the horizon",
						name, v, float64(grid.At(i)), alt)

					break
				}
			}
		}
	}
}

// Every component must return finite, non-negative radiance everywhere above
// the horizon, including the awkward places: a hair above it, the zenith
// exactly, and either side of the azimuth wrap.
func TestComponentsStayFiniteAndPositive(t *testing.T) {
	t.Parallel()

	grid := skybrightness.DefaultOpticalGrid()
	scene := auditScene(t)

	altitudes := []float64{0.001, 0.01, 0.1, 1, 5, 30, 60, 89, 89.999, 90}
	azimuths := []float64{0, 0.001, 90, 179.999, 180, 270, 359.999}

	for name, c := range auditComponents(t, grid) {
		for _, alt := range altitudes {
			for _, az := range azimuths {
				dir := coord.NewAltAz(angle.Deg(alt), angle.Deg(az))

				dst, err := evaluate(t, c, scene, grid, dir)
				if err != nil {
					// An error is an acceptable answer; a wrong number is not.
					continue
				}

				for i, v := range dst {
					switch {
					case math.IsNaN(v):
						t.Fatalf("%s produced NaN at %.0f nm, alt %.3f az %.3f",
							name, float64(grid.At(i)), alt, az)
					case math.IsInf(v, 0):
						t.Fatalf("%s produced %v at %.0f nm, alt %.3f az %.3f",
							name, v, float64(grid.At(i)), alt, az)
					case v < 0:
						t.Fatalf("%s produced negative radiance %.4e at %.0f nm, alt %.3f az %.3f",
							name, v, float64(grid.At(i)), alt, az)
					}
				}
			}
		}
	}
}

// A component must accumulate into dst rather than overwrite it. The model sums
// components into one buffer, so a component that assigns silently erases every
// component evaluated before it — and the total would still look like a sky.
func TestComponentsAccumulateRatherThanOverwrite(t *testing.T) {
	t.Parallel()

	grid := skybrightness.DefaultOpticalGrid()
	scene := auditScene(t)
	dir := coord.NewAltAz(angle.Deg(50), angle.Deg(120))

	const seed = 1.234e-9

	for name, c := range auditComponents(t, grid) {
		clean := skybrightness.NewSpectralRadiance(grid)
		if _, err := c.AddRadiance(context.Background(), clean, grid, dir, scene); err != nil {
			continue
		}

		seeded := skybrightness.NewSpectralRadiance(grid)
		for i := range seeded {
			seeded[i] = seed
		}

		if _, err := c.AddRadiance(context.Background(), seeded, grid, dir, scene); err != nil {
			continue
		}

		for i := range seeded {
			want := clean[i] + seed
			if math.Abs(seeded[i]-want) > 1e-18+1e-9*math.Abs(want) {
				t.Errorf("%s overwrote rather than accumulated at %.0f nm: %.6e, want %.6e",
					name, float64(grid.At(i)), seeded[i], want)

				break
			}
		}
	}
}

// Evaluating twice must give the same answer. A component holding scratch state
// that it does not clear returns a different number the second time, and the
// difference is invisible unless something asks twice.
func TestComponentsAreRepeatable(t *testing.T) {
	t.Parallel()

	grid := skybrightness.DefaultOpticalGrid()
	scene := auditScene(t)
	dir := coord.NewAltAz(angle.Deg(35), angle.Deg(200))

	for name, c := range auditComponents(t, grid) {
		first, err := evaluate(t, c, scene, grid, dir)
		if err != nil {
			continue
		}

		for pass := 2; pass <= 4; pass++ {
			again, err := evaluate(t, c, scene, grid, dir)
			if err != nil {
				t.Errorf("%s failed on pass %d having succeeded once: %v", name, pass, err)

				break
			}

			for i := range first {
				if first[i] != again[i] {
					t.Errorf("%s is not repeatable: pass 1 gave %.6e at %.0f nm, pass %d gave %.6e",
						name, first[i], float64(grid.At(i)), pass, again[i])

					break
				}
			}
		}
	}
}

// A component handed a grid it was not built for must say so rather than
// silently reading the wrong wavelengths.
func TestComponentsRejectAMismatchedGrid(t *testing.T) {
	t.Parallel()

	built := skybrightness.DefaultOpticalGrid()
	scene := auditScene(t)
	dir := coord.NewAltAz(angle.Deg(45), angle.Deg(90))

	other, err := unit.NewSpectralGrid(400, 2, 100)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	for name, c := range auditComponents(t, built) {
		dst := skybrightness.NewSpectralRadiance(other)

		_, err := c.AddRadiance(context.Background(), dst, other, dir, scene)
		if err != nil {
			continue // refused, which is correct
		}

		// Accepting is only acceptable if the component is grid-agnostic, which
		// means it must have written something sensible on the new grid.
		for i, v := range dst {
			if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
				t.Errorf("%s accepted a foreign grid and wrote %v at %.0f nm",
					name, v, float64(other.At(i)))

				break
			}
		}
	}
}

// A destination longer than the grid must be refused, not written past.
//
// unit.SpectralGrid.At does no bounds checking - it returns
// StartNM + i*StepNM for any i - so a component that loops over dst and trusts
// the index evaluates its physics at wavelengths outside the grid it was handed
// and writes the answers into slots the grid does not describe. Nothing panics
// and nothing is obviously wrong afterwards.
//
// Model.Estimate always allocates dst from the grid, so this cannot happen from
// inside. Component is a public interface, and three of the components already
// defend against it, which is the argument that all of them should.
func TestComponentsRejectAnOversizedDestination(t *testing.T) {
	t.Parallel()

	grid := skybrightness.DefaultOpticalGrid()
	scene := auditScene(t)
	dir := coord.NewAltAz(angle.Deg(45), angle.Deg(90))

	for name, c := range auditComponents(t, grid) {
		oversized := make(skybrightness.SpectralRadiance, grid.Len()+16)

		var (
			err      error
			panicked any
		)

		func() {
			defer func() { panicked = recover() }()

			_, err = c.AddRadiance(context.Background(), oversized, grid, dir, scene)
		}()

		switch {
		case panicked != nil:
			t.Errorf("%s panicked on a destination %d longer than the grid: %v",
				name, 16, panicked)
		case err == nil:
			// Accepted. The slots past the grid must at least be untouched,
			// since the grid says nothing about those wavelengths.
			for i := grid.Len(); i < len(oversized); i++ {
				if oversized[i] != 0 {
					t.Errorf("%s wrote %.4e into slot %d, which is %d past the end of the grid",
						name, oversized[i], i, i-grid.Len()+1)

					break
				}
			}
		}
	}
}
