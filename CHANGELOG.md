# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] — 2026-04-16

First observatory-grade release. Validated against USNO, JPL Horizons, and NASA Eclipse Catalogs.

### Added

#### Time Package
- Full bidirectional time scale conversion graph: `UTC↔TAI↔TT↔TDB`, `UTC↔UT1`
- Fairhead & Bretagnon (1990) single-term TDB−TT correction (±3 µs residual, 85 ns/call)
- `UT1()` now returns `(Time, error)` — explicit IERS EOP data unavailability
- Cross-scale `Before`, `After`, `Equal`, `Sub`, `SubDays` with TT auto-unification
- Zero-overhead same-scale fast path (~2 ns)

#### Visibility & Planning
- Sub-second visibility boundary refinement via Chandrupatla root-finding and bisection
- `VisibleIntervals`, `Find`, `ObservableWindows` refined from ±step to <1s precision
- `SwapOptimizedStrategy` — local search scheduler with adjacent swaps + gap insertion
- `ConstraintCtx` interface for cached `coord.Context` in scheduler hot paths
- `Altitude`, `Airmass`, and `Sun` constraints implement `ConstraintCtx`

#### Event Solver
- `EventFamilyIllumination` — lunar phase events via ecliptic longitude
- `solveIllumination` with Chandrupatla refinement on signed elongation distance
- `NextNewMoon`, `NextFullMoon` convenience helpers
- `EventAnyPhase` wildcard constant
- `isPhaseEvent` guard for validation exemption

#### Atmosphere
- `AtAltitude` now returns `Model: nil` at **all** altitudes (including sea level)
- SOFA's rigorous internal refraction model used consistently everywhere
- 19 correctness tests: refraction, airmass, wavelength dispersion, pressure/temperature

### Changed

- `Reducer.Reduce` uses `EOP.DUT1` directly instead of calling `time.UT1()`
- `scoreBlockPlacement` evaluates at block midpoint for cross-strategy comparability
- `checkConstraintsInterval` creates one `coord.Context` per time step (was 1+N per step)
- `Strategy` interface documented as the primary extension point for custom scheduling

### Fixed

- `NewSite` now guards against nil geodetic location (`ErrNilLocation`)
- `Site.Equal` uses epsilon-tolerant comparison (1e-12 rad) instead of exact float equality
- `DeepSpace.Position` returns a defensive copy, preventing catalog pointer mutation
- `Custom.Position` returns a defensive copy, matching the `DeepSpace` pattern

### Performance

| Operation | Cost | Allocs |
|-----------|------|--------|
| `coord.NewContext` (SOFA Apco13) | 91 µs | 1 |
| `ICRSToAltAz` (cached Context) | 325 ns | 1 |
| 100-star batch (cached vs scalar) | 73× speedup | — |
| Time scale conversion | 18–90 ns | 0 |
| Refraction (rigorous) | 14 ns | 0 |
| Scheduler (100 blocks, SwapOptimized) | 123 ms | linear |

### Validation

- JPL Horizons: <1.0″ coordinate tolerance
- U.S. Naval Observatory: ≤1 min moon phases, <2.4 min rise/set
- NASA Eclipse Catalog: date-exact eclipse detection (2026)

### Known Limitations

- `SwapOptimizedStrategy` is a local search heuristic, not a global optimizer
- TDB correction has ±3 µs residual (sufficient for planning, not probe telemetry)
- `VisibleIntervals` creates independent Contexts per grid step (correct; each step is a different epoch)
- IERS EOP data fetched via `go:generate`, not at runtime

[0.1.0]: https://github.com/TuSKan/astrogo/releases/tag/v0.1.0
