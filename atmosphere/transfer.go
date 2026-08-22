package atmosphere

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/unit"
)

// SingleScatteredRadiance returns the radiance a collimated source above the
// atmosphere delivers to a ground observer by scattering once into the line
// of sight.
//
// This is the standard single-scattering solution for a homogeneous
// plane-parallel atmosphere, derived here rather than transcribed. The
// derivation is short enough to state, and stating it is what makes the
// result checkable:
//
// Let x be the vertical extinction optical depth from the observer up to a
// scattering point. The beam reaching that point has crossed the remaining
// depth along the source's path, giving e^{-(tau-x)*M_s}. A fraction
// tau_sca/tau_ext of the extinction there is scattering, redistributed by
// the phase function p. The scattered light then descends through depth x
// along the line of sight, giving e^{-x*M_v}, and the path element per unit
// vertical depth is M_v. Integrating over the column:
//
//	L = E p (tau_sca/tau_ext) M_v INT(0..tau_ext) e^{-(tau_ext-x)M_s} e^{-x M_v} dx
//	  = E p (tau_sca/tau_ext) M_v (e^{-tau_ext*M_s} - e^{-tau_ext*M_v}) / (M_v - M_s)
//
// **This is single scattering only.** It is not Winkler's (2022) revised
// simplified scattering model, which adds empirically fitted
// multiple-scattering terms this package does not have; nor is it a
// full radiative-transfer solution. In a clear atmosphere at optical
// wavelengths single scattering carries most of the signal away from the
// source, but the deficit grows with optical depth and toward the horizon,
// and a caller must record that as an approximation rather than treat this
// as a complete answer.
//
// Airmasses replace the plane-parallel sec(z) of the textbook derivation,
// so the result stays finite at the horizon where sec(z) diverges; use
// [Airmass], which is Pickering (2002). Both must be at least 1.
//
// The source irradiance is measured normal to the beam at the top of the
// atmosphere. Radiance comes back in the same units per steradian, so a
// spectral irradiance in W m^-2 nm^-1 yields W m^-2 sr^-1 nm^-1.
//
// Two limits are worth knowing, and both are tested. As tau_ext approaches
// zero the result approaches E p tau_sca M_v, the textbook optically-thin
// single-scatter radiance — an independent check on the whole expression.
// And when the source and the line of sight share an airmass the quotient
// is a removable singularity, evaluated here as a series-safe form rather
// than a bare difference of nearly equal exponentials.
func SingleScatteredRadiance(
	sourceIrradiance, phaseFunction float64,
	scattering, extinction unit.OpticalDepth,
	airmassSource, airmassView float64,
) (float64, error) {
	if sourceIrradiance < 0 || math.IsNaN(sourceIrradiance) {
		return 0, fmt.Errorf("%w: source irradiance %g", ErrSourceIrradiance, sourceIrradiance)
	}

	if scattering < 0 || extinction <= 0 || scattering > extinction {
		return 0, fmt.Errorf("%w: scattering %g, extinction %g",
			ErrOpticalDepth, float64(scattering), float64(extinction))
	}

	if airmassSource < 1 || airmassView < 1 || math.IsNaN(airmassSource) || math.IsNaN(airmassView) {
		return 0, fmt.Errorf("%w: source %g, view %g", ErrAirmassRange, airmassSource, airmassView)
	}

	tau := float64(extinction)

	// (e^{-tau*M_s} - e^{-tau*M_v}) / (M_v - M_s), written to stay accurate
	// as the two airmasses converge. Factoring out e^{-tau*M_s} turns the
	// difference into -expm1(-tau*u), which loses no precision for small u.
	u := airmassView - airmassSource

	var pathIntegral float64

	switch tauU := tau * u; tauU {
	case 0:
		pathIntegral = tau * math.Exp(-tau*airmassSource)
	default:
		pathIntegral = math.Exp(-tau*airmassSource) * -math.Expm1(-tauU) / u
	}

	l := sourceIrradiance * phaseFunction * (float64(scattering) / tau) * airmassView * pathIntegral

	if math.IsNaN(l) || math.IsInf(l, 0) {
		return 0, fmt.Errorf("%w: M_s = %g, M_v = %g, tau = %g overflowed",
			ErrOpticalDepth, airmassSource, airmassView, tau)
	}

	return l, nil
}

// Standard atmospheric constants used by the scale-height relation.
const (
	// DryAirGasConstant is the specific gas constant of dry air,
	// J kg^-1 K^-1, from the ISO 2533 / US Standard Atmosphere 1976
	// composition.
	DryAirGasConstant = 287.052874

	// StandardGravity is standard acceleration due to gravity, m s^-2,
	// exact by the 3rd CGPM (1901) definition.
	StandardGravity = 9.80665
)

