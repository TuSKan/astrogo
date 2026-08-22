package skybrightness

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/unit"
)

// ErrScattering reports that a scattering integral cannot be evaluated.
var ErrScattering = errors.New("skybrightness: scattering")

// SkyRadiance gives the radiance entering the atmosphere from one direction,
// accumulating into dst.
//
// This is the L0 of Masana et al. (2024) Eq. 11: the field above the
// atmosphere, before anything has been taken out of it or added to it along
// the way. A caller with a model supplies [Model.AboveAtmosphere]; a caller
// with a measured all-sky map supplies its own.
type SkyRadiance func(ctx context.Context, dst SpectralRadiance, dir coord.AltAz) error

// DefaultScatteringRings is the zenith-angle resolution [ScatteredIn] uses
// when none is given.
//
// # Chosen by measurement, and twenty-four was tried and rejected
//
// Against a smooth field the midpoint rule converges better than
// quadratically: 0.55 per cent here, 0.11 at twenty-four rings, 0.026 at
// forty-eight. Half a per cent is six thousandths of a magnitude, against the
// 0.046 mag by which this module and GAMBONS disagree about the same sky, so
// the quadrature is already an order of magnitude below the physical
// uncertainty.
//
// Twenty-four was tried. The direction count goes as the square of the rings
// and the per-direction sum is over all of them, so it costs four times as
// much everywhere rather than only in the one-time sampling — a whole
// reference sky went from sixteen seconds to a minute. Measured against the
// published star map it moved the Table 2 starlight-to-zodiacal ratio from
// 1.1758 to 1.1763, four hundredths of a per cent, against a discrepancy with
// the paper of five per cent. Four times the cost for a fortieth of the error
// that matters is not a trade worth making, and a caller who needs it can pass
// a ring count.
//
// # What no ring count fixes
//
// Against a field with an edge in it the error stops falling around a per cent
// however fine the grid gets, because it becomes a question of where the
// samples land relative to the edge rather than of the step size. The real
// incoming field is built from a HEALPix star map and a dust map, both
// piecewise constant. That, and not this constant, is what limits the
// integral; closing it needs the field averaged over each quadrature cell
// rather than sampled at its centre. Recorded in docs/skybrightness.md.
const DefaultScatteringRings = 12

// ScatteredIn evaluates Masana et al. (2024) Eq. 11: the radiance scattered
// into the line of sight from the rest of the sky.
//
//	L_s(lambda, u, h) = INT_Omega L_0(lambda, s) Phi(lambda, u, s, h) dOmega
//
// The kernel Phi is first-order scattering after Kocifaj & Kránicz (2011),
// with the effective phase function the paper specifies: aerosol and molecular
// components weighted by their own optical depths. Both are already in this
// module — [github.com/TuSKan/astrogo/atmosphere.SingleScatteredRadiance] is
// the kernel and
// [github.com/TuSKan/astrogo/atmosphere.CombinedPhaseFunction] the phase
// function — so nothing here is a new coefficient, only a new integral over
// them.
//
// # What this is for
//
// The transfer the components apply on their own is the web-service
// simplification: an effective optical depth tau_eff = kappa*tau that stands
// in for the light scattered back into the beam. It is a common factor, so it
// cannot change the ratio between two extended components in one direction.
// This integral is the thing it stands in for, and it is not a common factor:
// it weights each component by its own distribution over the sky, so a
// component with light near the horizon contributes more of it at the zenith
// than one concentrated overhead.
//
// Masana et al. give the cost of the difference as under a tenth of a
// magnitude in most cases, the simplified model running bright near the
// horizon and dark at the zenith. The cost of the integral is one evaluation
// of above per source direction, which is why the web service does not use it.
//
// # Quadrature
//
// Midpoint in zenith angle over rings rings, with the number of azimuths in
// each ring proportional to sin(z) so that every sample carries roughly the
// same solid angle. Midpoint rather than endpoint keeps the outermost ring off
// the horizon, where the airmass is largest and the field is least reliable.
// Pass rings <= 0 for [DefaultScatteringRings].
//
// dst is accumulated into, not overwritten, matching [Component.AddRadiance].
func ScatteredIn(
	ctx context.Context,
	dst SpectralRadiance,
	above SkyRadiance,
	scene *Scene,
	view coord.AltAz,
	grid unit.SpectralGrid,
	rings int,
) error {
	if above == nil {
		return fmt.Errorf("%w: no incoming field", ErrScattering)
	}

	if err := scene.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrScattering, err)
	}

	if err := grid.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrScattering, err)
	}

	if len(dst) != grid.Len() {
		return fmt.Errorf("%w: %d destination slots, grid has %d",
			unit.ErrGridMismatch, len(dst), grid.Len())
	}

	if rings <= 0 {
		rings = DefaultScatteringRings
	}

	viewAirmass, err := atmosphere.Airmass(view.Alt())
	if err != nil {
		return fmt.Errorf("%w: view airmass: %w", ErrScattering, err)
	}

	kernel, err := newScatterKernel(scene, grid)
	if err != nil {
		return err
	}

	source := NewSpectralRadiance(grid)
	path := make([]float64, grid.Len())
	ring := -1

	for _, p := range hemisphereQuadrature(rings) {
		if p.ring != ring {
			sourceAirmass, err := atmosphere.Airmass(p.alt)
			if err != nil {
				return fmt.Errorf("%w: source airmass at %v: %w", ErrScattering, p.alt, err)
			}

			// Once per ring, not once per sample. The path integral depends
			// on the source airmass, which is a function of altitude alone, so
			// every azimuth around a ring shares it — and a ring near the
			// horizon carries dozens of them.
			kernel.pathFactor(path, sourceAirmass, viewAirmass)

			ring = p.ring
		}

		dir := coord.NewAltAz(p.alt, p.az)

		clear(source)

		if err := above(ctx, source, dir); err != nil {
			return fmt.Errorf("%w: incoming field at %v: %w", ErrScattering, dir, err)
		}

		// Once per sample, not once per wavelength: the phase functions depend
		// on the scattering angle alone, and only the weights that mix them
		// vary across the band.
		phaseRayleigh, phaseAerosol, err := kernel.phaseAt(separation(view, dir))
		if err != nil {
			return err
		}

		kernel.accumulate(dst, source, path, p.dOmega, phaseRayleigh, phaseAerosol)
	}

	return nil
}

