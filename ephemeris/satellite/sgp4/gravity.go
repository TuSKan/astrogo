package sgp4

import (
	"fmt"
	"math"
)

// Gravity selects the Earth gravity model SGP4 evaluates with.
//
// This is not an accuracy knob. The constants are part of the *definition* of
// the mean elements being propagated: Space-Track fits them by running SGP4
// with WGS-72, so [WGS72] is the only choice that asks the model the question
// the elements are an answer to. The others exist to reproduce output produced
// with them, not to improve on it.
type Gravity int

const (
	// WGS72 is the model TLEs are fitted with, and the default.
	//
	// Measured on Vallado's reference suite, the whole 473-state set, switching
	// astrogo's propagation from WGS-84 to WGS-72 moved the median position
	// error from 34.6 m to under a tenth of a millimetre. That is the size of
	// the mismatch, and it is why this is not configuration.
	WGS72 Gravity = iota

	// WGS84 is the later geodetic realization — and for a TLE, the wrong one.
	//
	// Offered because a caller may hold elements that were genuinely fitted
	// this way, and because Vallado's own code offers it. Note that its mu of
	// 398600.5 km³/s² is not modern WGS 84 either: the standard now publishes
	// 398600.4418 and DE440 measures 398600.4355. This is the 1984-vintage
	// value frozen into the model, which is exactly why it is spelled out here
	// rather than read from the constants package.
	WGS84

	// WGS72Old is WGS-72 with the pre-1972 value of mu and a hardcoded xke,
	// kept by Vallado to reproduce historical output.
	//
	// The hardcoded xke is the whole point of it: WGS72 derives xke from its
	// own radius and mu, and this does not, so the two differ in the eighth
	// decimal even where their published constants agree. Do not "simplify"
	// that away.
	WGS72Old
)

// String implements [fmt.Stringer].
func (g Gravity) String() string {
	switch g {
	case WGS72:
		return "WGS-72"
	case WGS84:
		return "WGS-84"
	case WGS72Old:
		return "WGS-72 (old)"
	default:
		return fmt.Sprintf("Gravity(%d)", int(g))
	}
}

// valid reports whether g names a model this package knows.
func (g Gravity) valid() bool { return g == WGS72 || g == WGS84 || g == WGS72Old }

// gravityModel is one row of Vallado's getgravconst, in SGP4's own units.
//
// # Why these are literals and not references to the constants package
//
// [github.com/TuSKan/astrogo/constants] publishes the best current value of a
// constant and tracks new realizations as they are adopted. That is the right
// contract for a package describing the Earth, and the wrong one for a table
// that defines a model: if mu here changed, every TLE ever fitted would be
// being propagated by a model that no longer matches the one that produced it,
// and nothing would say so. The WGS84 row above is the case in point — its mu
// has already been superseded twice and must not move.
//
// The relationship is asserted instead of assumed: TestGravityAgreesWithThe-
// ConstantsPackage checks that the WGS-72 row matches constants.WGS72 where
// the two describe the same thing, and that the WGS-84 row deliberately does
// not.
type gravityModel struct {
	// radiusKM is Earth's equatorial radius, SGP4's unit of length.
	radiusKM float64
	// muKM3S2 is the geocentric gravitational constant in km³/s².
	muKM3S2 float64
	// xke is sqrt(GM) expressed in earth radii^1.5 per minute — the conversion
	// between SGP4's length and time units, and the quantity almost every
	// expression in the model is written in terms of.
	xke float64
	// tumin is 1/xke: minutes in one canonical time unit.
	tumin float64
	// j2, j3, j4 are the zonal harmonics of the model's gravity field.
	j2, j3, j4 float64
	// j3oj2 is j3/j2, precomputed because the model uses the ratio and never
	// j3 alone.
	j3oj2 float64
}

// constantsFor returns the gravity model g names.
//
// Unexported and returned by value: a caller choosing a model picks from the
// [Gravity] constants, and a caller who could construct one of these could
// construct a propagator that is not SGP4.
func constantsFor(g Gravity) gravityModel {
	switch g {
	case WGS84:
		m := gravityModel{
			radiusKM: 6378.137,
			muKM3S2:  398600.5,
			j2:       0.00108262998905,
			j3:       -0.00000253215306,
			j4:       -0.00000161098761,
		}
		m.xke = derivedXKE(m.radiusKM, m.muKM3S2)

		return finish(m)

	case WGS72Old:
		// xke is NOT derived here. Vallado hardcodes this value, and deriving
		// it from the radius and mu below gives 0.07436685316871385 — a
		// difference in the eighth decimal that is the entire reason this
		// model is distinguishable from WGS72.
		m := gravityModel{
			radiusKM: 6378.135,
			muKM3S2:  398600.79964,
			xke:      0.0743669161,
			j2:       0.001082616,
			j3:       -0.00000253881,
			j4:       -0.00000165597,
		}

		return finish(m)

	case WGS72:
		fallthrough

	default:
		m := gravityModel{
			radiusKM: 6378.135,
			muKM3S2:  398600.8,
			j2:       0.001082616,
			j3:       -0.00000253881,
			j4:       -0.00000165597,
		}
		m.xke = derivedXKE(m.radiusKM, m.muKM3S2)

		return finish(m)
	}
}

// derivedXKE is sqrt(GM) in earth radii^1.5 per minute.
//
// Written as 60/sqrt(r³/mu) rather than the algebraically identical
// 60*sqrt(mu/r³), because that is the association Vallado's code uses and the
// two do not produce the same last bit. A model reproduced to the last bit is
// the only kind that can be checked against a reference to the last bit.
func derivedXKE(radiusKM, muKM3S2 float64) float64 {
	return 60.0 / math.Sqrt(radiusKM*radiusKM*radiusKM/muKM3S2)
}

// finish fills the two members that are always functions of the others.
func finish(m gravityModel) gravityModel {
	m.tumin = 1.0 / m.xke
	m.j3oj2 = m.j3 / m.j2

	return m
}
