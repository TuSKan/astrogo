# AstroGo Roadmap

**Pure-Go high-performance, scientifically reliable astronomy library** —
precision computation, observatory planning, and scalable data workflows.

---

# 🎯 Path to v1.0.0

astrogo is **pre-1.0**. The deliberate decision was to ship
minor versions first — the API is already in good shape after an extensive
correctness/robustness/release-readiness audit (see [CHANGELOG.md](../CHANGELOG.md))
— rather than commit to the v1.0.0 API-stability promise while two catalog
providers were still partial:

- `catalog/jpl` — ✅ now resolves both ambiguous (major/small-body match tables) and
  unambiguous (single-match header line) Horizons queries into real `resolve.Target`s
- `catalog/vizier` — ✅ `ConeSearch` now supports any VizieR table registered in
  `tables.go` (2MASS, Hipparcos, Gaia DR3 today), selected via `ConeRequest.Table`

Both v1.0.0 blockers are closed — v1.0.0 is back on the table, pending a decision
on when to commit to the API-stability promise.

---

## Writing a checkbox

A box that **delivers** a symbol should name it package-qualified and put it
first: `` - [ ] `plan.Weather` — constraint interface … ``. Written that way,
`internal/docsguard` checks the box against the code in both directions — an
unchecked box whose symbol already exists, and a tick whose symbol has been
deleted, both fail the build.

The qualifier is what makes it checkable. A bare `` `Horizon` `` matched a
field in `atmosphere` and a `*Site` method in `plan`, neither of which is the
constraint that box is about; a guard that reports three false alarms out of
four gets switched off. Boxes that describe work rather than name a deliverable
— "Cloud cover threshold", "Integration with `SatellitePasses`" — are skipped,
deliberately, since the symbol they mention is the thing being built *on*.

This is not decoration. `resolve.Target.HasRadialVelocity` sat unchecked while
being declared, populated by `catalog/simbad`, preserved through the
multi-provider merge and consumed by `plan` — finished work left looking open,
which sends the next contributor to build it twice.

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

## 28. Sky Brightness

**Status:** ✅ Phases 0–5 — **superseded and rewritten**

Originally delivered in v0.2.0 as a magnitude-space model with a
`LimitingMagnitudeConstraint` in `plan`. That version was **removed in its entirety**
and rebuilt as a spectral all-sky radiance engine with no backward compatibility, because
the original made three assumptions that cannot be repaired incrementally: it summed a
scalar per component rather than a spectrum, so a correct V magnitude could sit on an
entirely wrong spectrum and every instrument projection would be wrong; it took a
light-pollution floor as an input rather than propagating light from sources; and its Moon
was a closed-form V-band fit with no spectrum to project at all.

Nothing from the original checklist survives by name — `Floor`, `SQMGrid`,
`FloorFromBortle`, `CompositeModel`, `VisualLimitingMag`, `ScoreObservableSky` and
`LimitingMagnitudeConstraint` are all gone. See the CHANGELOG's `### Removed` entries.

What ships now:

- [x] Spectral radiance `L_λ(λ, direction, observer, time, atmosphere)`, summed in linear
      radiance space and kept spectral until projection
- [x] Integrated starlight (Gaia DR3 order-8 map), diffuse galactic light (Kawara 2017),
      extragalactic background, zodiacal light (Leinert 1998), airglow (van Rhijn over a
      fetched ESO SkyCalc spectrum)
- [x] Scattered moonlight — Kieffer & Stone (2005) ROLO reflectance, Winkler (2022)
      multiple scattering. Explicitly **not** Krisciunas & Schaefer (1991)
- [x] Artificial skyglow in clear air (Kocifaj, Bará & Falchi 2022) and under cloud
      (Kocifaj 2007 Eq. 27 + Kocifaj, Falchi & Kundracik 2025)
- [x] Four named presets carrying their own radiative transfer, so a model cannot be
      silently evaluated under another's transport
- [x] A dataset tier (`skybrightness/dataset`) that is the only part permitted to do I/O
- [ ] **Limiting magnitude returns when Phases 2–3 make a defensible one possible.** It was
      removed rather than kept, because a limiting magnitude derived from a wrong spectrum
      is a confident number rather than a useful one

