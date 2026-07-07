# AstroGo Roadmap

**Pure-Go high-performance, scientifically reliable astronomy library** —
precision computation, observatory planning, and scalable data workflows.

---

# 🎯 Path to v1.0.0

astrogo is pre-1.0 (currently **v0.2.0**). The deliberate decision was to ship v0.2.0
first — the API is already in good shape after an extensive correctness/robustness/
release-readiness audit (see [CHANGELOG.md](../CHANGELOG.md)) — rather than commit to
the v1.0.0 API-stability promise while two catalog providers are still partial:

- `catalog/jpl` — name resolution only; Horizons result-text parsing not yet implemented
- `catalog/vizier` — `ConeSearch` only queries a single hardcoded 2MASS table

Once both are fully implemented, v1.0.0 is back on the table.

---

# ✅ Completed

## Phase 1 — Precision Astronomy

| # | Capability | Status |
|---|---|---|
| 1 | **Earth Orientation Parameters** — UT1−UTC, polar motion, deterministic fallback | ✅ v0.1.0 |
| 2 | **Coordinate Pipeline** — aberration, proper motion, parallax, topocentric apparent | ✅ v0.1.0 |
| 3 | **Atmospheric Refraction** — SOFA Refa/Refb at all altitudes, pluggable models | ✅ v0.1.0 |
| 4 | **Numerical Solver** — Chandrupatla root-finding (1997), Brent's minimization | ✅ v0.1.0 |
| 5 | **Planetary & Lunar Phenomena** — phases, seasons, apsides, eclipses, geometry events | ✅ v0.1.0 |
| 6 | **Scale-Aware Time** — full UTC↔TAI↔TT↔TDB↔UT1 graph, Fairhead TDB, explicit UT1 errors | ✅ v0.1.0 |
| 7 | **Visibility Boundary Refinement** — sub-second Chandrupatla + bisection | ✅ v0.1.0 |
| 8 | **API Hygiene** — nil guards, epsilon equality, defensive copies | ✅ v0.1.0 |

## Phase 2 — Scheduling Engine

| # | Capability | Status |
|---|---|---|
| 9 | **Scheduling Strategies** — Greedy, Priority, SwapOptimized (monotonic local search) | ✅ v0.1.0 |
| 10 | **Transition Modeling** — slew-time, filter change costs, setup overhead | ✅ v0.1.0 |
| 11 | **Explainable Output** — structured schedule, score breakdown, rejection reasons | ✅ v0.1.0 |

## Phase 3 — Validation & Scientific Trust

| # | Capability | Status |
|---|---|---|
| 12 | **USNO Validation** — rise/set ≤0.6 min, phases ≤1 min, eclipses date-exact | ✅ v0.1.0 |
| 13 | **Scientific CI** — integration/validation tags, tolerance drift detection, FINK match | ✅ v0.1.0 |
| 14 | **Benchmark Suite** — 40+ benchmarks across coord, time, atmosphere, plan | ✅ v0.1.0 |

## Phase 4 — Data & Photometry

| # | Capability | Status |
|---|---|---|
| 15 | **Catalog Data Layer** — SIMBAD, MAST, SBDB, VizieR, Gaia, OpenNGC, NORAD, FINK | ✅ v0.1.2 |
| 16 | **NORAD Satellite Tracking** — CelestTrak GP, SGP4, TEME→GCRS, pass prediction | ✅ v0.1.2 |
| 17 | **Apparent Magnitude** — planets, asteroids (sHG1G2), comets, satellites, stars | ✅ v0.1.3 |
| 18 | **WCS** — TAN/ARC/STG/SIN/AIT, SIP distortion, TPV distortion, axis-order detection | ✅ v0.1.3 |
| 19 | **Parallel Batch Reduction** — `ReduceBatchParallel`, 4.3× on 16 threads | ✅ v0.1.3 |

## Phase 5 — Polymorphic Architecture

| # | Capability | Status |
|---|---|---|
| 20 | **Observable type hierarchy** — `Star`, `Planet`, `Asteroid`, `Comet`, `Satellite`, `DeepSkyObject` | ✅ v0.1.4 |
| 21 | **Interface dispatch** — `Observable`, `MovingBody`, `MagnitudeComputer` replace flag checks | ✅ v0.1.4 |
| 22 | **`FromCatalog` factory** — `catalog.Target` wire format → concrete typed Observable | ✅ v0.1.4 |
| 23 | **Legacy cleanup** — `Target` god-struct, `NewTarget`, boolean-flag dispatch deleted | ✅ v0.1.4 |

