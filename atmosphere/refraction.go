package atmosphere

import (
	"errors"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/internal/gofaext"
)

// lowAltitudeCutoffDeg is the true/apparent altitude below which
// RefractionApproximate and RefractionRigorous return zero refraction
// instead of evaluating Saemundsson (1986) or Bennett (1982).
//
// Both formulas divide by (h + a constant) — Saemundsson's tangent argument
// has 10.3/(h+5.11), Bennett's has 7.31/(h+4.4) — so as h approaches -5.11°
// or -4.4° respectively, that term diverges and, because it then feeds a
// tan()/atan() a huge argument reduced mod 360°, the result stops being a
// smooth extrapolation and becomes effectively arbitrary (it can spike or
// flip sign depending on where the reduced angle lands). -4.0° clears both
// singularities with margin (0.4° from Bennett's, 1.11° from Saemundsson's)
// while still covering the entire physically relevant range: real
// observations never need refraction correction this far below the horizon.
//
// This is a different, independently-justified cutoff from the one used in
// coord/context.go's SOFA-Refa/Refb path (-1°, i.e. z < 91°) — that model's
// tan(z) series is well-behaved much closer to the horizon, so it can safely
// extend further down than the two empirical tangent formulas here.
const lowAltitudeCutoffDeg = -4.0

// zenithArgumentLimit is the other end of the same problem, and the reason all
// four empirical methods check their tangent argument against 90°.
//
// The additive term that keeps each fit stable near the horizon also carries
// its argument *past* 90° near the zenith: Saemundsson's h + 10.3/(h+5.11) is
// 90.108° at h = 90°, and Bennett's h + 7.31/(h+4.4) is 90.077°. tan is
// negative just beyond its asymptote, so both formulas reported negative
// refraction — measured at −0.114″ and −0.080″ at the zenith, turning negative
// above 89.8916° and 89.9225° respectively.
//
// Refraction is never negative in a normal atmosphere: it raises an object, or
// at the zenith does nothing, because light arriving along the normal is not
// bent. Zero is the correct limit, and it is what SOFA returns there.
//
// Returning zero at and above the crossing discards at most 0.11″ — the value
// the formula itself gives just below it — against a fit whose own quoted
// accuracy is about 0.1 arcmin, i.e. 60 times larger. So this clamp costs
// nothing measurable and removes a sign error.
//
// Expressed as a limit on the argument rather than on the altitude because the
// two formulas cross at different altitudes, and because the argument is where
// the problem actually is.
const zenithArgumentLimit = 90.0

