package skybrightness_test

import (
	"context"
	"math"
	"sync"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/skybrightness"
)

// Every component's doc comment says a value is safe for concurrent use, and
// two of them hold a sync.Pool of scratch buffers to make the atmosphere
// scaling allocation-free. A pool used carelessly hands the same buffer to two
// goroutines, and the result is not a crash — it is two answers that are each
// plausible and one of them wrong.
//
// Run this under -race, which CI does.
func TestComponentsAreConcurrencySafe(t *testing.T) {
	t.Parallel()

	grid := skybrightness.DefaultOpticalGrid()
	scene := auditScene(t)

	// A spread of directions, so goroutines are not all asking the same
	// question and a shared buffer would show as a wrong number rather than
	// coincidentally the right one.
	dirs := []coord.AltAz{
		coord.NewAltAz(angle.Deg(15), angle.Deg(0)),
		coord.NewAltAz(angle.Deg(35), angle.Deg(90)),
		coord.NewAltAz(angle.Deg(55), angle.Deg(180)),
		coord.NewAltAz(angle.Deg(75), angle.Deg(270)),
	}

	for name, c := range auditComponents(t, grid) {
		// The answer each direction must give, computed serially first.
		want := make([]skybrightness.SpectralRadiance, len(dirs))

		for i, d := range dirs {
			v, err := evaluate(t, c, scene, grid, d)
			if err != nil {
				t.Fatalf("%s serial: %v", name, err)
			}

			want[i] = v
		}

		var wg sync.WaitGroup

		bad := make(chan string, 64)

		for range 8 {
			for i, d := range dirs {
				wg.Add(1)

				go func(i int, d coord.AltAz) {
					defer wg.Done()

					dst := skybrightness.NewSpectralRadiance(grid)
					if _, err := c.AddRadiance(context.Background(), dst, grid, d, scene); err != nil {
						return
					}

					for k := range dst {
						if dst[k] != want[i][k] {
							select {
							case bad <- name:
							default:
							}

							return
						}
					}
				}(i, d)
			}
		}

		wg.Wait()
		close(bad)

		if n := len(bad); n > 0 {
			t.Errorf("%s gave a different answer under concurrency in %d of 32 evaluations", name, n)
		}
	}
}

// Radiance transport is linear in its source. Doubling the star map, the dust
// column or the airglow spectrum must double what the component contributes,
// exactly — no threshold, no saturation, no offset.
//
// This is the invariant that catches an amplitude used twice, a cap applied to
// the wrong quantity, or a term added where it should have been multiplied. It
// holds for the four components whose source this test can scale; moonlight and
// artificial skyglow take their source through a different path and zodiacal
// has no caller-supplied amplitude at all.
func TestComponentsAreLinearInTheirSource(t *testing.T) {
	t.Parallel()

	grid := skybrightness.DefaultOpticalGrid()
	scene := auditScene(t)
	dir := coord.NewAltAz(angle.Deg(60), angle.Deg(45))

	const factor = 3

	build := map[string]func(scale float64) (skybrightness.Component, error){
		"starlight": func(scale float64) (skybrightness.Component, error) {
			return skybrightness.NewIntegratedStarlight(
				uniformSky{value: 8.05e-10 * scale, galactic: true},
				solarShape(grid), grid, testBand())
		},
		"airglow": func(scale float64) (skybrightness.Component, error) {
			zenith := skybrightness.NewSpectralRadiance(grid)
			for i := range zenith {
				zenith[i] = 1.5e-9 * scale
			}

			return skybrightness.NewAirglow(zenith, grid, 87_000, false)
		},
	}

	for name, make := range build {
		one, err := make(1)
		if err != nil {
			t.Fatalf("%s at scale 1: %v", name, err)
		}

		many, err := make(factor)
		if err != nil {
			t.Fatalf("%s at scale %d: %v", name, factor, err)
		}

		single, err := evaluate(t, one, scene, grid, dir)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		scaled, err := evaluate(t, many, scene, grid, dir)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		for i := range single {
			want := single[i] * factor
			if math.Abs(scaled[i]-want) > 1e-24+1e-12*math.Abs(want) {
				t.Errorf("%s is not linear at %.0f nm: %.6e at scale 1 gives %.6e at scale %d, want %.6e",
					name, float64(grid.At(i)), single[i], scaled[i], factor, want)

				break
			}
		}
	}
}

