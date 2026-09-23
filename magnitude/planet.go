package magnitude

import (
	"fmt"
	"math"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// PlanetApparent computes the V-band apparent magnitude of a Solar System body
// using the Mallama & Hilton (2018) model, matching the Skyfield reference
// implementation.
//
// Supported bodies: Sun, Moon, Mercury, Venus, Mars, Jupiter, Saturn (with rings),
// Uranus, Neptune, Pluto.
//
// Returns ErrUnsupportedBody for Earth, satellites, or unknown IDs.
func PlanetApparent(p eph.Provider, target eph.ID, t time.Time) (float64, error) {
	if target == eph.Sun {
		return SunApparent(p, t)
	}

	if target == eph.Moon {
		return MoonApparent(p, t)
	}

	planetSt, err := p.State(target, t)
	if err != nil {
		return 0, fmt.Errorf("magnitude: planet state: %w", err)
	}

	sunSt, err := p.State(eph.Sun, t)
	if err != nil {
		return 0, fmt.Errorf("magnitude: sun state: %w", err)
	}

	// Sun→Planet (heliocentric) = Earth→Planet − Earth→Sun.
	sunToPlanet := [3]float64{
		planetSt.Pos.X - sunSt.Pos.X,
		planetSt.Pos.Y - sunSt.Pos.Y,
		planetSt.Pos.Z - sunSt.Pos.Z,
	}
	observerToPlanet := [3]float64{
		planetSt.Pos.X,
		planetSt.Pos.Y,
		planetSt.Pos.Z,
	}

	r := vecLen(sunToPlanet)
	delta := vecLen(observerToPlanet)

	// Phase angle via angle between sun_to_planet and observer_to_planet vectors,
	// matching Skyfield: ph_ang = angle_between(sun_to_planet, observer_to_planet).
	phAngRad := angleBetween(sunToPlanet, observerToPlanet)
	phAng := phAngRad * 180 / math.Pi // degrees

	switch target {
	case eph.Mercury:
		return mercuryMag(r, delta, phAng), nil
	case eph.Venus:
		return venusMag(r, delta, phAng), nil
	case eph.Mars:
		return marsMag(r, delta, phAng), nil
	case eph.Jupiter:
		return jupiterMag(r, delta, phAng), nil
	case eph.Saturn:
		return saturnMag(sunToPlanet, observerToPlanet, r, delta, phAng), nil
	case eph.Uranus:
		return uranusMag(sunToPlanet, observerToPlanet, r, delta, phAng), nil
	case eph.Neptune:
		return neptuneMag(r, delta, phAng, t), nil
	case eph.Pluto:
		return plutoMag(r, delta, phAng), nil
	default:
		return 0, ErrUnsupportedBody
	}
}

// ── Vector helpers ───────────────────────────────────────────────────────────

func vecLen(v [3]float64) float64 {
	return math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2])
}

func angleBetween(a, b [3]float64) float64 {
	dot := a[0]*b[0] + a[1]*b[1] + a[2]*b[2]
	la := vecLen(a)

	lb := vecLen(b)
	if la < 1e-30 || lb < 1e-30 {
		return 0
	}

	cos := dot / (la * lb)
	cos = math.Max(-1, math.Min(1, cos))

	return math.Acos(cos)
}

// subLatitude computes the planetocentric sub-latitude of a direction
// vector relative to a body's pole direction.
// Returns the latitude in degrees.
func subLatitude(pole, direction [3]float64) float64 {
	aRad := angleBetween(pole, direction)
	return aRad*180/math.Pi - 90.0
}

// ── Mercury — Mallama & Hilton 2018, Table 3 ────────────────────────────────

func mercuryMag(r, delta, phAng float64) float64 {
	distMod := 5 * math.Log10(r*delta)
	p2 := phAng * phAng
	p3 := p2 * phAng
	p4 := p2 * p2
	p5 := p4 * phAng
	p6 := p4 * p2
	phAngFactor := 6.3280e-02*phAng -
		1.6336e-03*p2 +
		3.3644e-05*p3 -
		3.4265e-07*p4 +
		1.6893e-09*p5 -
		3.0334e-12*p6

	return -0.613 + distMod + phAngFactor
}

// ── Venus — Mallama & Hilton 2018, Table 4 ──────────────────────────────────
// Two regimes with different V(1,0) offsets matching Skyfield.