**Inspiration:** GAMBONS (Masana et al. 2021, 2024), Kocifaj's skyglow series,
Kieffer & Stone (2005), Leinert et al. (1998), ESO SkyCalc.

---

## 29. Horizon Profile Constraint

**Status:** 🟡 In Progress

Per-azimuth altitude minimums from terrain data, replacing the flat-horizon assumption.

- [x] `plan.HorizonProfile` type — azimuth → altitude function (`plan.HorizonProfile`, `Site.WithHorizonProfile`/`HorizonAt`)
- [ ] Load from CSV/JSON (azimuth, altitude pairs)
- [ ] Load from terrain raycasting (DEM/SRTM input)
- [ ] `Horizon` constraint — rejects targets below the local terrain horizon at their azimuth
- [x] Integration with `NewSite` — optional profile per observatory (via `WithHorizonProfile`, propagated through `Site.WithHorizon`/`WithTimeZone`)

**Inspiration:** astroplan's `AltitudeConstraint` with custom horizon, KStars terrain profiles.

---

## 30. Weather Constraint

**Status:** 🔲 Not Started

Real-time or forecast-based weather gating for scheduling decisions.

- [ ] `plan.Weather` constraint interface — cloud cover, wind, humidity, dew point
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

- [ ] `plan.SatelliteIllumination` constraint — Earth shadow geometry
- [ ] Cylindrical shadow model (sufficient for LEO/MEO)
- [ ] Integration with `SatellitePasses` — filter passes by illumination status
- [ ] Iridium flare prediction (specular reflection geometry)

---

## 32. Moon Illumination Constraint

**Status:** 🟢 Done

Companion to the existing `MoonSep` constraint: gate or penalize faint targets
when lunar phase / sky brightness from moonlight is too high.

- [x] `plan.MoonIllumination` constraint — `plan.MoonIllum` rejects/penalizes above an
      illumination fraction threshold, via the existing `plan.MoonIllumination` helper;
      always passes for the Moon itself. Implements `ConstraintCtx` (added to the
      compile-time assertion block alongside `MoonSep`).
- [ ] Optional coupling with `MoonSep` (separation × illumination scoring)
- [x] Integration with `ScoreObservable` — `MoonIllum` composes as an ordinary
      `Constraint`/`ConstraintCtx`, same as every other constraint `ScoreObservable`
      accepts.

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

**Status:** 🟡 In Progress

- [x] Batch coordinate transforms (vectorized) — already shipped in `coord/batch.go`
      (`ReduceBatch`/`ReduceBatchParallel`, `ICRSBatchToAltAz`/`ICRSBatchToAltAzParallel`);
      discovered still-current during this pass, no changes needed.
- [ ] Batch ephemeris evaluation
- [x] Batch visibility computation — `internal/parallel.Map` (order-preserving,
      `GOMAXPROCS`-bounded) now backs `Planner.FilterObservable`/`RankObservable`,
      package-level `RankObservables`, `gatherPlanetaryMoons`'s kernel fetch, and
      `VisibleTonight`'s three concurrent gathering stages — one shared primitive
      replacing five separately hand-rolled `errgroup` call sites.
- [ ] Batch event solving
- [x] Concurrency-safe kernel/cache usage — `ephemeris/jpl.Provider` already guards
      `AddKernel`/`State`/`FindSegment`/`SupportedBodies` with an internal
      `sync.RWMutex` (pre-existing, confirmed still current).

---

## 38. Cross-Match Algorithms

**Status:** 🟡 In Progress

- [x] Positional cross-match (nearest neighbor, cone radius) — `catalog/xmatch.Match`'s
      positional fallback pass (epoch-normalized via `coord.PropagateEpoch`, nearest
      match within `WithPositionMatchThreshold`, default 2″).
- [x] Multi-catalog cross-match (SIMBAD × Gaia × OpenNGC) — `catalog/xmatch.Match`
      operates on any two `resolve.Target` slices, independent of `catalog.Resolver`;
      alias-graph union-find (shared ID/`Aliases` entries) is the primary signal, the
      positional pass above the fallback for entries sharing neither. Deliberately does
      not merge fields — see the package doc for why, and `catalog.Resolver`'s own
      internal (unexported) cross-match for the field-reconciling counterpart this
      package intentionally does not duplicate.
- [ ] Probabilistic matching (Bayesian, with proper motion correction)