// A source outside the atmosphere gets dimmer as the path through it lengthens,
// with no exception between the zenith and the horizon.
//
// The extragalactic background is the clean case: it is isotropic, so the only
// thing that changes with direction is the airmass, and any departure from a
// monotonic decline is the transport rather than the sky. Integrated starlight
// over a uniform map is the same test with one more layer of machinery.
//
// This is the invariant that would have caught diffuse galactic light and
// zodiacal light being added unattenuated — they were flat in altitude where
// every other extra-atmospheric term fell.
func TestExtraAtmosphericComponentsDimTowardTheHorizon(t *testing.T) {
	t.Parallel()

	grid := skybrightness.DefaultOpticalGrid()
	scene := auditScene(t)

	isl, err := skybrightness.NewIntegratedStarlight(
		uniformSky{value: 8.05e-10, galactic: true}, solarShape(grid), grid, testBand())
	if err != nil {
		t.Fatalf("NewIntegratedStarlight: %v", err)
	}

	cases := map[string]skybrightness.Component{
		"extragalactic": skybrightness.NewExtragalacticBackground(),
		"starlight":     isl,
	}

	mid := grid.Len() / 2

	for name, c := range cases {
		var previous float64 = math.Inf(1)

		for _, alt := range []float64{90, 75, 60, 45, 30, 20, 10, 5, 2, 1} {
			dst, err := evaluate(t, c, scene, grid, coord.NewAltAz(angle.Deg(alt), angle.Deg(0)))
			if err != nil {
				t.Fatalf("%s at %.0f deg: %v", name, alt, err)
			}

			got := dst[mid]
			if got <= 0 {
				t.Fatalf("%s contributed nothing at %.0f degrees above the horizon", name, alt)
			}

			if got >= previous {
				t.Errorf("%s did not dim from the previous altitude down to %.0f degrees: "+
					"%.6e against %.6e — an extra-atmospheric source must lose light to a longer path",
					name, alt, got, previous)
			}

			previous = got
		}
	}
}

// The whole model must equal the sum of its parts, in linear radiance.
//
// Magnitudes never add, and a total assembled by averaging or by any other
// non-linear step would be wrong in a way no single component test could see.
func TestModelTotalIsTheLinearSumOfComponents(t *testing.T) {
	t.Parallel()

	grid := skybrightness.DefaultOpticalGrid()
	scene := auditScene(t)
	dir := coord.NewAltAz(angle.Deg(55), angle.Deg(210))

	components := auditComponents(t, grid)

	names := make([]string, 0, len(components))
	for n := range components {
		names = append(names, n)
	}

	list := make([]skybrightness.Component, 0, len(components))
	for _, n := range names {
		list = append(list, components[n])
	}

	model, err := skybrightness.NewModel("sum-check", list...)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	est, err := model.Estimate(context.Background(),
		skybrightness.Query{Scene: scene, Direction: dir, Grid: grid})
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}

	sum := make([]float64, grid.Len())

	for _, id := range est.ComponentIDs() {
		part, ok := est.Component(id)
		if !ok {
			continue
		}

		for i, v := range part {
			sum[i] += v
		}
	}

	total := est.SpectralRadiance()
	for i := range total {
		if math.Abs(total[i]-sum[i]) > 1e-24+1e-12*math.Abs(sum[i]) {
			t.Fatalf("the total is not the sum of the parts at %.0f nm: %.9e against %.9e",
				float64(grid.At(i)), total[i], sum[i])
		}
	}

}