// MolecularScaleHeight returns the pressure scale height of the molecular
// atmosphere at temperature t, in metres.
//
// This is the hydrostatic relation for an isothermal ideal gas,
//
//	H = R_d * T / g
//
// which gives 8435 m at the 288.15 K standard temperature — the familiar
// "about 8.4 km". It is derived rather than tabulated so that a warm site
// and a cold one get different answers, which matters because the molecular
// term of [github.com/TuSKan/astrogo/skybrightness.OpticalParameterT] scales
// inversely with it.
//
// The isothermal assumption is the approximation here: the real troposphere
// has a lapse rate, so this overstates the scale height of the lowest few
// kilometres slightly. For the horizontal-path optical depths it feeds, that
// is well inside the uncertainty on the aerosol term beside it.
func MolecularScaleHeight(t unit.TemperatureK) (float64, error) {
	if !positiveFinite(float64(t)) {
		return 0, fmt.Errorf("%w: got %g K", ErrTemperature, float64(t))
	}

	return DryAirGasConstant * float64(t) / StandardGravity, nil
}

// Gushchin (1988) airmass constants, as used by Kocifaj & Bará (2019) Eq. 3.
const (
	gushchinScale  = 2.0016
	gushchinOffset = 0.003147
)

// GushchinAirmass returns the optical airmass at altitude alt using the
// formula Kocifaj & Bará (2019) Eq. 3 adopt, after Gushchin (1988):
//
//	M(h) = 2.0016 / (sin h + sqrt(sin^2 h + 0.003147))
//
// It gives 1.0000139 at the zenith and 35.68 at the horizon, where a
// plane-parallel sec(z) diverges. The scale that would make the zenith
// exactly one is 2.001572, so the published 2.0016 is that value rounded and
// the 1.4e-5 excess is the rounding rather than a property of the fit.
//
// This exists alongside [Airmass], which is Pickering (2002), because the
// artificial-skyglow model is calibrated against *this* formula: its
// two-index fit, its horizon limit and its optical parameter t all assume
// M(horizon) is about 35, not Pickering's 38. Mixing the two would shift the
// kernel by roughly a tenth in the airmass that matters most. Use Pickering
// for general extinction work and this one only inside that model.
func GushchinAirmass(alt angle.Angle) (float64, error) {
	sinH := alt.Sin()
	if sinH < 0 {
		return 0, fmt.Errorf("%w: altitude %g deg is below the horizon", ErrAirmassRange, alt.Degrees())
	}

	return gushchinScale / (sinH + math.Sqrt(sinH*sinH+gushchinOffset)), nil
}

// MultipleScatteringFactor returns the ratio of total to singly scattered
// radiance, Winkler (2022) Section 5.2:
//
//	f = 1 + 4.5 * tau_R
//
// Single scattering underestimates sky brightness because photons scattered
// twice or more still reach the observer. Winkler quantifies the shortfall
// against his SAAO measurements as proportional to the molecular optical
// depth, revising the coefficient of Noll et al. (2012) — who give
// f = 1 + 2.2*tau_R — to 4.5, which he states better matches both his
// measured values and the most likely single-scattering albedos.
//
// It is deliberately a function of the molecular depth alone. The aerosol
// term is not simply scattering: Winkler notes a larger share of the Mie
// optical depth may be absorption than usually assumed, most noticeably at
// long wavelengths, so folding it in would overstate the correction exactly
// where it is least certain.
//
// This is a broadband empirical correction fitted at one site under low
// aerosol loading, not a radiative-transfer solution. It moves a
// single-scattering estimate toward the truth; it does not make it exact.
func MultipleScatteringFactor(rayleigh unit.OpticalDepth) (float64, error) {
	if rayleigh < 0 || math.IsNaN(float64(rayleigh)) {
		return 0, fmt.Errorf("%w: rayleigh %g", ErrOpticalDepth, float64(rayleigh))
	}

	return 1 + 4.5*float64(rayleigh), nil
}

// Airglow layer geometry.
const (
	// VanRhijnEarthRadiusKM is the Earth radius Leinert et al. (1998) Eq. 13
	// uses, in kilometres.
	VanRhijnEarthRadiusKM = 6378.0

	// AirglowLayerHeightM is the height of the emitting layer adopted by
	// Masana et al. (2021) after Hart (2019), in metres.
	//
	// The choice only matters near the horizon, and it is a simplification:
	// the OH, O2 and Na emissions arise near 90 km while the OI 630 nm lines
	// come from 200 to 300 km, so no single height describes the whole
	// spectrum. A line-dominated band evaluated at this height is wrong at
	// large zenith angles.
	AirglowLayerHeightM = 87_000.0
)

