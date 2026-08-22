package skybrightness

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
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
// Twelve rings is about 900 source directions over the hemisphere under the
// azimuth rule below, which is enough for the integral to converge to well
// under a hundredth of a magnitude while staying affordable — the cost of the
// whole integral is one evaluation of the incoming field per source direction,
// so this is the parameter that decides whether an all-sky map takes seconds
// or hours.
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

	pressure, _ := scene.Atmosphere.Surface()
	aerosol := scene.Atmosphere.Aerosol()

	// The optical depths do not depend on the source direction, so they are
	// formed once for the whole hemisphere rather than once per sample.
	rayleigh := make([]unit.OpticalDepth, grid.Len())
	aer := make([]unit.OpticalDepth, grid.Len())
	scattering := make([]unit.OpticalDepth, grid.Len())
	extinction := make([]unit.OpticalDepth, grid.Len())

	for i := range rayleigh {
		lambda := grid.At(i)

		r, err := atmosphere.RayleighOpticalDepth(lambda, float64(pressure))
		if err != nil {
			return fmt.Errorf("%w: %w", ErrScattering, err)
		}

		a := unit.OpticalDepth(aerosol.TauAt(lambda))

		rayleigh[i], aer[i] = r, a
		extinction[i] = r + a

		// Aerosol removes light by absorption as well as scattering; the
		// single-scattering albedo is the share that scatters. Molecular
		// extinction at optical wavelengths is scattering outright.
		scattering[i] = r + a*unit.OpticalDepth(aerosol.SingleScatteringAlbedo)
	}

	source := NewSpectralRadiance(grid)

	const halfPi = math.Pi / 2

	dz := halfPi / float64(rings)

	for k := range rings {
		z := (float64(k) + 0.5) * dz
		alt := angle.Rad(halfPi - z)

		sourceAirmass, err := atmosphere.Airmass(alt)
		if err != nil {
			return fmt.Errorf("%w: source airmass at %v: %w", ErrScattering, alt, err)
		}

		// Azimuths in proportion to the ring's circumference, so the samples
		// carry roughly equal solid angle and the pole is not oversampled.
		azimuths := max(4, int(math.Round(4*float64(rings)*math.Sin(z))))
		dOmega := math.Sin(z) * dz * (2 * math.Pi / float64(azimuths))

		for j := range azimuths {
			az := angle.Rad(2 * math.Pi * float64(j) / float64(azimuths))
			dir := coord.NewAltAz(alt, az)

			clear(source)

			if err := above(ctx, source, dir); err != nil {
				return fmt.Errorf("%w: incoming field at %v: %w", ErrScattering, dir, err)
			}

			theta := separation(view, dir)

			for i := range dst {
				if source[i] == 0 {
					continue
				}

				phase, err := atmosphere.CombinedPhaseFunction(theta,
					rayleigh[i], aer[i], float64(aerosol.Asymmetry),
					atmosphere.RayleighDepolarisation)
				if err != nil {
					return fmt.Errorf("%w: %w", ErrScattering, err)
				}

				// The patch's irradiance at the top of the atmosphere is its
				// radiance times the solid angle it subtends, which is what
				// turns a radiance field into the source term the kernel takes.
				l, err := atmosphere.SingleScatteredRadiance(source[i]*dOmega, phase,
					scattering[i], extinction[i], sourceAirmass, viewAirmass)
				if err != nil {
					return fmt.Errorf("%w: %w", ErrScattering, err)
				}

				dst[i] += l
			}
		}
	}

	return nil
}

// separation returns the angle between two horizontal directions, in radians.
//
// The scattering angle of Eq. 11: zero when the source lies along the line of
// sight, pi when it is directly behind the observer's head.
func separation(a, b coord.AltAz) float64 {
	sinA, cosA := math.Sincos(a.Alt().Radians())
	sinB, cosB := math.Sincos(b.Alt().Radians())

	cos := sinA*sinB + cosA*cosB*math.Cos(a.Az().Radians()-b.Az().Radians())

	return math.Acos(math.Min(1, math.Max(-1, cos)))
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

// naturalField builds the incoming field of a single component.
//
// The per-component form of [Model.AboveAtmosphere], and the reason
// [Model.Estimate] can attribute scattered light to the component that
// supplied it rather than reporting only the sum. Without that attribution the
// starlight-to-zodiacal comparison against Masana et al. Table 2 could not be
// made at all.
func naturalField(c Component, scene *Scene, grid unit.SpectralGrid) SkyRadiance {
	return func(ctx context.Context, dst SpectralRadiance, dir coord.AltAz) error {
		if dir.Alt() <= 0 {
			return nil
		}

		buf := NewSpectralRadiance(grid)

		if _, err := c.AddRadiance(ctx, buf, grid, dir, scene); err != nil {
			return fmt.Errorf("%w: %q: %w", ErrScattering, c.ID(), err)
		}

		gain, err := deExtinction(scene, dir, grid)
		if err != nil {
			return err
		}

		for i := range dst {
			dst[i] += buf[i] * gain[i]
		}

		return nil
	}
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
func (m *Model) addScatteredIn(
	ctx context.Context, est *Estimate, q Query, grid unit.SpectralGrid, rings int,
) error {
	for _, c := range m.components {
		buf, ok := est.components[c.ID()]
		if !ok {
			continue
		}

		if scattersIntoItsOwnBeam(c.ID()) {
			est.Quality.Add(PartialScattering)

			continue
		}

		scattered := NewSpectralRadiance(grid)

		if err := ScatteredIn(ctx, scattered, naturalField(c, q.Scene, grid),
			q.Scene, q.Direction, grid, rings); err != nil {
			return fmt.Errorf("%w: %q: %w", ErrComponentFailed, c.ID(), err)
		}

		// Higher scattering orders, when the scene asks for them. Eq. 11's
		// kernel is first order, so this is the one place they can be added
		// without double-counting: the direct term is extinction and has no
		// scattering order at all.
		if q.Scene.Atmosphere.MultipleScattering() {
			pressure, _ := q.Scene.Atmosphere.Surface()

			for i := range buf {
				rayleigh, err := atmosphere.RayleighOpticalDepth(grid.At(i), float64(pressure))
				if err != nil {
					return fmt.Errorf("%w: %q: %w", ErrComponentFailed, c.ID(), err)
				}

				multiple, err := atmosphere.MultipleScatteringFactor(rayleigh)
				if err != nil {
					return fmt.Errorf("%w: %q: %w", ErrComponentFailed, c.ID(), err)
				}

				scattered[i] *= multiple
			}

			est.Quality.Add(ApproximateMultipleScattering)
		}

		for i := range buf {
			buf[i] += scattered[i]
			est.total[i] += scattered[i]
		}
	}

	return nil
}