func venusMag(r, delta, phAng float64) float64 {
	distMod := 5 * math.Log10(r*delta)

	var phAngFactor float64
	if phAng < 163.7 {
		// Polynomial in ascending order for Horner evaluation:
		// a0=0, a1=-1.044e-3, a2=+3.687e-4, a3=-2.814e-6, a4=+8.938e-9
		phAngFactor = (((8.938e-09*phAng-2.814e-06)*phAng+3.687e-04)*phAng-1.044e-03)*phAng + 0
	} else {
		// a0 = 236.05828 + 4.384, a1 = -2.81914, a2 = 8.39034e-3
		phAngFactor = ((8.39034e-03*phAng - 2.81914e+00) * phAng) + 236.05828 + 4.384
	}

	return -4.384 + distMod + phAngFactor
}

// ── Mars — Mallama & Hilton 2018, Table 5 ───────────────────────────────────
// Two regimes at α = 50° boundary with different V(1,0), matching Skyfield.

func marsMag(r, delta, phAng float64) float64 {
	rMag := 2.5 * math.Log10(r*r)
	deltaMag := 2.5 * math.Log10(delta*delta)
	distMod := rMag + deltaMag

	const geocentricLimit = 50.0

	var (
		phAngFactor float64
		v10         float64
	)
	if phAng <= geocentricLimit {
		v10 = -1.601
		phAngFactor = 2.267e-02*phAng - 1.302e-04*phAng*phAng
	} else {
		v10 = -0.367
		phAngFactor = -0.02573*phAng + 0.0003445*phAng*phAng
	}

	return v10 + distMod + phAngFactor
}

// ── Jupiter — Mallama & Hilton 2018, Table 6 ────────────────────────────────
// Two regimes at α = 12° boundary, matching Skyfield.

func jupiterMag(r, delta, phAng float64) float64 {
	distMod := 5 * math.Log10(r*delta)

	const geocentricLimit = 12.0

	var (
		phAngFactor float64
		v10         float64
	)
	if phAng <= geocentricLimit {
		v10 = -9.395
		phAngFactor = (6.16e-04*phAng - 3.7e-04) * phAng
	} else {
		v10 = -9.428
		phAngPi := phAng / 180.0
		// 5th-order polynomial in (α/180):
		inner := ((((-1.876*phAngPi+2.809)*phAngPi-0.062)*phAngPi-0.363)*phAngPi-1.507)*phAngPi + 1.0
		if inner <= 0 {
			inner = 1e-30
		}

		phAngFactor = -2.5 * math.Log10(inner)
	}

	return v10 + distMod + phAngFactor
}

// ── Saturn — Mallama & Hilton 2018, Eq. 10–12 ──────────────────────────────
// Ring+globe magnitude with sub-latitude geometry, matching Skyfield exactly.
//
// Saturn pole (ICRS unit vector): [0.08547883, 0.07323576, 0.99364475]

// saturnPole is Saturn's north pole direction in ICRS (IAU 2015).
var saturnPole = [3]float64{0.08547883, 0.07323576, 0.99364475}

func saturnMag(sunToPlanet, observerToPlanet [3]float64, r, delta, phAng float64) float64 {
	rMag := 2.5 * math.Log10(r*r)
	deltaMag := 2.5 * math.Log10(delta*delta)
	distMod := rMag + deltaMag

	// Compute sub-solar and sub-Earth saturnicentric latitudes.
	sunSubLat := subLatitude(saturnPole, sunToPlanet)
	earthSubLat := subLatitude(saturnPole, observerToPlanet)

	// Ring inclination: the geometric mean sqrt(β_sun · β_earth) when the Sun
	// and the observer are on the same side of the ring plane, else 0. It is
	// never negative. The rings brighten Saturn by how far they are open, and
	// the south face brightens it exactly as the north face does — a signed
	// value dimmed the planet by up to 1.9 mag whenever the south face was
	// lit (#375).
	product := sunSubLat * earthSubLat

	var subLatGeoc float64
	if product >= 0 {
		subLatGeoc = math.Sqrt(product)
	}

	const (
		geocentricPhaseLimit       = 6.5
		geocentricInclinationLimit = 27.0
	)

	if phAng <= geocentricPhaseLimit && subLatGeoc <= geocentricInclinationLimit {
		// Eq. 10: globe + rings, geocentric circumstances.
		sinBeta := math.Sin(subLatGeoc * math.Pi / 180)

		return -8.914 - 1.825*sinBeta + 0.026*phAng -
			0.378*sinBeta*math.Exp(-2.25*phAng) + distMod
	}

	// Beyond geocentric limits: globe-only (Eq. 11/12).
	// Eq. 12: globe-alone beyond geocentric phase angle limit.
	p3 := phAng * phAng * phAng
	p4 := p3 * phAng

	return -8.94 + 2.446e-4*phAng + 2.672e-4*phAng*phAng -
		1.506e-6*p3 + 4.767e-9*p4 + distMod
}

