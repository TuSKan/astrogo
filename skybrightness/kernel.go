package skybrightness

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/unit"
)

// scatterKernel holds everything in Masana et al. (2024) Eq. 11 that does not
// depend on the source direction, so the innermost loop of the hemispheric
// integral is multiply-add and nothing else.
//
// # Why this exists
//
// Evaluated directly, the integral calls
// [github.com/TuSKan/astrogo/atmosphere.CombinedPhaseFunction] and
// [github.com/TuSKan/astrogo/atmosphere.SingleScatteredRadiance] once per
// source direction per wavelength, and each of those is a handful of
// transcendentals. Profiled over a reference-fidelity sky map, math.pow and
// math.exp together were 70 per cent of the run.
//
// Almost none of it varies with what the loop varies over. The optical depths
// depend on wavelength alone; the phase function on scattering angle alone;
// and the path integral on wavelength and the SOURCE ALTITUDE, which is
// constant around a ring of the quadrature. What is left inside the loop is
// the source radiance, the two phase terms and a product.
//
// # Equivalence
//
// This is a rearrangement, not an approximation, and it is not allowed to
// drift from the functions it rearranges:
// TestScatterKernelMatchesTheReferenceFunctions evaluates both forms over a
// spread of geometries and optical depths and requires them to agree to
// within floating-point reassociation. The reference implementations stay
// exported and stay the definition.
type scatterKernel struct {
	// grid is the spectral axis everything below is indexed on.
	grid unit.SpectralGrid

	// rayleighWeight and aerosolWeight are the phase-function weights,
	// tau_R/tau and tau_A/tau, per wavelength.
	rayleighWeight []float64
	aerosolWeight  []float64

	// scatterFraction is tau_sca/tau_ext per wavelength, the share of
	// extinction that scatters rather than absorbs.
	scatterFraction []float64

	// extinction is the total vertical optical depth per wavelength.
	extinction []float64

	// asymmetry and depolarisation parameterise the two phase functions.
	asymmetry      float64
	depolarisation float64
}

// newScatterKernel builds the wavelength-dependent factors for a scene.
func newScatterKernel(scene *Scene, grid unit.SpectralGrid) (*scatterKernel, error) {
	pressure, _ := scene.Atmosphere.Surface()
	aerosol := scene.Atmosphere.Aerosol()

	k := &scatterKernel{
		grid:            grid,
		rayleighWeight:  make([]float64, grid.Len()),
		aerosolWeight:   make([]float64, grid.Len()),
		scatterFraction: make([]float64, grid.Len()),
		extinction:      make([]float64, grid.Len()),
		asymmetry:       float64(aerosol.Asymmetry),
		depolarisation:  atmosphere.RayleighDepolarisation,
	}

	for i := range k.extinction {
		lambda := grid.At(i)

		rayleigh, err := atmosphere.RayleighOpticalDepth(lambda, float64(pressure))
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrScattering, err)
		}

		aer := unit.OpticalDepth(aerosol.TauAt(lambda))

		total := float64(rayleigh + aer)
		if total <= 0 {
			return nil, fmt.Errorf("%w: no extinction at %v", ErrScattering, lambda)
		}

		k.rayleighWeight[i] = float64(rayleigh) / total
		k.aerosolWeight[i] = float64(aer) / total
		k.extinction[i] = total

		// Aerosol removes light by absorption as well as scattering; the
		// single-scattering albedo is the share that scatters. Molecular
		// extinction at optical wavelengths is scattering outright.
		scattering := float64(rayleigh) + float64(aer)*float64(aerosol.SingleScatteringAlbedo)
		k.scatterFraction[i] = scattering / total
	}

	return k, nil
}

