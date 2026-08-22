package skybrightness

import (
	"context"
	"fmt"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/unit"
)

// hemisphereField is the incoming radiance of Masana et al. (2024) Eq. 11,
// sampled once over the quadrature and reusable for every direction of
// observation.
//
// # Why it is worth holding
//
// L_0(lambda, s) is the field above the atmosphere. It does not depend on
// where the observer is looking — only on where the light comes from — so an
// all-sky map that evaluates it per view direction evaluates the same thing
// tens of thousands of times. Profiled, that re-evaluation was the largest
// single cost in a reference-fidelity map: a third of the run was inside the
// SOFA nutation and Earth-position routines that a coordinate transform needs,
// reached through the star map's own direction lookup.
//
// Sampling it once turns the cost of a map from (view directions x source
// directions x components) evaluations into (source directions x components),
// plus an arithmetic sum per view direction that carries no transcendentals at
// all. For a one-degree sky that is a factor of twenty thousand on the
// expensive half.
//
// It also removes a smaller duplication: the de-extinction factor that
// recovers L_0 depends on the source direction and the wavelength but not on
// the component, so evaluating every component in one pass computes it once
// instead of once each.
type hemisphereField struct {
	grid  unit.SpectralGrid
	rings int

	// ringAirmass is the source airmass of each ring, which every sample
	// around that ring shares.
	ringAirmass []float64

	// samples are the quadrature points, in ring-major order.
	samples []hemisphereSample

	// components names what each sample's radiance slice holds, in order.
	components []ComponentID
}

// hemisphereSample is one quadrature point of the incoming field.
type hemisphereSample struct {
	dir    coord.AltAz
	ring   int
	dOmega float64

	// radiance holds one spectrum per component, parallel to
	// hemisphereField.components and concatenated into a single allocation.
	radiance []float64
}

// spectrumOf returns the sample's radiance for the component at index c.
func (s hemisphereSample) spectrumOf(c, width int) []float64 {
	return s.radiance[c*width : (c+1)*width]
}

// sampleHemisphere evaluates every scattering-eligible component's
// extra-atmospheric radiance over the quadrature.
//
// Components that are themselves scattering integrals over a source outside
// the sky field are left out — see [scattersIntoItsOwnBeam] — because passing
// one through Eq. 11 would scatter light that has already been scattered.
func (m *Model) sampleHemisphere(
	ctx context.Context, scene *Scene, grid unit.SpectralGrid, rings int,
) (*hemisphereField, error) {
	if rings <= 0 {
		rings = DefaultScatteringRings
	}

	eligible := make([]Component, 0, len(m.components))
	ids := make([]ComponentID, 0, len(m.components))

	for _, c := range m.components {
		if scattersIntoItsOwnBeam(c.ID()) {
			continue
		}

		eligible = append(eligible, c)
		ids = append(ids, c.ID())
	}

	field := &hemisphereField{
		grid:        grid,
		rings:       rings,
		ringAirmass: make([]float64, rings),
		components:  ids,
	}

	if len(eligible) == 0 {
		return field, nil
	}

	width := grid.Len()
	buf := NewSpectralRadiance(grid)

	for _, p := range hemisphereQuadrature(rings) {
		if field.ringAirmass[p.ring] == 0 {
			airmass, err := atmosphere.Airmass(p.alt)
			if err != nil {
				return nil, fmt.Errorf("%w: source airmass at %v: %w", ErrScattering, p.alt, err)
			}

			field.ringAirmass[p.ring] = airmass
		}

		dir := coord.NewAltAz(p.alt, p.az)

		// The de-extinction factor depends on the direction and the wavelength
		// but not on the component, so it is formed once here and applied to
		// all of them.
		gain, err := deExtinction(scene, dir, grid)
		if err != nil {
			return nil, err
		}

		sample := hemisphereSample{
			dir:      dir,
			ring:     p.ring,
			dOmega:   p.dOmega,
			radiance: make([]float64, len(eligible)*width),
		}

		for c, component := range eligible {
			clear(buf)

			if _, err := component.AddRadiance(ctx, buf, grid, dir, scene); err != nil {
				return nil, fmt.Errorf("%w: %q at %v: %w",
					ErrScattering, component.ID(), dir, err)
			}

			into := sample.spectrumOf(c, width)
			for i := range into {
				into[i] = buf[i] * gain[i]
			}
		}

		field.samples = append(field.samples, sample)
	}

	return field, nil
}

// scatterInto adds the Eq. 11 term for one direction of observation, per
// component, from a field already sampled.
//
// This is the whole per-direction cost of the full scattering model once the
// field is held: one path factor per ring, two phase-function values per
// sample, and a multiply-add per wavelength. No coordinate transform, no
// ephemeris, and no transcendental in the innermost loop.
func (f *hemisphereField) scatterInto(
	est *Estimate, kernel *scatterKernel, view coord.AltAz, into map[ComponentID][]float64,
) error {
	if len(f.samples) == 0 {
		return nil
	}

	viewAirmass, err := atmosphere.Airmass(view.Alt())
	if err != nil {
		return fmt.Errorf("%w: view airmass: %w", ErrScattering, err)
	}

	width := f.grid.Len()
	path := make([]float64, width)
	ring := -1

	for _, sample := range f.samples {
		if sample.ring != ring {
			kernel.pathFactor(path, f.ringAirmass[sample.ring], viewAirmass)
			ring = sample.ring
		}

		phaseRayleigh, phaseAerosol, err := kernel.phaseAt(separation(view, sample.dir))
		if err != nil {
			return err
		}

		for c, id := range f.components {
			dst, ok := into[id]
			if !ok {
				continue
			}

			kernel.accumulate(dst, sample.spectrumOf(c, width), path,
				sample.dOmega, phaseRayleigh, phaseAerosol)
		}
	}

	_ = est

	return nil
}
