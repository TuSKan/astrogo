# AstroGo Roadmap

AstroGo aims to become a **pure-Go high-performance, scientifically reliable astronomy library**, focused on precision computation, observatory planning, and scalable data workflows.

The project has achieved strong coverage of core astronomy primitives (time, coordinates, ephemerides, planning, FITS, WCS, catalogs, and validation). The roadmap now focuses on **batch workflows, cross-matching, and operational polish**.

---

# ✅ Current Status (Summary)

AstroGo already provides:

- Astronomical time scales (UTC, TAI, TT, **UT1**, TDB)
- Coordinate frames and transformations (ICRS, Galactic, Ecliptic, AltAz)
- Observatory modeling and sidereal time
- Sky calculations (alt/az, airmass, separation, position angle)
- Event solving (rise, set, transit, twilight)
- Constraint system and visibility evaluation
- JPL DE ephemerides with:
  - multi-kernel support
  - Horizons on-demand querying
  - cache + provider abstraction
- Catalog identity:
  - OpenNGC embedded
  - SBDB (NASA) query integration
  - SIMBAD, Gaia, MAST, VizieR remote providers
  - **NORAD/CelestTrak** GP data (OMM/JSON)
- **Satellite tracking (SGP4)**:
  - TEME→GCRS frame conversion
  - Sub-satellite ground track
  - Pass prediction (AOS/TCA/LOS)
- FITS I/O (multi-extension, tables, images)
- WCS support
- Units and quantities system
- Validation framework against SOFA / Horizons / USNO / analytical invariants
- CI with automated testing

The project is **past the foundational stage**.

---

# ✅ Phase 1 — Precision Astronomy Completion

**Goal:** achieve observatory-grade correctness for real-world usage.

## 1. Earth Orientation Parameters (EOP)

**Status:** ✅ Complete

- [x] UT1–UTC correction ingestion
- [x] EOP data loader and cache
- [x] polar motion support
- [x] deterministic fallback for stale/missing data
- [x] validation against reference datasets

---

## 2. Apparent / Observed Coordinate Pipeline

**Status:** ✅ Complete

- [x] aberration corrections
- [x] proper motion propagation
- [x] parallax handling
- [x] topocentric apparent coordinates
- [x] explicit API separation (geometric, astrometric, apparent, observed)

---

## 3. Atmospheric Refraction Model

**Status:** ✅ Complete

- [x] refraction model abstraction
- [x] standard atmosphere correction
- [x] optional pressure / temperature input
- [x] selectable modes (none, approximate, improved/SOFA)

---

## 4. Numerical Solver Architecture

**Status:** ✅ Complete

- [x] Unified `Solver` struct (`plan/solver.go`)
- [x] Chandrupatla root-finding (1997) — replaces Brent's method for roots
- [x] Brent's minimization — retained for smooth unimodal extremum-finding
- [x] Eliminated ~300 lines of duplicated solver code
- [x] Comprehensive unit tests (`solver_test.go` — 17 tests, black-box)
- [x] Transit detection edge case fix (non-bracketed intervals)

---

## 5. Planetary & Lunar Phenomena

**Status:** ✅ Complete

- [x] Moon phases (New, First Quarter, Full, Last Quarter) — ≤1 min vs USNO
- [x] Earth's seasons (equinoxes + solstices) — 2–4 min vs USNO
- [x] Perihelion/Aphelion (`plan.Apsides`) — **≤1 min** vs USNO
- [x] Lunar eclipse detection (`plan.LunarEclipses`) — ecliptic latitude filter
- [x] Solar eclipse detection (`plan.SolarEclipses`) — ecliptic latitude filter
- [x] Moon illumination fraction (`plan.MoonIllumination`)
- [x] Conjunctions, oppositions, greatest elongations
- [x] Validated against USNO API and NASA Eclipse Catalog (2026)

---

# ✅ Phase 2 — Scheduling Engine

**Goal:** evolve from planning primitives to full observatory scheduling.

## 6. Advanced Scheduling Optimization

**Status:** ✅ Complete

- [x] observing block abstraction
- [x] target prioritization
- [x] multi-target optimization
- [x] cadence-aware scheduling
- [x] pluggable strategies (greedy, priority-based, constraint-aware)
- [x] **`SwapOptimizedStrategy`** — monotonic local search with adjacent swaps + gap insertion
- [x] `ScoreObservable` at block midpoint for cross-strategy comparability
- [x] Linear scaling benchmarked to 100 blocks

