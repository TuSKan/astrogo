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

## Embedded data

There is no `go:generate` step in this codebase — it was removed deliberately. No package uses `go:embed` either — every data source is obtained explicitly at runtime through `remote.GetFile` or a `LoadFS`-style loader (`catalog/openngc`, see [catalog/openngc/openngc.go](catalog/openngc/openngc.go); `time`'s Earth Orientation Parameters, see [time/eop.go](time/eop.go) and the unexported `time/internal/iers`).

Never reintroduce a `go:generate`/download-tooling step, and never add `go:embed` to a new catalog provider — fetch through `remote` instead (see the caching primitives below).

## Network access & `remote`

Every external endpoint astrogo can reach is registered in [remote](remote/doc.go) (primitives layer — stdlib + `cenkalti/backoff/v5` + `ungerik/go-fs` only) as an `Endpoint`: URL, `Kind`, `Subsystem`, `Timeout`/`DownloadTimeout`, and `Mutable`. The `Endpoint` is the single source of truth for how a package should talk to a service.

- **Bulk file downloads (JPL SPK/LSK kernels, IERS EOP, OpenNGC CSVs) never happen without explicit consent.** A missing kernel makes `jpl.NewProvider`/`eph.NewProvider` (both take `ctx context.Context` as their first parameter) fail with an actionable `remote.ErrDownloadDenied` unless the caller granted `remote.EnableDownloads(id, maxSize)` first, or pre-seeded the file. Never add an implicit/automatic download anywhere in this codebase — route it through `remote.GetFile` and let the existing consent gate apply.
- **All HTTP access goes through `remote.Client`, built via `remote.NewClientFor(id, opts...)`** — never a bare constructor, never a raw `http.Client`/`http.DefaultClient`. It defaults the client's timeout to the endpoint's registered `Timeout` (`DefaultAPITimeout` if zero), so packages never hand-configure a timeout that duplicates the registry. Use `Client.Get`/`GetJSON` for GETs (`GetJSON` decodes the JSON body directly) and `Client.PostForm`/`PostJSON` for POSTs whose response format is fixed. All four return `io.ReadCloser`/decode directly, never `*http.Response`, since `Client.Do` already turns a non-2xx response into an error before a caller sees a body; `Client.Do` remains the low-level escape hatch. A provider whose response format must be sniffed from raw bytes (FINK's JSON-vs-parquet) stays on the raw-reader `PostForm`/`PostJSON`. `catalog/resolve` has no HTTP client of its own — every catalog provider imports `remote` directly.
- **Every file access goes through `remote`, and prefers `gofs.File`'s own methods over reinventing them.** `remote.CacheDir(id)` returns a `KindFile` endpoint's cache directory (a `github.com/ungerik/go-fs` `File`) — use its methods (`Exists`/`ReadAll`/`WriteAll`/`MakeAllDirs`/`Remove`/`Dir`/`Join`/`OpenReader`/`OpenReadSeeker`/`Size`/`Modified`/`ContentHash`/...) directly rather than wrapping them in astrogo-specific helpers; go-fs already covers local and (if one is ever registered) non-local backends uniformly. There is no raw `*os.File`/`io.ReaderAt` escape hatch in `remote` anymore — `gofs.File.OpenReadSeeker()`'s returned `ReadSeekCloser` already embeds `io.ReaderAt`, which is how `ephemeris/jpl/spk`'s `openReaderAt` helper gets the random access SPK segment reads need. There is no local-only bypass constructor for the public API (`ephemeris.Open` was removed for this reason; the lower-level `jpl.Open` still exists for internal/test use only). The two deliberate exceptions left as plain `os.*` calls are `fits.Open` (an arbitrary user-supplied FITS file, not astrogo-managed data) and `catalog/fink`'s `os.CreateTemp` scratch file (a transient buffer for a third-party parquet reader).
- **Reuse `remote.GetFile` instead of hand-rolling "ensure cached, then read/download" in a package.** `remote.GetFile(ctx, id, name, opts...) (gofs.File, error)` resolves `id`'s `CacheDir`, reuses a cached file on existence alone for `Mutable: false` endpoints (JPL kernels) or after a HEAD-probe shows nothing changed upstream for `Mutable: true` ones (IERS, OpenNGC), and downloads (consent-gated, using the endpoint's `DownloadTimeout` unless overridden by `WithDownloadTimeout`) on a miss. It returns the `gofs.File` itself, not an opened stream — the caller opens it however it needs (`OpenReader` for sequential, `OpenReadSeeker` for random access, `ReadAll` for whole-content). `WithCacheName` sets the on-disk filename when it differs from the URL path segment (IERS: URL is the whole resource, `name` is `""`, `WithCacheName("finals2000A.data")` supplies the cache filename). `WithValidate` runs a hook on freshly downloaded bytes before they're trusted/cached (`time/internal/iers`'s fetch path uses this so a corrupt response is never cached). `remote.Save(r io.Reader, dest)` is the generic atomic(ish) write primitive underneath it, still exported for content that doesn't arrive via `GetFile`'s own download path (a decoded API payload, a computed checksum sidecar).
- **A `KindFile` endpoint serving a small, fixed manifest of files (not arbitrary caller-named ones) lists them in `Endpoint.Files`** — see `remote.OpenNGC`'s two source CSVs. The registry owns which files an endpoint serves; a consuming package reads `remote.Lookup(id).Files` rather than hardcoding its own copy of the list. JPL kernels (`NAIFSPK`/`NAIFLSK`) leave `Files` nil since the caller names the kernel.