## Phase 5.5 — Constraints, Photometry Depth & Quality

| # | Capability | Status |
|---|---|---|
| 24 | **Constraint Framework** — `Altitude`, `Airmass`, `Sun` (twilight), `MoonSep` (lunar separation), all with shared `coord.Context` via `CheckCtx` | ✅ v0.1.4 |
| 25 | **Generic Moving Body** — `GenericBody` fallback for ephemeris targets without a photometric model (no spurious magnitude in `GetDetails`) | ✅ v0.2.0 |
| 26 | **Satellite Magnitude Models** — Lambertian-sphere / diffuse-cylinder phase functions, McCants standard-magnitude convention | ✅ v0.2.0 |
| 27 | **Lint-Zero Quality Gate** — full `golangci-lint` v2 compliance, zero violations, exported-symbol docs across all packages | ✅ v0.1.5 |

---

# 🔨 Phase 6 — Advanced Constraints & Realism

**Goal:** model the constraints that real observers face beyond altitude and airmass.

## 28. Sky Brightness & Limiting Magnitude Constraint

**Status:** ✅ v0.2.0

Delivered as the `skybrightness` package (physics engine) plus a
`LimitingMagnitudeConstraint` in `plan`. Sky surface brightness is decomposed into
additive components summed in linear flux space, from which a derived limiting
magnitude scores or gates targets — going well beyond a static light-pollution floor.

- [x] `Floor` component — scalar SQM, directional `SQMGrid`, lossy `FloorFromBortle` (SQM canonical)
- [x] `Moonlight` component — Krisciunas & Schaefer (1991) scattered moonlight (~8–23% accuracy)
- [x] `ZodiacalLight` component — Leinert (1998) Table 17, bilinear interpolation
- [x] `Airglow` component — dark-sky floor (Noll 2012 / Patat 2008)
- [x] `CompositeModel` — linear-flux-space summation, allocation-free hot path
- [x] `VisualLimitingMag` — Schaefer (1990) / Unihedron SQM→NELM conversion
- [x] Per-target minimum limiting magnitude threshold (`Required`)
- [x] Soft monotonic ramp scoring + `Boolean` hard-cutoff mode
- [x] Integration with `ScoreObservable` via `ScoreObservableSky`

**Inspiration:** ESO Cerro Paranal sky model (Noll et al. 2012), Falchi et al. 2016,
Krisciunas & Schaefer 1991, Leinert et al. 1998.

---

## 29. Horizon Profile Constraint

**Status:** 🔲 Not Started

Per-azimuth altitude minimums from terrain data, replacing the flat-horizon assumption.

- [ ] `HorizonProfile` type — azimuth → minimum altitude lookup (interpolated)
- [ ] Load from CSV/JSON (azimuth, altitude pairs)
- [ ] Load from terrain raycasting (DEM/SRTM input)
- [ ] `Horizon` constraint — rejects targets below the local terrain horizon at their azimuth
- [ ] Integration with `NewSite` — optional profile per observatory

**Inspiration:** astroplan's `AltitudeConstraint` with custom horizon, KStars terrain profiles.

---

## 30. Weather Constraint

**Status:** 🔲 Not Started

Real-time or forecast-based weather gating for scheduling decisions.

- [ ] `Weather` constraint interface — cloud cover, wind, humidity, dew point
- [ ] Provider abstraction for weather data sources (OpenMeteo, Visual Crossing, local station)
- [ ] Cloud cover threshold (reject if > N% overcast)
- [ ] Wind speed limit (telescope safety)
- [ ] Dew point proximity alert (condensation risk)
- [ ] Precipitation rejection
- [ ] Historical weather-weighted scoring for long-term planning

**Note:** Weather is inherently probabilistic. The constraint should support both
"hard reject" (active rain) and "soft penalty" (marginal clouds) modes.

---

## 31. Satellite Illumination Constraint

**Status:** 🔲 Not Started

Visual satellite observation requires three simultaneous conditions: the satellite is
above the observer's horizon, the observer is in darkness, and the satellite is in sunlight.