---

## 7. Transition & Operational Overhead Modeling

**Status:** ✅ Complete

- [x] slew-time estimation
- [x] configuration / filter change costs
- [x] setup overhead modeling
- [x] penalty-aware scheduling integration

---

## 8. Explainable Scheduling Output

**Status:** ✅ Complete

- [x] structured schedule object
- [x] score breakdown per decision
- [x] rejection explanations
- [x] reproducible scheduling traces

---

# ✅ Phase 3 — Validation & Scientific Trust

**Goal:** maintain and strengthen scientific reliability.

## 9. USNO Integration Validation

**Status:** ✅ Complete

- [x] Sun/Moon rise/set/transit — ≤1 min vs USNO API
- [x] Moon phases — ≤1 min vs USNO API
- [x] Earth's seasons — 2–4 min vs USNO API
- [x] Perihelion/Aphelion — ≤1 min vs USNO API
- [x] Lunar/Solar eclipses — date-exact vs NASA Eclipse Catalog
- [x] Celestial navigation (AltAz) — 0.002° vs USNO API
- [x] Julian Date conversion — exact
- [x] Sidereal time — sanity-checked

See [`USNO.md`](./USNO.md) and [`VALIDATION.md`](./VALIDATION.md) for full details.

---

## 10. Scientific CI Gating

**Status:** ✅ Complete

- [x] validation suite separated from unit tests (`-tags=integration`)
- [x] tolerance drift detection
- [x] corpus-based regression runs (JPL Horizons)
- [x] CI failure on scientific regressions

---

# ✅ Phase 3.5 — Observatory-Grade Hardening (v0.1.0)

**Goal:** eliminate scientific liabilities and establish production-grade correctness.

## 11. Scale-Aware Time System

**Status:** ✅ Complete

- [x] Full bidirectional conversion graph: `UTC↔TAI↔TT↔TDB`, `UTC↔UT1`
- [x] Fairhead & Bretagnon (1990) TDB−TT correction (±3 µs residual)
- [x] `UT1()` returns `(Time, error)` — explicit IERS data unavailability
- [x] Cross-scale `Before`, `After`, `Equal`, `Sub`, `SubDays` auto-unify via TT
- [x] Zero-overhead same-scale fast path (~2 ns)

## 12. Visibility Boundary Refinement

**Status:** ✅ Complete

- [x] Chandrupatla root-finding for continuous altitude crossings
- [x] Binary search for discrete constraint state transitions
- [x] `VisibleIntervals`, `Find`, `ObservableWindows` refined to sub-second precision

## 13. API Hygiene & Defensive Patterns

**Status:** ✅ Complete

- [x] `NewSite` nil-location guard (`ErrNilLocation`)
- [x] `Site.Equal` epsilon-tolerant comparison (1e-12 rad)
- [x] `DeepSpace.Position` and `Custom.Position` return defensive copies
- [x] Consistent SOFA refraction at all altitudes (fixed `AtAltitude(0)` path)

## 14. Illumination Event Family

**Status:** ✅ Complete

- [x] `EventFamilyIllumination` in EventSolver dispatch
- [x] `solveIllumination` via ecliptic longitude (delegates to `moonElongation`)
- [x] `isPhaseEvent` guard for validation exemption
- [x] `NextNewMoon`, `NextFullMoon` convenience helpers
- [x] `EventAnyPhase` wildcard

## 15. Benchmark Suite

**Status:** ✅ Complete (40+ benchmarks across 5 packages)

- [x] `coord/`: NewContext, ICRSToAltAz (cached vs uncached), 100-star batch, Reducer
- [x] `time/`: All scale conversions, round-trip, same-scale vs cross-scale comparison
- [x] `atmosphere/`: Rigorous, Approximate, horizon, Airmass, AtAltitude
- [x] `plan/`: VisibleIntervals (10 min / 1 min), ObservableWindows, EventSolver, scheduler scaling (10/50/100 blocks × 3 strategies), TransitEstimate

## 16. Atmosphere Correctness Tests

**Status:** ✅ Complete (19 tests)

