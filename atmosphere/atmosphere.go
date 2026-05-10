package atmosphere

import (
	"errors"
	"math"

	"github.com/TuSKan/astrogo/angle"
)

// RefractionModel defines an algorithm that computes the angular refraction shift.
// It explicitly parses the distinction between forward and reverse tracing.
type RefractionModel interface {
	// RefractFromTrue computes the atmospheric refraction correction by propagating a True geometric altitude
	// forward linearly into refracted Observed appearance (Saemundsson 1986).
	RefractFromTrue(trueAlt angle.Angle, env Atmosphere) angle.Angle

	// RefractFromApparent computes the atmospheric refraction correction necessary to un-refract an
	// Observed visual altitude backwards into pure geometric Truth (Bennett 1982).
	RefractFromApparent(obsAlt angle.Angle, env Atmosphere) angle.Angle
}

// Atmosphere represents meteorological parameters used for calculating atmospheric
// refraction during astronomical observations.
type Atmosphere struct {
	Model       RefractionModel
	Pressure    float64
	Temperature float64
	Humidity    float64
	Wavelength  float64
}

// ── Models ────────────────────────────────────────────────────────────────────

// RefractionNone entirely disables refraction.
type RefractionNone struct{}

// RefractFromTrue returns precisely 0 shifting.
func (RefractionNone) RefractFromTrue(_ angle.Angle, _ Atmosphere) angle.Angle {
	return 0
}

// RefractFromApparent returns precisely 0 shifting.
func (RefractionNone) RefractFromApparent(_ angle.Angle, _ Atmosphere) angle.Angle {
	return 0
}

// RefractionApproximate computes refraction extremely quickly using Saemundsson's
// tangent formula. Accurate to ~0.1 arcmin over 15 degrees.
type RefractionApproximate struct{}

// RefractFromTrue applies Saemundsson's refraction formula (S&T 1986).
func (RefractionApproximate) RefractFromTrue(trueAlt angle.Angle, env Atmosphere) angle.Angle {
	h := trueAlt.Degrees()
	if h < -5.0 {
		return 0 // Avoid absurd refraction below horizon
	}

	// Refraction R in arcminutes
	R := 1.02 / math.Tan((h+10.3/(h+5.11))*math.Pi/180.0)

	factor := (env.Pressure / 1010.0) * (283.0 / (273.15 + env.Temperature))

	return angle.Deg((R * factor) / 60.0)
}

// RefractFromApparent applies Bennett's empirical fraction.
func (RefractionApproximate) RefractFromApparent(obsAlt angle.Angle, env Atmosphere) angle.Angle {
	h := obsAlt.Degrees()
	if h < -5.0 {
		return 0
	}

	R := 1.0 / math.Tan((h+7.31/(h+4.4))*math.Pi/180.0)
	factor := (env.Pressure / 1010.0) * (283.0 / (273.15 + env.Temperature))

	return angle.Deg((R * factor) / 60.0)
}

// RefractionRigorous explicitly represents the analytical integration model derived from physical meteorological parameters.
type RefractionRigorous struct{}

// RefractFromTrue calculates the atmospheric refraction based on the rigorous Saemundsson (1986)
// model which remains stable and valid down to the true horizon.
func (RefractionRigorous) RefractFromTrue(trueAlt angle.Angle, env Atmosphere) angle.Angle {
	h := trueAlt.Degrees()
	if h < -5.0 {
		return 0
	}

	if env.Pressure <= 0 {
		return 0
	}

	// Saemundsson (1986) formula in arcminutes for true (geometric) altitude h
	denom := h + 5.11
	inner := h + (10.3 / denom)
	r0 := 1.02 / math.Tan(inner*math.Pi/180.0)

	correction := (env.Pressure / 1010.0) * (283.0 / (273.15 + env.Temperature))

	wlFactor := 1.0
	if env.Wavelength > 0 {
		wlFactor = 1.0 + 0.005*(0.55-env.Wavelength)
	}

	return angle.Deg((r0 * correction * wlFactor) / 60.0)
}

// RefractFromApparent derives atmospheric refraction analytically based on the observed visual altitude.
// Standardized on the robust Bennett (1982) formula which handles zero-altitude gracefully.
func (RefractionRigorous) RefractFromApparent(obsAlt angle.Angle, env Atmosphere) angle.Angle {
	h := obsAlt.Degrees()
	if h < -5.0 {
		return 0
	}

	if env.Pressure <= 0 {
		return 0
	}

	// Bennett (1982) formula in arcminutes for observed (apparent) altitude h
	denom := h + 4.4
	inner := h + (7.31 / denom)
	r0 := 1.0 / math.Tan(inner*math.Pi/180.0)

	correction := (env.Pressure / 1010.0) * (283.0 / (273.15 + env.Temperature))

	wlFactor := 1.0
	if env.Wavelength > 0 {
		wlFactor = 1.0 + 0.005*(0.55-env.Wavelength)
	}

	return angle.Deg((r0 * correction * wlFactor) / 60.0)
}

