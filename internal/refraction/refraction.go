// Package refraction is the arithmetic behind atmosphere's refraction models,
// shared with coord so that both apply one implementation: coord holds SOFA's
// refraction constants once per Context, and atmosphere computes them per
// call.
//
// Angles are in radians throughout, and every refraction is returned as the
// positive amount by which the atmosphere raises an object.
package refraction

import "math"

const deg = math.Pi / 180

// SOFA's clamps, from iauAtioq and iauAtoiq. selMin bounds sin(altitude) at
// 0.05, about 2.87°, so the tangent series is never evaluated where it
// diverges; celMin bounds cos(altitude) away from zero at the zenith.
const (
	selMin = 0.05
	celMin = 1e-6
)

// SOFAFromTrue is the refraction iauAtioq adds to a true altitude: the series
// A·tan z + B·tan³ z with its Newton-Raphson correction, applied as Atioq
// rotates the direction. refa and refb are iauRefco's constants.
func SOFAFromTrue(alt, refa, refb float64) float64 {
	sa, ca := math.Sincos(alt)

	r := ca
	if r <= celMin {
		r = celMin
	}

	z := sa
	if z <= selMin {
		z = selMin
	}

	tz := r / z
	w := refb * tz * tz
	del := (refa + w) * tz / (1.0 + (refa+3.0*w)/(z*z))

	cosdel := 1.0 - del*del/2.0
	f := cosdel - del*z/r

	return math.Atan2(cosdel*sa+del*r, math.Abs(f)*ca) - alt
}

// SOFAFromApparent is the refraction iauAtoiq removes from an observed
// altitude: the series in its defining form, where z is the observed zenith
// distance.
func SOFAFromApparent(alt, refa, refb float64) float64 {
	sa, ca := math.Sincos(alt)

	z := sa
	if z <= selMin {
		z = selMin
	}

	tz := ca / z

	return (refa + refb*tz*tz) * tz
}

// Bennett-NA's two constants: Bennett's (1982) formula as Hohenkerk refitted
// it to the Nautical Almanac's refraction tables, the Explanatory
// Supplement's (2013) Eq. 7.93 as Wilson (2018, Eq. 2.8) quotes it:
//
//	R = 0.28·P/(T + 273) · 0°.0167 / tan(h + 7.32/(h + 4.32))
//
// with P in hPa, T in °C and the apparent altitude h in degrees. Bennett's own
// constants were 7.31 and 4.4, fitted to Garfinkel's model. The test that
// holds this to the almanac's own table is what vouches for it.
const (
	bennettA = 7.32
	bennettB = 4.32
)

// bennettTurnover is the apparent altitude, in degrees, below which Bennett's
// formula stops being a fit to anything: its argument h + A/(h + B) has its
// minimum at h = √A − B, about −1.61°, and below it the argument grows again,
// so the formula falls from 52.5′ there to 3′ at −4° and then through its
// pole at −B, through a range no table it was fitted to covers.
//
// Below the turnover the refraction is instead tapered to zero, smoothly, so
// that a line of sight well into the ground, which no ray from the sky
// follows, is not refracted at all, as Skyfield gives none more than a degree
// below the horizon. Holding the turnover's value lower down instead raised
// the Sun at −70° by 0.6°, into everything that reads its observed altitude.
//
// The taper's width is three times the refraction at the turnover, so its
// slope, at most 1.5 times refraction over width, never exceeds 0.5 and the
// true altitude still rises with the apparent one whatever the air. At 10 °C
// and 1010 hPa the refraction is 52.5′ at the turnover and zero below −4.24°.
var bennettTurnover = math.Sqrt(bennettA) - bennettB

// bennettZenithLimit is the other end. Bennett's argument passes 90° just
// short of the zenith, at an apparent altitude of 89.92°, and tan beyond its
// pole is negative, so the formula reports a small negative refraction there:
// −0.08″ at the zenith. Refraction is never negative; light along the normal
// is not bent. Zero is the limit, as SOFA's series gives, and the clamp
// discards at most what the formula gives just below it, a fraction of an
// arcsecond against a fit good to 0.1′.
const bennettZenithLimit = 90.0

// bennettWavelength is the wavelength, in micrometers, at which Bennett-NA
// holds: the Nautical Almanac's tables are computed for 0.50169 µm, at 10 °C,
// 1010 hPa and 80% humidity (Wilson 2018, Sect. 2.1.4).
const bennettWavelength = 0.50169