- [x] Refraction at known altitudes (zenith, 45°, 20°, 10°, horizon, below)
- [x] Wavelength dispersion (blue > red)
- [x] Zero-pressure guard, RefractionNone contract
- [x] Airmass known values, monotonicity, below-horizon error
- [x] AtAltitude pressure/temperature validation (sea level through Everest)
- [x] Model nil consistency at all altitudes
- [x] HorizonDip, ZenithDistance

---

# 📊 Phase 4 — Data Workflow Layer

**Goal:** enable real catalog and pipeline workflows.

## 17. Catalog Table Infrastructure

**Status:** ✅ Complete (Remote Data Layer)

- [x] structured catalog table abstraction
- [x] remote provider bindings (SIMBAD, MAST, SBDB, VizieR, Gaia)
- [x] explicit offline regression caches (JSON/XML/CSV structural decoding)
- [x] memory-mapped hardware vectors via Arrow `RecordBatch`
- [x] resilient API rate-limit backoffs
- [x] integration with FITS tables and arrays

### Remaining

- [ ] cross-match logic algorithms (positional, multi-catalog)

---

## 17.5 NORAD Satellite Tracking

**Status:** ✅ Complete

- [x] CelestTrak GP JSON client (`catalog/norad`) — OMM-aligned field names (CCSDS 502.0-B-3 / Space Data Standards)
- [x] TLE generation from OMM fields for SGP4 initialization
- [x] SGP4 propagation via `go-satellite` (`ephemeris/satellite`)
- [x] TEME → GCRS frame conversion (via SOFA GAST)
- [x] `ephemeris.Provider` interface implementation (State in AU, AU/day)
- [x] Sub-satellite ground track (geodetic lat/lon/alt)
- [x] Topocentric look angles (azimuth, elevation, range)
- [x] Pass prediction (`plan.SatellitePasses`) — AOS/TCA/LOS with Chandrupatla rise/set refinement
- [x] Catalog integration (`catalog.NORAD` source in unified Resolver)
- [x] Live example (`examples/12_satellite_tracking/`) — ISS tracking with CelestTrak data
- [x] Validated: ISS altitude ~420 km, orbital period ~93 min, velocity ~7.7 km/s
---

## 18. Batch / High-Throughput APIs

**Status:** 🔲 Not Started

- [ ] batch coordinate transforms
- [ ] batch ephemeris evaluation
- [ ] batch visibility computation
- [ ] batch event solving
- [ ] concurrency-safe kernel/cache usage

### Outcome
Efficient large-scale processing (surveys, pipelines, services).

---

# 🎯 Strategic Direction

AstroGo should position itself as:

> **A high-performance Go-native astronomy engine focused on precision, ephemerides, and observatory planning — with strong support for large-scale and backend workflows.**

Not as a full clone of other ecosystems, but as:

- **more performant for pipelines**
- **more structured for backend services**
- **scientifically reliable for planning and ephemerides**

---

# ⚠️ Non-Goals (for now)

To maintain focus, the following are intentionally not prioritized:

- full photometry / image processing ecosystem
- full spectral analysis stack
- complete reproduction of all Astropy submodules

These can be explored later if aligned with project direction.

---

# 🧭 Summary

AstroGo has completed all core astronomy capabilities and v0.1.0 hardening:

- ✅ Scale-aware time system (full conversion graph, Fairhead TDB, explicit UT1 errors)
- ✅ SOFA-rigorous refraction at all altitudes
- ✅ Sub-second visibility boundary refinement
- ✅ Production scheduler (SwapOptimized, linear scaling)
- ✅ Complete event solver (visibility, geometry, illumination, eclipses)
- ✅ API hygiene (nil guards, epsilon equality, defensive copies)
- ✅ 40+ benchmarks validating performance claims
- ✅ Scientific validation (USNO, JPL Horizons, NASA Eclipse Catalog)
- ✅ Catalog remote data layer
- ✅ **NORAD satellite tracking** (CelestTrak GP, SGP4, pass prediction)

The remaining work is concentrated in:

- **Batch/vectorized APIs** for high-throughput use cases
- **Cross-match algorithms** for multi-catalog workflows
- **Satellite illumination constraints** (sunlit + dark sky for visual observation)

Completing these will elevate AstroGo from:

> **an observatory-grade astronomy library**

to:

> **a scalable astronomy platform for Go**