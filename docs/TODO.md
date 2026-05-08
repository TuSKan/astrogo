# TODO

Future improvements, organized by priority.

---

## Performance

### ~~Scheduler Context Hoisting~~ ✅ (v0.1.3)

`ScoreObservable` now reuses the `*coord.Context` from `isObservableCtx`
instead of creating a second one for urgency scoring. This eliminates one
redundant `NewContext` (~91 µs) per `ScoreObservable` call.

`checkConstraintsInterval` already shared its Context across all constraints
via `ConstraintCtx`. The remaining redundancy (separate contexts for constraint
checking vs. scoring at different time points) is inherent — they use different
epochs. The v0.2 "full fix" would require passing contexts through the public API.

**Files:** `plan/plan.go`, `plan/strategy.go`

---

## Features

### ~~Apparent Magnitude~~ ✅

New top-level `magnitude/` package covering all body types:

- **Planets**: Mallama & Hilton (2018) polynomials — Mercury through Neptune
- **Saturn rings**: Ring tilt correction from IAU 2015 pole direction
- **Neptune**: Secular brightening (−0.0028 mag/yr since 1980)
- **Sun**: V = −26.74 + distance modulus
- **Moon**: Allen (2000) phase polynomial
- **Asteroids**: H,G (Bowell 1989), H,G₁,G₂ (Muinonen 2010), H,G₁₂* (Penttilä 2016)
- **sHG1G2**: Carry et al. (2024) 7-parameter spin-geometry model — `AsteroidSHG1G2()`, `CosAspectAngle()`, `SpinCorrection()`
- **Comets**: IAU standard M₁/k₁ total + M₂/k₂ nuclear models
- **Satellites**: McCants/Molczan conventions, sphere/cylinder phase functions
- **Stars**: Bouguer extinction with altitude scaling, Gaia G→V/B transformations
- **SBDB auto-fetch**: Provider now requests `phys_par` for H/G/M1/k1
- **`plan/details.go`**: Auto-fills `TargetDetails.Magnitude` with priority: sHG1G2 → HG1G2 → HG

36 tests, zero regressions.

**Files:** `magnitude/*.go`, `catalog/sbdb/sbdb.go`, `catalog/resolve/target.go`, `plan/details.go`

### ~~FINK SSOFT Provider~~ ✅

New `catalog/fink` package — FINK/ZTF Solar System Object Fink Table:

- **Dual-mode access**: Fast single-object JSON endpoint + bulk parquet table (~60 MB)
- **Version pinning**: Defaults to `2025.04` (API defaults to current month, which may not exist)
- **r-band preference**: Uses ZTF filter 2 (closer to Johnson V than g-band)
- **7-parameter export**: H, G1, G2, R (oblateness), α₀, δ₀ (spin axis) → `resolve.Target`
- **E2E validation**: 186 r-band observations of 8467 Benoitcarry — mean |Δ|=0.011 mag, 100% within 0.025 mag

9 tests (4 offline + 1 network provider + 4 network validation).

**Files:** `catalog/fink/*.go`, `magnitude/fink_test.go`

### Showcase Documents

Science-as-showcase template (narrative + runnable code + verifiable tables):

- ~~**Equinox & Solstice Almanac**~~ ✅ — `examples/17_equinox_prediction/`, `docs/EQUINOX.md`
- **SOHO LASCO 2027 Planet Alignment** — predict the next major conjunction visible from SOHO
- **Historical Eclipse Reconstruction** — reconstruct a well-documented ancient eclipse

### Polymorphic Observable Architecture (v0.2)

Replace the current `Target` god-struct (flat `catalog.Target` with 30+ fields + flag booleans)
with a polymorphic type hierarchy using Go interfaces:

**Problem:** `computeMagnitude`, `fillMovingBody`, and `computeDetails` all inspect runtime
flags (`HasH`, `HasM1`, `HasVMag`, `HasG1G2`, `Kind == "Satellite"`) to determine behavior.
The type system should encode these distinctions, not boolean flags.

**Proposed interfaces:**

```go
// Observable — base interface (already exists).
type Observable interface {
    Name() string
    Position(t time.Time) (coord.ICRS, error)
    GetDetails(ctx *coord.Context, props ...string) (*TargetDetails, error)
}

// MagnitudeComputer — optionally implemented by targets with photometry.
type MagnitudeComputer interface {
    ApparentMagnitude(t time.Time) (float64, error)
}

// MovingBody — targets with ephemeris providers.
type MovingBody interface {
    Observable
    GeocentricVec(t time.Time) (vector.Vec3, error)
    Elongation(t time.Time) (angle.Angle, error)
}
```

