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

## Measured accuracy

The table below is **generated** from the validation suites themselves, and the
region between its markers is rewritten by tooling — see
[`internal/metrology`](../internal/metrology) and its
`TestRenderAccuracyReport`. Everything outside those markers, including the
status table and the known-limitation notes further down, is written by hand
and stays that way: the reasoning about *why* a number is what it is, and which
hypotheses were tested and refuted, is the part no generator can produce.

Two columns deserve reading together. **Contract** is a claim about what the
software must achieve and why; it moves only when that reasoning changes.
Everything to its left is what was **measured** on the date in the last column.
Encoding one as the other is how a validation suite stops being able to fail
for the reason it exists, and this repository had that in two places before the
generated table existed.

<!-- BEGIN GENERATED ACCURACY — do not edit by hand -->

| Suite | Reference | Independence | N | p50 | p95 | p99 | Max | Contract | Status | Last verified |
|---|---|---|---:|---:|---:|---:|---:|---:|---|---|
| `coord.rv.barycentric` | Astropy SkyCoord.radial_velocity_correction 8.0.1 | shares SOFA/ERFA epv00 — consistency check | 175 | 3.64e-07 | 1.93e-06 | 2.1e-06 | 2.42e-06 | 0.001 km/s | ✅ verified | 2026-08-30 · `d9156eed` |
| `coord.rv.heliocentric` | Astropy SkyCoord.radial_velocity_correction 8.0.1 | shares SOFA/ERFA epv00 — consistency check | 175 | 3.64e-07 | 1.93e-06 | 2.1e-06 | 2.42e-06 | 0.001 km/s | ✅ verified | 2026-08-30 · `d9156eed` |
| `coord.topocentric.corpus` | JPL Horizons OBSERVER + VECTORS, AIRLESS | independent | 116 | 0.434 | 1.92 | 2.11 | 2.15 | 3 arcsec | ✅ verified | 2026-08-30 · `d9156eed` |
| `coord.topocentric.crosstrack` | JPL Horizons OBSERVER ephemeris, AIRLESS | independent | 68 | 0.379 | 1.88 | 1.93 | 1.96 | 3 arcsec | ✅ verified | 2026-08-30 · `d9156eed` |
| `coord.topocentric.elevation` | JPL Horizons OBSERVER ephemeris, AIRLESS | independent | 68 | 0.0665 | 0.272 | 0.352 | 0.372 | 3 arcsec | ✅ verified | 2026-08-30 · `d9156eed` |
| `coord.topocentric.separation` | JPL Horizons OBSERVER ephemeris, AIRLESS | independent | 68 | 0.388 | 1.88 | 1.94 | 1.97 | 3 arcsec | ✅ verified | 2026-08-30 · `d9156eed` |
| `ephemeris.jpl.horizons.position` | JPL Horizons VECTORS, geocentric, ICRF | shares JPL DE — consistency check | 3 | 5.49e-14 | 4.05e-12 | 4.41e-12 | 4.5e-12 | 1e-09 AU | ✅ verified | 2026-08-30 · `d9156eed` |
| `ephemeris.jpl.horizons.velocity` | JPL Horizons VECTORS, geocentric, ICRF | shares JPL DE — consistency check | 3 | 1.22e-14 | 9.02e-13 | 9.81e-13 | 1e-12 | 1e-10 AU/day | ✅ verified | 2026-08-30 · `d9156eed` |
| `ephemeris.jpl.smallbody.position` | JPL Horizons VECTORS, geocentric, ICRF | shares JPL small-body solution — consistency check | 66 | 1.72e-13 | 1.94e-13 | 2.07e-13 | 2.19e-13 | 1e-11 AU | ✅ verified | 2026-08-30 · `d9156eed` |
| `ephemeris.jpl.smallbody.velocity` | JPL Horizons VECTORS, geocentric, ICRF | shares JPL small-body solution — consistency check | 66 | 3.96e-14 | 4.61e-14 | 4.62e-14 | 4.62e-14 | 1e-12 AU/day | ✅ verified | 2026-08-30 · `d9156eed` |
| `ephemeris.kepler.galilean.laplace` | JPL Horizons VECTORS, jovicentric, ICRF | independent | 44 | 2.59e+03 | 5.21e+03 | 5.62e+03 | 5.88e+03 | 1e+04 km | ✅ verified | 2026-08-30 · `d9156eed` |
| `ephemeris.kepler.pluto` | JPL DE440 de440.bsp | shares JPL — the elements are a fit to a JPL development ephemeris — consistency check | 1004 | 0.0417 | 0.115 | 0.137 | 0.138 | 0.7 AU | ✅ verified | 2026-08-30 · `d9156eed` |
| `ephemeris.kepler.smallbody` | JPL Horizons SPK (Type 21) Horizons-generated small-body kernel | shares JPL small-body solution — SBDB and the kernel are one orbit fit — consistency check | 78 | 1e-06 | 8.36e-06 | 1.67e-05 | 1.78e-05 | 0.0015 AU | ✅ verified | 2026-08-30 · `d9156eed` |
| `ephemeris.sofa.jupiter` | gofa (Epv00 / Moon98) v1.19.1 | shares SOFA — consistency check | 516 | 0.0004 | 0.00087 | 0.00104 | 0.00115 | 0.00214 AU | ✅ verified | 2026-08-30 · `d9156eed` |
| `ephemeris.sofa.mars` | gofa (Epv00 / Moon98) v1.19.1 | shares SOFA — consistency check | 516 | 3.71e-05 | 0.000101 | 0.000138 | 0.000167 | 0.000219 AU | ✅ verified | 2026-08-30 · `d9156eed` |
| `ephemeris.sofa.mercury` | gofa (Epv00 / Moon98) v1.19.1 | shares SOFA — consistency check | 516 | 1.71e-06 | 4.73e-06 | 6.53e-06 | 8.94e-06 | 1.63e-05 AU | ✅ verified | 2026-08-30 · `d9156eed` |
| `ephemeris.sofa.moon` | gofa (Epv00 / Moon98) v1.19.1 | shares SOFA — consistency check | 516 | 3.34e-08 | 7.43e-08 | 9.79e-08 | 1.21e-07 | 4e-07 AU | ✅ verified | 2026-08-30 · `d9156eed` |
| `ephemeris.sofa.neptune` | gofa (Epv00 / Moon98) v1.19.1 | shares SOFA — consistency check | 516 | 0.00109 | 0.00155 | 0.00166 | 0.00169 | 0.00233 AU | ✅ verified | 2026-08-30 · `d9156eed` |
| `ephemeris.sofa.saturn` | gofa (Epv00 / Moon98) v1.19.1 | shares SOFA — consistency check | 516 | 0.00102 | 0.00291 | 0.00344 | 0.00358 | 0.00464 AU | ✅ verified | 2026-08-30 · `d9156eed` |
| `ephemeris.sofa.sun` | gofa (Epv00 / Moon98) v1.19.1 | shares SOFA — consistency check | 516 | 2.19e-08 | 4.28e-08 | 4.91e-08 | 6.18e-08 | 1.5e-07 AU | ✅ verified | 2026-08-30 · `d9156eed` |
| `ephemeris.sofa.uranus` | gofa (Epv00 / Moon98) v1.19.1 | shares SOFA — consistency check | 516 | 0.0027 | 0.00629 | 0.0064 | 0.00642 | 0.00953 AU | ✅ verified | 2026-08-30 · `d9156eed` |
| `ephemeris.sofa.venus` | gofa (Epv00 / Moon98) v1.19.1 | shares SOFA — consistency check | 516 | 6.44e-06 | 1.37e-05 | 1.7e-05 | 1.81e-05 | 2.6e-05 AU | ✅ verified | 2026-08-30 · `d9156eed` |
| `skybrightness.gambons.band_medians` | GAMBONS (Masana, Carrasco, Bara & Ribas) 2021 MNRAS 501, 5443; 2024 | independent | 6 | 0.207 | 0.273 | 0.28 | 0.282 | 1 mag | ✅ verified | 2026-08-30 · `d9156eed` |
| `time.scale.roundtrip.arithmetic` | scale round trip, A to B and back | independent | 120 | 0 | 9.41e-12 | 9.59e-12 | 9.59e-12 | 1e-06 s | ✅ verified | 2026-08-30 · `d9156eed` |
| `time.scale.roundtrip.modelled` | scale round trip, A to B and back | independent | 180 | 0 | 6.83e-06 | 0.943 | 0.943 | 5 s | ✅ verified | 2026-08-30 · `d9156eed` |

