// Package constants provides physical, astronomical, and geodetic
// constants as typed, versioned, self-describing values.
//
// # Design
//
// Every constant is a [Constant]: a value in SI base units together with
// its standard uncertainty, its [unit.Unit], the publication it comes
// from, and whether it is exact by definition. Constants are grouped into
// named sets, one per source and vintage, rather than published as a flat
// list of bare floats:
//
//	constants.SI2019      // exact by the 2019 SI redefinition: c, h, k_B
//	constants.CODATA2022  // measured fundamental constants: G, m_e, m_p, alpha, sigma_e
//	constants.CODATA2018  // the same five, previous CODATA adjustment
//	constants.IAU2015     // au, nominal mean Earth radius, 10 body equatorial radii
//	constants.WGS84       // the WGS 84 ellipsoid's defining a and 1/f
//	constants.Derived     // exact arithmetic/derived factors, no epoch to version
//
// A caller reads the value out of the field it wants:
//
//	a := constants.WGS84.SemiMajorAxis.Value            // 6378137 m
//	q := constants.IAU.AstronomicalUnit.Quantity()      // a unit.Quantity
//	km, _ := q.In(unit.Kilometer)
//
// [Sets] enumerates every set, and each set's All method enumerates its
// members, so a caller can print or archive the exact provenance of every
// number a reduction used.
//
// # Current-vintage aliases
//
// [IAU] and [CODATA] are unversioned aliases pointing at the
// currently-recommended vintage of each revisable family ([IAU2015] and
// [CODATA2022] today). Internal code in this repo — and callers who don't
// specifically need to pin a vintage for reproducibility — should
// reference these, not the year-suffixed name: when the IAU adopts new
// nominal values or a new CODATA adjustment ships, a new vintage is added
// and exactly one assignment (e.g. "var IAU = IAU2024") is updated, with
// no other call site anywhere needing to change. A caller reproducing a
// published reduction should instead pin the exact vintage it used by
// name (e.g. constants.CODATA2018), since IAU/CODATA's value at any given
// moment depends on which astrogo release it was built against.
//
// Because a [Constant] embeds a [unit.Unit], it cannot be a Go constant:
// the sets are package-level variables and a value selected from one
// cannot appear in a constant expression. A caller deriving a scale
// factor from one needs
//
//	var kmPerAU = constants.IAU.AstronomicalUnit.Value / 1e3
//
// rather than a const declaration. Treat every set variable (and IAU/
// CODATA) as read-only.
//
// # Scope
//
// Values published by an international standards body or fixed by exact
// definition live here: BIPM/CGPM (SI), CODATA/NIST, IAU resolutions and
// the IAU WGCCRE reports, and NGA's WGS 84 standard. Unlike earlier
// versions of this package, that explicitly includes values subject to
// periodic revision — the CODATA fundamental constants are re-adjusted
// roughly every four years, which is exactly why they are published as
// separate, individually addressable sets ([CODATA2022], [CODATA2018])
// instead of one silently-updated symbol. A caller reproducing a
// published reduction pins the vintage it used.
//
// What still does not belong here is anything model-dependent or
// series-valued: TT-TDB coefficients, nutation and precession series
// terms, and IERS Earth orientation parameters stay in the packages that
// implement the model they belong to (see time, coord, ephemeris).
//
// [IAU2015.SunGravitationalParameter] and [IAU2015.ObliquityJ2000] are a
// narrow, deliberate exception to "planetary mass parameters stay
// elsewhere": both are single fixed values published by name in the same
// IAU resolutions already represented in this set (B3's Table 1 for the
// former, IAU 2006 Resolution B1/P03 for the latter's mean-obliquity
// term), not a general mass-parameter table or a time-varying series —
// ephemeris/kepler is the one current consumer, needed for the two-body
// Kepler equation's mean motion and the perifocal-to-equatorial rotation.
//
// # Units and exactness
//
// Every [Constant.Value] is in SI base units, and every [Constant.Unit]
// has ScaleFactor 1 — Value and the SI base-unit value are always the
// same number. Dimensionless ratios (flattening, the fine-structure
// constant, the radian/degree scale factors) carry [unit.One].
//
// [Constant.Exact] is the only test of exactness. An Uncertainty of 0
// with Exact false means the source quotes no uncertainty — the IAU
// WGCCRE body radii are the case in point — not that the value is exact.
//
// # Dependencies
//
// This package depends only on unit. Both are primitives-layer packages
// (see CLAUDE.md's Architecture section), so this is a peer import within
// the layer, never an upward one; unit itself imports nothing from
// astrogo.
package constants