// separation returns the angle between two horizontal directions, in radians.
//
// The scattering angle of Eq. 11: zero when the source lies along the line of
// sight, pi when it is directly behind the observer's head.
//
// # Why atan2 and not acos
//
// The obvious form, acos of the dot product, is ill-conditioned exactly where
// this is used most sharply. Near zero the dot product is 1 - theta^2/2, so an
// absolute error of one part in 10^16 in the dot becomes a relative error of
// 10^-16/theta^2 in the angle: eight significant digits gone by a hundredth of
// a radian, and worse closer in. Forward scattering is where the
// Henyey-Greenstein phase function is most peaked — at an asymmetry of 0.65 it
// is two orders of magnitude above its backward value — so the angles this
// resolves least well are the ones that contribute most.
//
// atan2 of the cross-product norm against the dot is well conditioned over the
// whole range, and it is the same form [github.com/TuSKan/astrogo/coord.Separation]
// uses for the same reason.
func separation(a, b coord.AltAz) float64 {
	sinAltA, cosAltA := math.Sincos(a.Alt().Radians())
	sinAzA, cosAzA := math.Sincos(a.Az().Radians())
	sinAltB, cosAltB := math.Sincos(b.Alt().Radians())
	sinAzB, cosAzB := math.Sincos(b.Az().Radians())

	ax, ay, az := cosAltA*cosAzA, cosAltA*sinAzA, sinAltA
	bx, by, bz := cosAltB*cosAzB, cosAltB*sinAzB, sinAltB

	cx := ay*bz - az*by
	cy := az*bx - ax*bz
	cz := ax*by - ay*bx

	return math.Atan2(math.Sqrt(cx*cx+cy*cy+cz*cz), ax*bx+ay*by+az*bz)
}

// AboveAtmosphere returns the extra-atmospheric field this model implies, for
// use as the incoming field of [ScatteredIn].
//
// # How
//
// Every natural component in this module computes an extra-atmospheric
// radiance and multiplies it by exp(-tau_eff*m), with tau_eff from
// [github.com/TuSKan/astrogo/atmosphere.ExtendedSourceOpticalDepth]. That
// factor is public, deterministic and the same for every one of them, so the
// field before it can be recovered exactly by dividing it back out. Nothing is
// re-derived and no component is evaluated differently; this is the estimate
// the model already produces, with a known factor undone.
//
// # When it does not apply
//
// Only to components that use the extended-source transfer, which is the
// natural sky: starlight, diffuse galactic light, the extragalactic
// background, zodiacal light and airglow. Moonlight and artificial skyglow are
// already scattering integrals over a source that is not part of the sky
// field, so dividing an extended-source factor out of them would be
// meaningless and then feeding the result back into a scattering integral
// would count them twice. A model registering either is refused rather than
// silently mistreated.
//
// Airglow is included, and is worth a note: its source is a layer at 87 km
// rather than a field outside the atmosphere, so what is recovered is the
// van Rhijn-brightened layer radiance before extinction. That is the quantity
// Eq. 10 sums and Eq. 11 scatters, but it is not literally extra-atmospheric,
// and a caller comparing against a paper should know which of the two is
// meant.
func (m *Model) AboveAtmosphere(q Query) (SkyRadiance, error) {
	for _, id := range m.Components() {
		switch id {
		case Moonlight, Artificial:
			return nil, fmt.Errorf(
				"%w: %s does not use the extended-source transfer, so the field above the "+
					"atmosphere cannot be recovered by undoing it", ErrScattering, id)

		case Starlight, DiffuseGalactic, Extragalactic, Zodiacal,
			AirglowContinuum, AirglowLines, Twilight:
		}
	}

	grid := q.grid()

	return func(ctx context.Context, dst SpectralRadiance, dir coord.AltAz) error {
		if len(dst) != grid.Len() {
			return fmt.Errorf("%w: %d destination slots, grid has %d",
				unit.ErrGridMismatch, len(dst), grid.Len())
		}

		// Below the horizon there is no sky to scatter, which is what makes
		// the integral a hemisphere rather than a sphere.
		if dir.Alt() <= 0 {
			return nil
		}

		local := q
		local.Direction = dir
		local.Grid = grid

		est, err := m.Estimate(ctx, local)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrScattering, err)
		}

		gain, err := deExtinction(local.Scene, dir, grid)
		if err != nil {
			return err
		}

		total := est.SpectralRadiance()
		for i := range dst {
			dst[i] += total[i] * gain[i]
		}

		return nil
	}, nil
}

