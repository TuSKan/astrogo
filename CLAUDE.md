# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

`astrogo` (`github.com/TuSKan/astrogo`) is an observatory-grade astronomy and observation-planning library for Go. Correctness, numerical reproducibility, and public-API stability rank above cosmetic style changes.

## Commands

```bash
# Quick local checks
go test ./...                       # all packages, default (unit) tests
go test ./coord/...                 # single package
go test -run TestApparentPlace ./coord/...   # single test by name
go test -race -short -count=1 ./... # race detector (CI runs this)

# Build-tagged test suites (see "Build tags" below)
go test -tags=integration ./...     # tests against external APIs / offline caches
go test -tags=network ./...         # live network calls to astronomical APIs
go test -tags=validation ./...      # JPL Horizons / SOFA reference comparisons

# Full verification gate — required before declaring any task complete
go test -tags="integration,network,validation" ./...
go mod tidy && gofmt -l . && golangci-lint run
```

- Lint uses **golangci-lint v2** (`default: all` with documented disables in [.golangci.yml](.golangci.yml)). `goimports` local-prefix is `github.com/TuSKan/astrogo`.
- Benchmarks: `go test -run=^$ -bench=. -benchmem ./coord/... ./plan/... ./magnitude/... ./time/... ./atmosphere/...`

## Build tags

Tests are partitioned by build tag — the default `go test ./...` runs only fast, deterministic, offline tests. Anything touching a network or a heavy reference corpus is gated:

- `network` — live calls to SIMBAD, MAST, Gaia, VizieR, JPL, SBDB, NORAD, FINK. These tests do a TCP pre-check and `t.Skipf` when the endpoint is unreachable (never fail CI for external downtime). Keep `t.Fatal` only for wrong data from a reachable endpoint.
- `validation` — numerical comparisons against JPL Horizons and SOFA fixtures, mostly under [ephemeris/jpl/validation/](ephemeris/jpl/validation/) and `plan/{usno,nasa_eclipse,astropixels}_test.go`.
- `integration` — cross-provider tests with offline caches.

## Generated / embedded data

Two `go:generate` directives produce embedded data files. CI runs `go generate ./...` then `git diff --exit-code`, so regenerating must be reproducible:

- [iers/iers.go](iers/iers.go) — downloads IERS `finals2000A.all` EOP data into `iers/data/`.
- [catalog/openngc/openngc.go](catalog/openngc/openngc.go) — builds the embedded OpenNGC catalog binaries.

If you run `go generate ./...` and files change, explain what regenerated them and why before treating it as intended.

## Architecture

Strictly layered, unidirectional imports (no cycles). Lower layers never import higher ones:

```
plan, catalog                      ← orchestration (observability, scheduling, events, resolvers)
ephemeris, coord, atmosphere, fits ← scientific engines
iers                               ← data provider (Earth orientation params)
time, angle, vector, unit, constants ← primitives
```

- **`coord`** is the transform core. `coord.Context` (in [coord/context.go](coord/context.go)) caches the expensive SOFA `Apco13` matrix computation (~91 µs) once per epoch so each subsequent transform is ~325 ns. **Hot paths must create one `Context` per epoch and reuse it** — never one per transform. The scheduler shares a single `Context` per time step across constraints via the `ConstraintCtx` interface; built-in `Altitude`/`Airmass` implement it.
- **`ephemeris`** provides Sun/Moon/planet positions (SOFA + JPL SPK). `ephemeris/jpl` is the multi-kernel SPK provider with on-demand Horizons fetching; `ephemeris/satellite` is SGP4 (TEME→GCRS, ground track, look angles).
- **`plan`** is the planning/event engine: `Observable` targets, `Constraint`s, the Chandrupatla/Brent `Solver` (rise/set/transit, phases, seasons, eclipses, conjunctions), and the `Strategy`-based scheduler (`Greedy`/`Priority`/`SwapOptimized`).
- **`catalog`** + `catalog/resolve` expose unified `resolve.Provider` interfaces over SIMBAD/MAST/Gaia/VizieR/JPL/SBDB/OpenNGC/NORAD/FINK, with Apache Arrow columnar caching.
- **`internal/gofaext`** wraps [github.com/hebl/gofa](https://github.com/hebl/gofa) (SOFA-derived algorithms). All low-level SOFA calls go through here to keep public APIs clean and the backend swappable.
- **`internal/testutil`** holds float/error test helpers used across packages.

## Conventions for this codebase

- **Git: never run mutating git commands** (`add`, `commit`, `tag`, `push`, `reset`, etc.). The user manages version control. Read-only `git status`/`diff`/`log` are fine.
- **Named returns are intentional** for astronomical quantities (`ra`, `dec`, `jd`, `az`, `alt`, `dist`); short domain variable names (`r`, `t`, `jd`, `tt`, `ut1`) are idiomatic here. `nonamedreturns`/`varnamelen` are disabled deliberately.
- **"Magic numbers" are physical constants, coefficients, and NAIF IDs** — `mnd`/`goconst` are off. Do not abstract constants out of published formulas; keep algorithms readable against their reference paper/SOFA routine/Horizons fixture rather than splitting into many helpers.
- **Errors**: prefer static sentinels wrapped with `%w` over dynamic `fmt.Errorf` strings. No hidden global mutation or `init()` side effects.
- **`//nolint` only when locally scoped with a documented reason.** Do not downgrade `.golangci.yml` or remove linters to pass CI.
- **Cross-platform**: tests must pass on Linux, macOS (ARM64), and Windows. Use tolerance-based float comparisons (account for FMA/atan2 rounding); prefer inequality bounds near precision limits; document any tolerance you relax.
- **Tests**: add a regression test for bug fixes; prefer known reference values, explicit tolerances, deterministic fixtures, and edge cases (poles, horizon, angle wrap 0→360, epoch boundaries, circumpolar/never-rising targets).