**Proposed concrete types:**

| Type | Implements | Physics |
|------|-----------|---------|
| `Star` | Observable, MagnitudeComputer | Bouguer extinction, proper motion |
| `Planet` | MovingBody, MagnitudeComputer | Mallama & Hilton (2018) |
| `Asteroid` | MovingBody, MagnitudeComputer | sHG1G2 / HG1G2 / HG |
| `Comet` | MovingBody, MagnitudeComputer | M1/k1 total magnitude |
| `Satellite` | MovingBody | SGP4, range scaling |
| `DeepSkyObject` | Observable | Catalog VMag |

**Design inspiration:** Astroplan's `FixedTarget`/`NonSiderealTarget` split, but with
Go-native interface dispatch instead of Python duck typing.

**Ephemeris linkage:** Each `MovingBody` concrete type embeds its own `eph.Provider` + typed
`eph.ID` directly — no more `strconv.ParseUint(Catalog.ID)` at every call site:

```go
type Planet struct {
    name     string
    id       eph.ID        // e.g. eph.Mars
    provider eph.Provider  // JPL DE kernel
}

type Asteroid struct {
    name     string
    id       eph.ID
    provider eph.Provider
    H, G1, G2 float64     // phase-curve params live on the type
    spin     *SpinAxis     // nil if unknown
}
```

Catalog providers (`fink.Resolve()`, `sbdb.Resolve()`) return the *concrete type* with
ephemeris already wired, instead of populating flag fields on a generic struct.

**Scope:** `plan/`, `catalog/resolve/`, all examples. Major migration — requires v0.2 branch.

**Files:** `plan/target.go`, `plan/details.go`, `catalog/resolve/target.go`

### ~~Galactic Coordinates~~ ✅

Already implemented: `coord.Galactic` type with `l`/`b` fields, `NewGalactic`
constructor, ICRS ↔ Galactic rotation matrix, and `TestGalacticRoundTrip` /
`TestGalacticExtremes` validation tests.

**Files:** `coord/coord.go`, `coord/transform.go`

### ~~Topocentric Planets~~ ✅

Topocentric RA/Dec and distance for all moving bodies via `ctx.ObsVec()` subtraction.
Corrects diurnal parallax (~1° for Moon, ~23″ for Mars at opposition).
Elongation also computed topocentrically.

**Files:** `coord/context.go` (added `ObsVec()`), `plan/details.go` (`fillMovingBody` rewritten)

---

## Testing

### ~~SwapOptimizedStrategy Ordering~~ — by design

`TestSwapOptimizedStrategy` was relaxed from strict priority ordering to
composite-score validation. `SwapOptimized` uses `score = merit × priority`
(line 436 plan.go), so priority already dominates at equal altitude. The only
edge case (altitude delta exactly canceling priority delta) is vanishingly rare
in real schedules. No code change needed — the test correctly validates the
composite-score invariant.

### ~~Legacy Test Cleanup~~ ✅

All `plan/` test files updated: added explicit `HasCoord: true` to every
`catalog.Target` literal that provides coordinates. 8 files, 35 call sites fixed.
Tests no longer depend on the `NewTarget` auto-detection fallback.

**Files:** `plan/plan_test.go`, `plan/constraint_test.go`, `plan/events_test.go`,
`plan/transition_test.go`, `plan/target_test.go`, `plan/scheduler_test.go`,
`plan/schedule_example_test.go`, `plan/example_test.go`, `plan/bench_test.go`

### ~~WCS Projection Round-Trip Tests~~ ✅

Completed. Tests found and fixed **3 bugs**:
- `deproject` had sinT/cosT swapped for TAN/ARC/STG (Calabretta & Greisen sign convention)
- `WorldToPixel` Newton-Raphson diverged for non-TAN projections (added analytical `project()` initial guess)
- AIT (Hammer-Aitoff) lacked native↔celestial rotation for non-zero delta0

729 round-trip points now verified across 9 configurations × 5 projections.

**Files:** `fits/wcs.go`, `fits/wcs_example_test.go`

---

## Infrastructure

### ~~CI Coverage~~ ✅

- [x] Race detection job (`go test -race -short`) — catches data races in concurrent code
- [x] Benchmark regression tracking (`go test -bench . -benchmem -count=3`) with artifact upload
- [x] Tagged test runs: `integration` (USNO, NASA eclipses, AstroPixels, NORAD, IMCCE) and `validation` (JPL Horizons, SOFA) as separate CI jobs