// deExtinction returns exp(+tau_eff*m) per wavelength: the reciprocal of the
// factor the natural components applied.
func deExtinction(scene *Scene, dir coord.AltAz, grid unit.SpectralGrid) ([]float64, error) {
	airmass, err := atmosphere.Airmass(dir.Alt())
	if err != nil {
		return nil, fmt.Errorf("%w: airmass: %w", ErrScattering, err)
	}

	pressure, _ := scene.Atmosphere.Surface()
	aerosol := scene.Atmosphere.Aerosol()
	height := scene.Observer.Height()
	kappa := scene.Atmosphere.DiffuseKappa()

	out := make([]float64, grid.Len())

	for i := range out {
		lambda := grid.At(i)

		rayleigh, err := atmosphere.RayleighOpticalDepth(lambda, float64(pressure))
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrScattering, err)
		}

		slant, err := atmosphere.ExtendedSourceOpticalDepth(
			rayleigh, unit.OpticalDepth(aerosol.TauAt(lambda)),
			airmass, airmass, height, kappa)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrScattering, err)
		}

		out[i] = math.Exp(float64(slant))
	}

	return out, nil
}

// scattersIntoItsOwnBeam reports whether a component is already a scattering
// integral over a source that is not part of the sky field.
//
// Moonlight and artificial skyglow are. Running them through [ScatteredIn]
// would scatter light that has already been scattered, counting it twice, so
// the reference-fidelity pass leaves them as they are and the estimate says so
// through [PartialScattering].
func scattersIntoItsOwnBeam(id ComponentID) bool {
	switch id {
	case Moonlight, Artificial:
		return true
	case Starlight, DiffuseGalactic, Extragalactic, Zodiacal,
		AirglowContinuum, AirglowLines, Twilight:
		return false
	default:
		return false
	}
}

// addScatteredIn adds the Eq. 11 term to every component that has one.
//
// Called only at [Reference] fidelity. The components have already written
// their direct radiance into est, which under a scene at kappa = 1 is the L_d
// of Masana et al. Eq. 8; this adds the L_s that completes it.
//
// field may be nil, in which case one is sampled for this direction alone.
// [Model.SkyMap] passes one it sampled once for the whole map, which is what
// makes an all-sky reference evaluation affordable: the incoming field does not
// depend on where the observer looks.
func (m *Model) addScatteredIn(
	ctx context.Context, est *Estimate, q Query, grid unit.SpectralGrid,
	field *hemisphereField, rings int,
) error {
	if field == nil {
		sampled, err := m.sampleHemisphere(ctx, q.Scene, grid, rings)
		if err != nil {
			return err
		}

		field = sampled
	}

	kernel, err := newScatterKernel(q.Scene, grid)
	if err != nil {
		return err
	}

	// One buffer per component, so the scattered light is attributed to
	// whatever supplied it rather than reported as a lump.
	into := make(map[ComponentID][]float64, len(field.components))

	for _, id := range field.components {
		if _, ok := est.components[id]; ok {
			into[id] = make([]float64, grid.Len())
		}
	}

	for _, c := range m.components {
		if scattersIntoItsOwnBeam(c.ID()) {
			est.Quality.Add(PartialScattering)
		}
	}

	if err := field.scatterInto(est, kernel, q.Direction, into); err != nil {
		return err
	}

	// Higher scattering orders, when the scene asks for them. Eq. 11's kernel
	// is first order, so this is the one place they can be added without
	// double-counting: the direct term is extinction and has no scattering
	// order at all.
	var multiple []float64

	if q.Scene.Atmosphere.MultipleScattering() {
		multiple = make([]float64, grid.Len())
		pressure, _ := q.Scene.Atmosphere.Surface()

		for i := range multiple {
			rayleigh, err := atmosphere.RayleighOpticalDepth(grid.At(i), float64(pressure))
			if err != nil {
				return fmt.Errorf("%w: %w", ErrScattering, err)
			}

			factor, err := atmosphere.MultipleScatteringFactor(rayleigh)
			if err != nil {
				return fmt.Errorf("%w: %w", ErrScattering, err)
			}

			multiple[i] = factor
		}

		est.Quality.Add(ApproximateMultipleScattering)
	}

	for id, scattered := range into {
		buf := est.components[id]

		for i := range buf {
			v := scattered[i]
			if multiple != nil {
				v *= multiple[i]
			}

			buf[i] += v
			est.total[i] += v
		}
	}

	return nil
}