See the README's "Data downloads & offline usage" section for the user-facing picture (sizes, `remote.SetOffline`, `time.LoadFS`).

## Architecture

Strictly layered, unidirectional imports (no cycles). Lower layers never import higher ones:

```
plan, catalog, fits/plan                         ← orchestration (observability, scheduling, events, resolvers, FITS↔plan bridge)
ephemeris, coord, atmosphere, fits, skybrightness ← scientific engines
skybrightness/lpmap                              ← data providers (live light-pollution API)
time, angle, vector, unit, constants, remote      ← primitives
```

- **`time`** is the sole gateway for Earth Orientation Parameters and epoch arithmetic. `time/internal/iers` (unexported — nothing outside `time/` can import it) fetches/parses IERS EOP data; `time` re-exports what's needed (`time.EOP`, `time.Fetch`/`FetchIfStale`/`LoadFS`/`RegisterModel`/`GetModel`/`Coverage`) and adds `Time.EOP()`, `Time.MJD()`, `Time.GAST()`, `Time.JulianEpochYear()`, `Time.DayOfYear()`. `coord` and every other package get EOP/epoch values through these `time` APIs — never by hand-rolling MJD/GAST arithmetic or importing EOP internals directly.
- **`coord`** is the transform core. `coord.Context` (in [coord/context.go](coord/context.go)) caches the expensive SOFA `Apco13` matrix computation (~91 µs) once per epoch so each subsequent transform is ~325 ns. **Hot paths must create one `Context` per epoch and reuse it** — never one per transform. The scheduler shares a single `Context` per time step across constraints via the `ConstraintCtx` interface; built-in `Altitude`/`Airmass` implement it.
- **`ephemeris`** provides Sun/Moon/planet positions (SOFA + JPL SPK). `ephemeris/jpl` is the multi-kernel SPK provider with on-demand Horizons fetching (`Provider.AddKernel`/`State`/`FindSegment`/`SupportedBodies` are guarded by an internal `sync.RWMutex` — safe to call concurrently); `ephemeris/satellite` is SGP4 (TEME→GCRS, ground track, look angles).
- **`plan`** is the planning/event engine: `Observable` targets, `Constraint`s, the Chandrupatla/Brent `Solver` (rise/set/transit, phases, seasons, eclipses, conjunctions), and the `Strategy`-based scheduler (`Greedy`/`Priority`/`SwapOptimized`). `plan` has no dependency on `fits` or Apache Arrow — the FITS↔plan bridge (`SiteFromFITS`/`TargetFromFITS`) lives in the separate `fits/plan` package.
- **`skybrightness`** (+ `skybrightness/atlas`, `skybrightness/lpmap`) models night-sky surface brightness (moonlight, zodiacal light, airglow, light-pollution floor) as additive linear-flux components; `atlas` decodes offline light-pollution atlas files, `lpmap` is a live client for the lightpollutionmap.info API — both feed `skybrightness.Floor` and neither is ever imported back by core `skybrightness` (enforced by an import-graph test). `plan.LimitingMagnitudeConstraint`/`ScoreObservableSky` wire this into observability scoring.
- **`catalog`** + `catalog/resolve` expose unified `resolve.Provider` interfaces over SIMBAD/MAST/Gaia/VizieR/JPL/SBDB/OpenNGC/NORAD/FINK, with Apache Arrow columnar caching. All network access goes through `remote.Client`.
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
