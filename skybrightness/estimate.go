package skybrightness

import (
	"fmt"

	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/optics"
	"github.com/TuSKan/astrogo/unit"
)

// MinPassbandCoverage is the fraction of a passband's response a spectral
// grid must cover before a projection is trusted.
//
// 0.99 rather than 1.0 because real response curves have long, low tails
// that contribute negligibly but would otherwise force an impractically
// wide grid; below that, the truncation starts to bias the answer rather
// than merely add noise.
const MinPassbandCoverage = 0.99

// Estimate is the result of one evaluation: the spectral sky state, and
// every derived quantity projected from it.
//
// Every projection below — a magnitude in any band, a luminance, a photon
// rate, a detector background rate — comes from the same stored spectrum.
// They are views of one physical state, never independently modelled
// numbers, which is what keeps them mutually consistent.
type Estimate struct {
	// Quality records how the prediction was constrained.
	Quality Quality

	// Uncertainty holds the per-component uncertainty budget.
	Uncertainty UncertaintyBudget

	// Reproducibility carries everything needed to explain the result.
	Reproducibility Reproducibility

	grid       unit.SpectralGrid
	total      SpectralRadiance
	components map[ComponentID]SpectralRadiance
}

// Grid reports the spectral axis the estimate was computed on.
func (e *Estimate) Grid() unit.SpectralGrid { return e.grid }

// SpectralRadiance returns the total sky spectral radiance, in
// W m^-2 sr^-1 nm^-1. The returned slice is the estimate's own buffer;
// treat it as read-only, or Clone it.
func (e *Estimate) SpectralRadiance() SpectralRadiance { return e.total }

// Component returns one component's contribution, and whether it was
// evaluated.
func (e *Estimate) Component(id ComponentID) (SpectralRadiance, bool) {
	s, ok := e.components[id]

	return s, ok
}

// ComponentIDs lists the components that contributed.
func (e *Estimate) ComponentIDs() []ComponentID {
	out := make([]ComponentID, 0, len(e.components))
	for id := range e.components {
		out = append(out, id)
	}

	return out
}

// Radiance returns the total radiance integrated across the grid's span,
// in W m^-2 sr^-1.
func (e *Estimate) Radiance() (unit.Radiance, error) {
	return e.total.Integrate(e.grid)
}

// SurfaceBrightness projects the spectrum into a magnitude per square
// arcsecond in the given passband and system.
//
// The passband and system are required arguments because a surface
// brightness without them is meaningless: the same sky gives different
// numbers in Johnson V, SDSS g and an SQM response, and the differences
// are spectrum-dependent rather than fixed offsets.
func (e *Estimate) SurfaceBrightness(p magnitude.Passband, sys magnitude.System) (float64, error) {
	v, err := magnitude.SurfaceBrightness(e.total, e.grid, p, sys, MinPassbandCoverage)
	if err != nil {
		return 0, fmt.Errorf("skybrightness: surface brightness: %w", err)
	}

	return v, nil
}

// PhotonRate returns the sky photon rate an instrument sees, in
// photons s^-1 m^-2 sr^-1, before collecting area and pixel solid angle.
func (e *Estimate) PhotonRate(inst optics.Instrument) (unit.PhotonRadiance, error) {
	r, err := inst.PhotonRate(e.total, e.grid)
	if err != nil {
		return 0, fmt.Errorf("skybrightness: photon rate: %w", err)
	}

	return r, nil
}

// ElectronRate returns the sky background in electrons per pixel per
// second — the quantity an exposure-time calculation actually needs.
func (e *Estimate) ElectronRate(inst optics.Instrument) (unit.ElectronsPerPixelPerSecond, error) {
	r, err := inst.BackgroundRate(e.total, e.grid)
	if err != nil {
		return 0, fmt.Errorf("skybrightness: electron rate: %w", err)
	}

	return r, nil
}

// TotalUncertainty combines the per-component budget, weighting each
// component by its share of the integrated radiance.
func (e *Estimate) TotalUncertainty() (float64, error) {
	weights := make(map[ComponentID]unit.Radiance, len(e.components))

	for id, s := range e.components {
		r, err := s.Integrate(e.grid)
		if err != nil {
			return 0, err
		}

		weights[id] = r
	}

	return e.Uncertainty.Total(weights), nil
}
