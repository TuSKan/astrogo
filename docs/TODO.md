# TODO

Future improvements, organized by priority.

---

## Performance

### Scheduler Context Hoisting (v0.1.3)

`coord.NewContext` costs ~91 µs (SOFA Apco13). The constraint-level `ConstraintCtx`
sharing already reduced per-evaluation cost from O(N) to O(1), but the scheduler
still rebuilds the Context for every candidate placement at the same timestep.

**Impact:** ~150K redundant calls per pass → ~40s overhead on a 60-block / 8h schedule.

**Fix:** Hoist `coord.NewContext(t, ...)` out of the inner candidate loop in
`swapPass`/`insertPass` and pass it through. Mechanically simple, no algorithmic change.

**Files:** `plan/strategy.go`

---

## Features

### Apparent Magnitude

`astrogo` does not compute apparent magnitudes. Adding this requires:

- Phase angle computation (Sun–target–observer geometry)
- Heliocentric + geocentric distance
- Per-planet magnitude models (Mallama & Hilton 2018 or IAU H-G system for minor bodies)
- Surface albedo / phase curves for planets

**Scope:** New `plan/magnitude.go` or `ephemeris/magnitude.go` module.

### Showcase Documents

Science-as-showcase template (narrative + runnable code + verifiable tables):

- **Newtonian Equinox Prediction** — compute equinox/solstice dates from first principles
- **SOHO LASCO 2027 Planet Alignment** — predict the next major conjunction visible from SOHO
- **Historical Eclipse Reconstruction** — reconstruct a well-documented ancient eclipse

### Galactic Coordinates

Add `coord.Galactic` frame and ICRS ↔ Galactic transformations (rotation matrix from
IAU 1958 pole + zero-point).

### Topocentric Planets

Full topocentric correction for planets (stellar aberration + diurnal parallax).
Currently planets use geocentric positions projected to the local horizon.

---

## Testing

### SwapOptimizedStrategy Ordering

`TestSwapOptimizedStrategy` was relaxed from strict priority ordering to
composite-score validation. The underlying issue: `SwapOptimized` uses a
combined score (priority × visibility) that can reorder blocks vs raw priority.
Consider whether the strategy should guarantee priority ordering as a tiebreaker.

### Legacy Test Cleanup

Several `plan/` tests were written against pre-`HasCoord` APIs. The `NewTarget`
auto-detection fix (setting `HasCoord = true` when non-zero coords are provided)
papered over these. Audit and add explicit `HasCoord: true` to all test fixtures
for clarity.

### WCS Projection Round-Trip Tests

SIN/ARC/STG/AIT have inverse projection code but **no unit tests**. Highest-leverage
one-hour fix: write `TestProjectionRoundTrip(t, projection, ra0, dec0, fieldDeg)` for
each of the four new projections. Without these, any refactor risks silent regression.

**Files:** `fits/wcs_test.go`

---

## Infrastructure

### CI Coverage

- Add tagged test runs (`integration`, `validation`) as a separate CI job
- Add benchmark regression tracking (`go test -bench . -count=5`)
- Consider `go test -race` for concurrency safety

### IERS EOP Auto-Update

Currently EOP data is fetched via `go:generate`. Consider a runtime fallback
that downloads finals2000A.data on first use (with caching).

### README Allocation Claims

"Zero-allocation hot paths" is aspirational. `ICRSBatchToAltAz` calls
`ctx.AstrometricToObserved` → `c.Astrometric()` which allocates an `Astrometric`
value, and the SOFA `Atcoq` wrapper may allocate too. Validate with
`go test -bench . -benchmem` and look for `0 allocs/op` on the hot path.
Soften to **"minimal-allocation hot paths"** until benchmark proof exists.

---

## v0.2 Roadmap

These are honest observations about remaining gaps — not bugs, but the difference
between "works" and "production-grade."

### WCS: SIP / TPV Distortion (largest WCS gap)

Currently TAN-only for the full distortion model. Any modern survey image
(DECam, Pan-STARRS, LSST) carries SIP A/B coefficients. Without them, positions
are 0.1–1″ off at the field edge — the difference between "useful for plate
solving" and "useful for cross-matching catalogs."

The SIP polynomial evaluator is ~30 lines of code. This is the single largest
remaining gap in the WCS subsystem.

**Files:** `fits/wcs.go`

### WCS: CTYPE Axis-Order Hardening

`extractProjection` only checks `CTYPE1`. If a FITS file has `CTYPE1="DEC--TAN"`
and `CTYPE2="RA---TAN"` (uncommon but legal — the FITS spec doesn't fix axis order),
the function returns `"TAN"` correctly but the deproject math assumes axis 0 is RA.

**Fix:** Verify that one CTYPE is `RA---<proj>` and the other is `DEC--<proj>`, and
swap the pixel-to-intermediate mapping accordingly.

**Files:** `fits/wcs.go`

### Reducer Cache Asymmetry

`coord.Reducer` is consistent with `Context` (refraction model aligned) but doesn't
cache — it rebuilds the C2t06a matrix per `Reduce()` call. If a user constructs
`NewReducer(site, t, atm)` and calls `.Reduce(v)` 100 times for the same `t`, they
pay the 91 µs matrix cost 100 times.

**Options:**
1. Deprecate `Reducer` in favour of `Context.GeocentricToObserved`
2. Make `Reducer` lazy-initialize its own cache on first `Reduce()` call

Current state is functionally correct but performance-asymmetric in a way users
won't expect.

**Files:** `coord/reduction.go`

### Parallel Batch Reduction

`Context.ReduceBatch` exists but has no parallel wrapper. A `ReduceBatchParallel`
using `golang.org/x/sync/errgroup` is ~30 lines and the obvious next step for
large-catalog workloads.

**Files:** `coord/context.go`

### Scheduler Context Sharing (full fix)

The v0.1.3 TODO covers hoisting `NewContext` out of the candidate loop, but the
**full** fix is passing `*coord.Context` through `checkConstraintsInterval` and all
candidate-evaluation paths. Today only constraints share the per-step Context;
placements at the same `t` rebuild it independently.

**Files:** `plan/strategy.go`, `plan/constraint.go`