// pathFactor fills dst with (tau_sca/tau_ext) * M_v * P(tau, M_s, M_v) per
// wavelength: everything in the kernel that depends on the two airmasses.
//
// Constant around a ring of the quadrature, because the source airmass depends
// on altitude and not azimuth. That is where the saving is — a ring near the
// horizon carries dozens of azimuth samples and they all share this.
func (k *scatterKernel) pathFactor(dst []float64, airmassSource, airmassView float64) {
	// (e^{-tau*M_s} - e^{-tau*M_v}) / (M_v - M_s), written to stay accurate as
	// the two airmasses converge, exactly as
	// atmosphere.SingleScatteredRadiance writes it.
	u := airmassView - airmassSource

	for i, tau := range k.extinction {
		var pathIntegral float64

		switch tauU := tau * u; tauU {
		case 0:
			pathIntegral = tau * math.Exp(-tau*airmassSource)
		default:
			pathIntegral = math.Exp(-tau*airmassSource) * -math.Expm1(-tauU) / u
		}

		dst[i] = k.scatterFraction[i] * airmassView * pathIntegral
	}
}

// phaseAt returns the two phase-function values at a scattering angle, which
// are all that the angle contributes.
//
// Split from the combination because the combination is per wavelength and
// these are not: the phase functions depend on the angle alone, and only the
// weights that mix them vary across the band.
func (k *scatterKernel) phaseAt(theta float64) (rayleigh, aerosol float64, err error) {
	aerosol, err = atmosphere.HenyeyGreensteinPhaseFunction(theta, k.asymmetry)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: %w", ErrScattering, err)
	}

	return atmosphere.RayleighPhaseFunction(theta, k.depolarisation), aerosol, nil
}

// accumulate adds one source direction's contribution to dst.
//
// The innermost loop of the whole model: one multiply-add chain per
// wavelength, with every transcendental already evaluated by pathFactor and
// phaseAt.
func (k *scatterKernel) accumulate(
	dst, source, path []float64, dOmega, phaseRayleigh, phaseAerosol float64,
) {
	for i, radiance := range source {
		if radiance == 0 {
			continue
		}

		phase := k.rayleighWeight[i]*phaseRayleigh + k.aerosolWeight[i]*phaseAerosol
		dst[i] += radiance * dOmega * phase * path[i]
	}
}

// quadraturePoint is one sample of the hemispheric integral.
type quadraturePoint struct {
	ring   int
	alt    angle.Angle
	az     angle.Angle
	dOmega float64
}

// hemisphereQuadrature returns the sample points of the Eq. 11 integral.
//
// One definition, used by both [ScatteredIn] and the sampled field
// [Model.sampleHemisphere]. They have to agree exactly — the second exists to
// let the first be reused across view directions — and two copies of a
// quadrature are two chances for them to stop agreeing.
//
// Midpoint in zenith angle, so the outermost ring stays off the horizon where
// the airmass diverges and most models leave their stated validity domain. The
// number of azimuths in each ring is proportional to sin(z), so every sample
// carries roughly the same solid angle rather than the pole being oversampled.
//
// # One thing tried and rejected
//
// Rotating each ring by the golden angle, so samples do not stack into radial
// columns across rings. It is the standard fix for aliasing and it does not
// help here: measured over a sweep of a sharp feature across the sky, the
// coefficient of variation went from 3.97 per cent unrotated to 4.32 per cent
// rotated. The aliasing this rule suffers is in altitude, not azimuth — a
// feature crossing the sky cuts across rings — so decorrelating the azimuths
// addresses the wrong axis. TestScatteredInIsStableAsAFeatureMoves is the
// measurement, and it is kept so the next person to have the idea can see it
// was tried.
func hemisphereQuadrature(rings int) []quadraturePoint {
	if rings <= 0 {
		rings = DefaultScatteringRings
	}

	const halfPi = math.Pi / 2

	dz := halfPi / float64(rings)

	out := make([]quadraturePoint, 0, 4*rings*rings)

	for k := range rings {
		z := (float64(k) + 0.5) * dz
		alt := angle.Rad(halfPi - z)

		azimuths := max(4, int(math.Round(4*float64(rings)*math.Sin(z))))
		dOmega := math.Sin(z) * dz * (2 * math.Pi / float64(azimuths))

		for j := range azimuths {
			out = append(out, quadraturePoint{
				ring:   k,
				alt:    alt,
				az:     angle.Rad(2 * math.Pi * float64(j) / float64(azimuths)),
				dOmega: dOmega,
			})
		}
	}

	return out
}