// Bennett is Bennett-NA's refraction at apparent altitude alt, for a pressure
// p in hPa, a temperature tc in °C and a wavelength wl in micrometers (zero
// or less for the formula's own). It ignores humidity, which the almanac
// fixes at 80%. A pressure of zero or less is a vacuum.
func Bennett(alt, p, tc, wl float64) float64 {
	if p <= 0 {
		return 0
	}

	h := alt / deg
	if h >= bennettTurnover {
		return bennett(h, p, tc, wl)
	}

	r0 := bennettAtTurnover(p, tc, wl)
	width := 3 * r0 / deg

	s := (h - (bennettTurnover - width)) / width
	if s <= 0 {
		return 0
	}

	return r0 * s * s * (3 - 2*s)
}

// cotAtTurnover is the cotangent of Bennett's argument at the turnover, where
// it is least: √A − B + A/√A = 2√A − B, in degrees.
var cotAtTurnover = 1 / math.Tan((2*math.Sqrt(bennettA)-bennettB)*deg)

// bennettAtTurnover is the formula's refraction at the turnover, in radians.
func bennettAtTurnover(p, tc, wl float64) float64 {
	return 0.28 * p / (tc + 273) * 0.0167 * cotAtTurnover * dispersion(wl) * deg
}

// bennettZero is the apparent altitude, in radians, below which Bennett's
// refraction is zero: the foot of the taper. A true altitude below it is its
// own apparent one, which spares the inversion every target that has set.
func bennettZero(p, tc, wl float64) float64 {
	return bennettTurnover*deg - 3*bennettAtTurnover(p, tc, wl)
}

// bennett is the formula itself, at apparent altitude h in degrees, at or
// above the turnover; the refraction is in radians.
func bennett(h, p, tc, wl float64) float64 {
	arg := h + bennettA/(h+bennettB)
	if arg >= bennettZenithLimit {
		return 0
	}

	return 0.28 * p / (tc + 273) * 0.0167 / math.Tan(arg*deg) * dispersion(wl) * deg
}

// BennettFromTrue is the refraction that carries a true altitude to the
// apparent one under [Bennett]: the same formula, inverted, so the two
// directions agree exactly.
func BennettFromTrue(alt, p, tc, wl float64) float64 {
	if p <= 0 || alt < bennettTurnover*deg && alt <= bennettZero(p, tc, wl) {
		return 0
	}

	return invert(alt, Bennett(alt, p, tc, wl), func(a float64) float64 { return Bennett(a, p, tc, wl) })
}

// The hand-over between SOFA's series and Bennett-NA, in degrees of apparent
// altitude. iauRefco says its series is for "applications where performance
// at low altitudes is not paramount", tested to a zenith distance of 75°.
// Against Hohenkerk & Sinclair's ray tracing it is within 0.7″ down to 10°,
// 23″ short at 5°, and 23′ short on the horizon, where it holds every
// altitude below 2.87° at the refraction of 2.87°. Bennett-NA runs about 4″
// high between 15° and 8°, and is within 13″ from 5° to the horizon. So
// above handoverHigh the refraction is SOFA's, below handoverLow it is
// Bennett-NA's, and between them the weight passes smoothly from one to the
// other, so that the refraction and its slope are continuous.
const (
	handoverLow  = 5.0
	handoverHigh = 10.0
)

// SOFAAbove is the observed altitude, in radians, at and above which
// [FromTrue] and [FromApparent] are SOFA's series exactly, so a caller holding
// SOFA's own routines may let them refract there.
const SOFAAbove = handoverHigh * deg

// radioWavelength is the wavelength, in micrometers, above which iauRefco
// uses its radio refractivity. Bennett-NA is an optical fit, so radio keeps
// SOFA's series at every altitude.
const radioWavelength = 100.0

// handoverWeight is SOFA's share of the refraction at apparent altitude h,
// in degrees: 1 above handoverHigh, 0 below handoverLow, and a smoothstep
// between.
func handoverWeight(h float64) float64 {
	s := (h - handoverLow) / (handoverHigh - handoverLow)

	switch {
	case s >= 1:
		return 1
	case s <= 0:
		return 0
	default:
		return s * s * (3 - 2*s)
	}
}

// FromApparent is the refraction removed from an observed altitude alt:
// SOFA's series above the hand-over, Bennett-NA below it, for SOFA's
// constants refa and refb and the pressure, temperature and wavelength they
// were computed from.
func FromApparent(alt, refa, refb, p, tc, wl float64) float64 {
	sofa := SOFAFromApparent(alt, refa, refb)
	if wl > radioWavelength {
		return sofa
	}

	w := handoverWeight(alt / deg)
	if w == 1 {
		return sofa
	}

	return w*sofa + (1-w)*Bennett(alt, p, tc, wl)
}