// VanRhijn returns the brightness of a thin, uniformly emitting atmospheric
// layer at zenith angle z, relative to its brightness at the zenith.
//
// Leinert et al. (1998) Eq. 13, after van Rhijn (1921):
//
//	I(z)/I(0) = 1 / sqrt(1 - [R/(R+h)]^2 * sin^2 z)
//
// A layer seen obliquely is thicker along the line of sight, so airglow
// brightens toward the horizon even with no scattering at all — the opposite
// of how an extinguished source behaves, and the reason airglow cannot be
// treated as a constant floor.
//
// This is the geometry only. Extinction and scattering along the longer path
// work against it, and Leinert et al. note they change the behaviour
// materially beyond about 40 degrees from the zenith; applying this factor
// alone overstates the horizon brightness.
//
// For h = 100 km the maximum is 5.7 at the horizon (Roach & Meinel 1955),
// which TestVanRhijnAgainstRoachAndMeinel checks.
func VanRhijn(z angle.Angle, layerHeightM float64) (float64, error) {
	if layerHeightM <= 0 || math.IsNaN(layerHeightM) || math.IsInf(layerHeightM, 0) {
		return 0, fmt.Errorf("%w: layer height %g m", ErrScaleHeightRange, layerHeightM)
	}

	radius := VanRhijnEarthRadiusKM * 1000
	ratio := radius / (radius + layerHeightM)

	sinZ := z.Sin()

	denom := 1 - ratio*ratio*sinZ*sinZ
	if denom <= 0 {
		return 0, fmt.Errorf("%w: zenith angle %g deg is past the layer's limb",
			ErrAirmassRange, z.Degrees())
	}

	return 1 / math.Sqrt(denom), nil
}

// Scale heights and the default effective-depth factor for extended sources,
// from Masana et al. (2021) Section 7.
const (
	// MolecularScaleHeightM is the exponential scale height of the molecular
	// atmosphere, Masana et al. (2021) Eq. 25.
	MolecularScaleHeightM = 8000.0

	// AerosolScaleHeightM is the same for aerosols, which are concentrated far
	// nearer the ground — a difference that only shows for an observer above
	// sea level, where a smaller fraction of the aerosol column remains
	// overhead than of the molecular one.
	AerosolScaleHeightM = 1540.0

	// DefaultDiffuseKappa is the effective-depth factor applied to sources
	// that fill the sky, and it is the value the GAMBONS web service uses.
	//
	// Hong et al. (1998) put the range at 0.5 to 0.9 depending on the aerosol
	// albedo and asymmetry parameter; Duriscoe (2013) uses 0.75 after Kwon
	// (1989). Set it per scene with Builder.DiffuseScattering rather than
	// treating this default as a physical constant.
	DefaultDiffuseKappa = 0.5
)

// ExtendedSourceOpticalDepth returns the slant optical depth an extended
// source is attenuated by, following Masana et al. (2021) Eq. 29.
//
//	tau(lambda, z; h) = kappa * [ tau_M(lambda) m_M(z) exp(-h/H_M)
//	                            + tau_A(lambda) m_A(z) exp(-h/H_A) ]
//
// Two things separate this from multiplying a vertical depth by an airmass.
//
// The molecular and aerosol columns are carried separately, each with its own
// scale height, so an observer above sea level sees the correct fraction of
// each remaining overhead rather than the same fraction of both. At sea level
// the exponentials are one and this reduces to the familiar product.
//
// And kappa scales the whole thing, because a source covering the sky does not
// lose everything scattered out of the line of sight: it is replaced by light
// scattered in from the rest of the sky. See [Atmosphere.DiffuseKappa].
//
// The airmasses are passed in rather than computed here so the caller chooses
// the law. Masana et al. use Kasten & Young (1989); this module's [Airmass] is
// Pickering (2002), and the two agree to better than three parts in a thousand
// above five degrees of altitude, diverging only in the last degrees where
// Pickering is the better behaved.
func ExtendedSourceOpticalDepth(
	rayleigh, aerosol unit.OpticalDepth,
	molecularAirmass, aerosolAirmass float64,
	observerHeightM, kappa float64,
) (unit.OpticalDepth, error) {
	switch {
	case rayleigh < 0 || math.IsNaN(float64(rayleigh)) || math.IsInf(float64(rayleigh), 0):
		return 0, fmt.Errorf("%w: rayleigh %g", ErrOpticalDepth, float64(rayleigh))
	case aerosol < 0 || math.IsNaN(float64(aerosol)) || math.IsInf(float64(aerosol), 0):
		return 0, fmt.Errorf("%w: aerosol %g", ErrOpticalDepth, float64(aerosol))
	case molecularAirmass < 1 || aerosolAirmass < 1:
		return 0, fmt.Errorf("%w: airmass %g/%g is below one",
			ErrAirmassRange, molecularAirmass, aerosolAirmass)
	case observerHeightM < 0 || math.IsNaN(observerHeightM):
		return 0, fmt.Errorf("%w: observer height %g m", ErrOpticalDepth, observerHeightM)
	case kappa <= 0 || kappa > 1:
		return 0, fmt.Errorf("%w: kappa %g is outside (0, 1]", ErrOpticalDepth, kappa)
	}

	molecular := float64(rayleigh) * molecularAirmass * math.Exp(-observerHeightM/MolecularScaleHeightM)
	particulate := float64(aerosol) * aerosolAirmass * math.Exp(-observerHeightM/AerosolScaleHeightM)

	return unit.OpticalDepth(kappa * (molecular + particulate)), nil
}