---

## 39. Central-Body Keplerian Propagation (Planetary Moons)

**Status:** 🔲 Not Started

`ephemeris/kepler` propagates asteroids/comets from heliocentric elements around the
Sun's GM; a planetary moon needs the same two-body machinery around its *parent
planet's* GM instead, plus a Laplace-plane frame correction — real, non-trivial
physics, not just missing plumbing. Verdict from investigation: forcing parity with
asteroids now would ship confidently wrong numbers, so moons stay kernel-only
(`ephemeris/jpl` SPK, gated by `remote.EnableDownloads`) until this is built properly.

- [x] `constants.DE440` — per-parent GM constants, both the system parameter (planet
      plus satellites, which the ephemeris integrates) and the body parameter (the
      planet alone, which governs a satellite's motion about it). IAU 2015 B3 Table 1
      publishes only Sun/Earth/Jupiter, so these come from DE440 via NAIF's
      `gm_de440.tpc`, with the per-planet values separately sourced from the natural
      satellite release forms. Verified against the kernel by a network test rather
      than trusted to transcription; `remote.NAIFPCK` was added to fetch it.
- [ ] Central-body (not heliocentric) two-body propagation in `ephemeris/kepler`
- [ ] Laplace-plane frame handling for published *mean* moon elements (tabulated pole,
      secular precession) — these are not osculating J2000-ecliptic elements and can't
      be fed into the existing propagator unmodified
- [ ] J₂-driven apsidal precession / mean-motion resonance corrections for moons where
      two-body motion diverges within weeks (e.g. the Galilean Laplace resonance)
- [ ] Offline base-state fallback for parents with no SOFA analytical source (Pluto,
      for Charon) — depends on `ephemeris.Default()`'s Pluto coverage (done, see below)

## 40. Radial-Velocity Corrections

**Status:** 🟡 In Progress

`coord.Context.BarycentricVelocity`/`BarycentricRVCorrection`/`HeliocentricRVCorrection`
ship (classical, non-relativistic projection of the observer's own barycentric
motion, ~1 m/s accuracy) and are demonstrated end-to-end in
`examples/23_radial_velocity_correction`. Now wired into `plan`'s observability
pipeline too — `plan.TargetDetails.RadialVelocity` auto-populates for any target
implementing `MeasuredRadialVelocity` (currently `*Star`).

- [x] `coord.Context.BarycentricVelocity` — observer's barycentric velocity from `Apco13`'s
      already-computed astrometry, no new SOFA call
- [x] `coord.BarycentricRVCorrection`/`HeliocentricRVCorrection` — classical velocity
      projection, sign convention documented and tested explicitly
- [x] Analytic property tests (bounded magnitude, annual sinusoid, antipodal sign flip,
      diurnal amplitude vs. site latitude)
- [x] `plan.TargetDetails.RadialVelocity` — a `MeasuredRadialVelocity` capability
      interface (mirroring `StaticMagnitude`) on `*Star`, wired into `computeDetails`
      alongside the magnitude dispatch block; `Context.ObservedRadialVelocity` is the
      inverse of `BarycentricRVCorrection`, tested to round-trip exactly
- [x] `resolve.Target.HasRadialVelocity` — distinguishes a true-zero measured RV from
      "no RV on file", which a zero `RadialVelocity` cannot. Set by `catalog/simbad`,
      preserved through the multi-provider merge in `catalog`, and consumed by
      `plan.NewStarFromTarget`; a genuine 0 km/s measurement is covered by tests in all
      three packages
- [x] Cross-implementation fixture test against Astropy's
      `SkyCoord.radial_velocity_correction` — 175 cases (5 named sites × 5 epochs × 7
      target directions) agreeing to 0.7 mm/s, generated by the locked
      `coord/testdata/rvfixture/` project and checked in as JSON so ordinary `go test`
      needs no Python. Compared like for like against the classical projection, since
      Astropy's barycentric value is relativistic; the 4.66 m/s gap between the two is
      asserted against the 4.649 predicted from the terms astrogo omits
- [ ] Full Wright & Eastman (2014) treatment for sub-1-m/s precision-RV work
      (gravitational redshift, light-travel-time to barycenter, target proper
      motion/parallax effects on the projection geometry) — the current classical
      projection is explicitly documented as insufficient for this

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