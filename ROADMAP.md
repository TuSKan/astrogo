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

# 📊 Phase 4 — Data Workflow Layer

**Goal:** enable real catalog and pipeline workflows.

## 11. Catalog Table Infrastructure

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

## 12. Batch / High-Throughput APIs

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

AstroGo has completed all core astronomy capabilities:

- ✅ Precision corrections (EOP, apparent place, refraction)
- ✅ Numerical solvers (Chandrupatla + Brent)
- ✅ Planetary phenomena (phases, seasons, apsides, eclipses)
- ✅ Scheduling depth (constraint-aware, explainable)
- ✅ Scientific validation (USNO, Horizons, SOFA)
- ✅ Catalog remote data layer

The remaining work is concentrated in:

- **Batch/vectorized APIs** for high-throughput use cases
- **Cross-match algorithms** for multi-catalog workflows

Completing these will elevate AstroGo from:

> **a production-grade astronomy library**

to:

> **a scalable astronomy platform for Go**