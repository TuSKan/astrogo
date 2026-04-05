# AstroGo Roadmap

AstroGo aims to become a **pure-Go high-performance, scientifically reliable astronomy library**, focused on precision computation, observatory planning, and scalable data workflows.

The project has already achieved strong coverage of core astronomy primitives (time, coordinates, ephemerides, planning, FITS, WCS, catalogs, and validation). The roadmap now focuses on **scientific completeness, operational planning, and workflow ergonomics**.

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
- FITS I/O (multi-extension, tables, images)
- WCS support
- Units and quantities system
- Validation framework against SOFA / Horizons / analytical invariants
- CI with automated testing

The project is **past the foundational stage**.

---

# 🚀 Phase 1 — Precision Astronomy Completion

**Goal:** achieve observatory-grade correctness for real-world usage.

## 1. Earth Orientation Parameters (EOP)

### Status
✅ Fully Operational

### Deliverables
- [x] UT1–UTC correction ingestion
- [x] EOP data loader and cache
- [x] polar motion support
- deterministic fallback for stale/missing data
- validation against reference datasets

### Outcome
Accurate sidereal time and topocentric positioning under real Earth rotation conditions.

---

## 2. Apparent / Observed Coordinate Pipeline

### Status
✅ Fully Operational

### Deliverables
- [x] aberration corrections
- [x] proper motion propagation
- [x] parallax handling
- topocentric apparent coordinates
- explicit API separation:
  - geometric
  - astrometric
  - apparent
  - observed

### Outcome
Coordinates suitable for real telescope pointing and observation comparison.

---

## 3. Atmospheric Refraction Model

### Status
✅ Fully Operational

### Deliverables
- [x] refraction model abstraction
- [x] standard atmosphere correction
- [x] optional pressure / temperature input
- [x] selectable modes:
  - [x] none
  - [x] approximate
  - [x] improved model (SOFA)

### Outcome
More realistic horizon and low-altitude behavior.

---

# 📅 Phase 2 — Scheduling Engine

**Goal:** evolve from planning primitives to full observatory scheduling.

## 4. Advanced Scheduling Optimization

### Deliverables
- observing block abstraction
- target prioritization
- multi-target optimization
- cadence-aware scheduling
- pluggable strategies:
  - greedy
  - score-maximizing
  - priority-based
  - window-aware

### Outcome
Automated generation of optimized observing plans.

---

## 5. Transition & Operational Overhead Modeling

### Deliverables
- slew-time estimation
- configuration / filter change costs
- setup overhead modeling
- penalty-aware scheduling integration

### Outcome
Schedules that reflect real observatory constraints.

---

## 6. Explainable Scheduling Output

### Deliverables
- structured schedule object
- score breakdown per decision
- rejection explanations
- reproducible scheduling traces

### Outcome
Transparent and debuggable planning decisions.

---

# 📊 Phase 3 — Data Workflow Layer

**Goal:** enable real catalog and pipeline workflows.

## 7. Catalog Table Infrastructure

### Status
Catalog identity exists; table workflows are limited.

### Deliverables
- structured catalog table abstraction
- typed fields and schemas
- unit-aware columns
- filtering and sorting
- cross-match primitives
- integration with FITS tables and Arrow

### Outcome
Catalogs become first-class, manipulable datasets.

---

## 8. Batch / High-Throughput APIs

### Deliverables
- batch coordinate transforms
- batch ephemeris evaluation
- batch visibility computation
- batch event solving
- concurrency-safe kernel/cache usage

### Outcome
Efficient large-scale processing (surveys, pipelines, services).

---

# 🧪 Phase 4 — Validation & Scientific Trust

**Goal:** maintain and strengthen scientific reliability.

## 9. Validation Expansion

### Deliverables
- extended comparison against Astropy / SOFA / Horizons
- additional edge-case datasets:
  - high latitude
  - circumpolar
  - horizon edge
- small-body validation coverage
- apparent-coordinate validation

### Outcome
Higher confidence across all domains.

---

## 10. Scientific CI Gating

### Deliverables
- validation suite separated from unit tests
- tolerance drift detection
- corpus-based regression runs
- CI failure on scientific regressions

### Outcome
Prevents silent numerical degradation over time.

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

AstroGo is no longer missing core astronomy capabilities.

The remaining work is concentrated in:

- precision corrections (EOP, apparent place, refraction)
- scheduling depth
- data workflow ergonomics
- validation hardening

Completing these will elevate AstroGo from:

> **a powerful astronomy library**

to:

> **a production-grade astronomy platform for Go**