Every figure above is a **measured** value over the corpus named in its suite, not a bound. The contract column is the bound, and it is a separate claim: it says what the software must achieve and why, and it does not move when a measurement does. See `internal/metrology` for the reasoning, and each suite's own doc comment for the rationale behind its contract.

<!-- END GENERATED ACCURACY -->

---

## Status Table

> **Five rows below cite `gofa` as their reference, and that is weaker evidence than a tick
> suggests.** astrogo reaches every IAU reduction *through* gofa, via `internal/gofaext`, so
> comparing astrogo against gofa compares the library with its own dependency. Such a row
> establishes that astrogo drives the routine correctly — argument order, units, time scale,
> the direction of a rotation — and cannot establish that the underlying model is right, nor
> catch a fault the two share. Astropy would not fix this: it reaches the same algorithms
> through ERFA, which is SOFA-derived in turn.
>
> The rows this applies to are ICRS ↔ Galactic, ICRS ↔ Ecliptic, ICRS ↔ AltAz, astronomical
> time scales, and local sidereal time. The generated table above states the same thing per
> row and mechanically, in its Independence column, which is why that column exists.
>
> Genuinely independent references in this table are JPL Horizons, USNO, the NASA eclipse
> canons, AstroPixels, FINK/ZTF, IRSA/IPAC, GAMBONS and the published papers. Analytical
> invariants are independent of any implementation and are the only check that can catch an
> error two implementations share.


