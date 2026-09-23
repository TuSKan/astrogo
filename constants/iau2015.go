package constants

import (
	"math"

	"github.com/TuSKan/astrogo/unit"
)

// IAUSet holds the currently-active IAU nominal astronomical constants:
// the astronomical unit, the nominal mean Earth radius, and the ten other
// Solar System bodies' published equatorial radii. "IAU2015" names the
// set's currently-active vintage, not a claim every member individually
// dates to 2015 — AstronomicalUnit is IAU 2012 Resolution B2;
// SunEquatorialRadius/JupiterEquatorialRadius are IAU 2015 Resolution B3,
// Table 1 (and Exact, since B3 defines its nominal conversion constants as
// exact by convention); the other eight body
// radii are IAU WGCCRE 2015 (Archinal et al. 2018, Celestial Mechanics
// and Dynamical Astronomy 130:22, Table 4) and are measured, not exact.
// Their Uncertainty values are the real published 1σ figures — verified
// against JPL SSD's Planetary Physical Parameters
// (ssd.jpl.nasa.gov/planets/phys_par.html) and Satellite Physical
// Parameters (ssd.jpl.nasa.gov/sats/phys_par/) pages, both of which cite
// this same Table 4 — not fabricated. Pluto's relative uncertainty
// (1.6/1188.3 ≈ 1.3e-3) is the largest in this whole package, well above
// every other member here, since its shape remains the least precisely
// determined of the bodies in this table.
//
// MeanEarthRadius is not from B3, which defines no mean Earth radius — only
// a nominal equatorial radius, 6,378,100 m, and a nominal polar radius,
// 6,356,800 m. It is 6,371 km, the WGS 84 ellipsoid's mean radius to the
// kilometer: the radius of the sphere of equal volume, (a²b)^(1/3), is
// 6,371,000.79 m, and the mean of the semi-axes, (2a+b)/3, 6,371,008.77 m.
// Not Exact. Until #403 it cited B3 and claimed to be exact.
//
// There is deliberately no EarthEquatorialRadius member — Earth's
// equatorial radius is WGS84.SemiMajorAxis, 6,378,137 m, exact to the WGS 84
// standard. B3's nominal value is 37 m shorter and is not carried.
//
// Two further members, SunGravitationalParameter and ObliquityJ2000, are
// single fixed values rather than the body-radius family above — see the
// package doc's Scope section for why they still belong here rather than
// in ephemeris/kepler, the one package that currently needs them.
type IAUSet struct {
	// Vintage names this set's currently-active IAU nominal-value realization.
	Vintage string

	AstronomicalUnit          Constant
	MeanEarthRadius           Constant
	SunEquatorialRadius       Constant
	MoonEquatorialRadius      Constant
	MercuryEquatorialRadius   Constant
	VenusEquatorialRadius     Constant
	MarsEquatorialRadius      Constant
	JupiterEquatorialRadius   Constant
	SaturnEquatorialRadius    Constant
	UranusEquatorialRadius    Constant
	NeptuneEquatorialRadius   Constant
	PlutoEquatorialRadius     Constant
	SunGravitationalParameter Constant
	ObliquityJ2000            Constant
}

// Name reports the set's vintage, implementing [Set].
func (s IAUSet) Name() string { return s.Vintage }

// All returns every member of the set, in declaration order, implementing
// [Set].
func (s IAUSet) All() []Constant {
	return []Constant{
		s.AstronomicalUnit, s.MeanEarthRadius,
		s.SunEquatorialRadius, s.MoonEquatorialRadius, s.MercuryEquatorialRadius,
		s.VenusEquatorialRadius, s.MarsEquatorialRadius, s.JupiterEquatorialRadius,
		s.SaturnEquatorialRadius, s.UranusEquatorialRadius, s.NeptuneEquatorialRadius,
		s.PlutoEquatorialRadius, s.SunGravitationalParameter, s.ObliquityJ2000,
	}
}