// ── Uranus — Mallama & Hilton 2018, Table 8 ─────────────────────────────────
// Includes sub-solar/sub-Earth latitude correction, matching Skyfield.
//
// Uranus pole (ICRS unit vector): [-0.21199958, -0.94155916, -0.26176809]

var uranusPole = [3]float64{-0.21199958, -0.94155916, -0.26176809}

func uranusMag(sunToPlanet, observerToPlanet [3]float64, r, delta, phAng float64) float64 {
	distMod := 5.0 * math.Log10(r*delta)

	// Sub-latitude correction.
	sunSubLat := subLatitude(uranusPole, sunToPlanet)
	earthSubLat := subLatitude(uranusPole, observerToPlanet)
	subLatAvg := (math.Abs(sunSubLat) + math.Abs(earthSubLat)) / 2.0
	subLatFactor := -0.00084 * subLatAvg

	apMag := -7.110 + distMod + subLatFactor

	// Phase angle correction beyond geocentric limit.
	const geocentricLimit = 3.1
	if phAng > geocentricLimit {
		apMag += (1.045e-4*phAng + 6.587e-3) * phAng
	}

	return apMag
}

// ── Neptune — Mallama & Hilton 2018, Eq. 16–17 ──────────────────────────────
// Secular brightening + phase correction for year ≥ 2000, matching Skyfield.

func neptuneMag(r, delta, phAng float64, t time.Time) float64 {
	rMag := 2.5 * math.Log10(r*r)
	deltaMag := 2.5 * math.Log10(delta*delta)
	distMod := rMag + deltaMag

	// Eq. 16: magnitude at unit distance as function of time.
	// Clamp to [-7.00, -6.89].
	year := t.JulianEpochYear()
	v10 := -6.89 - 0.0054*(year-1980.0)
	v10 = math.Max(-7.00, math.Min(-6.89, v10))

	apMag := v10 + distMod

	// Eq. 17: phase angle factor, only for year ≥ 2000.
	const geocentricLimit = 1.9
	if phAng > geocentricLimit {
		if year >= 2000.0 {
			apMag += 7.944e-3*phAng + 9.617e-5*phAng*phAng
		} else {
			return math.NaN() // Unknown before 2000 at large phase angles.
		}
	}

	return apMag
}

// ── Pluto — Explanatory Supplement (1992), Table 7.48.1 ────────────────────
// A historical approximation, and presented as one:
//
//	V = −1.01 + 5 log10(r·Δ) + 0.041 α
//
// for Pluto and Charon unresolved, in Johnson V, with α the phase angle at the
// target in degrees. Mallama & Hilton (2018) do not calibrate Pluto, and
// neither does Skyfield. V(1,0) = −1.01 is the Explanatory Supplement's, for
// the pair together (Harris 1961), so Charon is already in it and must not be
// added again. The phase coefficient is Binzel & Mulholland's (1984), 0.041 ±
// 0.003 mag per degree, measured between α = 0.615° and 1.659°; Pluto never
// exceeds about 1.9° from Earth, and the linear law is not established nearer
// opposition than those data go.
//
// What it leaves out is larger than anything the coefficients could be
// refined by. Its normalization is fixed: it has no rotational light curve
// (the Supplement quotes 0.11 mag peak to peak in 1954 and 0.29 in 1981), no
// change with viewing aspect or epoch, and no mutual events. Resolved Hubble
// photometry modeled by Buie et al. (2010) puts the pair 0.27 mag fainter than
// this at one of its geometries, so discrepancies of tenths of a magnitude are
// possible. It is fit for a visibility estimate — which is how VisibleTonight
// uses it — and not for predicting a measurement.
//
// JPL Horizons' APmag, which also includes Charon, uses the same historical
// law with V(1,0) = −1.00, and astrogo agrees with it to 0.01 mag. That is a
// consistency check between two implementations of one formula, not a
// validation of either. Until #407 PlanetApparent listed Pluto as supported
// and returned ErrUnsupportedBody for it.

func plutoMag(r, delta, phAng float64) float64 {
	return -1.01 + 5*math.Log10(r*delta) + 0.041*phAng
}
