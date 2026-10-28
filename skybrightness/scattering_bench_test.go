package skybrightness_test

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/skybrightness"
)

// BenchmarkScatteredIn is Eq. 11 for one direction, over two incoming fields.
//
// The integral costs one evaluation of the field per source direction, so what
// it costs a caller depends on theirs. "flat sky" is the cheapest field there
// is, which isolates the integral itself and bounds what any change to it can
// win. "natural-sky model" is the field a model recovers for itself with
// [skybrightness.Model.AboveAtmosphere], where evaluating the components
// dominates and the integral is the smaller share.
func BenchmarkScatteredIn(b *testing.B) {
	in := presetInputs(b)
	scene := presetGoldenScene(b, skybrightness.NaturalSky)

	model, err := skybrightness.NewPreset(skybrightness.NaturalSky, in)
	if err != nil {
		b.Fatalf("NewPreset: %v", err)
	}

	modelSky, err := model.AboveAtmosphere(skybrightness.Query{Scene: scene, Grid: in.Grid})
	if err != nil {
		b.Fatalf("AboveAtmosphere: %v", err)
	}

	view := coord.NewAltAz(angle.Deg(45), angle.Deg(120))

	for _, tc := range []struct {
		name  string
		above skybrightness.SkyRadiance
	}{
		{"flat sky", band(0, 90, 1e-9)},
		{"natural-sky model", modelSky},
	} {
		b.Run(tc.name, func(b *testing.B) {
			dst := skybrightness.NewSpectralRadiance(in.Grid)

			b.ReportAllocs()

			for b.Loop() {
				if err := skybrightness.ScatteredIn(
					b.Context(), dst, tc.above, scene, view, in.Grid, 0,
				); err != nil {
					b.Fatalf("ScatteredIn: %v", err)
				}
			}
		})
	}
}
