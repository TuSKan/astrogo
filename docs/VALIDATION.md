# Validation

`astrogo` is being validated incrementally against trusted astronomical references.

This document tracks:
- what has been validated
- what reference source was used
- expected tolerances
- known limitations

---

## Validation Philosophy

`astrogo` does not treat "plausible-looking output" as sufficient.

A feature is only considered scientifically trustworthy when it is validated against one or more of:

- `gofa` / SOFA-derived routines
- Astropy
- Published astronomical reference values
- Analytical invariants with known physical meaning

Validation should be:
- reproducible
- tolerance-based
- explicit about assumptions

---

## Status Table

| Area | Status | Reference | Tolerance | Notes |
|---|---|---|---:|---|
| Angle normalization | ✅ validated | analytical | exact / 1e-15 | boundary wrapping tested |
| Angle formatting/parsing | ✅ validated | round-trip tests | string + tolerance | sexagesimal (HMS/DMS) formatting |
| Vector spherical/cartesian | ✅ validated | analytical | 1e-12 | pole cases tested |
| Geodetic ↔ ECEF | ✅ validated | WGS84 formulas | 1e-6 m / angular | pole/equator/general tested |
| ICRS ↔ Galactic | ✅ validated | `gofa` | 1e-12 | poles, GC, round-trip verified |
| ICRS ↔ Ecliptic | ✅ validated | `gofa` (IAU 2006) | 2e-5 deg | poles, Aries, round-trip verified |
| ICRS ↔ AltAz | ✅ validated | `gofa` + invariants | 1e-7 deg | edge cases + round-trip verified |
| Coord FromUnitVector | ✅ validated | round-trip | 1e-10 deg | ICRS, Galactic, Ecliptic tested |
| Airmass | ✅ validated | analytical | 1e-4 | Pickering (2002) empirical interpolation |
| Atmospheric Refraction | ✅ validated | analytical | 1e-4 deg | Bidirectional Trace (Bennett 1982 / Saemundsson 1986) |
| Astronomical time scales | ✅ validated | gofa / SOFA | 1e-12 d | UTC ↔ TAI ↔ TT ↔ TDB verified |
| Local Sidereal Time | ✅ validated | gofa Gst06a (IAU 2006) | 0.5 deg | GAST at Greenwich J2000.0 |
| Ephemerides (JPL DE) | ✅ validated | JPL Horizons | 1e-7 AU / 1e-8 AU·d⁻¹ | Sun, Moon, Planets (pos + vel) |
| Apparent / Observed Coordinates | ✅ validated | JPL Horizons (OBSERVER) | 1 arcsecond | Full Astrometric -> Local Topocentric Pipeline (EOP mapped) |
| Units algebra | ✅ validated | analytical | exact | AU, Parsec, LightYear, Jansky verified |
| Quantity arithmetic | ✅ validated | analytical | 1e-15 | Scale, Abs, Compare, conversion |
| Catalog Providers | ✅ validated | API References/Offline Caches | exact schemas | Dual JSON/XML parsing (STScI), Strict ADQL parsing (CDS TAP) |
| Planning / visibility | ✅ validated | geometric sanity | logical | constraint system + scoring verified |
| Transit estimate | ✅ validated | geometric sanity | < 1 min | Brent's minimization, 10-min coarse bracket |
| Rise / Set / Transit events | ✅ validated | USNO API | < 2 min | Chandrupatla root-finding solver |
| Twilight events | ✅ validated | geometric sanity | < 1 s | Civil (−6°), Nautical (−12°), Astronomical (−18°); sequence ordering verified |
| Event solver edge cases | ✅ validated | analytical | logical | circumpolar, never-rise, polar midnight sun, high-lat no astronomical twilight |
| Sun Rise/Set/Transit | ✅ validated | USNO API | < 1.3 min | 3 locations × 3 dates, topocentric + horizon dip |
| Moon Rise/Set/Transit | ✅ validated | USNO API | < 1.6 min | 3 locations × 3 dates, topocentric parallax via Reducer |
| Moon Phases | ✅ validated | USNO API | ≤ 1 min | 12 consecutive phases (Jan–Mar 2026) |
| Moon Phases (historical) | ✅ validated | [AstroPixels](https://astropixels.com/ephemeris/phasescat/phasescat.html) | ≤ 5.2 min | 44,574 phases across 9 centuries (1–2100 CE), mean Δ=1.8 min |
| Earth's Seasons | ✅ validated | USNO API | 2–4 min | 4 events (2026), aberration-corrected ecliptic longitude |
| Celestial Navigation (AltAz) | ✅ validated | USNO API | 0.002° | Sub-arcsecond stellar altitude accuracy |
| Perihelion/Aphelion | ✅ validated | USNO API | ≤ 1 min | Brent's minimization on Earth-Sun distance |
| Lunar Eclipse Detection | ✅ validated | NASA Eclipse Catalog | date-exact | 2/2 eclipses detected for 2026 (Danjon limit filter) |
| Solar Eclipse Detection | ✅ validated | NASA Eclipse Catalog | date-exact | 2/2 eclipses detected for 2026 (ecliptic latitude filter) |
| Lunar Eclipse (historical) | ✅ validated | [NASA 5MC Lunar](https://eclipse.gsfc.nasa.gov/LEcat5/LEcatalog.html) | ≤ 1.3 min | 1424/1424 eclipses detected across 6 centuries (1–2000 CE), mean Δ=0.8 min |
| Solar Eclipse (historical) | ✅ validated | [NASA 5MC Solar](https://eclipse.gsfc.nasa.gov/SEcat5/SEcatalog.html) | ≤ 1.4 min | 1383/1383 eclipses detected across 6 centuries (1–2000 CE), mean Δ=0.8 min |
| ΔT (TT−UT1) | ✅ validated | [NASA ΔT Polynomial](https://eclipse.gsfc.nasa.gov/LEcat5/deltatpoly.html) | ≤ 0.9 s | Espenak & Meeus 2006 + n-dot correction, cross-validated against 1187 NASA catalog entries, mean error 0.3 s |

> **Note:** Both the [NASA Five Millennium Eclipse Catalogs](https://eclipse.gsfc.nasa.gov/LEcat5/LEcatalog.html) and the [AstroPixels Moon Phase Tables](https://astropixels.com/ephemeris/phasescat/phasescat.html) are computed by **Fred Espenak** using the same ΔT model (Espenak & Meeus 2006). The `time.DeltaT()` polynomial includes the secular acceleration correction `c = -0.000012932*(y-1955)²` to convert from Morrison & Stephenson's assumed n-dot (−26.0 arcsec/cy²) to the Lunar Laser Ranging value (−25.858 arcsec/cy²) used by both ELP-2000/82 and DE441. For historical dates (pre-1972), `TT()` and `TDB()` automatically apply ΔT, so users never need to handle time scale conversion manually.

---

## Known Incomplete Areas

The following areas are not yet considered scientifically complete:

- Advanced observation scheduling optimization

---

## Validation Method Categories

### 1. Analytical invariants
Used when exact or near-exact mathematical relationships are known.

Examples:
- angle wrapping boundaries
- unit vector norms
- celestial equator altitude at poles
- spherical/cartesian round-trips
- twilight sequence ordering (Astro < Nautical < Civil < Sunrise)

### 2. Reference implementation comparison
Used when a trusted scientific implementation exists.

Primary references:
- `gofa` (SOFA-derived)
- JPL Horizons
- **U.S. Naval Observatory API** — gold standard for rise/set/transit, moon phases, seasons, celestial navigation
- **AstroPixels** — Fred Espenak's Six Millennium Catalog of Phases of the Moon (2000 BCE – 4000 CE)
- **NASA GSFC Eclipse Catalog** — Five Millennium Catalogs of Solar and Lunar Eclipses (2000 BCE – 3000 CE)
- Astropy
- Published standards / tables

See [`USNO.md`](./USNO.md) for the full USNO validation report with per-event residual analysis.

### 3. Round-trip consistency
Used where inverse transforms should approximately recover original values.

Examples:
- geodetic → ECEF → geodetic
- ICRS → Galactic → ICRS
- ICRS → Ecliptic → ICRS
- ICRS → AltAz → ICRS

---

## Validation Rules for New Features

Before a feature is considered "implemented", it should ideally include:

- [ ] unit tests
- [ ] edge case tests
- [ ] at least one validation category above
- [ ] documented assumptions
- [ ] numerical tolerance justification

---

## Notes

A package or API being present does **not** imply scientific completeness.

When in doubt, treat results as provisional unless this document explicitly marks the feature as validated.