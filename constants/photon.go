package constants

import "github.com/TuSKan/astrogo/unit"

// ArcsecondSquaredToSteradian is the solid angle, in steradians, subtended
// by a (1 arcsec)x(1 arcsec) patch in the flat small-angle limit — the
// conversion factor "per square arcsecond" surface-brightness units need
// to become "per steradian". Computed from Derived.ArcSecondsPerRadian,
// not hardcoded, so it tracks the same arcsecond definition the rest of
// this package uses.
var ArcsecondSquaredToSteradian = func() float64 {
	rad := Derived.ArcSecondsPerRadian.Value // arcsec per radian
	perArcsec := 1 / rad                     // radians per arcsec

	return perArcsec * perArcsec
}()

// PhotonEnergyJ returns the energy of one photon at wavelength lambda, in
// joules: E = hc/lambda.
func PhotonEnergyJ(lambda unit.WavelengthNM) float64 {
	lambdaM := float64(lambda) * 1e-9
	if lambdaM <= 0 {
		return 0
	}

	return SI2019.PlanckConstant.Value * SI2019.SpeedOfLight.Value / lambdaM
}

// ToPhoton converts an energy-flux spectral radiance into the equivalent
// photon-flux spectral radiance at wavelength lambda: divide by the energy
// of one photon at that wavelength. Lives here, not in package unit,
// because unit must not import constants (see constants/doc.go) — this
// conversion needs Planck's constant and the speed of light, both owned
// by constants.SI2019.
func ToPhoton(l unit.SpectralRadiance, lambda unit.WavelengthNM) unit.PhotonSpectralRadiance {
	e := PhotonEnergyJ(lambda)
	if e <= 0 {
		return 0
	}

	return unit.PhotonSpectralRadiance(float64(l) / e)
}

// ToEnergy is the inverse of ToPhoton: converts a photon-flux spectral
// radiance back into energy-flux spectral radiance at wavelength lambda.
func ToEnergy(p unit.PhotonSpectralRadiance, lambda unit.WavelengthNM) unit.SpectralRadiance {
	return unit.SpectralRadiance(float64(p) * PhotonEnergyJ(lambda))
}
