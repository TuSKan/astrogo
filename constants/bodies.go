package constants

// ── Solar/planetary equatorial radii ──────────────────────────────────────────
//
// Equatorial (not mean or polar) radii, matching the conventional meaning
// of "angular diameter" and JPL Horizons' own Ang-diam ephemeris quantity.
// Source: IAU 2015 Resolution B3, Table 1 (Sun and Jupiter — the nominal
// solar/planetary conversion constants; Earth's equatorial radius is
// already available as WGS84SemiMajorAxis above, which is exact to the
// WGS84 standard and consistent with B3's own Earth value) and the Report
// of the IAU Working Group on Cartographic Coordinates and Rotational
// Elements: 2015 (Archinal et al. 2018, Celestial Mechanics and Dynamical
// Astronomy 130:22, Table 4 — every other body below).

// SunEquatorialRadius is the IAU 2015 nominal solar radius, exact by
// definition. Source: IAU 2015 Resolution B3, Table 1.
const SunEquatorialRadius = 696_000_000.0 // m

// MoonEquatorialRadius is the Moon's equatorial radius.
// Source: IAU WGCCRE 2015, Table 4.
const MoonEquatorialRadius = 1_737_400.0 // m

// MercuryEquatorialRadius is Mercury's equatorial radius.
// Source: IAU WGCCRE 2015, Table 4.
const MercuryEquatorialRadius = 2_440_500.0 // m

// VenusEquatorialRadius is Venus's equatorial radius.
// Source: IAU WGCCRE 2015, Table 4.
const VenusEquatorialRadius = 6_051_800.0 // m

// MarsEquatorialRadius is Mars's equatorial radius.
// Source: IAU WGCCRE 2015, Table 4.
const MarsEquatorialRadius = 3_396_190.0 // m

// JupiterEquatorialRadius is the IAU 2015 nominal Jovian equatorial radius.
// Source: IAU 2015 Resolution B3, Table 1.
const JupiterEquatorialRadius = 71_492_000.0 // m

// SaturnEquatorialRadius is Saturn's equatorial radius.
// Source: IAU WGCCRE 2015, Table 4.
const SaturnEquatorialRadius = 60_268_000.0 // m

// UranusEquatorialRadius is Uranus's equatorial radius.
// Source: IAU WGCCRE 2015, Table 4.
const UranusEquatorialRadius = 25_559_000.0 // m

// NeptuneEquatorialRadius is Neptune's equatorial radius.
// Source: IAU WGCCRE 2015, Table 4.
const NeptuneEquatorialRadius = 24_764_000.0 // m

// PlutoEquatorialRadius is Pluto's equatorial radius.
// Source: IAU WGCCRE 2015, Table 4.
const PlutoEquatorialRadius = 1_188_300.0 // m