**The Evidence column is what makes a tick checkable.** Every row cites either a
generated suite from the table above — which carries a contract, a measured
distribution and the commit it was verified at — or the test file that
establishes the claim. `internal/docsguard` fails the build if a cited file
does not exist, if a cited suite is no longer produced, or if a measured suite
has no row pointing at it.

That last check is not hypothetical: it is what found that small-body and
SOFA-analytical ephemerides were being measured with no row in this table at
all. Without it the two halves of this document drift, the generated one
growing while the hand-written one keeps describing an older shape of the
library.

Note that a tick still means "there is a test", not "the number is small". The
Tolerance column is the bound a test asserts, not an achieved accuracy; for a
measured distribution, follow the Evidence link to the generated table.

| Area | Status | Evidence | Reference | Tolerance | Notes |
|---|---|---|---|---:|---|
| Angle normalization | ✅ validated | `angle/angle_test.go` | analytical | exact / 1e-15 | boundary wrapping tested |
| Angle formatting/parsing | ✅ validated | `angle/angle_test.go` | round-trip tests | string + tolerance | sexagesimal (HMS/DMS) formatting |
| Vector spherical/cartesian | ✅ validated | `vector/vector_test.go` | analytical | 1e-12 | pole cases tested |
| Geodetic ↔ ECEF | ✅ validated | `coord/geodesy_test.go` | WGS84 formulas | 1e-6 m / angular | pole/equator/general tested |
| ICRS ↔ Galactic | ✅ validated | `coord/transform_roundtrip_test.go` | `gofa` | 1e-12 | poles, GC, round-trip verified |
| ICRS ↔ Ecliptic | ✅ validated | `coord/transform_roundtrip_test.go` | `gofa` (IAU 2006) | 2e-5 deg | poles, Aries, round-trip verified |
| ICRS ↔ AltAz | ✅ validated | `coord.topocentric.separation`, `coord.topocentric.crosstrack`, `coord.topocentric.elevation` | `gofa` + invariants | 1e-7 deg | edge cases + round-trip verified |
| Coord FromUnitVector | ✅ validated | `coord/coord_test.go` | round-trip | 1e-10 deg | ICRS, Galactic, Ecliptic tested |
| Radial velocity correction | ✅ validated | `coord.rv.barycentric`, `coord.rv.heliocentric` | Astropy 8.0.1 `radial_velocity_correction` + analytical invariants | 1 m/s | 175 cases (5 named sites × 5 epochs × 7 target directions) agree to **0.7 mm/s**, 1400× inside the bound — see `coord.rv.*` in the generated table above. Compared **like for like**: Astropy's barycentric value is relativistic while astrogo is classical and says so, so the test compares against the plain projection of Astropy's own observer velocity. The relativistic gap is asserted rather than ignored — measured **4.66 m/s** against **4.649 predicted** from the terms astrogo documents as omitted (second-order Doppler 1.481 + solar redshift 2.959 + Earth redshift 0.209). **Not an independent check of the Earth ephemeris**: both sides reach the observer's barycentric velocity by SOFA's `epv00`, Astropy through pyerfa and astrogo through gofa, so what this validates is the projection, the sign convention, the site geodesy and the time scales; the shared core is covered separately by `ephemeris.sofa.sun` against DE440. Invariants still hold beside it: annual sinusoid **59.86 km/s** peak-to-peak against twice Earth's mean orbital speed of 59.56, perpendicular target zero to 1e-9, antipodal sign flip, diurnal amplitude ∝ cos(latitude) |
| Airmass | ✅ validated | `atmosphere/refraction_test.go` | analytical | 1e-4 | Pickering (2002) empirical interpolation |
| Atmospheric Refraction | ✅ validated | `atmosphere/refraction_test.go` | SOFA + analytical | 1e-4 deg | SOFA Refa/Refb (Refco via Apco13) + Saemundsson 1986 fallback |
| Astronomical time scales | ✅ validated | `time.scale.roundtrip.arithmetic`, `time.scale.roundtrip.modelled` | gofa / SOFA | 1e-12 d | UTC ↔ TAI ↔ TT ↔ TDB verified in `time`. **Note the scope:** this row covers `time`'s own conversions and never covered `ephemeris/jpl/lsk`, which parses the NAIF leap-second kernel independently — and which dropped the final table entry, so every UTC epoch after 2017-01-01 converted **one second early** and put the geocentric Sun ~30 km from DE440. Fixed, with an offline regression test that also guards the next leap second; the gap was invisible because nothing compared the two paths against each other |
| Local Sidereal Time | ✅ validated | `plan/usno_test.go` | gofa Gst06a (IAU 2006) | 0.5 deg | GAST at Greenwich J2000.0 |
| Ephemerides (JPL DE) | ✅ validated | `ephemeris.jpl.horizons.position`, `ephemeris.jpl.horizons.velocity` | JPL Horizons (DE441) | 1e-9 AU / 1e-10 AU·d⁻¹ | Sun, Moon, Mars, geocentric position and velocity at J2000.0 — see `ephemeris.jpl.horizons.*` in the generated table above for the measured distribution. **The tolerance column previously read 1e-7 AU / 1e-8 AU·d⁻¹ and was published here as though it were the achieved accuracy.** It never was: measured, the two agree to **5.5×10⁻¹⁴ AU** for the Sun and Mars and **4.5×10⁻¹² AU — 0.67 m — for the Moon**, so the old bound sat about two million times above the largest real residual and could not have failed for the reason it existed. The bound is now derived from the smallest fault worth catching: both sides evaluate the same JPL integration, so a disagreement is a kernel, segment or time-scale fault rather than an ephemeris difference, and one second of time-scale error moves the Moon about a kilometre (6.7×10⁻⁹ AU) |
| Ephemerides (small bodies) | ✅ validated | `ephemeris.jpl.smallbody.position`, `ephemeris.jpl.smallbody.velocity` | JPL Horizons | 1e-11 AU | SPK Type 21, six bodies from 1 to 5 AU; agreement 33 mm |
| Ephemerides (SOFA analytical, Sun/Moon) | ✅ validated | `ephemeris.sofa.sun`, `ephemeris.sofa.moon` | `gofa` (Epv00 / Moon98) vs DE440 | 1.5e-7 / 4e-7 AU | Consistency check — astrogo reaches these routines through gofa. Measured over 1972-2100 quarterly (516 samples each): Sun **max 9.2 km**, Moon **max 18.1 km**, both inside the routines' own published worst cases of 11.2 and 31.7 km. Was three epochs, one of them the wall clock |
| Ephemerides (SOFA analytical, planets) | ✅ validated | `ephemeris.sofa.mercury`, `ephemeris.sofa.venus`, `ephemeris.sofa.mars`, `ephemeris.sofa.jupiter`, `ephemeris.sofa.saturn`, `ephemeris.sofa.uranus`, `ephemeris.sofa.neptune` | `gofa` Plan94 vs DE440 | per body, from SOFA's own table | What `eph.Default()` is worth without a kernel, and it is not uniform: **Mercury 1,337 km, Mars 25,000 km, Uranus 960,000 km** at worst over 1800-2100. Every contract is the root-sum-square of Plan94's published maximum differences against DE200, carried out to the body's aphelion — not a measurement. The window used to start at 1972, because before it the comparison measured the leap-second boundary rather than the ephemeris — the Sun's residual read 302.3 km at 1971 against 7.9 km at 1972. It now samples the full 1800-2100 interval SOFA quotes Plan94 over, every body holding its undoubled contract, and a guard asserts the step is *gone* rather than dodged |
| Ephemerides (Kepler two-body) | ✅ validated | `ephemeris.kepler.pluto`, `ephemeris.kepler.smallbody`, `ephemeris.kepler.galilean.laplace` | JPL DE440 and Horizons-generated SPK | not accuracy claims — see below | The cost of propagating elements instead of reading a kernel. **Pluto max 0.138 AU** (SOFA has no Pluto, so this is what `eph.Default()` answers with); **small bodies max 2,658 km** over ±30 days from the epoch of osculation, the `plan.NewAsteroid` path; **Galilean satellites max 5,880 km**. Each contract sits at the geometric mean of the measured approximation error and a measured structural error, so it fails for a wrong frame and passes while the perturbations are merely unmodelled |
| Apparent / Observed Coordinates | ✅ validated | `coord.topocentric.corpus` | JPL Horizons (OBSERVER, AIRLESS) | 3 arcsec | Full astrometric → local topocentric pipeline. Measured by two independent routes that agree: a live 68-point matrix (4 bodies × 4 sites × 9 epochs) at **max 1.97″, p50 0.39″**, and a frozen 255-entry corpus at **max 2.07″, p50 0.43″** — see `coord.topocentric.*` above. **The 3″ bound is unchanged in value and completely changed in meaning:** it was previously documented as having been chosen because a live run measured 2.66″, which is a bound pinned to its own measurement and unable to fail for the reason it exists. It is now derived — Earth orientation is the only input the two implementations do not share, and at 15.041″ of hour angle per second of UT1, 3″ is a fifth of a second of UT1. The cross-track residual has a **signed mean of −0.505″** over a range of [−1.96, +0.47], so the long-investigated azimuth discrepancy is a bias rather than scatter — which the previous maximum-only summary could not say |
| Units algebra | ✅ validated | `unit/units_test.go` | analytical | exact | AU, Parsec, LightYear, Jansky verified |
| Quantity arithmetic | ✅ validated | `unit/units_test.go` | analytical | 1e-15 | Scale, Abs, Compare, conversion |
| Catalog Providers | ✅ validated | `catalog/catalog_test.go` | API References/Offline Caches | exact schemas | Dual JSON/XML parsing (STScI), Strict ADQL parsing (CDS TAP) |
| Planning / visibility | ✅ validated | `plan/visibility_test.go` | geometric sanity | logical | constraint system + scoring verified |
| Transit estimate | ✅ validated | `plan/plan_test.go` | geometric sanity | < 1 min | Brent's minimization, 10-min coarse bracket |
| Rise / Set / Transit events | ✅ validated | `plan/usno_test.go` | USNO API | ≤ 0.6 min | Chandrupatla root-finding + SOFA refraction model |
| Twilight events | ✅ validated | `plan/events_test.go` | geometric sanity | < 1 s | Civil (−6°), Nautical (−12°), Astronomical (−18°); sequence ordering verified |
| Event solver edge cases | ✅ validated | `plan/solver_domain_test.go` | analytical | logical | circumpolar, never-rise, polar midnight sun, high-lat no astronomical twilight |
| Sun Rise/Set/Transit | ✅ validated | `plan/usno_test.go` | USNO API | ≤ 0.5 min | 3 locations × 3 dates × 9 events, USNO threshold convention |
| Moon Rise/Set/Transit | ✅ validated | `plan/usno_test.go` | USNO API | ≤ 0.6 min | 3 locations × 3 dates, topocentric parallax via GeocentricToObserved |
| Altitude correction (8849m) | ✅ validated | `plan/usno_test.go` | internal consistency | ±1 min | Horizon dip 2.76° produces ~13 min shift at Everest |
| Moon Phases | ✅ validated | `plan/usno_test.go` | USNO API | ≤ 1 min | 12 consecutive phases (Jan–Mar 2026) |
| Moon Phases (historical) | ✅ validated | `plan/astropixels_test.go` | [AstroPixels](https://astropixels.com/ephemeris/phasescat/phasescat.html) | ≤ 6.0 min | 44,524 phases across 9 centuries (1–2100 CE), mean Δ=1.87 min |
| Earth's Seasons | ✅ validated | `plan/usno_test.go` | USNO API | 2–4 min | 4 events (2026), aberration-corrected ecliptic longitude |
| Celestial Navigation (AltAz) | ✅ validated | `plan/usno_test.go` | USNO API | 0.002° | Sub-arcsecond stellar altitude accuracy |
| Perihelion/Aphelion | ✅ validated | `plan/usno_test.go` | USNO API | ≤ 1 min | Brent's minimization on Earth-Sun distance |
| Lunar Eclipse Detection | ✅ validated | `plan/phases_test.go` | NASA Eclipse Catalog | date-exact | 2/2 eclipses detected for 2026 (Danjon limit filter) |
| Solar Eclipse Detection | ✅ validated | `plan/phases_test.go` | NASA Eclipse Catalog | date-exact | 2/2 eclipses detected for 2026 (ecliptic latitude filter) |
| Lunar Eclipse (historical) | ✅ validated | `plan/nasa_eclipse_test.go` | [NASA 5MC Lunar](https://eclipse.gsfc.nasa.gov/LEcat5/LEcatalog.html) | ≤ 1.3 min | 1424/1424 eclipses detected across 6 centuries (1–2000 CE), mean Δ=0.8 min |
| Solar Eclipse (historical) | ✅ validated | `plan/nasa_eclipse_test.go` | [NASA 5MC Solar](https://eclipse.gsfc.nasa.gov/SEcat5/SEcatalog.html) | ≤ 1.4 min | 1383/1383 eclipses detected across 6 centuries (1–2000 CE), mean Δ=0.8 min |
| ΔT (TT−UT1) | ✅ validated | `time/deltat_test.go` | [NASA ΔT Polynomial](https://eclipse.gsfc.nasa.gov/LEcat5/deltatpoly.html) | ≤ 0.9 s | Espenak & Meeus 2006 + n-dot correction, cross-validated against 1187 NASA catalog entries, mean error 0.3 s |
| Planetary magnitude | ✅ validated | `magnitude/magnitude_test.go` | Mallama & Hilton (2018) | 0.1 mag | Mercury–Neptune phase-curve polynomials, Saturn ring tilt, Neptune secular brightening |
| Asteroid magnitude (HG) | ✅ validated | `magnitude/magnitude_test.go` | Bowell (1989) / Muinonen (2010) | 0.01 mag | H,G + H,G₁,G₂ + H,G₁₂* phase functions, spline knot validation at α=30°,60°,90° |
| Asteroid magnitude (sHG1G2) | ✅ validated | `magnitude/fink_test.go` | [FINK/ZTF phunk pipeline](https://api.ztf.fink-portal.org) | 0.025 mag | Carry et al. (2024) 7-parameter spin-geometry model, validated against 186 r-band observations of 8467 Benoitcarry: mean Δ=0.011, RMS=0.013, 100% within 0.025 mag |
| Comet magnitude | ✅ validated | `magnitude/magnitude_test.go` | IAU standard | 0.1 mag | M₁/k₁ total + M₂/k₂ nuclear models |
| Satellite magnitude | ✅ validated | `magnitude/magnitude_test.go` | McCants/Molczan | 0.1 mag | Sphere/cylinder phase functions, range scaling |
| Star extinction | ✅ validated | `atmosphere/transfer_test.go` | Bouguer law | 0.01 mag | Altitude-dependent k(λ), Gaia G→V transformation |
| FINK SSOFT provider | ✅ validated | `catalog/fink/fink_test.go` | [FINK REST API v2.5](https://api.ztf.fink-portal.org/swagger.json) | exact schema | Single-object JSON + bulk parquet, r-band preference, fit/status filtering, version pinning (v2025.04) |
| Natural sky, end to end | ✅ validated | `skybrightness.gambons.band_medians` | GAMBONS published run (Masana et al. 2021, 2024) | 0.05 mag | Astronomical sky with airglow removed: **21.79 against their 21.74** at the Barcelona zenith — two implementations sharing no code, no catalogue aggregation and no radiative transfer. Including airglow the gap is +0.28, which is a disagreement about airglow level rather than about transport |
| Scattered moonlight | ✅ validated | `magnitude/rolo_test.go` | Kieffer & Stone (2005) ROLO 311g + Winkler (2022) | ~1 mag | **18.9 mag/arcsec² in V** for a near-full Moon, against an independently-known ~18. Deliberately **not** Krisciunas & Schaefer (1991): that is a closed-form V-band fit with no spectrum to project through a passband or an instrument |
| Artificial skyglow, clear air | ⚠️ physical claims only | `skybrightness/artificial_component_test.go` | Kocifaj, Bará & Falchi (2022) | — | Falls with distance, and is homogeneous in source strength to **2×10⁻¹⁶**. An absolute check needs a real emitter inventory; satellite radiance alone cannot supply one, since the same VIIRS pixel is produced by many different real installations. See [`docs/skybrightness.md`](skybrightness.md) §16 |
| Artificial skyglow, under cloud | ✅ validated | `skybrightness/artificial_component_test.go` | Kocifaj, Falchi & Kundracik (2025) | sign and order | Over a city an overcast deck amplifies the zenith **88×**; 60 km away the same deck **screens at 0.80×**. Both signs come out of the geometry, which is what a universal cloud multiplier cannot do. Reproducing their Žilina run: **122.5×** at the zenith against their "more than fifteenfold", **57.8×** horizontal illuminance against "more than fourfold" |
| Integrated starlight map | ✅ validated | `skybrightness/component_invariants_test.go` | Gaia DR3 + Tycho-2 | see notes | The map's absolute scale has no free parameter, so validating it means validating its three links: Gaia G VEGAMAG zero point **25.687367** (scatter 3×10⁻⁷ over 177,426 sources), G→V transformation **−0.002 mag** against 4,000 Tycho-2 stars, and HEALPix tiling **exact** on counts with flux conserved to 2.4×10⁻¹¹ **This row asserted a validated map while its validation suite could not run:** the synchronous Gaia path requested CSV and parsed CSV, but the default endpoint answers VOTable regardless, so all five of its network- and validation-tagged tests failed on an XML document handed to a CSV reader. Neither tag runs in ordinary CI, so nothing reported it. Fixed by sniffing the payload; the figures beside this note were re-confirmed afterwards |
| SFD dust map, local vs service | ✅ validated | `skybrightness/dataset/dust/dust_test.go` | IRSA/IPAC dust service | median ratio 1.00001 | 1,979 directions; 5th–95th percentile 0.956–1.056, the spread being interpolation across a 2.37′ pixel |
| Gaia archive agreement | ✅ validated | `catalog/gaia/gaia_test.go` | ESA `gea.esac.esa.int` vs Gaia@AIP | 0.0000 mas | 340 sources over one cone at the north galactic pole, with identical source sets. The field is sized against the query's `TOP N` cap on purpose — a truncated result is an arbitrary subset, so two archives could differ in truncation rather than in data |
| CAMS aerosol optical depth | ✅ validated | `atmosphere/dataset/cams/ground_truth_test.go` | physical geography | orientation | The ECMWF grid convention is an assumption a reader cannot see, so it is checked against where aerosol actually is: Indo-Gangetic **1.07** and eastern China 0.69 against Antarctic **0.043** and mid-Pacific 0.157 |
| Zodiacal light | ✅ validated | `skybrightness/component_audit_test.go` | Leinert et al. (1998) Table 17 | analytical | Bilinear interpolation of the 500 nm SI radiance table; cross-validated against Table 16's S10(V)⊙ values via the 1.28×10⁻⁸ W conversion |
| Airglow | ✅ validated | `skybrightness/airglow_test.go` | ESO SkyCalc (live) | analytical | van Rhijn geometry over a fetched zenith spectrum, not a constant floor. The band mean over 500–600 nm is **22.37 mag/arcsec²** at Paranal with msolflux 100, which is what dark-site zenith airglow is; an independent hand-worked example in the offline tests lands at 22.00 |

> **Note:** Both the [NASA Five Millennium Eclipse Catalogs](https://eclipse.gsfc.nasa.gov/LEcat5/LEcatalog.html) and the [AstroPixels Moon Phase Tables](https://astropixels.com/ephemeris/phasescat/phasescat.html) are computed by **Fred Espenak** using the same ΔT model (Espenak & Meeus 2006). The `time.DeltaT()` polynomial includes the secular acceleration correction `c = -0.000012932*(y-1955)²` to convert from Morrison & Stephenson's assumed n-dot (−26.0 arcsec/cy²) to the Lunar Laser Ranging value (−25.858 arcsec/cy²) used by both ELP-2000/82 and DE441. For historical dates (pre-1972), `TT()` and `TDB()` automatically apply ΔT, so users never need to handle time scale conversion manually.

---

## Known Incomplete Areas

The following areas are not yet considered scientifically complete:

- Advanced observation scheduling optimization
- **Radial-velocity correction is now cross-checked against Astropy** (175 cases, 0.7 mm/s), which closes the gap this list previously recorded. What remains open is narrower: astrogo is a classical projection and does not implement the Wright & Eastman (2014) terms — gravitational redshift, light-travel time to the barycentre, and the effect of the target's own proper motion and parallax on the projection geometry. Measured, those amount to 4.66 m/s against Astropy's relativistic value, so sub-1-m/s precision-RV work needs the full treatment and this does not provide it.
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