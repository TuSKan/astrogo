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

### Transformations
- Frame-to-frame transformations
- Observer-dependent transforms
- SOFA-compliant algorithms via internal wrappers

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
	"github.com/TuSKan/astrogo/constraint"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/earth"
	"github.com/TuSKan/astrogo/observatory"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/sky"
	"github.com/TuSKan/astrogo/target"
	"github.com/TuSKan/astrogo/body"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	// 1. Setup the Observer at Mauna Kea
	loc, err := earth.NewGeodetic(angle.Deg(-155.46), angle.Deg(19.82), 4205)
	if err != nil {
		log.Fatalf("invalid coordinates: %v", err)
	}
	site, err := observatory.NewSite("Mauna Kea", loc, angle.Deg(20), nil)
	if err != nil {
		log.Fatalf("failed to setup site: %v", err)
	}

	// 2. Define Observation Constraints
	// We want targets at least 30 degrees above the horizon.
	constraints := []constraint.Constraint{
		constraint.Altitude{Threshold: angle.Deg(30)},
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
	m42 := target.NewFixed(catalog.Target{
		Name: "M42",
		Coord: coord.ICRS{RA: ra, Dec: dec},
	})
	
	// Mars (moving)
	mars := target.NewDefaultBody(body.Mars)

	// 4. Check Observability and Score
	now := time.NowUTC()
	
	for _, obj := range []target.Observable{m42, mars} {
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
src := coord.ICRS{
    RA:  angle.Deg(10.684),
    Dec: angle.Deg(41.269),
}

// Transform directly to Galactic coordinates
gal := transform.ICRSToGalactic(src)
fmt.Printf("Galactic L: %v, B: %v\n", gal.L, gal.B)

// Transform to Ecliptic coordinates using Terrestrial Time
ecl := transform.ICRSToEcliptic(src, tt)
fmt.Printf("Ecliptic Lon: %v, Lat: %v\n", ecl.Lon, ecl.Lat)
```

### Solar System Ephemerides

```go
// Fetch high precision planet positions using JPL Ephemerides (DE440 by default)
eph := ephemeris.DefaultProvider()

// Compute Barycentric Dynamical Time (TDB) for highest accuracy
t := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTCScale).TDB()

// Get position of Mars relative to the Earth (Geocentric)
pos, err := eph.Position(body.Mars, body.Earth, t)
if err != nil {
    log.Fatalf("failed to compute ephemeris: %v", err)
}

fmt.Printf("Distance from Earth to Mars: %.6f AU\n", pos.Length())
```

## Architecture

`astrogo` follows a layered design:

```mermaid
flowchart TD
    C[constants] --> V[math/vector] --> U[units] --> Q[quantity] --> A[angle] --> T[time]
    C --> E[earth] --> Co[coord] --> F[frame] --> Tr[transform]
    E --> Ob[observatory] --> S[sky] --> Vi[visibility]
    Vi --> Cs[constraint] --> P[plan]
    P --> Ep[ephemeris]
    P --> Cat[catalog]
    P --> IO[io]
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
| `earth` | Geodesy and Earth models (WGS84) | ✅ implemented | geodetic ↔ ECEF validated |
| `time` | Astronomical time scales (JD-based) | ✅ implemented | UTC ↔ TAI ↔ TT ↔ TDB validated |
| `frame` | Coordinate frame types and equality | ✅ implemented | ICRS, Galactic, Ecliptic, AltAz |
| `transform` | Frame-to-frame transformations | ✅ implemented | Galactic, Ecliptic, AltAz validated |
| `sky` | Alt/Az, airmass, separation, position angle | ✅ implemented | Pickering (1982) airmass model |
| `target` | Unified observation targets (fixed/moving/body) | ✅ implemented | |
| `constraint` | Planning constraints (altitude, airmass, …) | ✅ implemented | |
| `ephemeris` | Solar system ephemerides via JPL DE | ✅ implemented | multi-kernel; SPK Type 21; Horizons on-demand |
| `body` | Solar system body definitions | ✅ implemented | |
| `catalog` | Object identity and catalog entries | ✅ implemented | OpenNGC support |
| `plan` | Observation planning, scoring, event solving | ✅ implemented | rise/set/transit/twilight solver |
| `coord` | Celestial coordinate types | ✅ implemented | FromUnitVector, Equal; round-trips validated |
| `observatory` | Observer/site modeling | ✅ implemented | LocalSiderealTime (IAU 2006 GAST) |
| `visibility` | Visibility windows, transit estimate | ✅ implemented | golden-section transit; VisibleIntervals |
| `units` | Physical unit system | ✅ implemented | AU, Parsec, LightYear, Jansky |
| `quantity` | Value + unit representation | ✅ implemented | Scale, Abs, Compare, IsZero/IsNaN |
| `fits` / `io` | Data formats and interoperability | 🚧 stubbed | **TODO**: Unimplemented (planned for v1) |

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

🚧 **Early development**

### Current Focus
- Core primitives (angle, time, vector)
- Coordinate systems and transforms
- Observer and sky calculations
- Event solving for rise/set/transit/twilight

### Not Yet Stable or Unimplemented
- Advanced planning / scheduling
- Full time scale conversions
- FITS format encoding/decoding (**TODO**: Unimplemented)
- High-volume catalog dataset ingestion (**TODO**: Unimplemented)

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
