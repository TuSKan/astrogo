package skybrightness

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/unit"
)

// ErrUncertaintyUnknown is returned for a total uncertainty that cannot be
// computed, because a component contributing light has no entry in the
// budget. Its uncertainty is unknown, which is not zero.
var ErrUncertaintyUnknown = errors.New("skybrightness: uncertainty unknown")

// Uncertainty is the uncertainty of one contribution, expressed as a
// relative standard uncertainty on its radiance.
//
// Relative rather than absolute because the dominant terms — airglow
// variability, aerosol loading, assumed source spectra — are multiplicative
// in nature: airglow is quoted as a factor-of-two swing, not as a fixed
// W m^-2 sr^-1 nm^-1 offset.
type Uncertainty struct {
	// Component identifies what this uncertainty belongs to.
	Component ComponentID

	// Relative is the 1-sigma relative standard uncertainty, e.g. 0.3 for
	// 30 per cent.
	Relative float64

	// Source names the dominant contributor, e.g. "airglow variability",
	// "assumed source SPD".
	Source string
}

// UncertaintyBudget holds per-component uncertainties and combines them
// into a total.
//
// Component-level uncertainties are kept rather than collapsed immediately
// into one number, because which term dominates is itself the useful
// output: a caller can act on "airglow dominates" by taking a measurement,
// but cannot act on a single opaque percentage.
type UncertaintyBudget struct {
	// Components holds one entry per contributing component.
	Components []Uncertainty

	// Correlated marks the budget as combining terms in phase rather than
	// in quadrature. Independence is not assumed: two components sharing
	// an aerosol assumption are correlated through it, and a caller that
	// knows this sets the flag rather than accepting an optimistic total.
	Correlated bool
}

// Add records a component's uncertainty.
func (b *UncertaintyBudget) Add(u Uncertainty) {
	b.Components = append(b.Components, u)
}

// Total combines the per-component relative uncertainties into a relative
// uncertainty on the total radiance, weighting each component by its share
// of that total.
//
// weights maps a component to its radiance contribution. A component's
// uncertainty matters in proportion to how much it contributes: 50 per
// cent uncertainty on a term carrying 1 per cent of the flux is 0.5 per
// cent on the answer.
//
// Terms combine in quadrature when Correlated is false and linearly when
// it is true. Linear is the conservative choice and is what a shared
// systematic actually does.
func (b *UncertaintyBudget) Total(weights map[ComponentID]unit.Radiance) float64 {
	var total unit.Radiance

	for _, w := range weights {
		total += w
	}

	if total <= 0 {
		return 0
	}

	var sum float64

	for _, u := range b.Components {
		share := float64(weights[u.Component]) / float64(total)
		contribution := share * u.Relative

		if b.Correlated {
			sum += contribution

			continue
		}

		sum += contribution * contribution
	}

	if b.Correlated {
		return sum
	}

	return math.Sqrt(sum)
}

// Dominant returns the component contributing most to the combined
// uncertainty, and its share. It answers "what should I measure to improve
// this prediction?" — the question the budget exists to serve.
func (b *UncertaintyBudget) Dominant(weights map[ComponentID]unit.Radiance) (Uncertainty, float64) {
	var (
		total unit.Radiance
		best  Uncertainty
		bestC float64
	)

	for _, w := range weights {
		total += w
	}

	if total <= 0 {
		return Uncertainty{}, 0
	}

	for _, u := range b.Components {
		share := float64(weights[u.Component]) / float64(total)
		if c := share * u.Relative; c > bestC {
			best, bestC = u, c
		}
	}

	return best, bestC
}

// spectralUncertainty accumulates a component's relative uncertainty over the
// grid, each sample weighted by the radiance it adds with the trapezoid
// weights [unit.SpectralGrid.Integrate] uses, so the result is the relative
// uncertainty of exactly the integrated radiance [UncertaintyBudget.Total]
// weighs the component by.
//
// The samples combine linearly, as a fully correlated error does: the terms
// that dominate here - a calibration scale, a fitted slope, a scattering
// coefficient - move a whole spectrum together, not one wavelength at a time.
type spectralUncertainty struct {
	weighted, radiance float64
}

// add records sample i, which added radiance with the given relative
// uncertainty.
func (u *spectralUncertainty) add(grid unit.SpectralGrid, i int, radiance, relative float64) {
	w := radiance
	if i == 0 || i == grid.Len()-1 {
		w *= 0.5
	}

	u.weighted += w * relative
	u.radiance += w
}

// contribution reports what was accumulated, with flags. A component that
// added no radiance has nothing to be uncertain about, so its uncertainty
// stays unknown, which the budget does not ask of a dark component.
func (u *spectralUncertainty) contribution(flags Flag, source string) Contribution {
	if u.radiance <= 0 {
		return Contribution{Flags: flags}
	}

	return Contribution{Flags: flags, Relative: u.weighted / u.radiance, Source: source, Known: true}
}

// ErrUncertaintyValue is returned for a supplied relative uncertainty that is
// negative or not finite, or for no component to attach it to.
var ErrUncertaintyValue = errors.New("skybrightness: a supplied uncertainty needs a component and a non-negative, finite relative value")

// WithUncertainty returns c reporting relative, with source, as the 1-sigma
// relative uncertainty of every radiance it adds, in place of whatever c
// reports itself.
//
// It is how a caller supplies the uncertainty of what the caller supplied:
// airglow's zenith spectrum, integrated starlight's map and shape, an
// artificial-light source inventory. Those components report an unknown
// uncertainty on their own, because it belongs to the caller's data rather than
// to the model. A caller who knows it says so here, and one who does not leaves
// [Estimate.TotalUncertainty] reporting [ErrUncertaintyUnknown], naming the
// component.
//
// It also replaces a figure a component does report, for a caller who holds a
// better one, such as a measurement of the night being modeled.
func WithUncertainty(c Component, relative float64, source string) (Component, error) {
	if c == nil || relative < 0 || math.IsNaN(relative) || math.IsInf(relative, 0) {
		return nil, fmt.Errorf("%w: %g", ErrUncertaintyValue, relative)
	}

	return &suppliedUncertainty{Component: c, relative: relative, source: source}, nil
}

// suppliedUncertainty is a component reporting a caller's uncertainty.
type suppliedUncertainty struct {
	Component

	relative float64
	source   string
}

// AddRadiance implements [Component].
func (s *suppliedUncertainty) AddRadiance(
	ctx context.Context,
	dst SpectralRadiance,
	grid unit.SpectralGrid,
	dir coord.AltAz,
	scene *Scene,
) (Contribution, error) {
	contribution, err := s.Component.AddRadiance(ctx, dst, grid, dir, scene)
	if err != nil {
		return contribution, fmt.Errorf("skybrightness: %s: %w", s.ID(), err)
	}

	contribution.Relative, contribution.Source, contribution.Known = s.relative, s.source, true

	return contribution, nil
}