// FromTrue is the refraction added to a true altitude alt under
// [FromApparent]. Where the result is above the hand-over it is iauAtioq's
// own, so that above 10° nothing differs from SOFA; below 5° it is
// FromApparent inverted, so the two directions agree exactly.
//
// Between, it is FromApparent inverted plus, under the same weight, SOFA's
// own disagreement with itself: iauAtioq's Newton-corrected series is not
// the exact inverse of iauAtoiq's, and the two are 0.018″ apart at 10°,
// 0.045″ at 8° and 0.099″ at 6°. Without that term the refraction would step
// by 0.018″ where iauAtioq takes over; with it, the step is carried away
// smoothly, and the round trip is within 0.031″ in the hand-over and SOFA's
// own above it.
func FromTrue(alt, refa, refb, p, tc, wl float64) float64 {
	if wl <= radioWavelength && (p <= 0 || alt < bennettTurnover*deg && alt <= bennettZero(p, tc, wl)) {
		return 0
	}

	sofa := SOFAFromTrue(alt, refa, refb)
	if wl > radioWavelength || (alt+sofa)/deg >= handoverHigh {
		return sofa
	}

	inverse := invert(alt, sofa, func(a float64) float64 { return FromApparent(a, refa, refb, p, tc, wl) })

	w := handoverWeight((alt + inverse) / deg)
	if w == 0 {
		return inverse
	}

	return inverse + w*sofaDisagreement(alt, sofa, refa, refb)
}

// sofaDisagreement is how far iauAtioq's forward refraction sofa, at the true
// altitude alt, is from the exact inverse of iauAtoiq's series: one Newton
// step from alt + sofa, with the series' own slope. The disagreement is under
// 0.1″, so the step leaves an error of order its square, far below a
// microarcsecond, where a full inversion would cost several evaluations.
func sofaDisagreement(alt, sofa, refa, refb float64) float64 {
	a := alt + sofa
	g := sofa - SOFAFromApparent(a, refa, refb)

	sa, ca := math.Sincos(a)
	tz := ca / sa

	// d/da of (A + B·tan²z)·tan z, with tan z = cos a / sin a.
	slope := -(refa + 3*refb*tz*tz) / (sa * sa)

	return g / (1 - slope)
}

// invert solves a − fromApparent(a) = alt for the apparent altitude a by the
// secant method, starting from alt + guess, and returns the refraction
// a − alt. a − R(a) rises with a, steeply, since no model here lets the
// refraction change faster than half the altitude, so the root is unique and
// a few steps reach it, one evaluation each.
func invert(alt, guess float64, fromApparent func(float64) float64) float64 {
	const tolerance = 1e-14 // radians, about 2 microarcseconds

	a0 := alt + guess
	r0 := fromApparent(a0)
	g0 := a0 - r0 - alt
	a1 := alt + r0
	g1 := a1 - fromApparent(a1) - alt

	for range 30 {
		if g1 == 0 || g1 == g0 {
			break
		}

		a2 := a1 - g1*(a1-a0)/(g1-g0)
		a0, g0 = a1, g1
		a1 = a2

		if math.Abs(a1-a0) < tolerance {
			break
		}

		g1 = a1 - fromApparent(a1) - alt
	}

	return a1 - alt
}

// dispersion is the refractivity of air at wavelength wl, in micrometers,
// relative to its value at bennettWavelength. A wavelength of zero or less
// means the formula's own and gives 1, and one below 0.1 µm is taken as
// 0.1 µm, where iauRefco clamps it too.
//
// The refractivity is the IAG (1999) optical formula for dry air, in the form
// iauRefco uses it (Rueger 2002): proportional to
// 77.53484e-6 + (4.39108e-7 + 3.666e-9/λ²)/λ². Water vapor adds a term that
// does not depend on wavelength and is under a percent of the dry one, so it
// is left out of the ratio. Between 0.40 and 0.70 µm the factor changes by
// 2.5%, which is atmospheric dispersion, as SOFA's own model gives it (#527).
func dispersion(wl float64) float64 {
	if wl <= 0 {
		return 1
	}

	return dryAirRefractivity(math.Max(wl, 0.1)) / dryAirRefractivity(bennettWavelength)
}

// dryAirRefractivity is the wavelength-dependent part of iauRefco's optical
// refractivity, wl in micrometers. Only its ratio is used.
func dryAirRefractivity(wl float64) float64 {
	wlsq := wl * wl

	return 77.53484e-6 + (4.39108e-7+3.666e-9/wlsq)/wlsq
}
