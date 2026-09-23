package constants

import "github.com/TuSKan/astrogo/unit"

// WGS72Set holds the defining and adopted parameters of the World Geodetic
// System 1972 — superseded for geodesy by WGS 84, and still the system every
// two-line element set is expressed in.
//
// # Why a superseded standard is a first-class set here
//
// A TLE does not carry a position. It carries mean elements, which are the
// output of fitting observations *through SGP4 itself*, and that fit is
// performed with WGS 72 constants. The elements only mean what they say to a
// propagator configured the same way; handing them to one built on WGS 84 asks
// a different model to interpret numbers this one produced. Measured against
// Vallado's reference suite, astrogo's own satellite positions were 93 times
// further from the reference for exactly that reason until it was corrected.
//
// So this is not a historical curiosity kept for completeness. It is the active
// geodetic realization for an entire class of astrogo's inputs, and
// [constants.WGS84]'s doc comment anticipated it: "if a second ellipsoid
// standard is ever needed (GRS80, WGS72, ...), add it as its own set at that
// point."
//
// # A structural difference from WGS 84, and why J2 is here
//
// WGS 84 defines a, 1/f, GM and omega, and derives J2. WGS 72 defines a, GM,
// **J2** and omega, and *derives* 1/f. So the two sets are not mirror images:
// DynamicalFormFactor is one of this standard's defining four, while
// InverseFlattening is the value it publishes as a consequence. Both are exact
// in the sense that matters here — adopted by convention, not measured.
//
// That is also why J2 belongs in this package rather than with the propagator
// that consumes it. It is not a modeling choice somebody made; it is part of
// what "WGS 72" means.
//
// # What is deliberately not here
//
// J3 and J4. SGP4 evaluates them alongside the four above, and unlike J2 they
// are not defining parameters of the reference system — they are further terms
// of a gravity model, and this package publishes no Earth gravity model. They
// stay with the propagator, frozen, because they are part of *that* model's
// definition. See docs/sgp4.md.
//
// # Why reading these live is safe, when reading WGS 84's would not be
//
// WGS 72 is a closed standard. It was superseded in 1984 and will never gain a
// new realization, so a consumer that reads these values can never be moved
// underneath by a revision. That is not true of WGS 84 — whose GM here is
// already a different number from the one SGP4's own table calls "wgs84", and
// different again from DE440's measured value — which is why the propagator
// keeps a frozen copy of that one.
type WGS72Set struct {
	// Vintage names this realization of the standard.
	Vintage string

	SemiMajorAxis                   Constant
	InverseFlattening               Constant
	GeocentricGravitationalConstant Constant
	DynamicalFormFactor             Constant
	AngularVelocity                 Constant
}

// Name reports the set's vintage, implementing [Set].
func (s WGS72Set) Name() string { return s.Vintage }

// All returns every member of the set, in declaration order, implementing
// [Set].
func (s WGS72Set) All() []Constant {
	return []Constant{
		s.SemiMajorAxis,
		s.InverseFlattening,
		s.GeocentricGravitationalConstant,
		s.DynamicalFormFactor,
		s.AngularVelocity,
	}
}

// wgs72Source is the provenance shared by every member below.
const wgs72Source = "World Geodetic System 1972 (DMA); the realization Spacetrack " +
	"Report #3 and Vallado et al. (2006), AIAA 2006-6753, propagate TLEs with"

// WGS72 is the World Geodetic System 1972 realization TLEs are expressed in.
// Treat it as read-only: it is a package-level var only because a Constant
// (which embeds a unit.Unit) cannot be a Go const.
var WGS72 = WGS72Set{
	Vintage: "WGS 72",

	SemiMajorAxis: Constant{
		Name: "WGS 72 semi-major axis", Symbol: "a",
		Value: 6_378_135.0, Unit: unit.Meter,
		Reference: wgs72Source, Exact: true,
	},
	// Derived in WGS 72 from J2 rather than defining, unlike WGS 84's — see
	// the type's doc comment. The two ellipsoids differ by 2 m in a and by
	// about 0.2 m in the polar radius, which is why the distinction almost
	// never shows up in a ground track and always shows up in a propagator.
	InverseFlattening: Constant{
		Name: "WGS 72 reciprocal flattening", Symbol: "1/f",
		Value: 298.26, Unit: unit.One,
		Reference: wgs72Source, Exact: true,
	},
	// 398600.8 km³/s². Note it is NOT WGS 84's 398600.4418, and not DE440's
	// 398600.4355 either: the three differ in the fourth significant figure
	// and SGP4 means this one.
	GeocentricGravitationalConstant: Constant{
		Name: "WGS 72 geocentric gravitational constant", Symbol: "GM",
		Value: 3.986_008e14, Unit: cubicMeterPerSecondSquared,
		Reference: wgs72Source, Exact: true,
	},
	// One of WGS 72's defining four, and the term that makes an orbit
	// precess: SGP4 reads it directly, and ephemeris/satellite reproduces
	// SGP4's own perigee from it to decide which side of the simplified-drag
	// branch an element set falls on.
	DynamicalFormFactor: Constant{
		Name: "WGS 72 dynamical form factor", Symbol: "J2",
		Value: 1.082_616e-3, Unit: unit.One,
		Reference: wgs72Source, Exact: true,
	},
	AngularVelocity: Constant{
		Name: "WGS 72 angular velocity of the Earth", Symbol: "omega",
		Value: 7.292_115_147e-5, Unit: radianPerSecond,
		Reference: wgs72Source, Exact: true,
	},
}
