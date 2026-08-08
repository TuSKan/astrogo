package constants

import (
	"math"

	"github.com/TuSKan/astrogo/unit"
)

// DerivedSet holds values with no measurement uncertainty or publication
// epoch to version: pure arithmetic (the angle-conversion factors), an
// exact-by-definition arithmetic quantity (JulianDaySeconds), and a value
// computed from another set's field rather than directly cited from a
// standard (WGS84Flattening, computed from WGS84.InverseFlattening).
// Every member is Exact with Uncertainty 0.
type DerivedSet struct {
	// Vintage is fixed to "derived" — this set has no publication epoch.
	Vintage string

	JulianDaySeconds        Constant
	RadiansPerDegree        Constant
	DegreesPerRadian        Constant
	ArcSecondsPerRadian     Constant
	WGS84Flattening         Constant
	StefanBoltzmannConstant Constant
}

// Name reports the set's vintage, implementing [Set].
func (s DerivedSet) Name() string { return s.Vintage }

// All returns every member of the set, in declaration order, implementing
// [Set].
func (s DerivedSet) All() []Constant {
	return []Constant{
		s.JulianDaySeconds, s.RadiansPerDegree, s.DegreesPerRadian,
		s.ArcSecondsPerRadian, s.WGS84Flattening, s.StefanBoltzmannConstant,
	}
}

// Derived holds this package's pure-arithmetic and cross-set-derived
// values — see DerivedSet's doc comment. Treat it as read-only.
//
// WGS84Flattening is initialized from WGS84.InverseFlattening.Value
// (Go's package-level var initialization is dependency-ordered, so WGS84
// is fully built before Derived — no init() needed) rather than repeating
// the 298.257223563 literal, so the cross-reference is machine-checked,
// not just documented.
var Derived = DerivedSet{
	Vintage: "derived",

	JulianDaySeconds: Constant{
		Name: "Julian day", Symbol: "D",
		Value: 86_400.0, Unit: unit.Second,
		Reference: "exact by definition (1 d = 24 x 60 x 60 s)", Exact: true,
	},
	RadiansPerDegree: Constant{
		Name: "radians per degree", Symbol: "rad/deg",
		Value: math.Pi / 180, Unit: unit.One,
		Reference: "derived: pi/180", Exact: true,
	},
	DegreesPerRadian: Constant{
		Name: "degrees per radian", Symbol: "deg/rad",
		Value: 180 / math.Pi, Unit: unit.One,
		Reference: "derived: 180/pi", Exact: true,
	},
	ArcSecondsPerRadian: Constant{
		Name: "arcseconds per radian", Symbol: "arcsec/rad",
		Value: 3600 * 180 / math.Pi, Unit: unit.One,
		Reference: "derived: 3600 x 180/pi", Exact: true,
	},
	WGS84Flattening: Constant{
		Name: "WGS 84 flattening", Symbol: "f",
		Value: 1.0 / WGS84.InverseFlattening.Value, Unit: unit.One,
		Reference: "derived: 1 / WGS84.InverseFlattening (NGA.STND.0036_1.0.0_WGS84)",
		Exact:     true,
	},
	// StefanBoltzmannConstant is computed directly from SI2019's exact
	// c, h, k_B via sigma = 2*pi^5*k_B^4 / (15*h^3*c^2) — since the 2019 SI
	// redefinition made c, h, and k_B exact by definition, sigma is exact
	// too, not merely derived from measured inputs. Computed here (not
	// hardcoded) so the value is machine-checked against SI2019, matching
	// WGS84Flattening's own convention above; a unit test cross-checks the
	// result against the published CODATA value (5.670374419e-8 W/(m2 K4)).
	StefanBoltzmannConstant: Constant{
		Name: "Stefan-Boltzmann constant", Symbol: "sigma",
		Value: 2 * math.Pow(math.Pi, 5) * math.Pow(SI2019.BoltzmannConstant.Value, 4) /
			(15 * math.Pow(SI2019.PlanckConstant.Value, 3) * SI2019.SpeedOfLight.Value * SI2019.SpeedOfLight.Value),
		Unit:      wattPerSquareMeterKelvin4,
		Reference: "derived: 2*pi^5*k_B^4 / (15*h^3*c^2), exact since the 2019 SI redefinition",
		Exact:     true,
	},
}