- [ ] `SatelliteIllumination` constraint — Earth shadow geometry
- [ ] Cylindrical shadow model (sufficient for LEO/MEO)
- [ ] Integration with `SatellitePasses` — filter passes by illumination status
- [ ] Iridium flare prediction (specular reflection geometry)

---

## 32. Moon Illumination Constraint

**Status:** 🔲 Not Started

Companion to the existing `MoonSep` constraint: gate or penalize faint targets
when lunar phase / sky brightness from moonlight is too high.

- [ ] `MoonIllumination` constraint — reject/penalize above an illumination fraction
- [ ] Optional coupling with `MoonSep` (separation × illumination scoring)
- [ ] Integration with `ScoreObservable`

**Inspiration:** astroplan `MoonIlluminationConstraint`.

---

# 📊 Phase 7 — Visualization

**Goal:** publication-ready sky charts and planning diagrams, in the spirit of
[starplot.dev](https://starplot.dev) and astroplan's `plot_airmass` / `plot_sky` / `plot_parallactic`.

## 33. Airmass Diagram

**Status:** 🔲 Not Started

Classic observing-night airmass plot: time on x-axis, airmass (inverted) on y-axis,
one curve per target, twilight bands shaded.

- [ ] `plot.Airmass(targets, site, night)` → SVG/PNG
- [ ] Twilight shading (civil, nautical, astronomical)
- [ ] Moon altitude/illumination annotation
- [ ] Multi-target overlay with legend
- [ ] Interactive HTML variant (hover for exact values)

**Inspiration:** astroplan `plot_airmass`, Stellarium altitude graph.

---

## 34. Sky Chart

**Status:** 🔲 Not Started

Polar projection sky map showing target positions, horizon profile, and cardinal directions.

- [ ] `plot.SkyChart(targets, site, time)` → SVG/PNG
- [ ] Stereographic or orthographic polar projection
- [ ] Horizon profile overlay (if available)
- [ ] Target markers with labels
- [ ] Moon/Sun positions annotated
- [ ] Constellation grid (optional)

**Inspiration:** starplot.dev, Cartes du Ciel, astroplan `plot_sky`.

---

## 35. Observability Table

**Status:** 🔲 Not Started

Tabular summary of target visibility across a night or multi-night window.

- [ ] `plot.ObservabilityTable(targets, site, nights)` → SVG/PNG/HTML
- [ ] Color-coded cells (green = observable, red = below constraints, yellow = marginal)
- [ ] Time resolution (15 min default)
- [ ] Multi-night calendar view
- [ ] Constraint breakdown tooltip (which constraint failed)

**Inspiration:** astroplan `plot_schedule`, ESO Phase 2 visibility tables.

---

## 36. Parallactic Angle Diagram

**Status:** 🔲 Not Started

Parallactic angle vs. time for targets — critical for slit-spectroscopy and
atmospheric dispersion compensator planning.

- [ ] `plot.ParallacticAngle(targets, site, night)` → SVG/PNG
- [ ] Parallactic angle calculation (already available via coord pipeline)
- [ ] Optimal slit rotation overlay

**Inspiration:** astroplan `plot_parallactic`.

---

# 📊 Phase 8 — Batch & Pipeline

**Goal:** enable high-throughput catalog and pipeline workflows.

## 37. Batch / High-Throughput APIs

**Status:** 🔲 Not Started

- [ ] Batch coordinate transforms (vectorized)
- [ ] Batch ephemeris evaluation
- [ ] Batch visibility computation
- [ ] Batch event solving
- [ ] Concurrency-safe kernel/cache usage

---

## 38. Cross-Match Algorithms

**Status:** 🔲 Not Started

- [ ] Positional cross-match (nearest neighbor, cone radius)
- [ ] Multi-catalog cross-match (SIMBAD × Gaia × OpenNGC)
- [ ] Probabilistic matching (Bayesian, with proper motion correction)

---

# 🎯 Strategic Direction

AstroGo positions itself as:

> **A high-performance Go-native astronomy engine focused on precision, ephemerides, and observatory planning — with strong support for large-scale and backend workflows.**

Not as a full clone of other ecosystems, but as:

- **more performant for pipelines**
- **more structured for backend services**
- **scientifically reliable for planning and ephemerides**

---

# ⚠️ Non-Goals (for now)

- Full spectral analysis stack
- Complete reproduction of all Astropy submodules
- GUI application (the visualization phase targets programmatic output: SVG/PNG/HTML)