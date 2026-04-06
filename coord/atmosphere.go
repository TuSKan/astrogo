package coord

import (
	"errors"
	"math"

	"github.com/TuSKan/astrogo/angle"
)

// RefractionModel defines an algorithm that computes the angular refraction shift.
// It returns the shift to ADD to the true zenith distance, meaning observed ZD = true ZD - shift
// Equivalently, observed Altitude = true Altitude + shift.
type RefractionModel interface {
	// Refract returns the atmospheric refraction correction.
	// `trueAlt` is the geometric, unrefracted altitude above the horizon.
	Refract(trueAlt angle.Angle, env Atmosphere, site *Geodetic) angle.Angle
}

// Atmosphere represents meteorological parameters used for calculating atmospheric
// refraction during astronomical observations.
type Atmosphere struct {
	Pressure    float64 // Ambient pressure in hPa (millibars). Default ~1013.25
	Temperature float64 // Ambient temperature in degrees Celsius. Default ~15.0
	Humidity    float64 // Relative humidity [0.0 - 1.0]. Default ~0.5
	Wavelength  float64 // Observation wavelength in micrometers. Default ~0.55

	// Model dictates how the environmental parameters are structurally applied.
	// If nil, it defaults to RefractionSOFA internally.
	Model RefractionModel
}

// ── Models ────────────────────────────────────────────────────────────────────

// RefractionNone entirely disables refraction.
type RefractionNone struct{}

// Refract returns precisely 0 shifting.
func (RefractionNone) Refract(_ angle.Angle, _ Atmosphere, _ *Geodetic) angle.Angle {
	return 0
}

// RefractionApproximate computes refraction extremely quickly using Saemundsson's
// tangent formula. Accurate to ~0.1 arcmin over 15 degrees.
type RefractionApproximate struct{}

// Refract applies Saemundsson's refraction formula (S&T 1986).
func (RefractionApproximate) Refract(trueAlt angle.Angle, env Atmosphere, _ *Geodetic) angle.Angle {
	h := trueAlt.Degrees()
	if h < -5.0 {
		return 0 // Avoid absurd refraction below horizon
	}

	// Refraction R in arcminutes
	R := 1.02 / math.Tan((h+10.3/(h+5.11))*math.Pi/180.0)

	// Apply pressure/temperature scaling:
	// R_actual = R * (P / 1010) * (283 / (273 + T))
	factor := (env.Pressure / 1010.0) * (283.0 / (273.15 + env.Temperature))

	R *= factor
	return angle.Deg(R / 60.0) // convert arcmin to degrees
}

// RefractionSOFA explicitly represents the full analytical integration from the SOFA baseline.
type RefractionSOFA struct{}

// Refract is a stub here. Because SOFA also demands wavelength, site phi, TLR, and EPS
// structurally, the actual transform engine intercepts this type and calls `gofaext.Refro` natively
// or passes directly into Atco13.
func (RefractionSOFA) Refract(_ angle.Angle, _ Atmosphere, _ *Geodetic) angle.Angle {
	return 0
}

// StandardAtmosphere returns a typical sea-level atmospheric profile using the SOFA backend.
var StandardAtmosphere = Atmosphere{
	Pressure:    1013.25,
	Temperature: 15.0,
	Humidity:    0.5,
	Wavelength:  0.55,
	Model:       RefractionSOFA{},
}

// ── Observational Metrics ─────────────────────────────────────────────────────

var (
	ErrBelowHorizon = errors.New("object is below the horizon")
)

// ZenithDistance returns the zenith distance (90 - Alt) for a given altitude.
func ZenithDistance(alt angle.Angle) angle.Angle {
	return angle.Deg(90).Sub(alt)
}

// Airmass returns the relative airmass for a given altitude using the
// Kasten & Young (1989) formula.
//
// Assumptions:
//   - Standard sea-level atmosphere.
//   - Neglects curvature effects below ~5-10 degrees altitude for v1.
//
// Returns an error if the altitude is below the horizon (alt < 0).
func Airmass(alt angle.Angle) (float64, error) {
	if alt.Degrees() < 0 {
		return 0, ErrBelowHorizon
	}

	// Kasten & Young (1989) formula:
	// am = 1 / (sin(alt) + 0.50572 * (6.07995 + alt_deg)^-1.6364)
	altDeg := alt.Degrees()
	sinAlt := alt.Sin()

	am := 1.0 / (sinAlt + 0.50572*math.Pow(6.07995+altDeg, -1.6364))
	return am, nil
}
