package natural

import "github.com/TuSKan/astrogo/skybrightness"

// TopHatJohnsonV returns an analytic, flat-top-hat stand-in for the
// Johnson V passband, in astrogo v1's Garstang-nanolambert convention —
// NOT SI W*m^-2*sr^-1*nm^-1. It is meaningful only through
// VegaSurfaceBrightness against this exact passband (which reproduces the
// original v1 V mag/arcsec^2 value to historical precision, see
// garstang_units.go); do not integrate it against any other passband, and
// do not call ABSurfaceBrightness on its output.
//
// A caller wanting a real, SI-consistent Johnson V response curve should
// use skybrightness/dataset/passband instead — this exists only to keep
// ConstantAirglow/VBandMoonlight/example 21 fully offline
// with zero data dependency (docs/skybrightness.md §14 Phase 1 scope).
func TopHatJohnsonV() *skybrightness.Passband {
	return &skybrightness.Passband{
		ID:         "tophat.johnson.V",
		System:     skybrightness.SystemVega,
		Detector:   skybrightness.EnergyIntegrating,
		Wavelength: []skybrightness.WavelengthNM{topHatVLo, topHatVHi},
		Response:   []float64{1, 1},
		VegaZP: &skybrightness.VegaZeroPoint{
			MeanFlambda: skybrightness.SpectralRadiance(garstangVegaZeroPoint),
			Spectrum:    "Garstang nanolambert convention, not a real Vega spectrum",
			Uncertainty: 0,
		},
		Source: skybrightness.SourceRef{
			Name:     "top-hat V-band stand-in",
			Fidelity: skybrightness.FidelitySynthetic,
		},
	}
}

// TopHatVGrid returns a 7-point uniform SpectralGrid spanning the top-hat
// V-band stand-in's range (470-700 nm) — the grid a caller running
// ModeFast should evaluate on.
func TopHatVGrid() skybrightness.SpectralGrid {
	g, err := skybrightness.UniformSpectralGrid(topHatVLo, topHatVHi, 7)
	if err != nil {
		panic("natural: TopHatVGrid: " + err.Error()) // unreachable: fixed, valid inputs
	}

	return g
}

// fillFlat writes values[d] into direction d's row of out, at every grid
// wavelength inside [topHatVLo, topHatVHi], and 0 elsewhere — the
// flat-spectrum convention every ModeFast component uses so its output
// integrates back to the exact original V magnitude through
// VegaSurfaceBrightness against TopHatJohnsonV (see garstang_units.go).
// len(values) must equal out's direction count.
func fillFlat(grid skybrightness.SpectralGrid, out skybrightness.SpectralField, values []skybrightness.SpectralRadiance) {
	lambda := grid.Lambda()

	for d, v := range values {
		row := out.Row(d)

		for i, lam := range lambda {
			if lam >= topHatVLo && lam <= topHatVHi {
				row[i] = v
			}
		}
	}
}
