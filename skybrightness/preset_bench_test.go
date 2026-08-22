package skybrightness_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/skybrightness"
)

// What a whole-sky map costs, per preset.
//
// The number that matters to a caller is not nanoseconds per operation but
// how long a sky takes, and the answer differs by three orders of magnitude
// across the four presets. [skybrightness.Reference] fidelity runs the Eq. 11
// hemispheric integral once per component per direction, and that integral is
// itself about nine hundred evaluations of the incoming field, so a reference
// map is roughly a thousand times a standard one.
//
// SkyMap's ring count sets the resolution: the direction count is close to
// 4*rings^2*(2/pi), so rings = 90 is about twenty thousand directions, near one
// per square degree. Benchmarked at a low count and scaled, because a reference
// map at full resolution is exactly the thing being measured and running one
// per benchmark iteration is not affordable.
func BenchmarkPresetSkyMap(b *testing.B) {
	for _, p := range []skybrightness.Preset{
		skybrightness.GAMBONSWeb,
		skybrightness.NaturalSky,
		skybrightness.GAMBONSFull,
		skybrightness.Observatory,
	} {
		// Reference-fidelity presets are benchmarked over fewer rings, since
		// each direction is a thousand times dearer.
		// The same resolution for every preset, so the comparison is like for
		// like. It also has to be enough directions that the reference
		// presets' one-time hemisphere sampling is amortised the way a real
		// map amortises it: at three rings it is a tenth of the run and the
		// per-direction figure is meaningless.
		const rings = 12

		b.Run(string(p), func(b *testing.B) {
			in := benchPresetInputs(b, p)

			model, err := skybrightness.NewPreset(p, in)
			if err != nil {
				b.Fatalf("NewPreset: %v", err)
			}

			fidelity, err := p.Fidelity()
			if err != nil {
				b.Fatalf("Fidelity: %v", err)
			}

			scene := presetGoldenScene(b, p)

			q := skybrightness.Query{Scene: scene, Grid: in.Grid, Fidelity: fidelity}

			// One run outside the timer, both to warm any per-scene cache and
			// to learn the direction count for the report below.
			points, err := model.SkyMap(b.Context(), q, rings)
			if err != nil {
				b.Fatalf("SkyMap: %v", err)
			}

			b.ReportMetric(float64(len(points)), "directions")
			b.ResetTimer()

			for b.Loop() {
				if _, err := model.SkyMap(b.Context(), q, rings); err != nil {
					b.Fatalf("SkyMap: %v", err)
				}
			}

			b.StopTimer()

			// Seconds for a one-degree sky, scaled by direction count. The
			// per-direction cost is flat in the ring count — every direction
			// does the same work — so this scales linearly and is the figure
			// worth reporting.
			perDirection := float64(b.Elapsed().Nanoseconds()) /
				float64(b.N) / float64(len(points))
			full := perDirection * fullSkyDirections / 1e9

			b.ReportMetric(perDirection/1e6, "ms/direction")
			b.ReportMetric(full, "s/whole-sky")
		})
	}
}

// fullSkyDirections is how many samples SkyMap produces at one-degree
// resolution: rings = 90, and the direction count is close to
// 4*rings^2*(2/pi).
const fullSkyDirections = 4 * 90 * 90 * 2 / math.Pi

// benchPresetInputs builds inputs a preset will accept, adding the two the
// Observatory preset needs beyond the natural sky.
func benchPresetInputs(b *testing.B, p skybrightness.Preset) skybrightness.PresetInputs {
	b.Helper()

	if p == skybrightness.Observatory {
		return observatoryInputs(b)
	}

	return presetInputs(b)
}
