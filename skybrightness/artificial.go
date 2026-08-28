package skybrightness

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/unit"
)

// Sentinel errors for the artificial-skyglow propagation kernel.
var (
	// ErrAirmass is returned for a non-positive or non-finite airmass.
	ErrAirmass = errors.New("skybrightness: airmass must be positive and finite")

	// ErrOpticalParameter is returned for a non-positive or non-finite
	// value of the atmospheric parameter t.
	ErrOpticalParameter = errors.New("skybrightness: optical parameter t must be positive and finite")

	// ErrScaleHeight is returned for a non-positive scale height.
	ErrScaleHeight = errors.New("skybrightness: scale height must be positive and finite")

	// ErrSeparation is returned for a negative or non-finite source-observer
	// separation.
	ErrSeparation = errors.New("skybrightness: separation must be non-negative and finite")

	// ErrAsymmetryRange is returned for an asymmetry parameter outside
	// (-1, 1), where the anisotropy factor is undefined.
	ErrAsymmetryRange = errors.New("skybrightness: asymmetry parameter must lie in (-1, 1)")

	// ErrOpticalDepth is returned for a negative optical depth.
	ErrOpticalDepth = errors.New("skybrightness: optical depth must be non-negative")

	// ErrAsymmetryOutOfRange is returned when Eq. 4 evaluates to an
	// asymmetry parameter outside (-1, 1), where a Henyey-Greenstein phase
	// function is undefined. The fit is empirical and does not enforce that
	// bound itself.
	ErrAsymmetryOutOfRange = errors.New("skybrightness: fitted asymmetry parameter left the physical range")
)

// Artificial skyglow propagation, after Kocifaj, Bará & Falchi (2022).
//
//   - Model: semi-analytic two-parameter all-sky radiance from a ground
//     source, generalising Kocifaj & Bará (2019) to include scattering
//     orders up to the fifth.
//   - Primary reference: Kocifaj, M., Bará, S. & Falchi, F. (2022),
//     "Towards a global map of the artificial all-sky brightness",
//     MNRAS Letters 513, L25-L29; arXiv:2203.09322.
//   - Equations implemented: Eq. 2 (all-sky radiance) and Eq. 3 (the
//     atmospheric parameter t). Eq. 1 (the combined phase function) lives
//     in atmosphere as CombinedPhaseFunction, shared with the lunar model.
//   - Units: radiance in the same units as the supplied source radiance;
//     t dimensionless.
//   - Validity: the g parameterisation was solved at 550 nm and 450 nm and
//     is stated to represent a band roughly 20-30 nm wide. The model is
//     therefore NOT spectrally resolved to the same degree as the rest of
//     this module, which its provenance records.
//
// The whole-sky contribution of a real environment is the sum over
// surrounding sources. Within the model's validity, sources at equal
// distance but different azimuth produce rotated copies of one pattern,
// weighted by the radiance each emits toward the observer — which is what
// makes a global map tractable.

