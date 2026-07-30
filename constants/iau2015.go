package constants

import "github.com/TuSKan/astrogo/unit"

// IAUSet holds the currently-active IAU nominal astronomical constants:
// the astronomical unit, the nominal mean Earth radius, and the ten other
// Solar System bodies' published equatorial radii. "IAU2015" names the
// set's currently-active vintage, not a claim every member individually
// dates to 2015 — AstronomicalUnit is IAU 2012 Resolution B2;
// MeanEarthRadius/SunEquatorialRadius/JupiterEquatorialRadius are IAU 2015
// Resolution B3, Table 1 (and Exact, since B3 defines its nominal
// conversion constants as exact by convention); the other eight body
// radii are IAU WGCCRE 2015 (Archinal et al. 2018, Celestial Mechanics
// and Dynamical Astronomy 130:22, Table 4) and are measured, not exact —
// but that report's own headline Table 4 column carries no quoted
// uncertainty, so Uncertainty is 0 for those eight despite Exact being
// false; see Constant's own doc comment for why Exact, not Uncertainty ==
// 0, is the right test.
//
// There is deliberately no EarthEquatorialRadius member — Earth's
// equatorial radius is WGS84.SemiMajorAxis, exact to the WGS84 standard
// and consistent with B3's own Earth value.
//
// Known, deliberately unfixed wart: MercuryEquatorialRadius is WGCCRE
// Table 4's 2440.53 km rounded to 2440.5 km, unlike every other body here
// which matches Table 4 exactly. Un-rounding it changes a published
// number and belongs in a separate, value-changing change, not this one.
type IAUSet struct {
	// Vintage names this set's currently-active IAU nominal-value realization.
	Vintage string

	AstronomicalUnit        Constant
	MeanEarthRadius         Constant
	SunEquatorialRadius     Constant
	MoonEquatorialRadius    Constant
	MercuryEquatorialRadius Constant
	VenusEquatorialRadius   Constant
	MarsEquatorialRadius    Constant
	JupiterEquatorialRadius Constant
	SaturnEquatorialRadius  Constant
	UranusEquatorialRadius  Constant
	NeptuneEquatorialRadius Constant
	PlutoEquatorialRadius   Constant
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
		s.PlutoEquatorialRadius,
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
		Name: "nominal mean Earth radius", Symbol: "R_E",
		Value: 6_371_000.0, Unit: unit.Meter,
		Reference: "IAU 2015 Resolution B3, Table 1", Exact: true,
	},
	SunEquatorialRadius: Constant{
		Name: "nominal solar equatorial radius", Symbol: "R_Sun",
		Value: 696_000_000.0, Unit: unit.Meter,
		Reference: "IAU 2015 Resolution B3, Table 1", Exact: true,
	},
	MoonEquatorialRadius: Constant{
		Name: "Moon equatorial radius", Symbol: "R_Moon",
		Value: 1_737_400.0, Unit: unit.Meter,
		Reference: "IAU WGCCRE 2015 (Archinal et al. 2018, CMDA 130:22), Table 4",
	},
	MercuryEquatorialRadius: Constant{
		Name: "Mercury equatorial radius", Symbol: "R_Mercury",
		Value: 2_440_500.0, Unit: unit.Meter,
		Reference: "IAU WGCCRE 2015 (Archinal et al. 2018, CMDA 130:22), Table 4",
	},
	VenusEquatorialRadius: Constant{
		Name: "Venus equatorial radius", Symbol: "R_Venus",
		Value: 6_051_800.0, Unit: unit.Meter,
		Reference: "IAU WGCCRE 2015 (Archinal et al. 2018, CMDA 130:22), Table 4",
	},
	MarsEquatorialRadius: Constant{
		Name: "Mars equatorial radius", Symbol: "R_Mars",
		Value: 3_396_190.0, Unit: unit.Meter,
		Reference: "IAU WGCCRE 2015 (Archinal et al. 2018, CMDA 130:22), Table 4",
	},
	JupiterEquatorialRadius: Constant{
		Name: "nominal Jovian equatorial radius", Symbol: "R_Jup",
		Value: 71_492_000.0, Unit: unit.Meter,
		Reference: "IAU 2015 Resolution B3, Table 1", Exact: true,
	},
	SaturnEquatorialRadius: Constant{
		Name: "Saturn equatorial radius", Symbol: "R_Saturn",
		Value: 60_268_000.0, Unit: unit.Meter,
		Reference: "IAU WGCCRE 2015 (Archinal et al. 2018, CMDA 130:22), Table 4",
	},
	UranusEquatorialRadius: Constant{
		Name: "Uranus equatorial radius", Symbol: "R_Uranus",
		Value: 25_559_000.0, Unit: unit.Meter,
		Reference: "IAU WGCCRE 2015 (Archinal et al. 2018, CMDA 130:22), Table 4",
	},
	NeptuneEquatorialRadius: Constant{
		Name: "Neptune equatorial radius", Symbol: "R_Neptune",
		Value: 24_764_000.0, Unit: unit.Meter,
		Reference: "IAU WGCCRE 2015 (Archinal et al. 2018, CMDA 130:22), Table 4",
	},
	PlutoEquatorialRadius: Constant{
		Name: "Pluto equatorial radius", Symbol: "R_Pluto",
		Value: 1_188_300.0, Unit: unit.Meter,
		Reference: "IAU WGCCRE 2015 (Archinal et al. 2018, CMDA 130:22), Table 4",
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
