package atmosphere

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/unit"
)

// Sentinel errors for the scattering layer.
var (
	// ErrWavelength is returned for a non-positive or non-finite wavelength.
	ErrWavelength = errors.New("atmosphere: wavelength must be positive and finite")

	// ErrPressure is returned for a non-positive or non-finite pressure.
	ErrPressure = errors.New("atmosphere: pressure must be positive and finite")

	// ErrAsymmetry is returned for a Henyey-Greenstein asymmetry parameter
	// outside (-1, 1), where the phase function is singular or undefined.
	ErrAsymmetry = errors.New("atmosphere: asymmetry parameter must lie in (-1, 1)")

	// ErrOpticalDepth is returned for a negative optical depth, or for a
	// combined phase function with no scattering at all.
	ErrOpticalDepth = errors.New("atmosphere: optical depth must be non-negative and not identically zero")

	// ErrSourceIrradiance is returned for a negative or non-finite source
	// irradiance.
	ErrSourceIrradiance = errors.New("atmosphere: source irradiance must be non-negative and finite")

	// ErrAirmassRange is returned for an airmass below 1, which no real
	// line of sight through the atmosphere can have.
	ErrAirmassRange = errors.New("atmosphere: airmass must be at least 1")

	// ErrTemperature is returned for a non-positive or non-finite
	// temperature.
	ErrTemperature = errors.New("atmosphere: temperature must be positive and finite")

	// ErrScaleHeightRange is returned for a non-positive or non-finite layer
	// height or scale height.
	ErrScaleHeightRange = errors.New("atmosphere: layer height must be positive and finite")
)

// Scattering reference values and their provenance.
//
//   - Model: molecular (Rayleigh) and aerosol (Mie) single scattering.
//   - Primary reference: Winkler, H. (2022), "A revised simplified
//     scattering model for the moonlit sky brightness profile based on
//     photometry at SAAO", MNRAS 514, 208-226, doi:10.1093/mnras/stac1387.
//   - Secondary: Bucholtz, A. (1995), Appl. Opt. 34, 2765 (Rayleigh phase
//     function and depolarisation); Bodhaine, B. A., Wood, N. B., Dutton,
//     E. G. & Slusser, J. R. (1999), J. Atmos. Oceanic Technol. 16, 1854
//     (Rayleigh optical depth); Henyey, L. G. & Greenstein, J. L. (1941),
//     ApJ 93, 70.
//   - Equations implemented: Winkler (2022) Eq. 9, 10, 12; Bodhaine et al.
//     (1999) Eq. 30.
//   - Units: optical depth dimensionless; phase functions sr^-1.
//   - Validity: optical wavelengths; see each function.
//
// The phase function and depolarisation are the Bucholtz lineage Winkler
// (2022) uses, shared with the artificial-skyglow model (Kocifaj, Bara &
// Falchi 2022), so the two components propagate through one atmosphere.
// The Rayleigh optical depth is Bodhaine et al. (1999), for both at once:
// Winkler's own Eq. 13, a single power law after Dutton et al. (1994), read
// 4.2% low at 330 nm (#592).
const (
	// StandardPressureHPa is the sea-level pressure Bodhaine et al. (1999)
	// normalize the Rayleigh optical depth to, the standard atmosphere's
	// 1013.25 hPa (equivalently millibars). It was Winkler's (2022)
	// 1013.5 until #592.
	StandardPressureHPa = 1013.25

	// RayleighDepolarisation is the air depolarisation factor rho adopted
	// by Winkler (2022) after Bucholtz (1995), whose tabulated values span
	// 0.01384 to 0.01557 across the wavelength range studied. Winkler
	// adopts 0.0148, for which (1+3rho)/(1-rho) = 1.06 — the same
	// coefficient Krisciunas & Schaefer (1991) had fitted empirically.
	RayleighDepolarisation = 0.0148

	// rayleighMinNM and rayleighFitMaxNM bound the wavelengths Bodhaine et
	// al. (1999) fitted Eq. 30 over, 250 to 1000 nm.
	rayleighMinNM    = 250
	rayleighFitMaxNM = 1000
)

// RayleighOpticalDepth returns the vertical molecular optical depth at
// wavelength lambda for a site at pressure pressureHPa: Bodhaine et al.
// (1999) Eq. 30, for dry air with 360 ppm CO2 at sea level and 45°
// latitude, scaled by the site's pressure,
//
//	tau_R = (P / 1013.25) * 0.0021520 * (1.0455996 - 341.29061 L^-2 - 0.90230850 L^2)
//	                                 / (1 + 0.0027059889 L^-2 - 85.968563 L^2)
//
// with L the wavelength in micrometers. The fit reproduces their exactly
// computed Table 3 to better than 0.05% from 250 to 1000 nm. Its local
// exponent steepens toward the blue as the refractive index and the
// depolarisation of air both change with wavelength, which no single power
// law follows. Winkler's (2022) Eq. 13, (P/1013.5)·1.229e10·λ^−4.05 after
// Dutton et al. (1994), read 4.2% low at 330 nm, and its coefficient sits
// 0.8% below the 0.00877 µm^4.05 Bodhaine quotes from Dutton (#592).
//
// The pressure ratio scales the column with the mass of air above the site,
// at the gravity of 45° and sea level. Bodhaine's own Mauna Loa column,
// 19.5°N and 3.4 km up at 680 mb, is 0.29% larger than that scaling gives,
// which is the size of what holding gravity fixed leaves out.
//
// Beyond 1000 nm, where the fit was not made and its rational form drifts
// from λ^-4, it continues as λ^-4 from its 1000 nm value: the dispersion
// that steepens the law in the blue is a 1% correction there and shrinks as
// λ^-2. Below 250 nm it refuses with [ErrWavelength]; the fit's
// denominator vanishes near 117 nm.
func RayleighOpticalDepth(lambda unit.WavelengthNM, pressureHPa float64) (unit.OpticalDepth, error) {
	if !positiveFinite(float64(lambda)) || lambda < rayleighMinNM {
		return 0, fmt.Errorf("%w: got %g nm, want %d nm or more", ErrWavelength, float64(lambda), rayleighMinNM)
	}

	if !positiveFinite(pressureHPa) {
		return 0, fmt.Errorf("%w: got %g hPa", ErrPressure, pressureHPa)
	}

	scale := 1.0
	if lambda > rayleighFitMaxNM {
		scale = math.Pow(rayleighFitMaxNM/float64(lambda), 4)
		lambda = rayleighFitMaxNM
	}

	l2 := (float64(lambda) / 1000) * (float64(lambda) / 1000)
	seaLevel := 0.0021520 * (1.0455996 - 341.29061/l2 - 0.90230850*l2) / (1 + 0.0027059889/l2 - 85.968563*l2)

	return unit.OpticalDepth(pressureHPa / StandardPressureHPa * seaLevel * scale), nil
}

