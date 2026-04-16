# astrogo

[![Go Reference](https://pkg.go.dev/badge/github.com/TuSKan/astrogo.svg)](https://pkg.go.dev/github.com/TuSKan/astrogo)
[![Go Report Card](https://goreportcard.com/badge/github.com/TuSKan/astrogo)](https://goreportcard.com/report/github.com/TuSKan/astrogo)
[![CI](https://github.com/TuSKan/astrogo/actions/workflows/ci.yml/badge.svg)](https://github.com/TuSKan/astrogo/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/TuSKan/astrogo/branch/main/graph/badge.svg)](https://codecov.io/gh/TuSKan/astrogo)
[![GitHub release (latest by date)](https://img.shields.io/github/v/release/TuSKan/astrogo)](https://github.com/TuSKan/astrogo/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

![AstroGo Mascot](assets/image.png)

**Observatory-grade astronomy and observation-planning engine for Go.**

Scale-aware time arithmetic · SOFA-rigorous coordinate transforms · sub-second visibility boundaries · production scheduling · validated against USNO, JPL Horizons, and NASA Eclipse Catalogs.

---

## Overview

`astrogo` is a Go-native scientific library for professional-grade astronomy, providing:

- **Scale-aware time system** — Full `UTC↔TAI↔TT↔TDB` conversion graph with Fairhead & Bretagnon TDB corrections and explicit IERS UT1 error propagation
- **SOFA-rigorous coordinate transforms** — Cached `Context` amortizes expensive matrix computations (91 µs once → 325 ns per transform)
- **Sub-second visibility detection** — Chandrupatla root-finding refines grid-sampled boundaries to <1s precision
- **Production scheduling engine** — Greedy, Priority, and `SwapOptimized` strategies with monotonic improvement guarantees
- **Complete event solver** — Rise/Set/Transit, Moon Phases, Seasons, Apsides, Eclipses, Conjunctions, Elongations
- **JPL DE ephemerides** — Multi-kernel SPK with on-demand Horizons fetching
- **Observatory-grade refraction** — SOFA's rigorous model at all altitudes, with pluggable override interface

Designed from the ground up for Go: no dynamic magic, no hidden global state, zero-allocation hot paths.

---

## Why astrogo?

Existing astronomy tools are powerful, but often:

- tightly coupled to Python
- difficult to optimize for high-throughput workloads
- not designed for Go's type system and performance model

`astrogo` aims to bring:

- **Astropy-level capabilities**
- **Astroplan-style observation workflows**
- **Go-level performance and control**

---

## Features

### Core Scientific Primitives
- Angles (radians, degrees, sexagesimal — HMS/DMS parsing)
- Units and quantities
- **Scale-aware time system** (JD-based, full `UTC↔TAI↔TT↔TDB↔UT1` conversion graph)
  - Fairhead & Bretagnon (1990) TDB correction (±3 µs residual)
  - Cross-scale comparisons auto-unify via TT (2 ns same-scale fast path)
  - `UT1()` returns `(Time, error)` — explicit IERS data unavailability

### Coordinate systems
- ICRS
- Galactic
- Ecliptic
- Horizontal (Alt/Az)
- Geodesic

### Transformations
- Full mapping: Geometric ↔ Astrometric ↔ Apparent ↔ Observed 
- Frame-to-frame (Galactic, Ecliptic, ICRS, CIRS)
- Dynamic DUT1 tracking and Polar Motion (XP/YP) caching via IERS EOP rapid data
- One-time log warning when IERS data is unavailable (UT1 ≈ UTC fallback)
- Aberration, light deflection, proper motion, parallax handled natively

### Atmospheric Modeling (`atmosphere`)
- **SOFA-rigorous refraction by default** at all altitudes (ICAO standard atmosphere)
- Pluggable `RefractionModel` interface with bidirectional refraction
- `RefractionNone` — bypass refraction
- `RefractionApproximate` — Saemundsson/Bennett tangent formula (~12 ns/call)
- `RefractionRigorous` — full pressure/temperature/humidity/wavelength correction (~14 ns/call)
- Pickering (2002) airmass — stable down to 0° altitude (overcomes Kasten & Young limitations)
- Chromatic atmospheric dispersion via `Reducer.Disperse()`

### Observer Modeling
- Geodetic locations (WGS84) with nil-location guards
- Epsilon-tolerant site equality (1e-12 rad)
- Defensive catalog pointer copying
- **Stateful `Context`** caching for batch transforms (73× speedup for 100-star batches)

### Ephemerides
- Sun and Moon positions
- Planetary positions (Mercury → Neptune)
- **High-performance JPL SPK provider**:
    - Multi-kernel architecture (load planets and small-bodies simultaneously)
    - On-demand asteroid/comet fetching via **JPL Horizons API**
    - Support for **SPK Type 21** (Extended Modified Difference Arrays)
    - Precedence-aware segment indexing (~85× faster lookups)

### Catalogs & Data Services (`catalog/resolve`)
- Unified `resolve.Provider` interfaces (`ObjectResolver`, `ConeSearcher`)
- Hardware-optimized native caching via **Apache Arrow** columnar batches
- Modern Go 1.23 streaming `iter.Seq2` iteration for memory-safe big data fetching
- Resilient network layers with exponential backoff retry
- Production-grade bindings:
    - **SIMBAD** (ADQL TAP)
    - **MAST** (STScI CAOM Dual-Encoding support)
    - **JPL SBDB** (Small-Body Database Search)
    - **Gaia** & **VizieR** (Data TAP)
    - **OpenNGC** (Zero-I/O `//go:embed` binaries)

### FITS & World Coordinate System (`fits`)
- Read standard FITS files (Image, BinTable, ASCII Table HDUs)
- Gzip-compressed streams (`.fits.gz`), memory-mapped access (`OpenMmap`)
- Apache Arrow columnar export for catalog-scale table HDUs
- **WCS** — pixel-to-sky mapping with TAN (Gnomonic) projection and `ExtractWCS` header parser

### Visibility & Planning
- **Sub-second boundary refinement** — Chandrupatla (continuous altitude) + bisection (discrete constraints)
- Observable windows with constraint evaluation
- Altitude/airmass/separation constraints
- Target scoring and ranking (`ScoreObservable` at midpoint altitude × priority)
- **Production Scheduling Engine**:
  - `Block` and `Configuration` abstractions for observing requests
  - `TransitionModel` for slew and instrument setup time
  - Pluggable `Strategy` allocators:
    - `GreedyStrategy` — fast, linear scaling
    - `PriorityStrategy` — priority-sorted greedy
    - **`SwapOptimizedStrategy`** — local search with adjacent swaps + gap insertion (monotonic improvement)
  - Linear scaling benchmarked to 100 blocks

### Event Solver
- **Unified `Solver`** — Chandrupatla root-finding (1997) + Brent's minimization
- **Moon Phases**: New, First Quarter, Full, Last Quarter — ≤1 min vs USNO
- **Moon Phase Events**: `NextNewMoon`, `NextFullMoon`, `MoonPhases` via `EventFamilyIllumination`
- **Earth's Seasons**: Equinoxes and Solstices — 2–4 min vs USNO
- **Visibility Events**: Rise, Set, and Transit at sub-second precision
- **Relational Geometry**: Conjunction, Opposition, Greatest Elongation, Quadrature
- **Eclipse Detection**: `LunarEclipses`, `SolarEclipses` via ecliptic latitude filter (Danjon limit)
- **Convenience**: `SunriseSunset`, `CivilDawnDusk`, `VisibilityEvents`, `Conjunctions`, `Oppositions`, `GreatestElongations`

---

## Installation

```bash
go get github.com/TuSKan/astrogo
```

## Quick Start — Tonight's Observing Plan

```go
package main

import (
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	// ── Observatory Setup: ESO Paranal (VLT) ──
	loc, _ := coord.NewGeodetic(angle.Deg(-70.4046), angle.Deg(-24.6272), 2635)
	site, _ := plan.NewSite("Paranal", loc, angle.Deg(20), nil)

	// ── Night boundaries ──
	eph := ephemeris.Default()
	tonight := time.Date(2026, 4, 15, 22, 0, 0, 0, time.LocationUTC)
	tomorrow := tonight.AddDays(1)

	sunrise, sunset, _ := plan.SunriseSunset(tonight, tomorrow, site, eph)
	fmt.Printf("Sunset:  %s\n", sunset.Time)
	fmt.Printf("Sunrise: %s\n", sunrise.Time)

	dawn, dusk, _ := plan.AstronomicalDawnDusk(tonight, tomorrow, site, eph)
	fmt.Printf("Astro dusk: %s → Astro dawn: %s\n", dusk.Time, dawn.Time)

	// ── Moon phase check ──
	nextFull, _ := plan.NextFullMoon(tonight, eph)
	fmt.Printf("Next Full Moon: %s\n", nextFull.Time)

	frac, _, _ := plan.MoonIllumination(tonight, eph)
	fmt.Printf("Moon illumination: %.0f%%\n", frac*100)

	// ── Targets ──
	ra, _ := angle.ParseHMS("13h 29m 52.7s")
	dec, _ := angle.ParseDMS("-47° 12' 18\"")
	omegaCen := plan.NewFixed(catalog.Target{
		Name: "Omega Centauri", Coord: coord.NewICRS(ra, dec),
	})

	ra2, _ := angle.ParseHMS("17h 45m 40.0s")
	dec2, _ := angle.ParseDMS("-29° 00' 28\"")
	sgrA := plan.NewFixed(catalog.Target{
		Name: "Sgr A*", Coord: coord.NewICRS(ra2, dec2),
	})

	mars := plan.NewDefaultBody(ephemeris.Mars)

	// ── Observability + Scoring ──
	constraints := []plan.Constraint{
		plan.Altitude{Threshold: angle.Deg(30)},
		plan.AirmassConstraint{MaxAirmass: 2.0},
	}

	fmt.Println("\n── Observability ──────────────────────")
	for _, obj := range []plan.Observable{omegaCen, sgrA, mars} {
		eval, _ := plan.IsObservable(obj, tonight, site, constraints...)
		score, _ := plan.ScoreObservable(obj, tonight, site, constraints...)
		fmt.Printf("  %-18s  Observable: %-5v  Score: %5.1f\n",
			obj.Name(), eval.Observable, score)
	}

	// ── Schedule the night ──
	planner, _ := plan.NewPlanner(site, nil)
	blocks := []*plan.Block{
		{ID: "OmCen", Target: omegaCen, Duration: 45 * time.Minute, Priority: 3},
		{ID: "SgrA",  Target: sgrA,     Duration: 60 * time.Minute, Priority: 5},
		{ID: "Mars",  Target: mars,     Duration: 20 * time.Minute, Priority: 2},
	}

	strategy := &plan.SwapOptimizedStrategy{
		Base:      &plan.PriorityStrategy{},
		MaxPasses: 5,
	}
	window := plan.Window{Start: dusk.Time, End: dawn.Time}
	tm := &plan.BasicTransitionModel{BaseSetup: 5 * time.Minute}

	schedule, _ := strategy.Schedule(planner, window, blocks, tm)

	fmt.Println("\n── Schedule ──────────────────────────")
	for _, sb := range schedule.Blocks {
		fmt.Printf("  %s: %s → %s  (score: %.1f)\n",
			sb.Block.ID, sb.Start, sb.End, sb.Score)
	}
	for _, ub := range schedule.Unscheduled {
		fmt.Printf("  [skip] %s: %s\n", ub.Block.ID, ub.Reason)
	}
}
```

### Batch Coordinate Transforms (73× Speedup)

```go
// Create one Context per epoch — amortizes the 91 µs SOFA Apco13 cost.
loc, _ := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
atm := atmosphere.AtAltitude(2635)  // SOFA refraction at all altitudes
ctx := coord.NewContext(time.NowUTC(), loc, atm)

// Transform 100 catalog stars for ~325 ns each (instead of ~91 µs each).
for _, star := range catalogStars {
    altaz, _ := ctx.ICRSToAltAz(star.ICRS)
    if altaz.Alt().Degrees() > 30 {
        observable = append(observable, star)
    }
}
```

### Moon Phases & Eclipse Detection

```go
eph := ephemeris.Default()
start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)
end := start.AddDays(365)

// All lunar phases for 2026
phases, _ := plan.MoonPhases(start, end, eph)
for _, p := range phases {
    fmt.Printf("%s: %s\n", p.Phase, p.Time)
}

// Lunar eclipses — filtered by Danjon ecliptic latitude limit
eclipses, _ := plan.LunarEclipses(start, end, eph)
for _, e := range eclipses {
    fmt.Printf("Lunar Eclipse: %s (γ=%.2f, lat=%.2f°)\n",
        e.Time, e.Gamma, e.EclipticLatitude.Degrees())
}

// Next Full Moon from tonight
nextFull, _ := plan.NextFullMoon(time.NowUTC(), eph)
fmt.Printf("Next Full Moon: %s (illumination: %.0f%%)\n",
    nextFull.Time, nextFull.Value*100)
```

### Planetary Geometry

```go
eph := ephemeris.Default()
venus := plan.NewBody(ephemeris.Venus, eph)
sun := plan.NewBody(ephemeris.Sun, eph)

// Greatest elongations of Venus in 2026
elongations, _ := plan.GreatestElongations(start, end, venus, sun)
for _, e := range elongations {
    fmt.Printf("%s: %.1f° at %s\n", e.Kind, e.Value, e.Time)
}

// Mars-Jupiter conjunctions
mars := plan.NewBody(ephemeris.Mars, eph)
jupiter := plan.NewBody(ephemeris.Jupiter, eph)
conj, _ := plan.Conjunctions(start, end, mars, jupiter)
for _, c := range conj {
    fmt.Printf("Mars-Jupiter conjunction: %s\n", c.Time)
}
```

## Architecture

`astrogo` follows a layered design:

```mermaid
flowchart TD
    %% High-level Orchestration
    plan[plan]
    catalog[catalog]

    %% Scientific Engines
    ephemeris[ephemeris]
    coord[coord]
    atmosphere[atmosphere]
    fits[fits]
    
    %% Data Providers
    iers[iers]

    %% Primitive Foundation
    subgraph Primitives
        direction LR
        time[time]
        angle[angle]
        vector[vector]
        unit[unit]
        constants[constants]
    end

    %% Dependency mappings (Top-Down: A imports B)
    plan --> coord
    plan --> ephemeris
    plan --> catalog
    plan --> atmosphere
    
    catalog --> coord
    catalog --> time
    catalog --> angle
    
    fits --> coord
    
    ephemeris --> time
    ephemeris --> vector
    ephemeris --> coord
    
    coord --> iers
    coord --> atmosphere
    coord --> time
    coord --> vector
    coord --> angle
    
    atmosphere --> angle

    iers --> time

    style Primitives fill:transparent,stroke:#888,stroke-dasharray: 5 5
```

### Key Principles
- **No cyclic dependencies**: Clean unidirectional imports.
- **Explicit data models**: Structures over magic mappings.
- **Separation of concerns**: Domain physics (`atmosphere`) decoupled from coordinate geometry (`coord`).
- **Batch-friendly computation paths**: `Context` caches expensive SOFA matrices once per epoch.

---

## Implementation Status

| Package | Purpose | Status |
| :--- | :--- | :--- |
| `constants` | Universal and astronomical constants | ✅ Stable |
| `angle` | Angular types, HMS/DMS parsing | ✅ Stable |
| `vector` | 3D geometry primitives | ✅ Stable |
| `time` | Astronomical time scales (JD-based, UTC/TAI/TT/TDB/UT1) | ✅ Stable |
| `atmosphere` | Refraction models, airmass, dispersion | ✅ Stable |
| `coord` | Coordinate types, transforms, topocentric reduction | ✅ Stable |
| `iers` | Earth Orientation Parameters (DUT1, polar motion) | ✅ Stable |
| `ephemeris` | Solar system ephemerides (SOFA + JPL SPK) | ✅ Stable |
| `catalog/resolve` | Provider interface, HTTP client, Arrow cache | ✅ Stable |
| `catalog/*` | SIMBAD, MAST, Gaia, VizieR, JPL, SBDB, OpenNGC | ✅ Stable |
| `fits` | FITS I/O, WCS (TAN projection), mmap, Arrow export | ✅ Stable |
| `plan` | Observability, constraints, events, scheduling engine | ✅ Stable |
| `unit` | Physical unit and quantity system | ✅ Stable |

See [`VALIDATION.md`](./VALIDATION.md) for scientific validation status and [`USNO.md`](./USNO.md) for the U.S. Naval Observatory accuracy report.

---

## Scientific Backend

`astrogo` uses [github.com/hebl/gofa](https://github.com/hebl/gofa) as a backend for standards-based astronomical algorithms (derived from SOFA).

These are wrapped internally to ensure:
- Clean public APIs
- Flexibility for future backends
- Isolation of low-level numerical details

---

## Project Status

🚀 **v0.1.0 — Observatory-Grade Release**

### Hardened Core
- **Scale-aware time system:** Full `UTC↔TAI↔TT↔TDB↔UT1` conversion graph, Fairhead & Bretagnon TDB correction, explicit IERS error propagation
- **SOFA-rigorous refraction:** Consistent across all altitudes (sea level through 8849m)
- **Sub-second visibility boundaries:** Chandrupatla + bisection refinement on all grid transitions
- **Production scheduler:** `SwapOptimizedStrategy` with monotonic local search, benchmarked linear scaling to 100 blocks
- **Illumination events:** `EventFamilyIllumination` with `NextNewMoon`, `NextFullMoon`, `MoonPhases` via ecliptic longitude
- **API hygiene:** Nil-location guards, epsilon-tolerant equality, defensive catalog copies

### Scientific Validation
- **JPL Horizons:** <1.0″ coordinate tolerance
- **U.S. Naval Observatory:** ≤1 min moon phases, <2.4 min rise/set
- **NASA Eclipse Catalog:** Date-exact eclipse detection (2026)

### Performance (benchmarked on i9-11980HK)
| Operation | Cost | Allocs |
|-----------|------|--------|
| `coord.NewContext` (SOFA Apco13) | 91 µs | 1 |
| `ICRSToAltAz` (cached Context) | 325 ns | 1 |
| 100-star batch (cached vs scalar) | 73× speedup | — |
| Time scale conversion | 18–90 ns | 0 |
| Refraction (rigorous) | 14 ns | 0 |
| Scheduler (100 blocks) | 123 ms | linear |

### Remaining (See [Roadmap](ROADMAP.md))
- Batch/vectorized APIs for high-throughput pipelines
- Cross-match algorithms for multi-catalog workflows

> [!IMPORTANT]
> Expect API changes until v1.0.

---

## Project Roadmap

We actively track our development pipeline across multiple capability tiers focusing on High-Performance Vectorization, Scheduling Engines, and external Data Ecosystem integration.

Please see our full [**Project Roadmap**](ROADMAP.md) to understand current milestones, tracking priorities, and architectural expansion goals.

---

## Design Goals
- Deterministic, testable scientific results
- Minimal allocations in hot paths
- Explicit handling of units and frames
- No hidden global state
- Clear separation between:
    - Scientific primitives
    - Astronomy domain logic
    - Planning layer

---

## Contributing

We strongly welcome contributions! Please refer to our [Contributing Guide](CONTRIBUTING.md) for instructions on how to set up your development environment, run numerical tests, and submit pull requests.

By participating in this project, you agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md).

---

## Testing Philosophy
- **No silent assumptions**: Fail early if ambiguity exists.
- **Explicit tolerances**: Mandatory for floating-point comparisons.
- **Edge cases**:
    - Poles
    - Horizon
    - Angle wrapping
    - Time boundaries
    - Circumpolar and never-rise targets

---

## License

MIT

---

## Inspiration
- [Astropy](https://www.astropy.org/)
- [Astroplan](https://astroplan.readthedocs.io/)

---

## Disclaimer

**This is a scientific computing library under active development.**
Results should be validated against trusted references for critical applications.