// StandardAtmosphere returns a typical sea-level atmospheric profile using the rigorous backend.
//
//nolint:gochecknoglobals // ICAO ISA reference profile — immutable physical constant
var StandardAtmosphere = Atmosphere{
	Pressure:    1013.25,
	Temperature: 15.0,
	Humidity:    0.5,
	Wavelength:  0.55,
	Model:       RefractionRigorous{},
}

// ── Observational Metrics ─────────────────────────────────────────────────────

var ErrBelowHorizon = errors.New("object is below the horizon")

// ZenithDistance returns the zenith distance (90 - Alt) for a given altitude.
func ZenithDistance(alt angle.Angle) angle.Angle {
	return angle.Deg(90).Sub(alt)
}

// Airmass returns the relative airmass for a given apparent altitude using the
// Pickering (2002) formula. This interpolative model resolves horizon stability properly,
// overcoming the earlier Kasten & Young approach limitations down to visual zero.
func Airmass(alt angle.Angle) (float64, error) {
	if alt.Degrees() < 0 {
		return 0, ErrBelowHorizon
	}

	// Pickering (2002) empirical air mass formulation (apparent altitude based).
	// X = 1 / sin(h + 244 / (165 + 47 * h^1.1))
	h := alt.Degrees()
	inner := h + (244.0 / (165.0 + 47.0*math.Pow(h, 1.1)))
	am := 1.0 / math.Sin(inner*math.Pi/180.0)

	return am, nil
}

// ── Elevation-Aware Corrections ──────────────────────────────────────────────

// const (
// 	meanEarthRadius = 6371000.0 // Mean Earth radius in meters (IAU nominal)
// )

// HorizonDip returns the apparent dip angle of the horizon for an observer at
// height h meters above the reference ellipsoid. The dip is the angular depression
// of the visible horizon below the mathematical (level) horizon, corrected for
// standard atmospheric refraction.
//
// Formula: dip ≈ 1.76' × √h (arcminutes), where h is in meters.
//
// This is the standard navigational/astronomical formula that accounts for the
// atmospheric refraction coefficient k ≈ 0.13 (light bending reduces the geometric
// dip by roughly 1/7). At sea level (h=0), dip = 0. At 786m, dip ≈ 0.82°.
func HorizonDip(h float64) angle.Angle {
	if h <= 0 {
		return angle.Zero()
	}
	// 1.76 arcminutes per sqrt(meter), converted to degrees
	dipArcmin := 1.76 * math.Sqrt(h)

	return angle.Deg(dipArcmin / 60.0)
}

// AtAltitude returns an Atmosphere with pressure and temperature adjusted for the
// given altitude h (meters) using the ICAO International Standard Atmosphere model.
//
// Barometric formula (troposphere, h < 11000 m):
//
//	P(h) = P₀ × (1 − L·h / T₀)^(g·M / (R*·L))
//	T(h) = T₀ − L·h   (in °C)
//
// Constants:
//   - L  = 0.0065 K/m (temperature lapse rate)
//   - T₀ = 288.15 K (sea-level standard temperature)
//   - g  = 9.80665 m/s²
//   - M  = 0.0289644 kg/mol (molar mass of dry air)
//   - R* = 8.31447 J/(mol·K) (universal gas constant)
//
// The refraction model and wavelength are inherited from [StandardAtmosphere].
func AtAltitude(h float64) Atmosphere {
	if h <= 0 {
		// Sea level: use standard ISA values but let SOFA handle refraction
		// (Model: nil) for consistency with all other altitudes.
		return Atmosphere{
			Pressure:    StandardAtmosphere.Pressure,
			Temperature: StandardAtmosphere.Temperature,
			Humidity:    StandardAtmosphere.Humidity,
			Wavelength:  StandardAtmosphere.Wavelength,
			Model:       nil,
		}
	}

	const (
		P0       = 1013.25             // Sea-level pressure (hPa)
		T0       = 288.15              // Sea-level temperature (K)
		L        = 0.0065              // Temperature lapse rate (K/m)
		g        = 9.80665             // Gravitational acceleration (m/s²)
		M        = 0.0289644           // Molar mass of dry air (kg/mol)
		Rstar    = 8.31447             // Universal gas constant (J/(mol·K))
		exponent = g * M / (Rstar * L) // ≈ 5.25588
	)

	pressure := P0 * math.Pow(1.0-L*h/T0, exponent)
	temperature := (T0 - L*h) - 273.15 // Convert to Celsius

	return Atmosphere{
		Pressure:    pressure,
		Temperature: temperature,
		Humidity:    StandardAtmosphere.Humidity,
		Wavelength:  StandardAtmosphere.Wavelength,
		Model:       nil, // Let SOFA compute refraction rigorously via Atcoq
	}
}
