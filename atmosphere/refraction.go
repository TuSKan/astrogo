package atmosphere

import (
	"errors"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/internal/refraction"
	"github.com/TuSKan/astrogo/unit"
)

// RefractionModel computes atmospheric refraction in either direction: the
// amount it raises a true altitude to the observed one, and the amount to
// take off an observed altitude to recover the true one. Both are positive.
type RefractionModel interface {
	// RefractFromTrue is the refraction that carries the true (geometric)
	// altitude trueAlt to the observed one: observed = true + this.
	RefractFromTrue(trueAlt angle.Angle, env Refraction) angle.Angle

	// RefractFromApparent is the refraction to take off the observed altitude
	// obsAlt to recover the true one: true = observed − this.
	RefractFromApparent(obsAlt angle.Angle, env Refraction) angle.Angle
}

// Refraction represents meteorological parameters used for calculating
// atmospheric refraction during astronomical observations.
//
// Renamed from Atmosphere (v0.14.0 and earlier) as part of freeing that name
// for a new, richer type — see State's own doc comment on Atmosphere for the
// full rationale. This is a deliberate, same-release hard break with no
// deprecation alias: Go cannot alias one identifier to two different
// meanings at once, so freeing "Atmosphere" for the richer type necessarily
// retires this struct's old name immediately.
//
// Refraction stays a small, freely-literal-constructed value type — it is
// consumed by RefractionModel in hot paths across coord/plan (an airmass or
// refraction correction may run per observation, per scheduling step). It is
// composed, not merged, into the richer Atmosphere type (see atmosphere.go):
// Atmosphere.Refraction() returns one of these directly, letting a caller
// with a full Atmosphere reach real refraction-model machinery without a
// second, parallel pressure/temperature representation.
//
// Pressure is in hPa, Temperature in °C, Humidity a fraction from 0 to 1,
// and Wavelength in micrometers.
type Refraction struct {
	Model       RefractionModel
	Pressure    float64
	Temperature float64
	Humidity    float64
	Wavelength  float64
}

// EffectiveModel returns the model this environment actually refracts with,
// which is never nil.
//
// # Why a nil Model is not an error
//
// Refraction is a freely-literal-constructed value type, and the constructors
// deliberately leave Model nil — [AtAltitude] says so in as many words. A nil
// Model means "no opinion, use the default", not "no refraction": with a
// pressure set it resolves to [RefractionSOFA], and only a zero pressure means
// a vacuum.
//
// That convention used to live in coord, which read Model == nil and reached
// for SOFA's constants itself. atmosphere had no such default, so the meaning
// of a zero value depended on which package consumed it, and anyone following
// the documented pluggable-model API into env.Model.RefractFromTrue got a nil
// dereference. The convention belongs here, with the type it describes.
func (r Refraction) EffectiveModel() RefractionModel {
	switch {
	case r.Model != nil:
		return r.Model
	case r.Pressure > 0:
		return RefractionSOFA{}
	default:
		return RefractionNone{}
	}
}

// RefractFromTrue is the refraction that carries a true geometric altitude to
// the observed one, using [Refraction.EffectiveModel].
//
// Positive: refraction raises an object, so observed = true + this. Prefer it
// over reaching into Model directly, which is nil under every constructor.
func (r Refraction) RefractFromTrue(trueAlt angle.Angle) angle.Angle {
	return r.EffectiveModel().RefractFromTrue(trueAlt, r)
}

// RefractFromApparent is the refraction to remove from an observed altitude to
// recover the true geometric one, using [Refraction.EffectiveModel].
//
// Positive, like [Refraction.RefractFromTrue], so true = observed − this.
func (r Refraction) RefractFromApparent(obsAlt angle.Angle) angle.Angle {
	return r.EffectiveModel().RefractFromApparent(obsAlt, r)
}

// ── Models ────────────────────────────────────────────────────────────────────

// RefractionNone entirely disables refraction.
type RefractionNone struct{}

// RefractFromTrue returns precisely 0 shifting.
func (RefractionNone) RefractFromTrue(_ angle.Angle, _ Refraction) angle.Angle {
	return 0
}

// RefractFromApparent returns precisely 0 shifting.
func (RefractionNone) RefractFromApparent(_ angle.Angle, _ Refraction) angle.Angle {
	return 0
}

// RefractionBennett is Bennett's (1982) formula as Hohenkerk refitted it to
// the Nautical Almanac's refraction tables, which since 2004 are Hohenkerk &
// Sinclair's ray tracing at 10 °C, 1010 hPa, 80% humidity and 0.50169 µm:
//
//	R = 0.28·P/(T + 273) · 0°.0167 / tan(h + 7.32/(h + 4.32))
//
// for the apparent altitude h in degrees, P in hPa and T in °C. It reproduces
// the almanac's table within 0.12′ from the horizon to the zenith, where
// Bennett's own constants, 7.31 and 4.4, are 0.7′ high on the horizon. Its
// wavelength scales it by the refractivity of dry air, as SOFA's does; it
// ignores humidity, which the almanac fixes.
//
// The true-to-observed direction inverts the same formula, so the two agree
// exactly. Below an apparent altitude of about −1.61° the formula stops
// being a fit to anything, turning down through its pole at −4.32°; there
// the refraction tapers smoothly from its value at −1.61°, 52.5′ at the
// standard pressure and temperature, to zero at −4.24°, so a line of sight
// into the ground is not refracted.
//
// This is the model to use near the horizon on its own. It runs 2–3% high
// against Hohenkerk & Sinclair's ray tracing between 70° and 30° altitude,
// about 1.6″ at 45°, which the almanac's table, in tenths of an arcminute,
// cannot show, and which [RefractionSOFA] does not have.
type RefractionBennett struct{}