// RayleighPhaseFunction returns the normalized molecular scattering phase
// function at scattering angle theta, in sr^-1.
//
// Winkler (2022) Eq. 9, after Bucholtz (1995):
//
//	p_R(theta) = 3(1-rho) / (16*pi*(1+2rho)) * [ (1+3rho)/(1-rho) + cos^2(theta) ]
//
// theta is the angle between the incident and scattered directions, so
// theta = 0 is forward scattering. The depolarisation term rho accounts for
// the anisotropy of air molecules; setting it to zero recovers the ideal
// dipole form (1 + cos^2)*3/(16*pi).
//
// The function integrates to exactly 1 over the sphere, which
// TestRayleighPhaseFunctionNormalization verifies.
func RayleighPhaseFunction(theta float64, depolarisation float64) float64 {
	rho := depolarisation
	cosT := math.Cos(theta)

	prefactor := 3 * (1 - rho) / (16 * math.Pi * (1 + 2*rho))

	return prefactor * ((1+3*rho)/(1-rho) + cosT*cosT)
}

// HenyeyGreensteinPhaseFunction returns the normalized aerosol scattering
// phase function at scattering angle theta, in sr^-1.
//
// Winkler (2022) Eq. 10, after Henyey & Greenstein (1941):
//
//	p_M(theta) = (1 - g^2) / (4*pi * (1 + g^2 - 2g*cos(theta))^(3/2))
//
// g is the asymmetry parameter: 0 is isotropic, positive is
// forward-peaked, and Earth's atmospheric aerosols are typically around
// 0.5. This one-parameter form is used rather than a full Mie calculation
// because it is what both adopted models specify — Winkler (2022) for
// moonlight and Kocifaj, Bara & Falchi (2022) for artificial skyglow —
// and because a Mie computation needs a size distribution and refractive
// index that operational aerosol products do not supply.
//
// Returns an error for |g| >= 1, where the expression is singular.
func HenyeyGreensteinPhaseFunction(theta float64, g float64) (float64, error) {
	if g <= -1 || g >= 1 || math.IsNaN(g) {
		return 0, fmt.Errorf("%w: got %g", ErrAsymmetry, g)
	}

	denom := 1 + g*g - 2*g*math.Cos(theta)

	return (1 - g*g) / (4 * math.Pi * math.Pow(denom, 1.5)), nil
}

// CombinedPhaseFunction returns the total scattering phase function of a
// molecular-plus-aerosol atmosphere at scattering angle theta, in sr^-1.
//
// Winkler (2022) Eq. 12:
//
//	p(theta) = (tau_R/tau_s) * p_R(theta) + (tau_M/tau_s) * p_M(theta)
//
// with tau_s = tau_R + tau_M. The two mechanisms are weighted by their
// share of the scattering optical depth, so a clean high site is
// Rayleigh-dominated and forward-scattering grows with aerosol loading.
//
// Kocifaj, Bara & Falchi (2022) Eq. 1 expresses the same combination
// weighted by volume scattering coefficients with the aerosol term scaled
// by its single-scattering albedo; over a homogeneous column the two
// reduce to the same thing, which is why one function serves both models.
//
// Both optical depths must be non-negative and not both zero.
func CombinedPhaseFunction(theta float64, rayleigh, aerosol unit.OpticalDepth, g float64, depolarisation float64) (float64, error) {
	if rayleigh < 0 || aerosol < 0 {
		return 0, fmt.Errorf("%w: rayleigh %g, aerosol %g", ErrOpticalDepth, float64(rayleigh), float64(aerosol))
	}

	total := float64(rayleigh + aerosol)
	if total <= 0 {
		return 0, fmt.Errorf("%w: both are zero", ErrOpticalDepth)
	}

	pm, err := HenyeyGreensteinPhaseFunction(theta, g)
	if err != nil {
		return 0, err
	}

	pr := RayleighPhaseFunction(theta, depolarisation)

	return (float64(rayleigh)/total)*pr + (float64(aerosol)/total)*pm, nil
}

// Transmission converts a slant optical depth into a transmission
// fraction, T = exp(-tau).
//
// The slant optical depth is the vertical optical depth times the airmass;
// [Airmass] supplies the latter using Pickering (2002), which stays
// well-behaved to the horizon where a plane-parallel sec(z) diverges.
func Transmission(slant unit.OpticalDepth) unit.Transmission {
	return slant.ToTransmission()
}

// positiveFinite reports whether v is positive and finite.
func positiveFinite(v float64) bool {
	return v > 0 && !math.IsInf(v, 0) && !math.IsNaN(v)
}
