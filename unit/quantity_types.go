package unit

import "math"

// Fast, zero-cost named float64 types for physical quantities that need
// real Go-level type safety in hot numeric paths — e.g. skybrightness's
// spectral sky-radiance engine, which evaluates on the order of 10^4
// directions x 10^2-10^3 wavelengths per call and cannot afford a
// [Quantity] struct (Unit + Value) per element. Each type below is a
// distinct Go type, so — exactly like the existing [Radiance]/[Irradiance]
// unit-var precedent documented for the runtime layer — a Radiance can
// never be assigned to an Irradiance by accident, but here the protection
// is real: it comes from the Go compiler's own type system, the same way
// `angle.Angle` gets real protection against being handed a plain float64
// meant for something else (see doc.go's "Distinction from angle.Angle").
//
// These types are pure data. They deliberately have no methods requiring
// constants.SI2019/CODATA — unit must not import constants (see
// constants/doc.go's "this package depends only on unit", which would
// become a cycle otherwise). Conversions that need physical constants
// (e.g. photon energy = h*c/lambda) live in the consuming package
// (skybrightness) as free functions instead; only the two conversions
// below that need nothing but math (Transmission<->OpticalDepth) are
// methods here.
type (
	// WavelengthNM is a vacuum wavelength in nanometres, unless a
	// dataset's own documentation states it is in air.
	WavelengthNM float64

	// SpectralRadiance is spectral radiance, W*m^-2*sr^-1*nm^-1 — the
	// primary quantity of skybrightness's spectral sky-radiance engine,
	// L_lambda(lambda, altitude, azimuth, site, epoch).
	SpectralRadiance float64

	// PhotonSpectralRadiance is the photon-counting analogue of
	// SpectralRadiance, photon*s^-1*m^-2*sr^-1*nm^-1.
	PhotonSpectralRadiance float64

	// SpectralIrradiance is W*m^-2*nm^-1.
	SpectralIrradiance float64

	// Radiance is a passband-integrated radiance, W*m^-2*sr^-1.
	Radiance float64

	// PhotonRadiance is a passband-integrated photon radiance,
	// photon*s^-1*m^-2*sr^-1.
	PhotonRadiance float64

	// Irradiance is W*m^-2.
	Irradiance float64

	// LuminanceCdM2 is photopic or scotopic luminance, cd*m^-2.
	LuminanceCdM2 float64

	// SurfaceBrightnessAB is an AB-system surface brightness,
	// mag/arcsec^2 — only meaningful paired with the passband it was
	// computed in.
	SurfaceBrightnessAB float64

	// SurfaceBrightnessVega is a Vega-system surface brightness,
	// mag/arcsec^2 — only meaningful paired with a passband and that
	// passband's Vega zero-point version.
	SurfaceBrightnessVega float64

	// Transmission is atmospheric transmission, in [0,1].
	Transmission float64

	// OpticalDepth is a vertical optical depth (>= 0) unless documented
	// as a slant path.
	OpticalDepth float64

	// AerosolOpticalDepth is aerosol optical depth at a stated reference
	// wavelength (>= 0).
	AerosolOpticalDepth float64

	// SingleScatteringAlbedo is in [0,1].
	SingleScatteringAlbedo float64

	// AsymmetryParameter is the Henyey-Greenstein asymmetry parameter g,
	// in [-1,1].
	AsymmetryParameter float64

	// AngstromExponent is the Angstrom wavelength exponent for aerosol
	// optical depth scaling.
	AngstromExponent float64

	// CloudFraction is SKY COVER, in [0,1] — NOT an optical depth, NOT an
	// opacity. See CloudOpticalDepth for the separate, independent
	// quantity.
	CloudFraction float64

	// CloudOpticalDepth is a cloud layer's optical depth (>= 0),
	// independent of CloudFraction.
	CloudOpticalDepth float64

	// EffectiveRadiusUM is a cloud droplet/ice-crystal effective radius,
	// in micrometres.
	EffectiveRadiusUM float64

	// OzoneColumnDU is a total-column ozone amount, in Dobson units.
	OzoneColumnDU float64

	// PrecipitableWaterMM is precipitable water vapour, in millimetres.
	PrecipitableWaterMM float64

	// PressureHPa is atmospheric pressure, in hectopascals.
	PressureHPa float64

	// TemperatureK is temperature, in kelvin.
	TemperatureK float64

	// AltitudeM is geometric height above the WGS84 ellipsoid, in metres.
	AltitudeM float64

	// SpectralAlbedo is a surface reflectance, in [0,1].
	SpectralAlbedo float64

	// ElectronsPerPixelPerSecond is a detector background rate.
	ElectronsPerPixelPerSecond float64

	// ElectronsPerSecond is a detector count rate from a source, summed
	// over whatever pixels it falls on rather than per pixel.
	//
	// Distinct from ElectronsPerPixelPerSecond because the two combine
	// only through the aperture size, and mixing them is the unit error
	// this package exists to make impossible: a point source's rate is a
	// total, a sky background's is a density, and the signal-to-noise
	// ratio needs both at once.
	ElectronsPerSecond float64
)

// ToOpticalDepth converts a transmission fraction to the corresponding
// vertical optical depth: tau = -ln(T). Returns +Inf for T <= 0 (fully
// opaque) and 0 for T >= 1.
func (t Transmission) ToOpticalDepth() OpticalDepth {
	switch {
	case t <= 0:
		return OpticalDepth(math.Inf(1))
	case t >= 1:
		return 0
	default:
		return OpticalDepth(-math.Log(float64(t)))
	}
}

// ToTransmission is the inverse of Transmission.ToOpticalDepth:
// T = exp(-tau).
func (o OpticalDepth) ToTransmission() Transmission {
	if o < 0 {
		o = 0
	}

	return Transmission(math.Exp(-float64(o)))
}
