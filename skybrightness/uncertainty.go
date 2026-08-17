package skybrightness

import (
	"math"

	"github.com/TuSKan/astrogo/unit"
)

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
