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

// ToPhoton and ToEnergy are photon<->energy spectral-radiance conversions,
// aliased from package constants — they need Planck's constant and the
// speed of light (constants.SI2019), which unit must not import (see
// constants/doc.go), so they live one layer up from the quantity types
// themselves. ArcsecondSquaredToSteradian is the solid-angle conversion
// factor "per square arcsecond" surface-brightness units need.
var (
	ToPhoton     = constants.ToPhoton
	ToEnergy     = constants.ToEnergy
	arcsecond2SR = constants.ArcsecondSquaredToSteradian
)