// AllSkyRadiance returns the artificial sky radiance in one viewing
// direction from one ground source, Kocifaj, Bará & Falchi (2022) Eq. 2:
//
//	L(z,A) = L_S · P(g,θ) · (1-g)²/(1+g) · M(z)/(M_S·t)
//	         · (e^{[M_S-M(z)]·t} - 1) / (M_S - M(z))
//
// z and A are the observing zenith and azimuth angles, M(z) the airmass
// along the line of sight, M_S the airmass toward the source, P the
// combined phase function of atmosphere.CombinedPhaseFunction (Eq. 1) at
// scattering angle θ, g the asymmetry parameter, and t the atmospheric
// parameter of [OpticalParameterT] (Eq. 3).
//
// **sourceRadiance is L_S as it reaches the observer, not as it leaves the
// source.** Eq. 2 contains no distance term of its own — the paper's own
// horizon limit, which this reduces to exactly, is L_S·P·(1-g)²/(1+g) with
// no attenuation at all. Distance enters twice, and both are the caller's
// responsibility: through t, which Eq. 3 makes proportional to the
// source-observer separation D, and through L_S, which must already carry
// the transmission e^{-M_S·t} over that separation.
//
// Getting that contract wrong is not a small error. The kernel alone grows
// with M_S·t, so holding L_S fixed while increasing distance makes a
// distant city come out brighter than a near one — by two orders of
// magnitude at 80 km against 20 km under ordinary aerosol loading. Applying
// the transmission that belongs with it restores the expected fall-off, as
// TestKocifaj2022Eq2FallsWithDistance checks. An earlier revision of this
// package withdrew the kernel over exactly that behaviour before the
// contract was understood; the equation was right and the reading of it
// was wrong.
//
// The removable singularity at M_S = M(z) — looking at the horizon in the
// source's azimuth, where the two airmasses coincide — is evaluated as
// t·expm1(u·t)/(u·t) with u = M_S - M(z), which stays accurate as u
// approaches zero where a bare (e^{u·t}-1)/u loses all precision.
//
// Radiance is returned in whatever units sourceRadiance carries.
func AllSkyRadiance(
	sourceRadiance, phaseFunction, g float64,
	airmassSource, airmassView float64,
	t float64,
) (float64, error) {
	if sourceRadiance < 0 || math.IsNaN(sourceRadiance) {
		return 0, fmt.Errorf("%w: source radiance %g", ErrNegativeRadiance, sourceRadiance)
	}

	if g <= -1 || g >= 1 || math.IsNaN(g) {
		return 0, fmt.Errorf("%w: got %g", ErrAsymmetryRange, g)
	}

	if airmassSource < 1 || airmassView < 1 || math.IsNaN(airmassSource) || math.IsNaN(airmassView) {
		return 0, fmt.Errorf("%w: source %g, view %g", ErrAirmass, airmassSource, airmassView)
	}

	if t <= 0 || math.IsInf(t, 0) || math.IsNaN(t) {
		return 0, fmt.Errorf("%w: t = %g", ErrOpticalParameter, t)
	}

	// The anisotropy factor. Eq. 2 carries it separately from the phase
	// function: P describes the angular shape of a single scattering event,
	// while (1-g)²/(1+g) accounts for the higher orders the model folds in.
	anisotropy := (1 - g) * (1 - g) / (1 + g)

	// (e^{u·t} - 1)/u, written so that u = 0 is exact rather than a limit.
	u := airmassSource - airmassView

	var pathIntegral float64

	switch ut := u * t; ut {
	case 0:
		pathIntegral = t
	default:
		pathIntegral = t * math.Expm1(ut) / ut
	}

	l := sourceRadiance * phaseFunction * anisotropy * airmassView / (airmassSource * t) * pathIntegral

	if math.IsNaN(l) || math.IsInf(l, 0) {
		return 0, fmt.Errorf("%w: M_S = %g, M(z) = %g, t = %g overflowed",
			ErrOpticalParameter, airmassSource, airmassView, t)
	}

	return l, nil
}

