# Validation

`astrogo` is being validated incrementally against trusted astronomical references.

This document tracks:
- what has been validated
- what reference source was used
- expected tolerances
- known limitations

---

## Validation Philosophy

`astrogo` does not treat “plausible-looking output” as sufficient.

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
| Angle formatting/parsing | ✅ validated | round-trip tests | string + tolerance | sexagesimal formatting |
| Vector spherical/cartesian | ✅ validated | analytical | 1e-12 | pole cases tested |
| Geodetic ↔ ECEF | ✅ validated | WGS84 formulas | 1e-6 m / angular tolerance | pole/equator tested |
| ICRS ↔ Galactic | ✅ validated | `gofa` | 1e-12 | pole cases + round-trip verified |
| ICRS ↔ Ecliptic | ✅ validated | `gofa` (IAU 2006) | 2e-5 deg | poles + round-trip verified |
| ICRS ↔ AltAz | ✅ validated | `gofa` + invariants | 1e-8 deg | validated via `sky.AltAz` tests |
| Airmass | ✅ validated | analytical | 10⁻⁴ | plane-parallel + Pickering (1982) |
| Astronomical time scales | ✅ validated | gofa / JPL | 10⁻¹² d | UTC ↔ TT ↔ TDB conversions verified |
| Ephemerides | ✅ validated | JPL Horizons / SOFA | 10⁻⁷ AU | Sun, Moon, Planets verified |
| Planning / visibility | ✅ validated | geometric sanity | logical | unified constraint system verified |

---

## Known Incomplete Areas

The following areas are not yet considered scientifically complete:

- leap second handling
- UTC ↔ TAI ↔ TT ↔ TDB conversions
- Earth orientation parameters (EOP)
- high-precision apparent place calculations
- ephemerides beyond initial scaffolding
- atmospheric refraction models
- advanced scheduling optimization

---

## Validation Method Categories

### 1. Analytical invariants
Used when exact or near-exact mathematical relationships are known.

Examples:
- angle wrapping boundaries
- unit vector norms
- celestial equator altitude at poles
- spherical/cartesian round-trips

### 2. Reference implementation comparison
Used when a trusted scientific implementation exists.

Primary references:
- `gofa`
- Astropy
- published standards / tables

### 3. Round-trip consistency
Used where inverse transforms should approximately recover original values.

Examples:
- geodetic → ECEF → geodetic
- ICRS → Galactic → ICRS
- ICRS → AltAz → ICRS (where applicable)

---

## Validation Rules for New Features

Before a feature is considered “implemented”, it should ideally include:

- [ ] unit tests
- [ ] edge case tests
- [ ] at least one validation category above
- [ ] documented assumptions
- [ ] numerical tolerance justification

---

## Notes

A package or API being present does **not** imply scientific completeness.

When in doubt, treat results as provisional unless this document explicitly marks the feature as validated.