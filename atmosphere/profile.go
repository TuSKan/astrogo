package atmosphere

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/unit"
)

// Sentinel errors for the vertical-profile primitives.
var (
	// ErrScaleHeight is returned when a scale height is not positive and
	// finite.
	ErrScaleHeight = errors.New("atmosphere: scale height must be positive and finite")

	// ErrColumnDepth is returned when a column optical depth is negative or
	// not finite.
	ErrColumnDepth = errors.New("atmosphere: column optical depth must be non-negative and finite")

	// ErrAltitude is returned when an altitude is negative or not finite.
	ErrAltitude = errors.New("atmosphere: altitude must be non-negative and finite")
)

// ExponentialExtinction returns the volume extinction coefficient at an
// altitude, in m^-1, for a constituent whose density falls exponentially.
//
//	k(h) = (tau_0 / H) * exp(-h / H)
//
// where tau_0 is the whole vertical column and H the scale height. The
// normalisation is what makes the two arguments mean what they say: the
// integral of k from the ground to infinity is exactly tau_0, so a caller
// who knows the column and the scale height has fully specified the profile
// and cannot accidentally describe an atmosphere with more or less
// extinction than they asked for.
//
// # Why an exponential rather than a layered profile
//
// Because it is what the models that consume it specify. Kocifaj (2007)
// Eq. 36 writes the molecular term as (tau_M,0 / h_0) exp(-h/h_0) and the
// aerosol term as beta * tau_A,0 * exp(-beta*h), which is this same function
// with H = 1/beta. Molecular density really is close to exponential over the
// optical path that matters here, and aerosol is not, but a single decay rate
// is the only thing an operational aerosol product supplies. A layered
// profile is a different capability, not a refinement of this one, and
// [VerticalProfile] is where it will go.
func ExponentialExtinction(
	altitudeM unit.AltitudeM, column unit.OpticalDepth, scaleHeightM float64,
) (float64, error) {
	if err := checkProfile(altitudeM, column, scaleHeightM); err != nil {
		return 0, err
	}

	return float64(column) / scaleHeightM * math.Exp(-float64(altitudeM)/scaleHeightM), nil
}

// ExponentialDepth returns the vertical optical depth between the ground and
// an altitude, for the same profile [ExponentialExtinction] describes.
//
//	tau(0, h) = tau_0 * (1 - exp(-h / H))
//
// This is the integral of that function, and the pair has to be used
// together: a transmission computed from a depth that does not integrate its
// own extinction is an atmosphere that absorbs at one rate and transmits at
// another. It tends to tau_0 as h grows, so the whole column is recovered at
// the top rather than approached from above.
func ExponentialDepth(
	altitudeM unit.AltitudeM, column unit.OpticalDepth, scaleHeightM float64,
) (unit.OpticalDepth, error) {
	if err := checkProfile(altitudeM, column, scaleHeightM); err != nil {
		return 0, err
	}

	return column * unit.OpticalDepth(1-math.Exp(-float64(altitudeM)/scaleHeightM)), nil
}

// checkProfile validates the arguments both profile functions share.
func checkProfile(altitudeM unit.AltitudeM, column unit.OpticalDepth, scaleHeightM float64) error {
	switch {
	case scaleHeightM <= 0 || math.IsInf(scaleHeightM, 0) || math.IsNaN(scaleHeightM):
		return fmt.Errorf("%w: got %g m", ErrScaleHeight, scaleHeightM)

	case column < 0 || math.IsInf(float64(column), 0) || math.IsNaN(float64(column)):
		return fmt.Errorf("%w: got %g", ErrColumnDepth, float64(column))

	case altitudeM < 0 || math.IsInf(float64(altitudeM), 0) || math.IsNaN(float64(altitudeM)):
		return fmt.Errorf("%w: got %g m", ErrAltitude, float64(altitudeM))
	}

	return nil
}

// VolumeScatteringFunction returns the angular volume scattering coefficient
// of a molecular-plus-aerosol atmosphere, in m^-1 sr^-1.
//
//	beta(theta) = k_sca,M * p_M(theta) + k_sca,A * p_A(theta)
//
// This is the local counterpart of [CombinedPhaseFunction]. That one returns
// a normalised phase function, weighted by each mechanism's share of the
// column; this one keeps the dimensional volume scattering coefficients and
// so answers a different question — not "in what direction does this
// atmosphere scatter" but "how much light does this cubic metre of it put
// into this solid angle". A height integral needs the second.
//
// Kocifaj (2007) Eq. 18 is exactly this quantity, written there as
// Psi(h, z, phi) and given equivalently in terms of extinction coefficients
// scaled by single-scattering albedo, since k_sca = omega * k_ext. Pass
// scattering coefficients and the two forms agree.
//
// # The factor of 4*pi that is not here
//
// Eq. 18 reads (1/4*pi)[k_sca,M * P_M + k_sca,A * P_A], and this has no such
// factor. The two agree because the normalisations differ: Kocifaj's P
// satisfies the condition he states after his Eq. 3, that P integrates to
// 4*pi over the sphere, while [RayleighPhaseFunction] and
// [HenyeyGreensteinPhaseFunction] integrate to 1 and carry the 1/4*pi
// internally. So P = 4*pi * p, and the explicit factor cancels the one
// already inside. Transcribing the equation literally on top of these
// functions would divide the whole model by 4*pi — a wrong answer that is
// smooth, positive and out by a constant, which is the kind that survives
// every plausibility check.
//
// The molecular phase function is Rayleigh at the given depolarisation and
// the aerosol one is Henyey-Greenstein at asymmetry g, matching every other
// scattering path in this package.
func VolumeScatteringFunction(
	theta float64, molecularScatter, aerosolScatter float64, g, depolarisation float64,
) (float64, error) {
	switch {
	case molecularScatter < 0 || math.IsNaN(molecularScatter) || math.IsInf(molecularScatter, 0):
		return 0, fmt.Errorf("%w: molecular %g m^-1", ErrColumnDepth, molecularScatter)

	case aerosolScatter < 0 || math.IsNaN(aerosolScatter) || math.IsInf(aerosolScatter, 0):
		return 0, fmt.Errorf("%w: aerosol %g m^-1", ErrColumnDepth, aerosolScatter)
	}

	// Each term carries its own phase function rather than sharing an
	// averaged one: Rayleigh is symmetric about 90 degrees and aerosol is
	// forward-peaked, so averaging first and weighting after would move light
	// from one angular regime into the other.
	rayleigh := RayleighPhaseFunction(theta, depolarisation)

	aerosol, err := HenyeyGreensteinPhaseFunction(theta, g)
	if err != nil {
		return 0, err
	}

	return molecularScatter*rayleigh + aerosolScatter*aerosol, nil
}
