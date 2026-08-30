package skybrightness_test

import (
	"context"
	"testing"

	"github.com/TuSKan/astrogo/atmosphere"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/optics"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// These record the Phase 0 baseline: the cost of the framework itself,
// with trivial components, before any physics exists. Later phases measure
// their own cost against this, so a regression in the machinery is
// distinguishable from the expected cost of real models.
//
// The spec's performance work is Phase 8. Nothing here is optimised yet;
// the point is to have numbers before opinions.

func benchScene(b *testing.B) *skybrightness.Scene {
	b.Helper()

	loc, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		b.Fatalf("NewGeodetic: %v", err)
	}

	return &skybrightness.Scene{
		Observer:   loc,
		Time:       benchTime,
		Atmosphere: benchAtmosphere(2635),
	}
}

// benchModel builds a model with one component per physical contribution
// the Phase 0 framework distinguishes, so the baseline reflects a
// realistic component count rather than a single term.
func benchModel(b *testing.B) *skybrightness.Model {
	b.Helper()

	ids := []skybrightness.ComponentID{
		skybrightness.Starlight,
		skybrightness.Zodiacal,
		skybrightness.AirglowContinuum,
		skybrightness.Moonlight,
		skybrightness.Artificial,
	}

	comps := make([]skybrightness.Component, 0, len(ids))
	for _, id := range ids {
		comps = append(comps, constantComponent{id: id, value: 1e-9})
	}

	m, err := skybrightness.NewModel("bench", comps...)
	if err != nil {
		b.Fatalf("NewModel: %v", err)
	}

	return m
}

func BenchmarkEstimateSingleDirection(b *testing.B) {
	m := benchModel(b)
	q := skybrightness.Query{
		Scene:     benchScene(b),
		Direction: coord.NewAltAz(angle.Deg(60), angle.Deg(120)),
	}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if _, err := m.Estimate(context.Background(), q); err != nil {
			b.Fatalf("Estimate: %v", err)
		}
	}
}

func BenchmarkEstimate100Directions(b *testing.B) {
	m := benchModel(b)
	scene := benchScene(b)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		for d := range 100 {
			q := skybrightness.Query{
				Scene:     scene,
				Direction: coord.NewAltAz(angle.Deg(float64(d%90)+1), angle.Deg(float64(d)*3.6)),
			}

			if _, err := m.Estimate(context.Background(), q); err != nil {
				b.Fatalf("Estimate: %v", err)
			}
		}
	}
}

func BenchmarkSkyMapFullSky(b *testing.B) {
	m := benchModel(b)
	q := skybrightness.Query{Scene: benchScene(b)}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if _, err := m.SkyMap(context.Background(), q, 18); err != nil {
			b.Fatalf("SkyMap: %v", err)
		}
	}
}

// Full spectrum versus a single narrow band, to show how evaluation cost
// scales with spectral resolution — the trade-off a Fast fidelity level
// exists to exploit.
func BenchmarkEstimateSpectralResolution(b *testing.B) {
	m := benchModel(b)
	scene := benchScene(b)

	narrow, err := unit.NewSpectralGrid(540, 1, 21) // one 20 nm band
	if err != nil {
		b.Fatalf("NewSpectralGrid: %v", err)
	}

	for _, tc := range []struct {
		name string
		grid unit.SpectralGrid
	}{
		{"single-band", narrow},
		{"full-spectrum", skybrightness.DefaultOpticalGrid()},
	} {
		b.Run(tc.name, func(b *testing.B) {
			q := skybrightness.Query{
				Scene:     scene,
				Direction: coord.NewAltAz(angle.Deg(60), angle.Deg(120)),
				Grid:      tc.grid,
			}

			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				if _, err := m.Estimate(context.Background(), q); err != nil {
					b.Fatalf("Estimate: %v", err)
				}
			}
		})
	}
}

// Projection cost, separate from evaluation: a scheduler evaluates once
// and projects into several bands.
func BenchmarkInstrumentProjection(b *testing.B) {
	m := benchModel(b)

	est, err := m.Estimate(context.Background(), skybrightness.Query{
		Scene:     benchScene(b),
		Direction: coord.NewAltAz(angle.Deg(60), angle.Deg(120)),
	})
	if err != nil {
		b.Fatalf("Estimate: %v", err)
	}

	band := magnitude.Passband{
		Name:         "bench",
		WavelengthNM: []unit.WavelengthNM{499, 500, 600, 601},
		Response:     []float64{0, 1, 1, 0},
		Detector:     magnitude.PhotonCounting,
	}

	inst := optics.Instrument{
		Name:              "bench",
		CollectingAreaM2:  1,
		PixelSolidAngleSR: 1e-10,
		Throughput: []optics.Throughput{{
			Name:         "system",
			WavelengthNM: []unit.WavelengthNM{300, 1100},
			Efficiency:   []float64{0.5, 0.5},
		}},
	}

	b.Run("surface-brightness", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			if _, err := est.SurfaceBrightness(band, magnitude.AB); err != nil {
				b.Fatalf("SurfaceBrightness: %v", err)
			}
		}
	})

	b.Run("electron-rate", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			if _, err := est.ElectronRate(inst); err != nil {
				b.Fatalf("ElectronRate: %v", err)
			}
		}
	})
}

// benchTime and benchAtmosphere keep the benchmark scene fixed, so the
// numbers compare across runs.
var benchTime = time.GoDate(2026, 8, 14, 3, 0, 0, 0, time.LocationUTC)

func benchAtmosphere(heightM float64) *atmosphere.Atmosphere {
	return atmosphere.StandardDefault(heightM)
}
