# astrogo

[![Go Reference](https://pkg.go.dev/badge/github.com/TuSKan/astrogo.svg)](https://pkg.go.dev/github.com/TuSKan/astrogo)
[![Go Report Card](https://goreportcard.com/badge/github.com/TuSKan/astrogo)](https://goreportcard.com/report/github.com/TuSKan/astrogo)
[![CI](https://github.com/TuSKan/astrogo/actions/workflows/ci.yml/badge.svg)](https://github.com/TuSKan/astrogo/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/TuSKan/astrogo/branch/main/graph/badge.svg)](https://codecov.io/gh/TuSKan/astrogo)
[![GitHub release (latest by date)](https://img.shields.io/github/v/release/TuSKan/astrogo)](https://github.com/TuSKan/astrogo/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

![AstroGo Mascot](assets/image.png)

**High-performance astronomy and observation-planning toolkit for Go, inspired by Astropy and Astroplan.**

---

## Overview

`astrogo` is a Go-native scientific library for astronomy, designed to provide:

- Precise celestial coordinate transformations
- Astronomical time handling and time scales
- Observer-based sky calculations (Alt/Az, airmass, visibility)
- Solar system ephemerides (Sun, Moon, Planets via JPL DE)
- Observation planning, constraints, and event solving

It is built with a strong emphasis on:

- **Performance** (low allocations, batch-friendly APIs)
- **Numerical correctness** (SOFA-compliant algorithms)
- **Explicit, composable APIs**
- **Clean package boundaries**

Unlike Python ecosystems, `astrogo` is designed from the ground up for Go:
no dynamic magic, no hidden global state, and no implicit unit conversions.

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

### Core scientific primitives
- Angles (radians, degrees, sexagesimal — HMS/DMS parsing)
- Units and quantities
- High-precision time representation (JD-based, UTC/TAI/TT/TDB/UT1)

### Coordinate systems
- ICRS
- Galactic
- Ecliptic
- Horizontal (Alt/Az)
- Geodesic

### Transformations
- Full mapping: Geometric <-> Astrometric <-> Apparent <-> Observed 
- Frame-to-frame (Galactic, Ecliptic, ICRS, CIRS)
- Dynamic DUT1 tracking and Polar Motion (XP/YP) caching via IERS EOP rapid data
- Modular Atmospheric Refraction system (SOFA analytical vs approximation plugins)
- Aberration, light deflection, proper motion, parallax handled natively

### Observer modeling
- Geodetic locations (WGS84)
- Local sky computations
- Airmass and zenith distance

### Ephemerides
- Sun and Moon positions
- Planetary positions (Mercury → Neptune)
- **High-performance JPL SPK provider**:
    - Multi-kernel architecture (load planets and small-bodies simultaneously)
    - On-demand asteroid/comet fetching via **JPL Horizons API**
    - Support for **SPK Type 21** (Extended Modified Difference Arrays)
    - Precedence-aware segment indexing (~85x faster lookups)

### Catalogs & Data Services
- Unified `catalog.Provider` interfaces (`ObjectResolver`, `ConeSearcher`)
- Hardware-optimized native caching via **Apache Arrow** columnar batches
- Modern Go 1.23 streaming `iter.Seq2` iteration for memory-safe big data fetching
- Resilient network layers with exponent backoff testing
- Production-grade bindings:
    - **SIMBAD** (ADQL TAP)
    - **MAST** (STScI CAOM Dual-Encoding support)
    - **JPL SBDB** (Small-Body Database Search)
    - **Gaia** & **VizieR** (Data TAP)
    - **OpenNGC** (Zero-io `//go:embed` binaries)

### Visibility & planning
- Observable windows (sampled constraint evaluation)
- Altitude/airmass/separation constraints
- Target scoring and ranking

### Event Solver *(new)*
- **`EventFinder`** — two-stage numerical solver (coarse bracketing → bisection / golden-section)
- Rise, Set, and Transit events for any target (fixed or moving)
- **Sun events**: `SunEvents`, `SunriseSunset`
- **Moon events**: `MoonEvents`, `MoonriseMoonset`
- **Twilight events**: `CivilDawnDusk`, `NauticalDawnDusk`, `AstronomicalDawnDusk`
- Sub-second precision; handles circumpolar and never-rise edge cases

---

## Installation

```bash
go get github.com/TuSKan/astrogo
```

## Quick Example

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
	// 1. Setup the Observer at Mauna Kea
	loc, err := coord.NewGeodetic(angle.Deg(-155.46), angle.Deg(19.82), 4205)
	if err != nil {
		log.Fatalf("invalid coordinates: %v", err)
	}
	site, err := plan.NewSite("Mauna Kea", loc, angle.Deg(20), nil)
	if err != nil {
		log.Fatalf("failed to setup site: %v", err)
	}

	// 2. Define Observation Constraints
	// We want targets at least 30 degrees above the horizon.
	constraints := []plan.Constraint{
		plan.Altitude{Threshold: angle.Deg(30)},
	}

	// 3. Define Targets
	// Orion Nebula (fixed)
	ra, err := angle.ParseHMS("05h 35m 17.3s")
	if err != nil {
		log.Fatalf("failed to parse RA: %v", err)
	}
	dec, err := angle.ParseDMS("-05° 23' 28\"")
	if err != nil {
		log.Fatalf("failed to parse Dec: %v", err)
	}
	m42 := plan.NewFixed(catalog.Target{
		Name:  "M42",
		Coord: coord.NewICRS(ra, dec),
	})
	
	// Mars (moving)
	mars := plan.NewDefaultBody(ephemeris.Mars)

	// 4. Check Observability and Score
	now := time.NowUTC()
	
	for _, obj := range []plan.Observable{m42, mars} {
		eval, err := plan.IsObservable(obj, now, site, constraints...)
		if err != nil {
			log.Printf("skipping observability check for %s: %v", obj.Name(), err)
			continue
		}
		
		score, err := plan.ScoreObservable(obj, now, site, constraints...)
		if err != nil {
			log.Printf("skipping scoring for %s: %v", obj.Name(), err)
			continue
		}
		
		fmt.Printf("Target: %-10s  Observable: %-5v  Score: %5.1f\n", 
			obj.Name(), eval.Observable, score)
	}
}
```

### Event Solving Example

```go
// Find sunrise and sunset for tonight at Mauna Kea
eph := ephemeris.DefaultProvider()
rise, set, err := plan.SunriseSunset(tonight, tomorrow, site, eph)
if err != nil {
    log.Fatalf("failed to find sunrise/sunset: %v", err)
}
fmt.Println("Sunrise:", rise)
fmt.Println("Sunset:", set)

// Find astronomical twilight (Sun at -18°)
dawn, dusk, err := plan.AstronomicalDawnDusk(tonight, tomorrow, site, eph)
if err != nil {
    log.Fatalf("failed to find dawn/dusk: %v", err)
}
fmt.Println("Astro Dawn:", dawn)
fmt.Println("Astro Dusk:", dusk)

// Generic event finder for any target with custom threshold
finder := plan.NewEventFinder(15*time.Minute, 1*time.Second)
events, err := finder.FindEvents(m42, start, end, site, angle.Deg(30))
if err != nil {
    log.Fatalf("failed to find events: %v", err)
}
for _, e := range events {
    fmt.Println(e) // Rise/Set/Transit at sub-second precision
}
```

### Coordinate Transformations and Time Scales

```go
// Convert between coordinate frames and time scales
now := time.NowUTC()
tt := now.TT()

// Define coordinates in ICRS (e.g., Andromeda Galaxy)
src := coord.NewICRS(angle.Deg(10.684), angle.Deg(41.269))

// Transform directly to Galactic coordinates
gal := coord.ICRSToGalactic(src)
fmt.Printf("Galactic L: %v, B: %v\n", gal.L(), gal.B())

// Transform to Ecliptic coordinates using Terrestrial Time
ecl := coord.ICRSToEcliptic(src, tt)
fmt.Printf("Ecliptic Lon: %v, Lat: %v\n", ecl.Lon(), ecl.Lat())
```

### Solar System Ephemerides

```go
// Fetch high precision planet positions using JPL Ephemerides (DE440 by default)
eph := ephemeris.DefaultProvider()

// Compute Barycentric Dynamical Time (TDB) for highest accuracy
t := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTCScale).TDB()

// Get position of Mars relative to the Earth (Geocentric)
pos, err := eph.Position(ephemeris.Mars, ephemeris.Earth, t)
if err != nil {
    log.Fatalf("failed to compute ephemeris: %v", err)
}

fmt.Printf("Distance from Earth to Mars: %.6f AU\n", pos.Length())
```

## Architecture

`astrogo` follows a layered design:

```mermaid
flowchart TD
    C[constants] --> V[math/vector] --> U[units] --> A[angle] --> T[time]
    C --> Co[coord]
    Co --> P[plan]
    P --> Ep[ephemeris]
    P --> Cat[catalog]
	I[iers] --> Co
    P --> IO[io / fits]
```

### Key Principles
- **No cyclic dependencies**: Clean unidirectional imports.
- **Explicit data models**: Structures over magic mappings.
- **Separation of concerns**: Primitives isolated from domain logic.
- **Batch-friendly computation paths**: Designed for high-throughput.

---

## Implementation Status

| Package | Purpose | Status | Notes |
| :--- | :--- | :--- | :--- |
| `constants` | Universal and astronomical constants | ✅ implemented | |
| `angle` | Angular types, HMS/DMS parsing | ✅ implemented | boundary wrapping validated |
| `vector` | 3D geometry primitives | ✅ implemented | pole cases validated |
| `time` | Astronomical time scales (JD-based) | ✅ implemented | UTC ↔ TAI ↔ TT ↔ TDB ↔ UT1 verified natively |
| `coord` | Celestial coordinate types, Geodesy, Transformations | ✅ implemented | Geometric ↔ Apparent ↔ Observed verified |
| `iers` | Earth Orientation Parameters (EOP) and polar motion | ✅ implemented | Dynamic DUT1/XP/YP caching |
| `ephemeris` | Solar system ephemerides via JPL DE | ✅ implemented | multi-kernel; SPK Type 21; Horizons on-demand |
| `catalog` | Remote object resolution and cone searches | ✅ implemented | SIMBAD, MAST, Gaia, VizieR, JPL SBDB, OpenNGC |
| `plan` | Target abstraction, Observatory, Constraints, Planning | ✅ implemented | visibility, rise/set/transit solver |
| `unit` | Physical unit and quantity system | ✅ implemented | AU, Parsec, LightYear, Jansky |
| `fits` | Data formats and interoperability | ✅ implemented | `OpenMmap`, `.gz` streams, Apache Arrow tables & images |
| `wcs` | World Coordinate Systems | ✅ implemented | Spherical Gnomonic paths (`TAN`), `fits.ExtractWCS` |

See [`VALIDATION.md`](./VALIDATION.md) for scientific validation status and accuracy notes.

---

## Scientific Backend

`astrogo` uses [github.com/hebl/gofa](https://github.com/hebl/gofa) as a backend for standards-based astronomical algorithms (derived from SOFA).

These are wrapped internally to ensure:
- Clean public APIs
- Flexibility for future backends
- Isolation of low-level numerical details

---

## Project Status

🚀 **Active Development (Stable Core)**

### Completed & Stable Foundations (Phase 1 & 4)
- **Precision Core:** Core primitives (angle, time, vector) and coordinate transforms
- **Ephemeris Engine:** Unified Ephemeris (JPL SPK) with rigorous local/remote abstractions
- **Observation Planning:** Unified `plan` constraints, observability scoring, and event solving
- **Scientific Validation:** Mathematically hardened and tested against NASA JPL Horizons (<1.0" tolerance)
- **I/O & Data:** FITS Interoperability and Memory Execution (`mmap`, Arrow tables)

### Current Focus & Unimplemented (See Roadmap)
- Vectorized Batch APIs & Hardware Optimizations
- Remote Ecosystem Integrations (Simbad, VizieR)
- Advanced Observation Schedule Optimization
- Image-Domain & Photometric Output Pipelines

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
