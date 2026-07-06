# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

#### Satellite Photometry
- `plan/satellite.go`: `Satellite.ApparentMagnitudeCtx` — apparent visual magnitude from topocentric range (via `LookAngle`) and the Sun–Satellite–Observer phase angle
- `WithStdMag(stdMag, convention)` and `WithPhaseModel(model)` functional options on `NewSatellite`
- `Satellite` now implements `MagnitudeComputer`; `ApparentMagnitude` (no context) returns a sentinel error directing callers to `ApparentMagnitudeCtx`
- Sentinel errors `errNoObserverCtx`, `errNoStdMag`, `errDegenerateGeometry`

#### Generic Moving Body
- `plan/generic.go`: `GenericBody` — fallback `Observable` for ephemeris-backed targets with no photometric model. Deliberately does **not** implement `MagnitudeComputer`, so `GetDetails` no longer reports a spurious magnitude for unrecognized bodies

#### Static Magnitude
- `plan/observable.go`: `StaticMagnitude` interface for catalog magnitudes that do not vary with time or observer geometry, implemented by `Star`, `DeepSkyObject`, and `Satellite`

#### Sky Brightness & Observability (Phase 6, roadmap #28)
- New `skybrightness` package — night-sky surface-brightness model decomposed into additive components summed in linear flux space (`Nanolambert`) and converted to V `mag/arcsec²` only at the boundary:
  - `Floor` — light-pollution baseline from scalar SQM, directional `SQMGrid`, or lossy `FloorFromBortle` (SQM is the canonical input)
  - `Moonlight` — scattered moonlight, Krisciunas & Schaefer (1991) closed form (~8–23% accuracy); zero when the Moon is below the horizon
  - `ZodiacalLight` — Leinert et al. (1998) Table 17 (500 nm SI radiance) with bilinear interpolation; cross-validated against the Table 16 S10(V)⊙ values via the 1.28×10⁻⁸ W conversion
  - `Airglow` — constant dark-sky floor (Noll et al. 2012 / Patat 2008)
  - `CompositeModel` / `Model` / `Component` — allocation-free linear-flux summation
  - `VisualLimitingMag` (`LimitingMagModel`) — Schaefer (1990) / Unihedron SQM→NELM conversion with airmass extinction
- New `skybrightness/atlas` subpackage — pure-Go, offline artificial-brightness atlas providers, all returning **artificial-only** surface brightness (composable with `Floor`/`Airglow`/`Zodiacal` without double-counting the natural background):
  - `NewFalchiProvider` / `LoadFalchiGrid` — windowed or in-memory reader for the Falchi et al. (2016) World Atlas GeoTIFF (mcd/m²)
  - `NewVIIRSProvider` / `NewVIIRSGridProvider` — VIIRS-DNB radiance→SB empirical fit (Sánchez de Miguel et al. 2020 ISS coefficients as a documented stand-in; override via `WithVIIRSCoefficients` once a DNB-calibrated pair is published)
  - `NewLorenzProvider` — intentionally stubbed (`ErrLorenzNoNumericData`): the Lorenz LPA atlas is only published as non-numeric PNG zone maps
  - `Grid` / `GeoTransform` — shared in-memory raster + bilinear sampling used by both providers
- New `lightpollution` package — live client for the lightpollutionmap.info QueryRaster API (Jurij Stare), World Atlas 2015 layer by default:
  - `Client` / `New` / `WithAPIKey` / `WithLayer` / `WithHTTPClient`
  - `Client.SQM` — total (artificial+natural) zenith brightness, a self-contained answer
  - `Client.Floor` — artificial-only `skybrightness.Floor`, safe to compose with `Airglow`/`Zodiacal`/`Moonlight`
- `plan/skybrightness.go`: `LimitingMagnitudeConstraint` — soft monotonic (logistic) observability merit by default, optional `Boolean` hard cutoff; `ScoreObservableSky` folds the sky merit into `ScoreObservable`
- `examples/18_sky_brightness` — scattered-moonlight sky brightness and limiting magnitude vs. Moon separation, with constraint-based scoring