// RefractionModel defines an algorithm that computes the angular refraction shift.
// It explicitly parses the distinction between forward and reverse tracing.
type RefractionModel interface {
	// RefractFromTrue computes the atmospheric refraction correction by propagating a True geometric altitude
	// forward linearly into refracted Observed appearance (Saemundsson 1986).
	RefractFromTrue(trueAlt angle.Angle, env Refraction) angle.Angle

	// RefractFromApparent computes the atmospheric refraction correction necessary to un-refract an
	// Observed visual altitude backwards into pure geometric Truth (Bennett 1982).
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

// RefractionApproximate computes refraction extremely quickly using Saemundsson's
// tangent formula. Accurate to ~0.1 arcmin over 15 degrees.
type RefractionApproximate struct{}

// RefractFromTrue applies Saemundsson's refraction formula (S&T 1986).
func (RefractionApproximate) RefractFromTrue(trueAlt angle.Angle, env Refraction) angle.Angle {
	h := trueAlt.Degrees()
	if h < lowAltitudeCutoffDeg {
		return 0 // Avoid absurd refraction below horizon
	}

	inner := h + 10.3/(h+5.11)
	if inner >= zenithArgumentLimit {
		return 0 // see zenithArgumentLimit
	}

	// Refraction R in arcminutes
	R := 1.02 / math.Tan(inner*math.Pi/180.0)

	factor := (env.Pressure / 1010.0) * (283.0 / (273.15 + env.Temperature))

	return angle.Deg((R * factor) / 60.0)
}

// RefractFromApparent applies Bennett's empirical fraction.
func (RefractionApproximate) RefractFromApparent(obsAlt angle.Angle, env Refraction) angle.Angle {
	h := obsAlt.Degrees()
	if h < lowAltitudeCutoffDeg {
		return 0
	}

	inner := h + 7.31/(h+4.4)
	if inner >= zenithArgumentLimit {
		return 0 // see zenithArgumentLimit
	}

	R := 1.0 / math.Tan(inner*math.Pi/180.0)
	factor := (env.Pressure / 1010.0) * (283.0 / (273.15 + env.Temperature))

	return angle.Deg((R * factor) / 60.0)
}

// RefractionSOFA is the refraction model SOFA itself uses: the two-term series
// dz = A·tan z + B·tan³ z, with A and B derived from the pressure,
// temperature, humidity and wavelength in the [Refraction] passed to it.
//
// This is the model a nil [Refraction.Model] resolves to when a pressure is
// set — see [Refraction.EffectiveModel]. It exists so that "nil means SOFA"
// is a statement atmosphere can act on, rather than a convention each consumer
// has to reimplement.
//
// Unlike the Saemundsson and Bennett formulas beside it, the constants are not
// a fixed empirical fit rescaled by a pressure ratio: gofa's Refco integrates
// the refractive index of moist air for the specific conditions given, so the
// wavelength dependence is real dispersion rather than the linear 0.005/µm
// approximation [RefractionRigorous] applies.
type RefractionSOFA struct{}

// Refraction clamps, copied from SOFA's iauAtioq rather than chosen here.
//
// selMin bounds sin(altitude) at 0.05 — about 2.87° — so the tangent series is
// never evaluated where it diverges. celMin bounds cos(altitude) away from
// zero at the zenith, where the horizontal component vanishes.
//
// coord/context.go carries the same two constants for its own vector-form copy
// of Atioq. They are duplicated rather than shared because they belong to the
// algorithm, not to either package, and neither should import the other.
const (
	selMin = 0.05
	celMin = 1e-6
)

// RefractFromTrue applies Atioq's Newton-corrected form of the series, which is
// the direction SOFA uses when going from a true topocentric direction to the
// observed one.
func (RefractionSOFA) RefractFromTrue(trueAlt angle.Angle, env Refraction) angle.Angle {
	if env.Pressure <= 0 {
		return 0
	}

	refa, refb := gofaext.Refco(env.Pressure, env.Temperature, env.Humidity, env.Wavelength)

	// In units of sine and cosine of altitude, as Atioq works on a unit vector.
	z := math.Max(math.Sin(trueAlt.Radians()), selMin)
	r := math.Max(math.Cos(trueAlt.Radians()), celMin)

	tz := r / z
	w := refb * tz * tz

	return angle.Rad((refa + w) * tz / (1.0 + (refa+3.0*w)/(z*z)))
}

// RefractFromApparent applies the series in its defining form, where the
// zenith distance is the observed one — which is how gofa's Refco documents A
// and B, so no correction term belongs here.
func (RefractionSOFA) RefractFromApparent(obsAlt angle.Angle, env Refraction) angle.Angle {
	if env.Pressure <= 0 {
		return 0
	}

	refa, refb := gofaext.Refco(env.Pressure, env.Temperature, env.Humidity, env.Wavelength)

	z := math.Max(math.Sin(obsAlt.Radians()), selMin)
	r := math.Max(math.Cos(obsAlt.Radians()), celMin)

	tz := r / z

	return angle.Rad(refa*tz + refb*tz*tz*tz)
}

// RefractionRigorous explicitly represents the analytical integration model derived from physical meteorological parameters.
type RefractionRigorous struct{}

// RefractFromTrue calculates the atmospheric refraction based on the rigorous Saemundsson (1986)
// model which remains stable and valid down to the true horizon.
func (RefractionRigorous) RefractFromTrue(trueAlt angle.Angle, env Refraction) angle.Angle {
	h := trueAlt.Degrees()
	if h < lowAltitudeCutoffDeg {
		return 0
	}

	if env.Pressure <= 0 {
		return 0
	}

	// Saemundsson (1986) formula in arcminutes for true (geometric) altitude h
	denom := h + 5.11

	inner := h + (10.3 / denom)
	if inner >= zenithArgumentLimit {
		return 0 // see zenithArgumentLimit
	}

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
func (RefractionRigorous) RefractFromApparent(obsAlt angle.Angle, env Refraction) angle.Angle {
	h := obsAlt.Degrees()
	if h < lowAltitudeCutoffDeg {
		return 0
	}

	if env.Pressure <= 0 {
		return 0
	}

	// Bennett (1982) formula in arcminutes for observed (apparent) altitude h
	denom := h + 4.4

	inner := h + (7.31 / denom)
	if inner >= zenithArgumentLimit {
		return 0 // see zenithArgumentLimit
	}

	r0 := 1.0 / math.Tan(inner*math.Pi/180.0)

	correction := (env.Pressure / 1010.0) * (283.0 / (273.15 + env.Temperature))

	wlFactor := 1.0
	if env.Wavelength > 0 {
		wlFactor = 1.0 + 0.005*(0.55-env.Wavelength)
	}

	return angle.Deg((r0 * correction * wlFactor) / 60.0)
}

// StandardRefraction returns a typical sea-level refraction environment
// using the rigorous backend.
//
// Renamed from StandardAtmosphere alongside the Atmosphere→Refraction
// rename, for consistency between the type and its standard-value var.
//
//nolint:gochecknoglobals // ICAO ISA reference profile — immutable physical constant
var StandardRefraction = Refraction{
	Pressure:    1013.25,
	Temperature: 15.0,
	Humidity:    0.5,
	Wavelength:  0.55,
	Model:       RefractionRigorous{},
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
func HorizonDip(h float64) angle.Angle {
	if h <= 0 {
		return angle.Zero()
	}
	// 1.76 arcminutes per sqrt(meter), converted to degrees
	dipArcmin := 1.76 * math.Sqrt(h)

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
// Humidity and wavelength are inherited from [StandardRefraction]; the model
// is deliberately left nil, which [Refraction.EffectiveModel] resolves to
// [RefractionSOFA]. StandardRefraction's own RefractionRigorous is *not*
// inherited, which this comment used to claim.
func AtAltitude(h float64) Refraction {
	if h <= 0 {
		// Sea level: use standard ISA values but let SOFA handle refraction
		// (Model: nil) for consistency with all other altitudes.
		return Refraction{
			Pressure:    StandardRefraction.Pressure,
			Temperature: StandardRefraction.Temperature,
			Humidity:    StandardRefraction.Humidity,
			Wavelength:  StandardRefraction.Wavelength,
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

	return Refraction{
		Pressure:    pressure,
		Temperature: temperature,
		Humidity:    StandardRefraction.Humidity,
		Wavelength:  StandardRefraction.Wavelength,
		Model:       nil, // Let SOFA compute refraction rigorously via Atcoq
	}
}