**Files:** `.github/workflows/ci.yml`

### ~~IERS EOP Auto-Update~~ ✅

Opt-in `iers.FetchIfStale(mjd)` downloads fresh finals2000A.all from IERS data
center if the embedded table doesn't cover the requested epoch. Cached to
`iers/data/finals2000A.data` with 7-day staleness check. Safe for concurrent
use via `sync.Once`.

**Files:** `iers/fetch.go`

### ~~README Allocation Claims~~ ✅

Validated via `go test -bench . -benchmem ./coord/...`:
- `BenchmarkICRSToAltAz_CachedContext`: **0 allocs/op** — hot path is zero-allocation ✓
- `BenchmarkICRSToAltAz`: 1 alloc (512 B) — Context struct itself, by design
- No allocation claim found in current README — the TODO was preemptive. No change needed.

---

## v0.2 Roadmap

These are honest observations about remaining gaps — not bugs, but the difference
between "works" and "production-grade."

### ~~WCS: SIP / TPV Distortion (largest WCS gap)~~ ✅

SIP (Simple Imaging Polynomial) distortion is now fully supported:
- **Forward** (pixel→world): `sipA`/`sipB` coefficients applied to pixel offsets
  before the CD matrix, per Shupe et al. 2005 convention
- **Inverse** (world→pixel): `sipAP`/`sipBP` polynomials applied after CD matrix
  inversion for direct initial guess, refined by Newton-Raphson
- **CTYPE detection**: `-SIP` suffix stripped automatically by `extractProjection`
- **Header extraction**: `parseSIPPoly` reads `A_ORDER`/`A_p_q` etc. from FITS headers
- **API**: `SetSIP(a, b)` and `SetSIPInverse(ap, bp)` for programmatic use

Validated with `TestSIPDistortion`: 0.65″ distortion at field edge on a
2048×2048 detector with 3rd-order polynomials, <0.1 px round-trip residuals.

TPV is not yet implemented (requires a separate polynomial convention).

**Files:** `fits/wcs.go`, `fits/wcs_example_test.go`

### ~~WCS: CTYPE Axis-Order Hardening~~ ✅

`extractProjection` now inspects both CTYPE axes to determine which carries
longitude (RA/GLON/ELON) and which carries latitude (DEC/GLAT/ELAT). The WCS
struct stores `lonAxis`/`latAxis` indices, and `PixelToWorld`, `WorldToPixel`,
and the Newton-Raphson solver all route intermediate coordinates through the
correct axis mapping.

Supports standard (`RA---TAN`, `DEC--TAN`) and swapped (`DEC--TAN`, `RA---TAN`)
layouts. Validated with `TestProjection_SwappedAxes` (exact match to 1e-10°).

**Files:** `fits/wcs.go`, `fits/wcs_example_test.go`

### ~~Reducer Cache Asymmetry~~ ✅

`Reducer` now lazily initializes a `*Context` via `sync.Once` on the first
`Reduce()` call. The SOFA C2t06a matrix, IERS EOP, and observer ICRS vector are
computed once and reused for all subsequent calls.

**Benchmark results (i9-11980HK):**
- `BenchmarkReducer` (new Reducer per call): 145 µs, 3 allocs
- `BenchmarkReducer_Cached` (reused Reducer): **115 ns, 1 alloc** → **1,260× faster**

The single remaining allocation (112 B) is the `Reduction` result struct itself.

**Files:** `coord/reduction.go`, `coord/bench_test.go`

### ~~Parallel Batch Reduction~~ ✅

Added `ReduceBatchParallel` and `ICRSBatchToAltAzParallel` to `Context`. Uses
`sync.WaitGroup` with chunked goroutines (no external deps). Falls back to
serial for small batches (< 2× GOMAXPROCS).

**Benchmark results (i9-11980HK, 16 threads, 10k elements):**
- `ReduceBatch/Serial`: 480 µs, 0 allocs
- `ReduceBatch/Parallel`: **112 µs, 33 allocs** → **4.3× faster**

Correctness validated: bit-identical results vs serial (1e-14° tolerance).

**Files:** `coord/batch.go`, `coord/bench_test.go`, `coord/transform_test.go`

### Scheduler Context Sharing (full fix)

The v0.1.3 TODO covers hoisting `NewContext` out of the candidate loop, but the
**full** fix is passing `*coord.Context` through `checkConstraintsInterval` and all
candidate-evaluation paths. Today only constraints share the per-step Context;
placements at the same `t` rebuild it independently.

**Files:** `plan/strategy.go`, `plan/constraint.go`