// RefractFromTrue is Bennett-NA's refraction at the observed altitude a true
// altitude refracts to.
func (RefractionBennett) RefractFromTrue(trueAlt angle.Angle, env Refraction) angle.Angle {
	return angle.Rad(refraction.BennettFromTrue(trueAlt.Radians(), env.Pressure, env.Temperature, env.Wavelength))
}

// RefractFromApparent is Bennett-NA's refraction at the observed altitude.
func (RefractionBennett) RefractFromApparent(obsAlt angle.Angle, env Refraction) angle.Angle {
	return angle.Rad(refraction.Bennett(obsAlt.Radians(), env.Pressure, env.Temperature, env.Wavelength))
}

// RefractionSOFA is SOFA's refraction, A·tan z + B·tan³ z with A and B from
// iauRefco for the pressure, temperature, humidity and wavelength given,
// handed over to [RefractionBennett] near the horizon.
//
// This is the model a nil [Refraction.Model] resolves to when a pressure is
// set — see [Refraction.EffectiveModel]. It exists so that "nil means SOFA"
// is a statement atmosphere can act on, rather than a convention each consumer
// has to reimplement.
//
// # The hand-over
//
// iauRefco says its series is for "applications where performance at low
// altitudes is not paramount", and SOFA clamps it at 2.87°, so that it holds
// everything lower at the refraction there. Against Hohenkerk & Sinclair's
// ray tracing it is within 0.7″ down to 10° altitude, 23″ short at 5°, and
// 23′ short on the horizon: 10.3′ there at sea level against the almanacs'
// 34′. For an observation planner the horizon is where it matters most.
//
// So above 10° of observed altitude this is SOFA's series, exactly as
// iauAtioq and iauAtoiq apply it; below 5° it is Bennett-NA, within 13″ of
// the ray tracing down to the horizon; and between the two the weight passes
// smoothly from one to the other. At radio wavelengths, above 100 µm, there
// is no hand-over, since Bennett-NA is an optical fit.
//
// Unlike Bennett-NA, A and B are not an empirical fit rescaled by a pressure
// ratio: iauRefco integrates the refractive index of moist air for the
// conditions given.
type RefractionSOFA struct{}

// RefractFromTrue is the refraction carrying a true altitude to the observed
// one: iauAtioq's Newton-corrected series wherever the observed altitude is
// above 10°.
func (RefractionSOFA) RefractFromTrue(trueAlt angle.Angle, env Refraction) angle.Angle {
	if env.Pressure <= 0 {
		return 0
	}

	refa, refb := gofaext.Refco(env.Pressure, env.Temperature, env.Humidity, env.Wavelength)

	return angle.Rad(refraction.FromTrue(trueAlt.Radians(), refa, refb, env.Pressure, env.Temperature, env.Wavelength))
}

// RefractFromApparent is the refraction to take off an observed altitude: the
// series in its defining form above 10°, where the zenith distance is the
// observed one, as iauAtoiq removes it.
func (RefractionSOFA) RefractFromApparent(obsAlt angle.Angle, env Refraction) angle.Angle {
	if env.Pressure <= 0 {
		return 0
	}

	refa, refb := gofaext.Refco(env.Pressure, env.Temperature, env.Humidity, env.Wavelength)

	return angle.Rad(refraction.FromApparent(obsAlt.Radians(), refa, refb, env.Pressure, env.Temperature, env.Wavelength))
}

// StandardRefraction returns a typical sea-level refraction environment: the
// ICAO standard atmosphere's 1013.25 hPa and 15 °C, half humidity, at
// 0.55 µm, with no model set, so [RefractionSOFA] applies. It is
// [AtAltitude] at zero height.
//
// It is a function returning a fresh value, not a var, so that no importer
// can change it for every other one in the process (#537): as a var, one
// `atmosphere.StandardRefraction.Model = nil` anywhere in a program switched
// every caller's refraction model.
func StandardRefraction() Refraction {
	return Refraction{
		Pressure:    1013.25,
		Temperature: 15.0,
		Humidity:    0.5,
		Wavelength:  0.55,
	}
}

// ── Observational Metrics ─────────────────────────────────────────────────────

// ErrBelowHorizon is returned when the target altitude is below the horizon.
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
func HorizonDip(h unit.Length) angle.Angle {
	if h <= 0 {
		return angle.Zero()
	}
	// 1.76 arcminutes per sqrt(meter), converted to degrees
	dipArcmin := 1.76 * math.Sqrt(h.Meters())

	return angle.Deg(dipArcmin / 60.0)
}

// AtAltitude returns a Refraction with pressure and temperature adjusted for
// the given altitude h (meters) using the ICAO International Standard
// Atmosphere model.
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
// Humidity and wavelength are inherited from [StandardRefraction], and the
// model is left nil, which [Refraction.EffectiveModel] resolves to
// [RefractionSOFA].
func AtAltitude(height unit.Length) Refraction {
	std := StandardRefraction()

	if height <= 0 {
		return std
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

	h := height.Meters()

	pressure := P0 * math.Pow(1.0-L*h/T0, exponent)
	temperature := (T0 - L*h) - 273.15 // Convert to Celsius

	return Refraction{
		Pressure:    pressure,
		Temperature: temperature,
		Humidity:    std.Humidity,
		Wavelength:  std.Wavelength,
	}
}
