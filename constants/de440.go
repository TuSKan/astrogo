package constants

// ─── JPL planetary ephemeris mass parameters ─────────────────────────────────

// EphemerisSet is a set of gravitational parameters from a JPL planetary
// ephemeris.
//
// # Why these are not in [IAUSet]
//
// IAU 2015 Resolution B3 publishes nominal mass parameters for exactly three
// bodies — the Sun, the Earth and Jupiter — and those are conversion
// constants, fixed by convention so that published quantities stay
// comparable. They are not a table of the solar system's masses, and B3 says
// so. Anything needing Mars or Uranus has to look elsewhere, and "elsewhere"
// is the ephemeris the positions are already coming from.
//
// # System versus body
//
// Every planet appears twice, and the difference is not a rounding detail.
// A *System* parameter is the planet plus its satellites, which is what a
// planetary ephemeris integrates and therefore what governs the motion of the
// system's barycentre about the Sun. The plain parameter is the planet's own
// mass, which is what governs a satellite's motion about it.
//
// The two differ by the mass of the satellites. Measured against the kernel:
// one part in 4,830 at Jupiter, one part in 4,045 at Saturn, and one part in
// 19.5 million at Mars, where Phobos and Deimos are nearly weightless.
//
// Which one a satellite orbit needs is not simply "the body". Two-body
// relative motion is governed by G(M_primary + M_satellite), so the right
// value is the sum, and the two published figures bracket it: the body
// parameter omits the satellite, the system parameter adds every satellite.
// For a negligible moon the body parameter is the closer of the two; for one
// that dominates its system, the system parameter is.
//
// Pluto is the case that settles it. Charon is 12% of Pluto, and its
// published period of 6.3872 days comes out at 6.3871 using Pluto's *system*
// parameter and 6.7648 — six percent long — using Pluto's own. An earlier
// version of this comment asserted the opposite; the period measurement
// corrected it.
//
// The Sun and the Earth-Moon barycentre have no plain counterpart here: the
// Sun has no satellites in this sense, and Earth's own parameter is
// [EarthGravitationalParameter], listed beside the Moon's for the same reason.
type EphemerisSet struct {
	// Vintage names the ephemeris these values are taken from.
	Vintage string

	SunGravitationalParameter Constant

	MercurySystemGravitationalParameter Constant
	VenusSystemGravitationalParameter   Constant
	EarthMoonGravitationalParameter     Constant
	MarsSystemGravitationalParameter    Constant
	JupiterSystemGravitationalParameter Constant
	SaturnSystemGravitationalParameter  Constant
	UranusSystemGravitationalParameter  Constant
	NeptuneSystemGravitationalParameter Constant
	PlutoSystemGravitationalParameter   Constant

	EarthGravitationalParameter   Constant
	MoonGravitationalParameter    Constant
	MarsGravitationalParameter    Constant
	JupiterGravitationalParameter Constant
	SaturnGravitationalParameter  Constant
	UranusGravitationalParameter  Constant
	NeptuneGravitationalParameter Constant
	PlutoGravitationalParameter   Constant
}

// Name reports the set's vintage, implementing [Set].
func (s EphemerisSet) Name() string { return s.Vintage }

// All returns every member of the set, in declaration order, implementing
// [Set].
func (s EphemerisSet) All() []Constant {
	return []Constant{
		s.SunGravitationalParameter,
		s.MercurySystemGravitationalParameter, s.VenusSystemGravitationalParameter,
		s.EarthMoonGravitationalParameter, s.MarsSystemGravitationalParameter,
		s.JupiterSystemGravitationalParameter, s.SaturnSystemGravitationalParameter,
		s.UranusSystemGravitationalParameter, s.NeptuneSystemGravitationalParameter,
		s.PlutoSystemGravitationalParameter,
		s.EarthGravitationalParameter, s.MoonGravitationalParameter,
		s.MarsGravitationalParameter, s.JupiterGravitationalParameter,
		s.SaturnGravitationalParameter, s.UranusGravitationalParameter,
		s.NeptuneGravitationalParameter, s.PlutoGravitationalParameter,
	}
}

// deSource cites where the system parameters come from.
const deSource = "JPL DE440 (Park, Folkner, Williams & Boggs 2021, AJ 161:105, " +
	"doi:10.3847/1538-3881/abd414), via NAIF gm_de440.tpc"

// satSource cites where the individual planet parameters come from.
//
// A different provenance from the system values above, and worth keeping
// distinct: NAIF's kernel takes bodies 1-10 from DE440's own ASTRO-VALUES
// file and the per-planet parameters from the natural-satellite ephemeris
// release forms, which are fitted separately and updated on their own
// schedule.
const satSource = "JPL natural satellite ephemeris release forms " +
	"(https://ssd.jpl.nasa.gov/ftp/sats/), via NAIF gm_de440.tpc"

