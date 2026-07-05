package skybrightness

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// benchContext builds a fixed coord.Context (J2000, equator, sea level) for the
// time/observer-dependent component benchmarks.
func benchContext(b *testing.B) *coord.Context {
	b.Helper()

	loc, err := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	if err != nil {
		b.Fatalf("NewGeodetic: %v", err)
	}

	return coord.NewContext(time.FromJD(2451545.0, time.UTC), loc, atmosphere.AtAltitude(0))
}

// (The allocation-free hot path is covered by BenchmarkCompositeSurfaceBrightness
// in model_test.go; the KS closed form by BenchmarkMoonBrightnessNL in
// moonlight_test.go. The benchmarks here exercise the ephemeris-backed path.)

// BenchmarkCompositeFull benchmarks the full model (floor + airglow + zodiacal +
// moonlight) for end-to-end timing. The moonlight and zodiacal components query
// the ephemeris, so this path is not allocation-free.
func BenchmarkCompositeFull(b *testing.B) {
	b.ReportAllocs()

	ctx := benchContext(b)
	model := NewCompositeModel(
		NewFloorSQM(20.0),
		NewAirglow(),
		NewZodiacalLight(nil),
		NewMoonlight(),
	)
	aa := coord.NewAltAz(angle.Deg(60), angle.Deg(120))

	var sink SurfaceBrightnessV

	for range b.N {
		sb, err := model.SurfaceBrightness(aa, ctx)
		if err != nil {
			b.Fatalf("SurfaceBrightness: %v", err)
		}

		sink = sb
	}

	_ = sink
}