// IAU2015 is this package's IAU nominal-value constant table. Every value
// is byte-identical to this package's previous flat-const form — this
// refactor changes no number, only the access shape. Treat it as
// read-only.
var IAU2015 = IAUSet{
	Vintage: "IAU 2015",

	AstronomicalUnit: Constant{
		Name: "astronomical unit", Symbol: "au",
		Value: 1.495_978_707e11, Unit: unit.Meter,
		Reference: "IAU 2012 Resolution B2", Exact: true,
	},
	MeanEarthRadius: Constant{
		Name: "mean Earth radius", Symbol: "R_E",
		Value: 6_371_000.0, Unit: unit.Meter,
		Reference: "WGS 84 ellipsoid mean radius to the kilometer: R3 = (a²b)^(1/3) = 6,371,000.79 m, R1 = (2a+b)/3 = 6,371,008.77 m",
	},
	// SunEquatorialRadius is B3's nominal solar radius, 695,700 km: the
	// photospheric radius of Haberreiter et al. (2008), 695,658 ± 140 km, to
	// the figures its uncertainty supports. It is a photospheric radius, not
	// an equatorial one; the name is the set's. JPL Horizons uses the same
	// value for the Sun's angular diameter.
	//
	// Until #403 this was 696,000 km, which is not B3's value but Auwers's
	// (1891) 959.63″ semi-diameter at 1 AU — the figure eclipse and almanac
	// work still uses, and which such code should carry under its own name.
	SunEquatorialRadius: Constant{
		Name: "nominal solar radius", Symbol: "R_Sun",
		Value: 695_700_000.0, Unit: unit.Meter,
		Reference: "IAU 2015 Resolution B3 (arXiv:1510.07674), nominal solar radius", Exact: true,
	},
	MoonEquatorialRadius: Constant{
		Name: "Moon equatorial radius", Symbol: "R_Moon",
		Value: 1_737_400.0, Uncertainty: 100.0, Unit: unit.Meter,
		Reference: "IAU WGCCRE 2015 (Archinal et al. 2018, CMDA 130:22), Table 4",
	},
	MercuryEquatorialRadius: Constant{
		Name: "Mercury equatorial radius", Symbol: "R_Mercury",
		Value: 2_440_530.0, Uncertainty: 40.0, Unit: unit.Meter,
		Reference: "IAU WGCCRE 2015 (Archinal et al. 2018, CMDA 130:22), Table 4",
	},
	VenusEquatorialRadius: Constant{
		Name: "Venus equatorial radius", Symbol: "R_Venus",
		Value: 6_051_800.0, Uncertainty: 1_000.0, Unit: unit.Meter,
		Reference: "IAU WGCCRE 2015 (Archinal et al. 2018, CMDA 130:22), Table 4",
	},
	MarsEquatorialRadius: Constant{
		Name: "Mars equatorial radius", Symbol: "R_Mars",
		Value: 3_396_190.0, Uncertainty: 100.0, Unit: unit.Meter,
		Reference: "IAU WGCCRE 2015 (Archinal et al. 2018, CMDA 130:22), Table 4",
	},
	JupiterEquatorialRadius: Constant{
		Name: "nominal Jovian equatorial radius", Symbol: "R_Jup",
		Value: 71_492_000.0, Unit: unit.Meter,
		Reference: "IAU 2015 Resolution B3, Table 1", Exact: true,
	},
	SaturnEquatorialRadius: Constant{
		Name: "Saturn equatorial radius", Symbol: "R_Saturn",
		Value: 60_268_000.0, Uncertainty: 4_000.0, Unit: unit.Meter,
		Reference: "IAU WGCCRE 2015 (Archinal et al. 2018, CMDA 130:22), Table 4",
	},
	UranusEquatorialRadius: Constant{
		Name: "Uranus equatorial radius", Symbol: "R_Uranus",
		Value: 25_559_000.0, Uncertainty: 4_000.0, Unit: unit.Meter,
		Reference: "IAU WGCCRE 2015 (Archinal et al. 2018, CMDA 130:22), Table 4",
	},
	NeptuneEquatorialRadius: Constant{
		Name: "Neptune equatorial radius", Symbol: "R_Neptune",
		Value: 24_764_000.0, Uncertainty: 15_000.0, Unit: unit.Meter,
		Reference: "IAU WGCCRE 2015 (Archinal et al. 2018, CMDA 130:22), Table 4",
	},
	PlutoEquatorialRadius: Constant{
		Name: "Pluto equatorial radius", Symbol: "R_Pluto",
		Value: 1_188_300.0, Uncertainty: 1_600.0, Unit: unit.Meter,
		Reference: "IAU WGCCRE 2015 (Archinal et al. 2018, CMDA 130:22), Table 4",
	},
	SunGravitationalParameter: Constant{
		Name: "nominal solar mass parameter", Symbol: "(GM)_Sun",
		Value: 1.327_124_4e20, Unit: cubicMeterPerSecondSquared,
		Reference: "IAU 2015 Resolution B3, Table 1", Exact: true,
	},
	ObliquityJ2000: Constant{
		Name: "mean obliquity of the ecliptic at J2000.0", Symbol: "eps_0",
		Value: 84_381.406 * math.Pi / 648_000, Unit: unit.Radian,
		Reference: "IAU 2006 Resolution B1 (P03; Hilton et al. 2006), mean obliquity at J2000.0",
		Exact:     true,
	},
}

// IAU is the currently-recommended IAU nominal-value vintage — reference
// this, not IAU2015 directly, unless you specifically need to pin that
// exact vintage for reproducibility. When the IAU adopts new nominal
// conversion constants, a new IAU-vintage Set/var is added and this
// single assignment is updated to point at it — no other call site in
// this repo needs to change. As with CODATA, this is a value copied at
// package-init time, not a live pointer.
var IAU = IAU2015