// OpticalParameterT returns the atmospheric parameter t of Kocifaj, Bará &
// Falchi (2022).
//
// Eq. 3:
//
//	t = (τ_a/H_a + τ_R/H_R) · D / M_S
//
// where τ_a and H_a are the vertical aerosol optical thickness and aerosol
// scale height, τ_R and H_R the molecular equivalents, D the horizontal
// separation between source and observer, and M_S the airmass toward the
// source.
//
// The product of t and M_S is what sets the optical transmission
// e^{-M_S·t} of the air column between source and observer, which is the
// physical meaning the paper attaches to it. Eq. 3 assumes an exponential
// atmosphere: aerosol extinction falling as e^{-h/H_a} and molecular
// extinction as e^{-h/H_R}.
//
// Distances and scale heights must share units; the paper works in
// kilometres.
func OpticalParameterT(
	aerosolOpticalDepth, aerosolScaleHeight unit.OpticalDepth,
	molecularOpticalDepth, molecularScaleHeight unit.OpticalDepth,
	separation, airmassSource float64,
) (float64, error) {
	switch {
	case !positiveFinite(float64(aerosolScaleHeight)):
		return 0, fmt.Errorf("%w: aerosol %g", ErrScaleHeight, float64(aerosolScaleHeight))
	case !positiveFinite(float64(molecularScaleHeight)):
		return 0, fmt.Errorf("%w: molecular %g", ErrScaleHeight, float64(molecularScaleHeight))
	case !positiveFinite(airmassSource):
		return 0, fmt.Errorf("%w: source airmass %g", ErrAirmass, airmassSource)
	case separation < 0 || math.IsNaN(separation) || math.IsInf(separation, 0):
		return 0, fmt.Errorf("%w: got %g", ErrSeparation, separation)
	case aerosolOpticalDepth < 0 || molecularOpticalDepth < 0:
		return 0, fmt.Errorf("%w: aerosol %g, molecular %g", ErrOpticalDepth,
			float64(aerosolOpticalDepth), float64(molecularOpticalDepth))
	}

	extinctionPerLength := float64(aerosolOpticalDepth)/float64(aerosolScaleHeight) +
		float64(molecularOpticalDepth)/float64(molecularScaleHeight)

	return extinctionPerLength * separation / airmassSource, nil
}

// positiveFinite reports whether v is positive and finite.
func positiveFinite(v float64) bool {
	return v > 0 && !math.IsInf(v, 0) && !math.IsNaN(v)
}

// AsymmetryParameter returns the atmospheric asymmetry parameter g from
// the aerosol asymmetry parameter and the aerosol optical thickness.
//
// Kocifaj, Bará & Falchi (2022) Eq. 4:
//
//	g = c0 + c1·g_a + c2·g_a²
//
// with Eq. 5:
//
//	c0 = 0.33 + 0.15·tau_a
//	c1 = 0.9·tau_a^0.51
//	c2 = 1.3·tau_a^1.85
//
// The constant 0.33 is the paper's own asymptote: as the aerosol optical
// thickness approaches zero, g tends to 0.33 rather than to zero, because
// a multiply-scattering molecular atmosphere is still anisotropic. That
// excludes isotropic scattering even in the cleanest air, and it is what
// TestKocifaj2022Eq4CleanAtmosphereAsymptote checks.
//
// The fit is empirical and is not bounded to the physical range on its
// own: at high aerosol loading combined with a strongly forward-scattering
// aerosol — around tau_a = 0.5 with g_a = 0.9 — it evaluates above 1,
// where a Henyey-Greenstein phase function is undefined. That is a real
// limit of the published parameterisation, not of this implementation, so
// the value is returned alongside ErrAsymmetryOutOfRange rather than being
// silently clamped: a caller in that regime needs to know their inputs
// left the fit's domain.
func AsymmetryParameter(aerosolAsymmetry unit.AsymmetryParameter, aerosolOpticalDepth unit.OpticalDepth) (float64, error) {
	if aerosolOpticalDepth < 0 {
		return 0, fmt.Errorf("%w: aerosol %g", ErrOpticalDepth, float64(aerosolOpticalDepth))
	}

	tau := float64(aerosolOpticalDepth)
	ga := float64(aerosolAsymmetry)

	c0 := 0.33 + 0.15*tau
	c1 := 0.9 * math.Pow(tau, 0.51)
	c2 := 1.3 * math.Pow(tau, 1.85)

	g := c0 + c1*ga + c2*ga*ga

	if g <= -1 || g >= 1 || math.IsNaN(g) {
		return g, fmt.Errorf("%w: g = %g for tau_a = %g, g_a = %g", ErrAsymmetryOutOfRange, g, tau, ga)
	}

	return g, nil
}
