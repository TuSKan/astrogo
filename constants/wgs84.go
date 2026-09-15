package constants

import "github.com/TuSKan/astrogo/unit"

// WGS84Set holds the four defining parameters of the WGS 84 reference
// system: the semi-major axis, the reciprocal (inverse) flattening, the
// geocentric gravitational constant and Earth's angular velocity, all fixed
// exact by the standard itself. Unlike IAU2015/CODATA, there
// is a single active realization (NGA.STND.0036_1.0.0_WGS84, 2014) this
// library targets, so there is no epoch-versioned family here.
//
// A second ellipsoid standard was eventually needed, exactly as this comment
// used to predict: see [WGS72], which is the system every two-line element set
// is expressed in.
//
// The derived flattening f = 1/(1/f) is not a value this standard
// tabulates directly — see Derived.WGS84Flattening.
type WGS84Set struct {
	// Vintage names this realization of the standard.
	Vintage string

	SemiMajorAxis                   Constant
	InverseFlattening               Constant
	GeocentricGravitationalConstant Constant
	AngularVelocity                 Constant
}

// Name reports the set's vintage, implementing [Set].
func (s WGS84Set) Name() string { return s.Vintage }

// All returns every member of the set, in declaration order, implementing
// [Set].
func (s WGS84Set) All() []Constant {
	return []Constant{
		s.SemiMajorAxis,
		s.InverseFlattening,
		s.GeocentricGravitationalConstant,
		s.AngularVelocity,
	}
}

// WGS84 is the WGS 84 geodetic realization astrogo targets. Treat it as
// read-only: it is a package-level var only because a Constant (which
// embeds a unit.Unit) cannot be a Go const.
var WGS84 = WGS84Set{
	Vintage: "WGS 84 (NGA.STND.0036_1.0.0)",

	SemiMajorAxis: Constant{
		Name: "WGS 84 semi-major axis", Symbol: "a",
		Value: 6_378_137.0, Unit: unit.Meter,
		Reference: "NGA.STND.0036_1.0.0_WGS84 (2014), Table 3.1", Exact: true,
	},
	InverseFlattening: Constant{
		Name: "WGS 84 reciprocal flattening", Symbol: "1/f",
		Value: 298.257_223_563, Unit: unit.One,
		Reference: "NGA.STND.0036_1.0.0_WGS84 (2014), Table 3.1", Exact: true,
	},
	// The third of WGS 84's four defining parameters. It is not the best
	// current value of GM — DE440 fits 3.9860043550702266e14, and the two
	// are different kinds of number: this one is fixed by the standard so
	// that coordinates computed in WGS 84 are reproducible, while DE440's
	// is a measurement. Nor is it WGS 72's 3.986008e14, which is the one
	// SGP4 means; see [WGS72].
	GeocentricGravitationalConstant: Constant{
		Name: "WGS 84 geocentric gravitational constant", Symbol: "GM",
		Value: 3.986_004_418e14, Unit: cubicMeterPerSecondSquared,
		Reference: "NGA.STND.0036_1.0.0_WGS84 (2014), Table 3.1", Exact: true,
	},
	// The fourth of WGS 84's four defining parameters, alongside the three
	// above. It is the rotation rate of a site about the Earth's axis, and
	// so the size of the diurnal term in a topocentric radial velocity:
	// 0.465 km/s at the equator.
	AngularVelocity: Constant{
		Name: "WGS 84 nominal mean angular velocity", Symbol: "omega",
		Value: 7.292_115e-5, Unit: radianPerSecond,
		Reference: "NGA.STND.0036_1.0.0_WGS84 (2014), Table 3.1", Exact: true,
	},
}
