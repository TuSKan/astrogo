package skybrightness

import (
	"fmt"
	"math"
	"sort"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/magnitude"
)

// Transfer applies this preset's radiative transfer to an atmosphere under
// construction, and returns the builder so it composes.
//
// # What it sets, and why the caller should not
//
// A preset is a configuration of components *and* of transfer. [NewPreset]
// builds the components; the transfer lives on the caller's atmosphere as the
// effective-optical-depth factor kappa and the higher-scattering-order
// switch. Those have exactly one correct value per preset, a caller cannot
// improve on them, and [Model.Estimate] rejects a scene carrying the wrong
// ones — so leaving them to be transcribed by hand was leaving a step that
// could only be got wrong.
//
// Everything else about the atmosphere stays the caller's: surface
// conditions, aerosol, clouds, terrain. Those describe the night being
// modelled and nothing here can know them.
//
//	air, err := preset.Transfer(atmosphere.RuralAerosol(1538, atmosphere.CleanMountainAOD550).
//		SurfaceAtAltitude(2635))
//	atm, err := air.Build()
func (p Preset) Transfer(b *atmosphere.Builder) (*atmosphere.Builder, error) {
	if b == nil {
		return nil, fmt.Errorf("%w %q: needs an atmosphere builder", ErrPreset, p)
	}

	kappa, err := p.DiffuseKappa()
	if err != nil {
		return nil, err
	}

	multiple, err := p.MultipleScattering()
	if err != nil {
		return nil, err
	}

	return b.DiffuseScattering(kappa).MultipleScattering(multiple), nil
}

// ComponentShare is one component's contribution to a sky.
type ComponentShare struct {
	// Component names the term.
	Component ComponentID

	// Brightness is this term alone, in the requested magnitude system, as a
	// surface brightness in mag/arcsec^2. A component contributing nothing —
	// moonlight with the Moon down — is +Inf, which is what zero flux is
	// worth in magnitudes.
	Brightness float64

	// Fraction is this term's share of the band-integrated radiance, in
	// [0,1], and the shares sum to one.
	//
	// This is the field that answers "why is this sky bright", and it is
	// deliberately the linear one. Radiance adds and magnitudes do not, so
	// Brightness values cannot be combined into the total by any arithmetic a
	// reader might try, while these can. Summing magnitudes is a correctness
	// bug this package treats as such elsewhere; the type is shaped so the
	// question does not arise.
	Fraction float64
}

// Composition reports what a sky is made of, brightest term first.
//
// # Why this is a method rather than a loop at the call site
//
// Getting here by hand means iterating [Estimate.ComponentIDs], pulling each
// spectrum with [Estimate.Component], integrating it against the passband,
// and sorting — or, more usually, giving up and printing raw radiance at one
// wavelength, which is what this package's own example did. Nobody reads
// W m^-2 sr^-1 nm^-1 at 550 nm; they read "airglow is thirty per cent of it".
//
// It works at [Reference] fidelity as well as [Standard]: the Eq. 11
// scattering term is added into each component's own buffer precisely so that
// a breakdown still attributes scattered light to whatever supplied it.
//
// Ordering is by fraction descending, so the first row answers the question
// and terms contributing nothing sort to the end where a top-N print never
// reaches them.
func (e *Estimate) Composition(p magnitude.Passband, sys magnitude.System) ([]ComponentShare, error) {
	ids := e.ComponentIDs()
	if len(ids) == 0 {
		return nil, nil
	}

	out := make([]ComponentShare, 0, len(ids))

	var total float64

	for _, id := range ids {
		spec, ok := e.Component(id)
		if !ok {
			continue
		}

		// The passband-averaged radiance: linear, so shares of it add up,
		// and the same quantity SurfaceBrightness is the logarithm of.
		radiance, err := magnitude.MeanFluxDensity(spec, e.Grid(), p, MinPassbandCoverage)
		if err != nil {
			return nil, fmt.Errorf("skybrightness: composition: %q: %w", id, err)
		}

		brightness, err := magnitude.SurfaceBrightness(spec, e.Grid(), p, sys, MinPassbandCoverage)
		if err != nil {
			return nil, fmt.Errorf("skybrightness: composition: %q: %w", id, err)
		}

		total += radiance

		out = append(out, ComponentShare{
			Component:  id,
			Brightness: brightness,
			Fraction:   radiance,
		})
	}

	// The fractions are shares of the total, so they cannot be computed until
	// every term is in. A sky with no positive radiance leaves them zero
	// rather than dividing by it.
	if total > 0 {
		for i := range out {
			out[i].Fraction /= total
		}
	} else {
		for i := range out {
			out[i].Fraction = 0
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		// Descending, with NaN pushed to the end rather than left to make the
		// ordering undefined.
		a, b := out[i].Fraction, out[j].Fraction
		if math.IsNaN(a) {
			return false
		}

		if math.IsNaN(b) {
			return true
		}

		return a > b
	})

	return out, nil
}
