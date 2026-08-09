package skybrightness

import (
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/unit"
)

// Canonical scalar types for the spectral sky-radiance engine, declared as
// a group below: WavelengthNM, SpectralRadiance, PhotonSpectralRadiance,
// SpectralIrradiance, Radiance, PhotonRadiance, Irradiance, LuminanceCdM2,
// SurfaceBrightnessAB, SurfaceBrightnessVega, Transmission, OpticalDepth,
// AerosolOpticalDepth, SingleScatteringAlbedo, AsymmetryParameter,
// AngstromExponent, CloudFraction, CloudOpticalDepth, EffectiveRadiusUM,
// OzoneColumnDU, PrecipitableWaterMM, PressureHPa, TemperatureK, AltitudeM,
// SpectralAlbedo, ElectronsPerPixelPerSecond. These are Go type ALIASES
// (not new types) onto the zero-cost quantity types declared in
// unit/quantity_types.go — unit is the single source of truth for them
// (docs/skybrightness.md §3), and every method already defined there (e.g.
// Transmission.ToOpticalDepth/OpticalDepth.ToTransmission) is automatically
// available on these names too, since an alias is literally the same type.
// skybrightness only re-declares the names here so the rest of this
// package's source can write SpectralRadiance instead of
// unit.SpectralRadiance.
//
//nolint:revive // one doc comment intentionally describes this whole 26-member alias block, not any single member's name
type (
	WavelengthNM               = unit.WavelengthNM
	SpectralRadiance           = unit.SpectralRadiance
	PhotonSpectralRadiance     = unit.PhotonSpectralRadiance
	SpectralIrradiance         = unit.SpectralIrradiance
	Radiance                   = unit.Radiance
	PhotonRadiance             = unit.PhotonRadiance
	Irradiance                 = unit.Irradiance
	LuminanceCdM2              = unit.LuminanceCdM2
	SurfaceBrightnessAB        = unit.SurfaceBrightnessAB
	SurfaceBrightnessVega      = unit.SurfaceBrightnessVega
	Transmission               = unit.Transmission
	OpticalDepth               = unit.OpticalDepth
	AerosolOpticalDepth        = unit.AerosolOpticalDepth
	SingleScatteringAlbedo     = unit.SingleScatteringAlbedo
	AsymmetryParameter         = unit.AsymmetryParameter
	AngstromExponent           = unit.AngstromExponent
	CloudFraction              = unit.CloudFraction
	CloudOpticalDepth          = unit.CloudOpticalDepth
	EffectiveRadiusUM          = unit.EffectiveRadiusUM
	OzoneColumnDU              = unit.OzoneColumnDU
	PrecipitableWaterMM        = unit.PrecipitableWaterMM
	PressureHPa                = unit.PressureHPa
	TemperatureK               = unit.TemperatureK
	AltitudeM                  = unit.AltitudeM
	SpectralAlbedo             = unit.SpectralAlbedo
	ElectronsPerPixelPerSecond = unit.ElectronsPerPixelPerSecond
)

// arcsecond2SR is the solid angle, in steradians, subtended by a
// (1 arcsec)x(1 arcsec) patch in the flat small-angle limit — exactly the
// convention "per square arcsecond" surface-brightness units already use.
// Computed from constants, not hardcoded, so it tracks the same arcsecond
// definition the rest of the library uses.
var arcsecond2SR = func() float64 {
	rad := constants.Derived.ArcSecondsPerRadian.Value // arcsec per radian
	perArcsec := 1 / rad                               // radians per arcsec

	return perArcsec * perArcsec
}()

// photonEnergyJ returns the energy of one photon at wavelength lambda, in
// joules: E = hc/lambda.
func photonEnergyJ(lambda WavelengthNM) float64 {
	lambdaM := float64(lambda) * 1e-9
	if lambdaM <= 0 {
		return 0
	}

	return constants.SI2019.PlanckConstant.Value * constants.SI2019.SpeedOfLight.Value / lambdaM
}

// ToPhoton converts an energy-flux spectral radiance into the equivalent
// photon-flux spectral radiance at wavelength lambda: divide by the energy
// of one photon at that wavelength. A free function, not a method — l's
// underlying type is declared in package unit, which must not import
// constants (see constants/doc.go), so this conversion (which needs
// Planck's constant and the speed of light) lives here instead.
func ToPhoton(l SpectralRadiance, lambda WavelengthNM) PhotonSpectralRadiance {
	e := photonEnergyJ(lambda)
	if e <= 0 {
		return 0
	}

	return PhotonSpectralRadiance(float64(l) / e)
}

// ToEnergy is the inverse of ToPhoton: converts a photon-flux spectral
// radiance back into energy-flux spectral radiance at wavelength lambda.
func ToEnergy(p PhotonSpectralRadiance, lambda WavelengthNM) SpectralRadiance {
	return SpectralRadiance(float64(p) * photonEnergyJ(lambda))
}
