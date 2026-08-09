package atmos

import (
	"errors"
	"math"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/skybrightness"
)

// ErrTargetBelowHorizon is returned by RayleighOnly.LineOfSight for a
// direction at or below the horizon, where airmass is undefined.
var ErrTargetBelowHorizon = errors.New("atmos: direction is at or below the horizon")

// RayleighOnly is a molecular-scattering-only, aerosol-free, cloud-free
// TransmissionModel: an analytic approximation of Rayleigh optical depth
// (Hansen & Travis 1974, Space Sci. Rev. 16, 527, their commonly-cited
// sea-level fit), scaled to the AtmosphereState's own surface pressure
// and combined with Pickering (2002) airmass (atmosphere.Airmass — the
// same primitive plan/constraint.go already uses).
//
// This is a deliberate Phase 1 simplification, not the full mandate: no
// molecular absorption, no ozone, no aerosol, no cloud. Phase 3's
// atmos.LayeredTransmission (Bodhaine et al. 1999, JAOT 16, 1854, full
// molecular treatment plus aerosol/cloud optics) replaces it behind the
// same skybrightness.TransmissionModel interface — see
// docs/skybrightness.md §14.
type RayleighOnly struct{}

// NewRayleighOnly returns a RayleighOnly transmission model.
func NewRayleighOnly() *RayleighOnly { return &RayleighOnly{} }

// Algorithm implements skybrightness.TransmissionModel.
func (r *RayleighOnly) Algorithm() skybrightness.AlgorithmRef {
	return skybrightness.AlgorithmRef{
		Name: "atmos.RayleighOnly", Version: "1.0.0",
		Citation: "Hansen & Travis (1974), Space Sci. Rev. 16, 527 (approximate Rayleigh optical depth); Pickering (2002) airmass",
	}
}

// LineOfSight implements skybrightness.TransmissionModel.
func (r *RayleighOnly) LineOfSight(dir coord.AltAz, st *skybrightness.AtmosphereState, g skybrightness.SpectralGrid, out []skybrightness.Transmission) error {
	airmass, err := atmosphere.Airmass(dir.Alt())
	if err != nil {
		return ErrTargetBelowHorizon
	}

	pressureHPa := 1013.25

	if st != nil {
		p, _ := st.Surface()
		if p > 0 {
			pressureHPa = float64(p)
		}
	}

	pressureRatio := pressureHPa / 1013.25

	lambda := g.Lambda()
	for i, lam := range lambda {
		tau := rayleighOpticalDepth(float64(lam)) * pressureRatio
		out[i] = skybrightness.Transmission(math.Exp(-tau * airmass))
	}

	return nil
}

// rayleighOpticalDepth returns the approximate sea-level Rayleigh optical
// depth at wavelength lambdaNM, via the Hansen & Travis (1974) fit:
// tau_R(lambda) = 0.0088 * lambda^(-4.15 + 0.2*lambda), lambda in
// micrometres.
func rayleighOpticalDepth(lambdaNM float64) float64 {
	lambdaUM := lambdaNM / 1000
	if lambdaUM <= 0 {
		return 0
	}

	return 0.0088 * math.Pow(lambdaUM, -4.15+0.2*lambdaUM)
}
