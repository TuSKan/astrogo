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
| Atmospheric Refraction | ✅ validated | SOFA + analytical | 1e-4 deg | SOFA Refa/Refb (Refco via Apco13) + Saemundsson 1986 fallback |
| Astronomical time scales | ✅ validated | gofa / SOFA | 1e-12 d | UTC ↔ TAI ↔ TT ↔ TDB verified |
| Local Sidereal Time | ✅ validated | gofa Gst06a (IAU 2006) | 0.5 deg | GAST at Greenwich J2000.0 |
| Ephemerides (JPL DE) | ✅ validated | JPL Horizons | 1e-7 AU / 1e-8 AU·d⁻¹ | Sun, Moon, Planets (pos + vel) |
| Apparent / Observed Coordinates | ✅ validated | JPL Horizons (OBSERVER) | 3 arcseconds (measured max 2.66″; real observatories mostly < 1″) | Full Astrometric -> Local Topocentric Pipeline (EOP mapped); 68-point matrix (4 bodies × 4 sites × up to 9 epochs, `TestObserverPrecisionMatrix`); total angular separation is the metric that behaves consistently — the Az/El split resisted 4 tested single-variable hypotheses (near-zenith projection, parallactic angle, day-of-year, EOP-prediction divergence — all refuted live) and one confirmed non-cause (airless-column assumption, confirmed correct against Horizons' own response header); see the test's doc comment for the full investigation |
| Units algebra | ✅ validated | analytical | exact | AU, Parsec, LightYear, Jansky verified |
| Quantity arithmetic | ✅ validated | analytical | 1e-15 | Scale, Abs, Compare, conversion |
| Catalog Providers | ✅ validated | API References/Offline Caches | exact schemas | Dual JSON/XML parsing (STScI), Strict ADQL parsing (CDS TAP) |
| Planning / visibility | ✅ validated | geometric sanity | logical | constraint system + scoring verified |
| Transit estimate | ✅ validated | geometric sanity | < 1 min | Brent's minimization, 10-min coarse bracket |
| Rise / Set / Transit events | ✅ validated | USNO API | ≤ 0.6 min | Chandrupatla root-finding + SOFA refraction model |
| Twilight events | ✅ validated | geometric sanity | < 1 s | Civil (−6°), Nautical (−12°), Astronomical (−18°); sequence ordering verified |
| Event solver edge cases | ✅ validated | analytical | logical | circumpolar, never-rise, polar midnight sun, high-lat no astronomical twilight |
| Sun Rise/Set/Transit | ✅ validated | USNO API | ≤ 0.5 min | 3 locations × 3 dates × 9 events, USNO threshold convention |
| Moon Rise/Set/Transit | ✅ validated | USNO API | ≤ 0.6 min | 3 locations × 3 dates, topocentric parallax via GeocentricToObserved |
| Altitude correction (8849m) | ✅ validated | internal consistency | ±1 min | Horizon dip 2.76° produces ~13 min shift at Everest |
| Moon Phases | ✅ validated | USNO API | ≤ 1 min | 12 consecutive phases (Jan–Mar 2026) |
| Moon Phases (historical) | ✅ validated | [AstroPixels](https://astropixels.com/ephemeris/phasescat/phasescat.html) | ≤ 6.0 min | 44,524 phases across 9 centuries (1–2100 CE), mean Δ=1.87 min |
| Earth's Seasons | ✅ validated | USNO API | 2–4 min | 4 events (2026), aberration-corrected ecliptic longitude |
| Celestial Navigation (AltAz) | ✅ validated | USNO API | 0.002° | Sub-arcsecond stellar altitude accuracy |
| Perihelion/Aphelion | ✅ validated | USNO API | ≤ 1 min | Brent's minimization on Earth-Sun distance |
| Lunar Eclipse Detection | ✅ validated | NASA Eclipse Catalog | date-exact | 2/2 eclipses detected for 2026 (Danjon limit filter) |
| Solar Eclipse Detection | ✅ validated | NASA Eclipse Catalog | date-exact | 2/2 eclipses detected for 2026 (ecliptic latitude filter) |
| Lunar Eclipse (historical) | ✅ validated | [NASA 5MC Lunar](https://eclipse.gsfc.nasa.gov/LEcat5/LEcatalog.html) | ≤ 1.3 min | 1424/1424 eclipses detected across 6 centuries (1–2000 CE), mean Δ=0.8 min |
| Solar Eclipse (historical) | ✅ validated | [NASA 5MC Solar](https://eclipse.gsfc.nasa.gov/SEcat5/SEcatalog.html) | ≤ 1.4 min | 1383/1383 eclipses detected across 6 centuries (1–2000 CE), mean Δ=0.8 min |
| ΔT (TT−UT1) | ✅ validated | [NASA ΔT Polynomial](https://eclipse.gsfc.nasa.gov/LEcat5/deltatpoly.html) | ≤ 0.9 s | Espenak & Meeus 2006 + n-dot correction, cross-validated against 1187 NASA catalog entries, mean error 0.3 s |
| Planetary magnitude | ✅ validated | Mallama & Hilton (2018) | 0.1 mag | Mercury–Neptune phase-curve polynomials, Saturn ring tilt, Neptune secular brightening |
| Asteroid magnitude (HG) | ✅ validated | Bowell (1989) / Muinonen (2010) | 0.01 mag | H,G + H,G₁,G₂ + H,G₁₂* phase functions, spline knot validation at α=30°,60°,90° |
| Asteroid magnitude (sHG1G2) | ✅ validated | [FINK/ZTF phunk pipeline](https://api.ztf.fink-portal.org) | 0.025 mag | Carry et al. (2024) 7-parameter spin-geometry model, validated against 186 r-band observations of 8467 Benoitcarry: mean Δ=0.011, RMS=0.013, 100% within 0.025 mag |
| Comet magnitude | ✅ validated | IAU standard | 0.1 mag | M₁/k₁ total + M₂/k₂ nuclear models |
| Satellite magnitude | ✅ validated | McCants/Molczan | 0.1 mag | Sphere/cylinder phase functions, range scaling |
| Star extinction | ✅ validated | Bouguer law | 0.01 mag | Altitude-dependent k(λ), Gaia G→V transformation |
| FINK SSOFT provider | ✅ validated | [FINK REST API v2.5](https://api.ztf.fink-portal.org/swagger.json) | exact schema | Single-object JSON + bulk parquet, r-band preference, fit/status filtering, version pinning (v2025.04) |
| Natural sky, end to end | ✅ validated | GAMBONS published run (Masana et al. 2021, 2024) | 0.05 mag | Astronomical sky with airglow removed: **21.79 against their 21.74** at the Barcelona zenith — two implementations sharing no code, no catalogue aggregation and no radiative transfer. Including airglow the gap is +0.28, which is a disagreement about airglow level rather than about transport |
| Scattered moonlight | ✅ validated | Kieffer & Stone (2005) ROLO 311g + Winkler (2022) | ~1 mag | **18.9 mag/arcsec² in V** for a near-full Moon, against an independently-known ~18. Deliberately **not** Krisciunas & Schaefer (1991): that is a closed-form V-band fit with no spectrum to project through a passband or an instrument |
| Artificial skyglow, clear air | ⚠️ physical claims only | Kocifaj, Bará & Falchi (2022) | — | Falls with distance, and is homogeneous in source strength to **2×10⁻¹⁶**. An absolute check needs a real emitter inventory; satellite radiance alone cannot supply one, since the same VIIRS pixel is produced by many different real installations. See [`docs/skybrightness.md`](skybrightness.md) §16 |
| Artificial skyglow, under cloud | ✅ validated | Kocifaj, Falchi & Kundracik (2025) | sign and order | Over a city an overcast deck amplifies the zenith **88×**; 60 km away the same deck **screens at 0.80×**. Both signs come out of the geometry, which is what a universal cloud multiplier cannot do. Reproducing their Žilina run: **122.5×** at the zenith against their "more than fifteenfold", **57.8×** horizontal illuminance against "more than fourfold" |
| Integrated starlight map | ✅ validated | Gaia DR3 + Tycho-2 | see notes | The map's absolute scale has no free parameter, so validating it means validating its three links: Gaia G VEGAMAG zero point **25.687367** (scatter 3×10⁻⁷ over 177,426 sources), G→V transformation **−0.002 mag** against 4,000 Tycho-2 stars, and HEALPix tiling **exact** on counts with flux conserved to 2.4×10⁻¹¹ |
| SFD dust map, local vs service | ✅ validated | IRSA/IPAC dust service | median ratio 1.00001 | 1,979 directions; 5th–95th percentile 0.956–1.056, the spread being interpolation across a 2.37′ pixel |
| Gaia archive agreement | ✅ validated | ESA `gea.esac.esa.int` vs Gaia@AIP | 0.0000 mas | 340 sources over one cone at the north galactic pole, with identical source sets. The field is sized against the query's `TOP N` cap on purpose — a truncated result is an arbitrary subset, so two archives could differ in truncation rather than in data |
| CAMS aerosol optical depth | ✅ validated | physical geography | orientation | The ECMWF grid convention is an assumption a reader cannot see, so it is checked against where aerosol actually is: Indo-Gangetic **1.07** and eastern China 0.69 against Antarctic **0.043** and mid-Pacific 0.157 |
| Zodiacal light | ✅ validated | Leinert et al. (1998) Table 17 | analytical | Bilinear interpolation of the 500 nm SI radiance table; cross-validated against Table 16's S10(V)⊙ values via the 1.28×10⁻⁸ W conversion |
| Airglow | ✅ validated | ESO SkyCalc (live) | analytical | van Rhijn geometry over a fetched zenith spectrum, not a constant floor. The band mean over 500–600 nm is **22.37 mag/arcsec²** at Paranal with msolflux 100, which is what dark-site zenith airglow is; an independent hand-worked example in the offline tests lands at 22.00 |

> **Note:** Both the [NASA Five Millennium Eclipse Catalogs](https://eclipse.gsfc.nasa.gov/LEcat5/LEcatalog.html) and the [AstroPixels Moon Phase Tables](https://astropixels.com/ephemeris/phasescat/phasescat.html) are computed by **Fred Espenak** using the same ΔT model (Espenak & Meeus 2006). The `time.DeltaT()` polynomial includes the secular acceleration correction `c = -0.000012932*(y-1955)²` to convert from Morrison & Stephenson's assumed n-dot (−26.0 arcsec/cy²) to the Lunar Laser Ranging value (−25.858 arcsec/cy²) used by both ELP-2000/82 and DE441. For historical dates (pre-1972), `TT()` and `TDB()` automatically apply ΔT, so users never need to handle time scale conversion manually.

---

## Known Incomplete Areas

The following areas are not yet considered scientifically complete:

- Advanced observation scheduling optimization
- **Artificial skyglow in clear air** is tested on the model's physical claims rather than against a measured sky. An absolute check needs a per-emitter inventory — flux, spectrum and upward emission function — and satellite radiance alone can determine only the first: the same VIIRS pixel is produced by many real installations differing in spectrum and in how much light they throw sideways rather than up.
- **Cloud reaches only the artificial term.** A cloud deck in the scene's atmosphere changes artificial skyglow and nothing else; moonlight, integrated starlight, diffuse galactic light, zodiacal light and airglow are all evaluated as though the sky were clear. Three separate models are missing behind that one sentence, not one.
- **The Illumina-v2 comparison at Observatorio del Teide** is a Level-3 target whose published numbers are already transcribed. It is blocked on Tenerife's lighting inventory rather than on the numbers.

All three are recorded with their unblocking conditions in [`docs/skybrightness.md`](skybrightness.md) §16.

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
- **FINK/ZTF phunk pipeline** — production sHG1G2 photometry for solar system objects (Carry et al. 2024)
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