#### CI / Tooling
- `.github/workflows/pre-release.yml` (replaces `nightly.yml`)
- `.agents/rules/rules.md` — agent contribution rules
- `catalog/fink`: network test support

#### IERS Staleness Visibility
- `iers.Coverage()` — reports the currently-registered EOP model's valid MJD range (`ok=false` for `ZeroModel`), so a caller can proactively check whether the embedded/fetched data still covers an epoch of interest instead of relying on the one-time degradation warning `coord.NewContext`/`time.Time` log internally on the first out-of-range query

### Changed
- `magnitude/satellite.go`: `SatelliteApparent` now honors the `StdMagConvention` argument, normalizing Molczan standard magnitudes to the McCants reference frame via `molczanOffset = 1.45 mag` — the full ~1.4 mag Molczan↔McCants difference per [McCants](https://www.mmccants.org/tles/intrmagdef.html), combining the ~0.75 mag illumination/phase convention (`2.5·log₁₀(2)`) and the ~0.7 mag mean-vs-maximum brightness definition
- `plan/factory.go`: `FromCatalog` returns `GenericBody` (not `Planet`) for unrecognized moving-body sub-types
- `plan/details.go`: `fillStaticMagnitude` dispatches through the `StaticMagnitude` interface instead of a per-type switch; documented `TargetDetails.RA`/`Dec` as astrometric topocentric ICRS (J2000) — includes diurnal parallax, excludes precession-nutation and stellar aberration
- `go.mod`: `go` directive lowered from 1.26 to 1.25 — nothing in the module actually requires 1.26-only stdlib features (verified by a clean build+test under 1.25)
- Added top-level `NOTICE` file and an `internal/gofaext` package-doc section documenting the SOFA attribution required by the SOFA Software License (astrogo wraps `github.com/hebl/gofa`, itself a Go port of IAU SOFA routines)

### Fixed
- `magnitude/satellite.go`: `SatelliteApparent` previously ignored its `StdMagConvention` parameter, so Molczan-referenced standard magnitudes were not converted to the McCants frame; the full ~1.4 mag offset is now applied
- `time/time.go`: `.TT()`'s pre-1972 detection gated on `dat == 0 && year < 1972`, but SOFA's `Dat` only returns exactly 0 before 1960 (not before 1972); dates from 1960–1971 silently took the leap-second-table path instead of the documented ΔT-polynomial path. Now gates purely on `year < 1972`. Real-world impact was small (~0.01–0.13s across the window, not the ~36s originally estimated), but the formula used contradicted the function's own documented design
- `ephemeris/jpl/lsk/reader.go`: `parseSpiceDate` discarded `strconv.Atoi` errors on the year/day fields, silently producing a bogus deep-past JD for a malformed leap-second entry instead of rejecting it; now returns `ErrInvalidDate`
- `plan/events.go`: several rise/set/transit code paths discarded ephemeris/hour-angle evaluation errors into zero-valued sign-crossing logic and display fields, risking spurious or wrongly-displayed events; now propagate the error (skipping the affected window) instead
- `plan/phases.go`: `LunarEclipses`/`SolarEclipses` now fall back to the already-validated ecliptic latitude if the post-refinement re-evaluation fails, instead of silently zeroing it
- `plan/details.go`: `computeDetails`'s non-moving-body Alt/Az conversion now returns its error instead of discarding it; `fillRiseSetTransit` now returns early if `NewSite` fails instead of proceeding with a broken `Observer` (was a latent nil-pointer-panic risk)
- `plan/constraint.go`: `MoonSep.CheckCtx`'s signature didn't match the `ConstraintCtx` interface (missing `t`/`site` parameters), so `MoonSep` silently never got the scheduler's Context-reuse fast path; signature corrected
- `plan/schedule.go`: `BasicTransitionModel.Overhead` built two `coord.Context`s for the same epoch whenever `TransitionContext.FromTime == ToTime` (the common case); now shares one Context
- `ephemeris/jpl/spk/api.go`, `internal/tools/download.go`: the Horizons API request and kernel-file download had no timeout (`http.DefaultClient`), risking an indefinite hang on a stalled connection; both now bound the request with a context timeout
- `catalog/resolve/remote.go`: `Client.Do`'s retry loop reused the same `*http.Request` without rewinding the body via `req.GetBody()`, so a retried POST (SIMBAD/Gaia/VizieR/MAST) could resend an empty body instead of replaying the query
- `lightpollution/lightpollution.go`: `Client.Floor` built its `skybrightness.Floor` from `SQM`'s TOTAL (artificial+natural) brightness, silently double-counting the natural background when composed with `Airglow`/`Zodiacal`/`Moonlight` in a `CompositeModel`; `Floor` now returns the artificial-only value, matching `skybrightness/atlas`'s contract

### Removed
- `plan.EvalContext`, `NewEvalContext`, `NewEvalContextWith`, `plan.Slot`, `plan.Observation` — unused exported symbols with zero callers anywhere in the codebase, cut before the v1.0 API freeze
- `catalog/resolve.TargetSchema`, `ToRecordBatch`, `FromRecordBatch` — dead Arrow (de)serialization helpers left over from `MapCache`'s prior implementation; `MapCache` has stored `Target` slices directly (no Arrow round-trip) since an earlier change, and nothing else in the codebase called these
- `plan.SiteFromFITS`, `plan.TargetFromFITS` (and their 4 FITS-specific sentinel errors) — moved to the new `fits/plan` package (see Added). `plan` no longer imports `fits` at all, so `plan`'s dependency graph is now fully free of Apache Arrow — building/using just `coord`+`plan` (the scheduling engine) no longer pulls it in. `catalog/`'s own Arrow dependency was already dropped by the `TargetSchema`/`ToRecordBatch`/`FromRecordBatch` removal above; the only remaining Arrow-dependent leaves are `fits` itself (binary-table/image support) and `catalog/fink` (parquet)

### Added (continued)
- New `fits/plan` package — `SiteFromFITS`/`TargetFromFITS`, extracted from `plan` so that the FITS↔plan bridge (and its transitive Arrow dependency) is opt-in rather than bundled into core `plan`

## [0.1.5] — 2026-05-10

Lint-zero release: full `golangci-lint` compliance with zero violations across all enabled linters.

### Changed

#### Static Analysis — Zero-Violation State
- **revive**: resolved all 50+ violations
  - Added doc comments to all exported symbols across 30+ source files
  - Added package comments to all `examples/` packages
  - Fixed comment format (`Name:` → `Name is`) for const blocks
  - Blanked unused parameters in test callbacks and stub methods
  - Fixed `errId` → `errID`, `SpkId` → `SpkID` naming conventions
  - Renamed `JPL_KERNEL_URI` → `JPLKernelURI`, `KM_PER_AU` → `KMPerAU`
  - Fixed `min` builtin redefinition in satellite example
- **forbidigo**: replaced `fmt.Printf` with `log.Printf` in parser CLI tool
- **gosec**: added targeted path/rule exclusions in `.golangci.yml`
  - G115 (integer overflow): excluded for `ephemeris/jpl/`, `unit/` (NAIF IDs, SPK format fields)
  - G301/G306 (file permissions): excluded for cache directories
  - G304 (file inclusion): excluded for kernel/data file readers
  - G704/G703/G706 (SSRF/path/log): excluded for known-API HTTP clients and CLI tools
- **dupl**: added `//nolint:dupl` to 4 intentionally-similar functions (eclipse pairs, test pairs)
- **wrapcheck**: contextual error wrapping across all packages
- **err113**: sentinel errors for all error paths

#### Linter Configuration (`.golangci.yml`)
- `gocognit`: threshold raised to 100
- Disabled globally: `nestif`, `ireturn`, `recvcheck`, `goprintffuncname`, `inamedparam`, `noinlineerr`
- Each disabled linter has documented rationale in config comments

### Fixed
- `internal/tools/download.go`: fixed double-close error during `go generate` temp file cleanup
- `ephemeris/doc.go`: package comment `Package eph` → `Package ephemeris`
- `angle/parse.go`: `max` variable renamed to `limit` (builtin shadowing)
- `iers/fetch.go`: `min`/`max` variables renamed to `lo`/`hi` (builtin shadowing)

## [0.1.4] — 2026-05-08

Observable polymorphism, scheduler context sharing, TPV distortion, NORAD test hardening, and production lint audit.

### Added

#### Observable Polymorphism
- `plan/planet.go`, `plan/star.go`, `plan/deepsky.go`, `plan/asteroid.go`, `plan/comet.go`, `plan/satellite.go` — concrete `Observable` implementations replacing the monolithic `Target` type
- `plan/factory.go` — `NewTarget()` factory dispatching to typed constructors based on catalog kind and ephemeris source
- `plan/observable.go` — shared `Observable` interface and helpers

#### WCS/FITS — TPV Distortion
- `fits/wcs.go`: TPV (Tangent Plane Polynomial) distortion projection support
- 40-term standard SCAMP/SExtractor polynomial evaluation via `PV1_j`/`PV2_j` FITS headers
- Round-trip pixel↔sky accuracy <0.01 pixel validated
- `fits/wcs_example_test.go`: example test suite

#### CI
- `.github/workflows/nightly.yml`: nightly integration test workflow

### Changed

#### Scheduler Performance
- Unified `coord.Context` sharing through single code path (`ScoreObservable`, `isObservableCtx`, `checkConstraintsIntervalCtx`)
- `GreedyStrategy`, `swapPass`, `insertPass` all reuse midpoint Context
- Eliminated ~6 redundant Context allocations per scheduler iteration
- Deleted dead `checkConstraintsInterval` wrapper, `scoreObservableWithCtx`, `scoreBlockPlacementCtx` (~94 lines removed)

#### Production Hardening
- `errors.Is` for all sentinel comparisons (constraint, SPK, OpenNGC parser)
- `strings.ReplaceAll`, compound assignment operators, if-else → switch
- Lowercase local variables for IAU params (captLocal compliance)
- Fixed `tpvEval` empty-map semantics (return 0, not x)

#### Integration Tests
- FINK, NORAD, USNO, NASA, AstroPixels tests use graceful `t.Skipf()` when endpoints are unreachable

### Removed
- `plan/target.go` — monolithic Target type replaced by polymorphic Observable implementations
- `docs/TODO.md` — consolidated into `docs/ROADMAP.md`

## [0.1.3] — 2026-05-07

FINK/ZTF SSOFT photometry provider, sHG1G2 spin-geometry model, `computeDetails` refactor,
topocentric planet corrections, CI hardening, IERS auto-update, and Equinox showcase.

### Added

#### Photometry — sHG1G2 Model (Carry et al. 2024)
- `magnitude/asteroid.go`: `AsteroidSHG1G2()` — 7-parameter spin-geometry apparent magnitude
- `magnitude/asteroid.go`: `CosAspectAngle()` — aspect angle between geocentric position and spin pole
- `magnitude/asteroid.go`: `SpinCorrection()` — oblateness-dependent magnitude correction
- `magnitude/asteroid.go`: `Oblateness()` — triaxial ellipsoid → R parameter conversion

#### FINK SSOFT Catalog Provider
- `catalog/fink/` — new package implementing `resolve.Provider` for the FINK/ZTF Solar System Object Fink Table
- **Dual-mode access**: fast single-object JSON queries + bulk parquet table download (~60 MB)
- **Version pinning**: defaults to `2025.04` (API defaults to current month which may not exist)
- **r-band preference**: uses ZTF filter 2 (closer to Johnson V than g-band)
- `NewWithVersion()` — query a specific SSOFT release
- 4 offline tests + 1 network test + 5 FINK E2E validation tests

#### Target Extensions
- `catalog/resolve/target.go`: added `G1`, `G2`, `HasG1G2`, `SpinRA`, `SpinDec`, `HasSpin`, `Oblateness`, `HasOblateness` fields

#### Topocentric Planets
- `coord/context.go`: added `ObsVec()` — exports observer's geocentric ICRS position vector (AU)
- `plan/details.go`: `fillMovingBody()` now computes topocentric RA/Dec and distance by subtracting the observer vector
- Diurnal parallax correction: ~1° for the Moon, ~23″ for Mars at opposition
- Elongation also computed topocentrically

#### IERS EOP Auto-Update
- `iers/fetch.go`: `FetchIfStale(mjd)` — opt-in runtime download of fresh EOP data
- Cache at `iers/data/finals2000A.data` with 7-day staleness check
- Safe for concurrent use via `sync.Once`

#### CI Hardening
- `.github/workflows/ci.yml`: 5 jobs (was 1):
  - `lint-and-test` — existing job
  - `race-detection` — `go test -race -short`
  - `benchmarks` — artifact upload with 90-day retention
  - `integration` — tagged `integration` tests (USNO, NASA, NORAD, IMCCE) with `continue-on-error`
  - `validation` — tagged `validation` tests (JPL Horizons, SOFA)

#### Showcase
- `examples/17_equinox_prediction/` — 10-year equinox/solstice almanac + season durations + apsides + eclipses + topocentric Moon
- `docs/EQUINOX.md` — narrative showcase document with verified tables (all BRT)

### Changed

#### Magnitude Priority Chain
- `plan/details.go`: asteroid magnitude now uses **sHG1G2 → HG1G2 → HG** priority (was HG only)

#### `computeDetails` Refactor
- `plan/details.go`: extracted 8 focused helpers from 240-line monolith
  - `fillMovingBody()` — topocentric AltAz + RA/Dec + elongation (rewritten for v0.1.3)
  - `computeMagnitude()` — priority-dispatched magnitude computation
  - `cometMagnitude()`, `asteroidMagnitude()` — per-type magnitude methods
  - `helioGeometry()` — shared heliocentric distance/phase angle computation
  - `fillCatalogProps()` — parallax, proper motion, aliases
  - `applyProps()` — custom property overrides
  - `fillRiseSetTransit()` — event solver block
- `plan/target.go`: `ephID()` helper, `Position()` and `GeocentricVec()` refactored to use it

### Documentation
- `README.md`: added **Showcases** section linking Equinox, Planet Parade, Jesus, and Satellite Tracking
- `docs/EQUINOX.md`: verified almanac with BRT times for São Paulo
- `docs/VALIDATION.md`: removed topocentric from incomplete areas (now implemented)
- `docs/TODO.md`: marked CI Coverage, IERS Auto-Update, Topocentric Planets, Equinox showcase as ✅
- `docs/ROADMAP.md`: removed topocentric from remaining work

### Validation

| Metric | Result |
|--------|--------|
| sHG1G2 vs FINK phunk (8467 Benoitcarry, r-band) | mean Δ=0.011 mag, 100% within 0.025 mag |
| 2026 Eclipses vs NASA | all 4 within ≤1 min |
| 2024–2033 Seasons vs USNO | all within ≤1 min (41/41 tests) |
| Orbital eccentricity | e=0.016671 (matches IAU) |
| Topocentric Moon parallax | ~1° correction applied |

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

[Unreleased]: https://github.com/TuSKan/astrogo/compare/v0.1.5...HEAD
[0.1.5]: https://github.com/TuSKan/astrogo/releases/tag/v0.1.5
[0.1.4]: https://github.com/TuSKan/astrogo/releases/tag/v0.1.4
[0.1.3]: https://github.com/TuSKan/astrogo/releases/tag/v0.1.3
[0.1.2]: https://github.com/TuSKan/astrogo/releases/tag/v0.1.2
[0.1.1]: https://github.com/TuSKan/astrogo/releases/tag/v0.1.1
[0.1.0]: https://github.com/TuSKan/astrogo/releases/tag/v0.1.0
