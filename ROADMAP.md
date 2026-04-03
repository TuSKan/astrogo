# AstroGo Project Roadmap

AstroGo aims to become the definitive astronomy and astrophysics standard library for the Go ecosystem. While Python's `astropy` leads the industry in available algorithms, Go offers unparalleled potential for high-throughput concurrency, massive batch processing, and pipeline integrations that Python execution traditionally struggles with.

The roadmap is categorized by priority tiers. Completed objectives reflect milestones that have successfully been integrated into the active `main` branch.

---

## ✅ Completed Milestones

### 1. High-Performance FITS I/O Engine
- **Status:** Complete (Phase 0-2)
- **Impact:** Astronomical image workflows are now completely unblocked.
- **Deliverables:**
  - Full Multi-Extension FITS support (`Primary`, `Image`, `ASCII`, `BINTABLE`).
  - Zero-copy mapping via `golang.org/x/exp/mmap` natively bridging FITS blocks via pure memory references.
  - Apache Arrow (`apache/arrow-go/v18`) structures storing multi-dimensional image arrays and binary table dataframes dynamically without reflection. 
  - Transparent `.gz` pipeline decompression accelerating via multithreaded `pgzip`.

### 2. World Coordinate Systems (WCS)
- **Status:** Complete (Phase 3)
- **Impact:** Allows physical coordinates to geometrically tie against absolute sky maps.
- **Deliverables:**
  - Automated `fits.ExtractWCS` mechanisms cleanly lifting coordinate boundaries from generic headers.
  - Pixel-to-World coordinate paths utilizing exact mathematical Gnomonic (`TAN`) spherical tracking natively bypassing flat-Earth GIS algorithm distortions.

---

## 🚀 Tier 1 — Core Execution & Baseline Parity
*These priorities close fundamental usability gaps and establish AstroGo's core performance advantages.*

### 1. Vectorized Batch APIs & Hardware Optimizations
Astropy currently dictates the industry, however AstroGo will dominate server-side astronomy by leaning heavily into Go's concurrency structures.
- **Why it matters:** Can a user efficiently process 100,000 target alt/az checks, or structurally score an entire sky catalog dynamically across multi-core pipeline workers? 
- **Deliverables:**
  - Concurrent batch evaluation handlers for ephemerides and horizon visibility grids.
  - Thread-safe coordinate transformations routing pipeline arrays.

### 2. Scientific Validation & Authority Layer 
- **Why it matters:** Serious users need verified mathematical boundaries and quantifiable precision error thresholds out-of-the-box. 
- **Deliverables:**
  - Comprehensive interoperability corpus verified dynamically against Astropy/SPICE/SOFA standard dataset outputs.
  - Documented programmatic error budgets for numerical drift.

### 3. Unified Ephemeris Abstractions
- **Why it matters:** Modern observers use a broad mosaic of SPK binaries and internet APIs. Users should not need to rewrite call sites when pivoting between a local DE planetary kernel cache network versus a remote JPL Horizons dynamic orbital query.
- **Deliverables:**
  - Unified Go interfaces safely bridging local `SPK` planetary caches, remote orbital queries, and offline graceful fallbacks transparently.

---

## 🔭 Tier 2 — Data Ecosystem & Astro Tables 
*These milestones bring AstroGo fully out of raw physical mathematics and directly into structural catalog pipelining.*

### 1. Catalog & Arrow Table Infrastructure
By leveraging `apache/arrow-go/v18` already configured within our FITS Binary Tables module, we structurally establish rigorous execution systems for resolving source lists and instrument catalogs completely sidestepping Python's severe Pandas/NumPy memory bloat configurations.
- **Why it matters:** Modern Astronomy pipelines live and die by sorting billion-row star catalogs or massive reduced photometry tabular dataframes.
- **Deliverables:**
  - Unit-aware dataframe logic bridging explicit columnar tables.
  - High-volume catalog intersections interoperating natively with interoperable files formats (`CSV`, `Parquet`, explicit FITS binary tables).

### 2. Remote Astroquery-style Ecosystem Integrations
- **Why it matters:** Connecting real-time remote scientific data retrieval pipelines directly into Go expands backend microservice usability dramatically.
- **Deliverables:**
  - Native integrated HTTP ingestion APIs targeting massive datasets like Simbad, VizieR, and explicit Gaia spherical cone searches.
  - MPC (Minor Planet Center) / Small body tracking integration workflows.

---

## 📅 Tier 3 — Complex Operations & Deep Scheduling
*Achieving parity with platforms like `astroplan` by tracking physical mechanical overhead limitations against the cosmos.*

### 1. Observatory Schedule Optimization Engines
- **Why it matters:** Driving complex robotic observatories requires ranking thousands of dynamic active targets simultaneously while tracking altitude transitions against active filter delays or camera configuration limits.
- **Deliverables:**
  - Observation block handlers generating priority-based and observability-score-maximizing operational schedules natively mapping observability windows over nights and celestial seasons.
  - Physical transition models structurally measuring observatory configuration slew-time penalties.

### 2. Deep Time Infrastructure (`EOP` / `UT1`)
While `UTC`, `TAI`, and `TT` are sufficient for 95% of tasks, ultra-precision observatory pointing demands integrating strict Earth Orientation Parameters dynamically.
- **Deliverables:** 
  - Resolving pure physical `UT1` scales evaluating dynamic external IERS tables.
  - Polishing precision stale-data system behavior executing against stale or disconnected cache behavior logic.

### 3. Image-Domain & Photometric Output Pipelines
If AstroGo expands to handle complete physical image extraction, building out pixel-level analytical instruments rounds out the final roadmap barriers targeting "Complete Platform Dominance".
- **Deliverables:** Advanced Spectral handlers, explicit Point Spread Function (PSF) source measuring, explicit aperture tooling, and image calibration tracking utilities generating propagated photometric datasets dynamically!