// DE440 is the mass-parameter table from the DE440 planetary ephemeris,
// as NAIF publishes it in gm_de440.tpc.
//
// NAIF states these in km³/s²; they are stated here in m³/s² for consistency
// with the rest of this package, which is a shift of the decimal exponent by
// nine and changes no digit. TestDE440MatchesNAIF re-fetches the kernel and
// checks that, so a transcription slip fails rather than ships.
//
// No uncertainties: an ephemeris mass parameter is a fitted value whose
// meaning is tied to the fit it came from, and the kernel publishes no
// formal errors for them. They are not [Constant.Exact] either — nothing
// here is exact by convention, unlike the IAU nominal values.
var DE440 = EphemerisSet{
	Vintage: "DE440",

	SunGravitationalParameter: Constant{
		Name: "Sun mass parameter", Symbol: "GM_Sun",
		Value: 1.3271244004127942e20, Unit: cubicMeterPerSecondSquared,
		Reference: deSource,
	},

	MercurySystemGravitationalParameter: Constant{
		Name: "Mercury system mass parameter", Symbol: "GM_1",
		Value: 2.2031868551400003e13, Unit: cubicMeterPerSecondSquared,
		Reference: deSource,
	},
	VenusSystemGravitationalParameter: Constant{
		Name: "Venus system mass parameter", Symbol: "GM_2",
		Value: 3.2485859200000000e14, Unit: cubicMeterPerSecondSquared,
		Reference: deSource,
	},
	EarthMoonGravitationalParameter: Constant{
		Name: "Earth-Moon barycentre mass parameter", Symbol: "GM_3",
		Value: 4.0350323562548019e14, Unit: cubicMeterPerSecondSquared,
		Reference: deSource,
	},
	MarsSystemGravitationalParameter: Constant{
		Name: "Mars system mass parameter", Symbol: "GM_4",
		Value: 4.2828375815756102e13, Unit: cubicMeterPerSecondSquared,
		Reference: deSource,
	},
	JupiterSystemGravitationalParameter: Constant{
		Name: "Jupiter system mass parameter", Symbol: "GM_5",
		Value: 1.2671276409999998e17, Unit: cubicMeterPerSecondSquared,
		Reference: deSource,
	},
	SaturnSystemGravitationalParameter: Constant{
		Name: "Saturn system mass parameter", Symbol: "GM_6",
		Value: 3.7940584841799997e16, Unit: cubicMeterPerSecondSquared,
		Reference: deSource,
	},
	UranusSystemGravitationalParameter: Constant{
		Name: "Uranus system mass parameter", Symbol: "GM_7",
		Value: 5.7945563999999985e15, Unit: cubicMeterPerSecondSquared,
		Reference: deSource,
	},
	NeptuneSystemGravitationalParameter: Constant{
		Name: "Neptune system mass parameter", Symbol: "GM_8",
		Value: 6.8365271005803989e15, Unit: cubicMeterPerSecondSquared,
		Reference: deSource,
	},
	PlutoSystemGravitationalParameter: Constant{
		Name: "Pluto system mass parameter", Symbol: "GM_9",
		Value: 9.7550000000000000e11, Unit: cubicMeterPerSecondSquared,
		Reference: deSource,
	},

	EarthGravitationalParameter: Constant{
		Name: "Earth mass parameter", Symbol: "GM_399",
		Value: 3.9860043550702266e14, Unit: cubicMeterPerSecondSquared,
		Reference: deSource,
	},
	MoonGravitationalParameter: Constant{
		Name: "Moon mass parameter", Symbol: "GM_301",
		Value: 4.9028001184575496e12, Unit: cubicMeterPerSecondSquared,
		Reference: deSource,
	},
	MarsGravitationalParameter: Constant{
		Name: "Mars mass parameter", Symbol: "GM_499",
		Value: 4.282837362069909e13, Unit: cubicMeterPerSecondSquared,
		Reference: satSource,
	},
	JupiterGravitationalParameter: Constant{
		Name: "Jupiter mass parameter", Symbol: "GM_599",
		Value: 1.266865319003704e17, Unit: cubicMeterPerSecondSquared,
		Reference: satSource,
	},
	SaturnGravitationalParameter: Constant{
		Name: "Saturn mass parameter", Symbol: "GM_699",
		Value: 3.793120623436167e16, Unit: cubicMeterPerSecondSquared,
		Reference: satSource,
	},
	UranusGravitationalParameter: Constant{
		Name: "Uranus mass parameter", Symbol: "GM_799",
		Value: 5.793951256527211e15, Unit: cubicMeterPerSecondSquared,
		Reference: satSource,
	},
	NeptuneGravitationalParameter: Constant{
		Name: "Neptune mass parameter", Symbol: "GM_899",
		Value: 6.835103145462294e15, Unit: cubicMeterPerSecondSquared,
		Reference: satSource,
	},
	PlutoGravitationalParameter: Constant{
		Name: "Pluto mass parameter", Symbol: "GM_999",
		Value: 8.696138177608748e11, Unit: cubicMeterPerSecondSquared,
		Reference: satSource,
	},
}

// Ephemeris is the currently-recommended JPL ephemeris mass-parameter
// vintage — reference this, not [DE440] directly, unless a specific vintage
// has to be pinned for reproducibility. When a newer ephemeris is adopted, a
// new vintage set is added and this single assignment moves.
var Ephemeris = DE440
