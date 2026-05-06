# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
## [0.1.2] — 2026-05-06

Refraction hardening: USNO-standard rise/set pipeline, sub-minute accuracy, Planet Parade showcase.

### Added

#### Documentation
- `docs/PLANET_PARADE.md` — showcase reconstructing the Feb 28, 2025 seven-planet evening alignment from São Paulo using DE442, with 1-minute altitude timeline, conjunction detection, ecliptic clustering analysis
- `examples/16_planet_parade/` — runnable program reproducing all numbers in the showcase document

### Changed

#### Refraction Pipeline
- `coord/context.go`: apply SOFA Refa/Refb refraction as fallback when `Atmosphere.Model` is nil, extended guard to −1° altitude
- `coord/reduction.go`: same Refa/Refb fallback in `Reducer.Reduce` for consistency
- `plan/observatory.go`: bake 34' standard atmospheric refraction into Sun/Moon rise/set thresholds (−0.8333° at sea level), matching USNO/Explanatory Supplement convention
- `plan/events.go`: use geometric (zero-pressure) atmosphere in event solver root-finding, eliminating refraction discontinuity at horizon; `GeometricAltitude` is now truly geometric

#### Documentation
- `docs/USNO.md`: full rewrite with verified sub-minute numbers, USNO API height limitation documented, Everest 0m vs 8849m altitude-corrected tables, refraction model section
- `docs/VALIDATION.md`: tightened tolerances (Sun ≤0.5 min, Moon ≤0.6 min), refreshed AstroPixels numbers (44,524 events), added altitude correction row
- `README.md`: updated precision claims throughout (rise/set ≤0.6 min, 41/41 USNO tests)

### Fixed
- `plan/usno_test.go`: fix Tromsø DST mismatch (enforce UTC, not US DST rules for European locations), set height=0 for São Paulo (USNO API ignores height parameter), restructure Everest test for sea-level + altitude-shift validation

### Validation

| Metric | v0.1.1 | v0.1.2 |
|--------|--------|--------|
| Sun rise/set vs USNO | <1.3 min | **≤0.5 min** |
| Moon rise/set vs USNO | <1.6 min | **≤0.6 min** |
| USNO integration tests | 41/41 | 41/41 |
| AstroPixels moon phases | 44,524 matched | 44,524 matched (mean Δ=1.87 min) |
| NASA lunar eclipses | 1,424/1,424 | 1,424/1,424 (mean Δ=0.8 min) |
| NASA solar eclipses | 1,383/1,383 | 1,383/1,383 (mean Δ=0.8 min) |

## [0.1.1] — 2026-04-21

Ephemeris provider unification, unified Target architecture, lunar crescent visibility module, and plan package hardening.

### Added

#### Ephemeris
- `ephemeris/core.Provider` — provider-agnostic interface unifying planetary and satellite ephemerides
- `ephemeris.Default()` — single-call factory returning the built-in SOFA provider
- Satellite observer logic moved from `ephemeris/satellite` to `plan` (topocentric concerns belong in the planning layer)

#### Unified Target
- `plan.NewTarget(catalog.Target, ephemeris.Provider)` — universal factory for fixed and moving targets
- Convenience wrappers: `NewSun`, `NewMoon`, `NewMars`, `NewBody`, `NewDefaultBody`, `NewFixed`
- `plan.Target` implements `Observable` and `coord.Object` — single type replaces fragmented legacy types
- `plan.TargetDetails` with `GetDetails()` for on-demand property retrieval

#### Crescent Visibility
- `plan/crescent.go` — 20 historical lunar crescent visibility criteria (1910–2021)
  - Category 1: Altitude & Azimuth — Fotheringham, Maunder, Ilyas 1988, Fatoohi, Krauss-Athenian
  - Category 2: Calendrical — MABIMS 1995, Istanbul 2016, MABIMS 2021
  - Category 3: Elongation — Danjon, Schaefer, Ilyas 1984
  - Category 4: ArcV vs Width — Bruin, Alrefay, Yallop (6 zones), Odeh (4 zones), Qureshi (5 zones)
  - Category 5: Lag Time — Caldwell Naked-Eye, Caldwell Optical, Gautschy
- `CrescentParams` input struct, `CrescentResult` with `EvaluateAll()` and `String()`
- `plan/crescent_test.go` — boundary and smoke tests for all 20 criteria
- `examples/13_crescent_visibility/` — runnable example

#### Scoring
- `ScoreConfig` struct with configurable weights and `DefaultScoreConfig()`
- Moon position cache (`moonSepCache`) for efficient batch scoring
- `estimateHoursUntilSet` — lightweight forward-scan urgency estimator

### Changed

#### Scoring
- **Composite merit function** replaces naive altitude-based scoring in `ScoreObservable`
  - Altitude merit: `alt/90°` (0–1), rewarding lower airmass
  - Urgency merit: `1/max(hours_until_set, 0.5)`, prioritizes targets about to set
  - Moon separation: `min(separation/30°, 1.0)`, penalizes lunar proximity
  - Default weights: altitude 0.5, urgency 0.3, moon 0.2
- `IsObservable` shares `coord.Context` across constraints via `ConstraintCtx` (O(1) vs O(N) matrix allocations)
- `MoonSep` constraint implements `ConstraintCtx` interface

#### Concurrency
- `FilterObservable`, `RankObservable`, `RankObservables` execute concurrently via `errgroup`

#### Ephemeris Architecture
- `ephemeris/body.go` deleted — functionality merged into `ephemeris/ephemeris.go`
- `ephemeris/satellite` simplified — observer-dependent logic moved to `plan/satellite.go`
- All examples and tests updated to unified `NewTarget` / `ephemeris.Default()` API

### Removed
- `Environment` struct — empty v1 placeholder removed from `EvalContext`
- `ephemeris/body.go` — consolidated into main ephemeris package

### Fixed
- `VisibleIntervals`, `Find`, `ObservableWindows` return error for step sizes > 15 min
- `catalog/norad` — removed empty `if` branch (staticcheck)
- `ephemeris/satellite` — removed ineffectual `year` assignment (staticcheck)

### API Changes
- `ScoreObservable` signature: added `cfg *ScoreConfig` parameter (pass `nil` for defaults)
- `NewEvalContext` / `NewEvalContextWith`: removed `env *Environment` parameter
- `plan.NewTarget` replaces fragmented `plan.NewDeepSpace`, `plan.NewMoving`, etc.


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

[0.1.2]: https://github.com/TuSKan/astrogo/releases/tag/v0.1.2
[0.1.1]: https://github.com/TuSKan/astrogo/releases/tag/v0.1.1
[0.1.0]: https://github.com/TuSKan/astrogo/releases/tag/v0.1.0
