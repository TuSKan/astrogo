# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.6.1] — 2026-07-21

### Added
- `coord.Context.AtTime(t time.Time) *Context` — cheaply derives a new `Context` at a nearby instant by updating only Earth-rotation-dependent state (Earth Rotation Angle, the celestial-to-terrestrial matrix, the observer vector) instead of rebuilding the full SOFA `Apco13`/IAU 2006/2000A precession-nutation computation from scratch. Documented accuracy bound: ≲0.1″/hour of drift from the source `Context`'s epoch.

### Fixed
- `plan.EventSolver`'s rise/set/twilight/transit sweeps (`solveVisibility`) rebuilt a full `coord.NewContext` for every sampled instant and every bisection-refinement step, measured at ~65% of total CPU in a 14-night forecast benchmark ([#10](https://github.com/TuSKan/astrogo/issues/10)). It now rebuilds a full `Context` only once per hour of solve window and derives every sample/bisection step from it via the new `Context.AtTime`, cutting `BenchmarkFortnightEvents` from ~1.6s/op to ~0.6s/op on the reporter's repro shape. Reported event values are unaffected — the post-refinement display rebuilds are untouched.
- `catalog/mast`'s `ResolveObject` failed to decode MAST invoke-API responses when the server ignored the request's `"format": "json"` field and returned its default XML body instead (`invalid character '<' looking for beginning of value`). The response body is now sniffed and decoded as JSON or XML as appropriate, instead of assuming a 2xx response is always JSON.

## [0.6.0] — 2026-07-17

### Added
- `remote.WithProgress(func(downloaded, total int64))` — a `ReadOption` reporting a `GetFile` download's progress as it streams, on both the buffered (`WithValidate`) and direct-to-disk paths. Independent of whether a caller supplies it, `GetFile` now logs one line (via the stdlib `log` package) at the start of an actual download showing the endpoint and its registered `ApproxSize` — never logged on a cache hit.
- `ephemeris` package doc: a "Choosing a Provider" section comparing `Default()` against the JPL kernel family (de440s/de440/de442/de441) on accuracy, size, and offline-friendliness — previously only a size table existed, with no guidance on which provider to reach for.
- `plan` package doc: a "Finding what you need" task-oriented symbol index (site setup, targets, rise/set/twilight, observability scoring, geometric events, phases/eclipses, crescent visibility, scheduling, satellite passes, low-level solving) — previously prose-only with no way to locate a symbol among the package's ~150 exported names short of scanning godoc alphabetically.
- `time.MJD()`, `time.GAST()`, `time.JulianEpochYear()`, `time.DayOfYear()` — epoch-arithmetic accessors on `Time`, replacing hand-rolled duplicates of the same formulas that had accumulated in `coord.NewContext` (MJD), `plan.Site.LocalSiderealTime`/`ephemeris/satellite` (GAST — the latter's own copy was misleadingly named `computeGMST`; `Gst06a` computes the *apparent*, not mean, sidereal time), `magnitude/planet.go` (Julian epoch year), and `catalog/norad` (TLE day-of-year).
- `time.SetRetryCooldown(d time.Duration)` — configure (or disable, with `0`) the post-failure EOP-fetch throttle.

### Changed — BREAKING
- **`iers` is no longer a top-level package.** It moves to the unexported `time/internal/iers` (Go's `internal` visibility rule makes it compiler-enforced, not just documented, that nothing outside `time/` can import it) and `time` becomes the sole public gateway for Earth Orientation Parameters: `time.EOP`/`time.Model`/`time.ZeroModel`/`time.Table` (type aliases), `time.ErrOutOfRange`/`ErrNoRecords`/`ErrEOPHTTPStatus`, `time.RegisterModel`/`GetModel`/`Coverage`/`LoadFS`/`ParseFinals2000A`, and the new `Time.EOP()` method (the same degrade-to-zero-with-one-time-warning fallback `coord.NewContext` used to implement itself — `coord` no longer imports EOP internals directly, it calls `t.EOP()`). `iers.FetchNow` is renamed `time.Fetch`; `iers.FetchIfStale(mjd float64)` becomes `time.FetchIfStale(ctx, t Time)` (takes a `Time` directly, ctx-first, matching `Fetch`). The `go:embed` IERS snapshot (`iers.go`, `iers.FinalsData`, `iers/data/`) is gone entirely — no build ever silently bakes in local EOP data again; populate it explicitly via `time.Fetch`/`FetchIfStale`/`LoadFS`.
- **`lightpollution` moved to `skybrightness/lpmap`** (package name `lightpollution` → `lpmap`). It's a live-API sibling of `skybrightness/atlas` — both resolve the same World Atlas artificial-brightness data for a `skybrightness.Floor`, just from a downloaded file (`atlas`) versus a live per-request query (`lpmap`) — and the old top-level package name didn't make that relationship, or the live-client-vs-physics-model distinction from core `skybrightness`, visible. Update `import "github.com/TuSKan/astrogo/lightpollution"` to `import "github.com/TuSKan/astrogo/skybrightness/lpmap"`; `lightpollution.New()` is now `lpmap.New()`. `remote.LightPollution` (the endpoint registry key) is unchanged.
- **`plan.NewSite`'s `horizon angle.Angle` and `tz *time.Location` parameters are now the optional `WithHorizon(angle.Angle)`/`WithTimeZone(*time.Location)` `SiteOption`s**, defaulting to `angle.Zero()`/UTC. The signature changes from `NewSite(name, loc, horizon, tz)` to `NewSite(name, loc, opts...)` — the overwhelming majority of call sites passed a zero horizon and/or nil timezone anyway, so most callers now drop both arguments entirely (`NewSite("Site", loc)`); a non-default horizon or timezone becomes `NewSite("Site", loc, plan.WithHorizon(angle.Deg(20)), plan.WithTimeZone(tz))`. Matches the `WithX`-functional-option convention already used by `Asteroid`/`Comet`/`DeepSkyObject`/`Satellite`/`Star` in this package. `Site.WithHorizon`/`Site.WithTimeZone` (the copy-with-new-value methods) are unchanged.

### Changed
- Every `plan.NewSite` call site across examples, docs, and tests now spells a zero horizon limit as `angle.Zero()` (or omits it entirely now that it's optional — see above) — previously a mix of `angle.Zero()`, a bare `0`, and `angle.Deg(0)` (all numerically identical, but inconsistent to read).

## [0.5.0] — 2026-07-16

### Changed — BREAKING
- **`go:generate` is gone.** `internal/tools/cmd/download` and `catalog/openngc/parser` are deleted; `iers/iers.go` and `catalog/openngc/openngc.go` no longer have `go:generate` directives.
- **`catalog/openngc` no longer uses `go:embed` at all** — no `catalogFS`, no `catalog/openngc/data/`, no package-level cached CSV, no `loadOnce`. `openngc.New()` now fetches and merges the two upstream source CSVs on every call (content-checked against a local cache, so a re-run costs only a HEAD probe once cached), exactly like every other astrogo catalog provider does its own network access — nothing embedded, nothing to fall back to.
- `ephemeris.Open` and the CI/README references to a local-only "pre-seed then Open, bypassing remote" construction path are removed. Pre-seed a kernel at its normal `remote.DataDir()` path and call `eph.NewProvider` as usual instead — every downloader already checks disk before network, so this is zero-network once the file is there.
- `iers`: the 7-day `staleDays` wall-clock cache-expiration window is gone. `FetchIfStale`/`FetchNow` now go through `remote.GetFile`, which issues a cheap HEAD probe and reuses the on-disk cache whenever the upstream `finals2000A.all` content hasn't actually changed, no matter its age — instead of blindly trusting/distrusting it by a fixed time window.
- `iers.LoadFile` and `iers.UseEmbedded` (and `ErrEmbeddedUnavailable`) are removed — `LoadFS` is now the only file-loading entry point. Load a local path with `iers.LoadFS(os.DirFS(dir), name)`; there is no dedicated "reload the embedded snapshot" call anymore.
- `internal/tools` is deleted outright — it held only a placeholder `doc.go` and a coverage-workaround dummy test after `internal/tools/cmd/download` was removed; nothing imported it.
- **`remote`'s public API is rebuilt around `Endpoint`.** New `Endpoint.Timeout`/`DownloadTimeout`/`Mutable`/`Files` fields make each endpoint self-describing — timeout, cache-reuse policy, and (for a small fixed manifest like OpenNGC) the exact files it serves — instead of packages configuring that per call site. `remote.GetFile(ctx, id, name, opts...) (gofs.File, error)` is now the only caching entry point, replacing `EnsureCached`/`Open`/`FetchCached`/`OpenFile` (all deleted, along with `download.go`/`signature.go`, folded into `remote/fetch.go` as unexported internals). `remote.CacheDir(id)` replaces the string-keyed `SubsystemDir` (now unexported). `remote.NewClientFor(id, opts...)` replaces bare `remote.NewClient()`, defaulting to the endpoint's registered `Timeout`. `jpl.NewProvider`/`eph.NewProvider`/`spk.CacheDownload`/`spk.CacheAPI`/`lsk.Cache` all gained a `ctx context.Context` first parameter.
- Every catalog provider (SIMBAD/Gaia/VizieR/MAST/FINK/NORAD/SBDB/JPL) and `lightpollution` migrated off hand-rolled `http.NewRequestWithContext` request-building onto the new `Client.PostForm`/`PostJSON`/`GetJSON`/`Get` convenience methods (see Added below) — all return `io.ReadCloser`/decode directly instead of `*http.Response`, since `Client.Do` already converts a non-2xx response into an error before a caller ever sees a body.
- `jpl.WithDataDir` no longer redirects where NAIFSPK/NAIFLSK kernels are cached (that's always `remote.CacheDir`, endpoint-keyed) — it now only affects `LoadedKernels()` path labels and where Horizons-generated small-body kernels land. Use `remote.SetDataDir`/`SetDataDirPath` to relocate the shared cache.

### Added
- `remote.GetFile(ctx, id, name, opts...) (gofs.File, error)` — the one place astrogo implements "reuse the cache if nothing changed upstream, else download-with-consent, then persist." `iers`, `catalog/openngc`, `ephemeris/jpl`'s SPK/LSK kernel loading all call this instead of each hand-rolling the same check-cache/consent/download flow (they previously didn't — `catalog/openngc`'s copy never even enforced the consent gate, a real bug now fixed — see Fixed below). Endpoint-keyed `Mutable` decides the reuse strategy: a HEAD-probe content check for endpoints whose upstream can change (IERS, OpenNGC), plain existence for immutable/versioned ones (JPL kernels). `WithCacheName`/`WithValidate`/`WithDownloadTimeout` are its `ReadOption`s.
- `remote.CacheDir(id) (gofs.File, error)` — a `KindFile` endpoint's cache directory, keyed by its registered `Subsystem`.
- `remote.OpenNGC` is a real, usable endpoint again (pinned to the same commit SHA the old `go:generate` parser used), with an `Endpoint.Files` manifest (`NGC.csv`, `addendum.csv`) — the registry owns which files it serves, not the `catalog/openngc` package. `openngc.New()` downloads and merges the two upstream source CSVs directly into `resolve.Target`s on every call — the old runtime-CSV round-trip (`encodeRuntimeCSV`/`parseCSV`) is gone along with the embedded data it existed to read. Calling `remote.EnableDownloads(remote.OpenNGC, maxSize)` is the only thing a caller does; nothing needs to import `catalog/openngc` directly, matching the existing `ephemeris/jpl` convention. Without that consent, or on any fetch failure, `New` returns an empty, warning-logged provider — the same degraded behavior every other astrogo catalog provider has when its backing source is unreachable.
- 4 `examples/` programs that resolve against `catalog.OpenNGC` (`05_resolve_name`, `14_target_scoring`, `15_target_details/{deep-sky,stars}`) now only call `remote.EnableDownloads(remote.OpenNGC, ...)` — no `catalog/openngc` import, no explicit fetch call.
- `remote.Save(r io.Reader, dest gofs.File) error` — the generic atomic(ish) write primitive (temp file + rename on the local filesystem) `GetFile`'s download path is built on; still exported for content that arrives another way (a decoded API payload, a computed checksum sidecar). This is the only file-write primitive in `remote` — every file *read* goes through `gofs.File`'s own methods (`Exists`/`ReadAll`/`OpenReader`/`OpenReadSeeker`/...) directly; there is no raw `*os.File`/`io.ReaderAt` wrapper anymore (`gofs.File.OpenReadSeeker()`'s return already implements `io.ReaderAt`).
- `remote.NewClientFor(id, opts...) (*Client, error)` — the sole `Client` constructor, defaulting its timeout to the endpoint's registered `Timeout` (`DefaultAPITimeout` if zero). Replaces bare `remote.NewClient()`.
- `Client.GetJSON(ctx, id, path, query, out)`, `Client.PostForm(ctx, id, path, v)`, `Client.PostJSON(ctx, id, path, body)` — GET-and-decode and POST convenience methods returning `io.ReadCloser`/decoding directly, alongside the existing `Client.Get`. Every catalog provider and `lightpollution` now builds requests through these instead of hand-rolling `http.NewRequestWithContext` + header-setting + response-body plumbing at each call site.

### Fixed
- `iers/setup.go` no longer has three near-duplicate open/parse/register functions — just `LoadFS`, taking any `io/fs.FS`.
- **`iers.FetchNow`/`FetchIfStale` never actually enforced the download-consent gate.** They called `remote.Client.Get` directly instead of going through the registry's download path, so IERS data downloaded regardless of whether `remote.EnableDownloads(remote.IERSFinals2000A, ...)` had been called — silently violating astrogo's own "never download without consent" rule. Routing through `remote.GetFile` fixes this: `remote.EnableDownloads(remote.IERSFinals2000A, maxSize)` is now actually required, matching the documented behavior and every other endpoint.
- `remote.DataDirPath(subsystem) (string, error)` is removed — it returned `SubsystemDir(subsystem).LocalPath()`, which is silently `""` for a non-local `remote.SetDataDir` backend (e.g. an s3:// `gofs.File`). Callers now use `remote.CacheDir`/`GetFile` and work with the returned `gofs.File` (`.Join(name)`, `.Exists()`, `.ReadAll()`, ...) instead of assuming a local path string.
- `iers.CachePath() string` is renamed `iers.CacheFile() (gofs.File, error)` for the same reason — a bare string can't represent a non-local cache location.
- `examples/13_crescent_visibility` and `examples/19_offline_setup` were the only two examples not importing `ephemeris` as `eph`; now all 17 do, matching the README's convention.
- **`ephemeris/jpl/spk`'s SHA-256 checksum verification opened a cached kernel file a second time** to hash it, after `CacheDownload` had already opened it once for the `spk.Reader`. It now hashes through the already-open `io.ReaderAt` handle via `io.NewSectionReader` — one open per `CacheDownload` call, not two.
- **`catalog/simbad`'s `if resp.StatusCode >= 400 { ... }` block was unreachable dead code** — `Client.Do` already converts any non-2xx response into a returned error before a caller ever sees a response, so the check could never fire. Removed along with the migration to `Client.PostForm`.
- **`catalog/mast`'s JSON-then-XML response-format fallback was unreachable** — the request always sets `format: json` (MAST's Horizons-Lookup-style API defaults to XML only if the caller doesn't specify), so a 2xx response body is always JSON; the byte-sniffing/`encoding/xml` fallback path could never trigger. Removed — `ResolveObject` now decodes the JSON body directly.
- **`Endpoint.Files`'s slice wasn't defensively copied by `Endpoints()`/`Lookup()`** — every other `Endpoint` field is a value type, but a caller mutating a returned `Endpoint`'s `Files` slice would have silently corrupted the registry's own copy. `Endpoints()`/`Lookup()` now clone `Files` on the way out.
- `catalog/norad`'s `Search` had a redundant local `context.WithTimeout(..., 30*time.Second)` wrapper — `remote.NewClientFor(remote.CelesTrak)` already bounds the request at the endpoint's registered `Timeout` (also 30s). Removed the duplicate.

## [0.4.0] — 2026-07-13

### Changed — BREAKING

- **astrogo no longer auto-downloads anything.** Constructing a JPL ephemeris provider (`jpl.NewProvider`/`eph.NewProvider`) against a kernel that isn't already present locally now fails with an actionable `remote.ErrDownloadDenied` (naming the file, its size, and how to proceed) instead of silently downloading it. Grant consent per endpoint with `remote.EnableDownloads(remote.NAIFSPK, maxSize)` (and `remote.NAIFLSK` for the tiny leap-second kernel), or pre-seed the file, or use the new offline-only `jpl.Open`/`eph.Open`. See the README's "Data downloads & offline usage" section.
- `catalog/resolve.Client`, `.HTTPError`, `.RetryPolicy`, `.DefaultRetryPolicy`, and `.NewClient` are removed; every catalog provider now uses `remote.Client`/`remote.NewClient` directly. `resolve.HTTPError.Error()`'s message prefix changes from `catalog:` to `remote:`.
- All hardcoded endpoint URL constants (`spk.JPLSPKKernelURI`, `lsk.JPLLSKKernelURI`, `spk.JPLHorizonsAPI`, `jpl.JPLKernelURI`, and each catalog provider's private `tapSyncURL`/`mastAPI`/`gpAPIBase`/`sbdbQueryAPI`/`ssoftURL`/`queryAPI` constants) are removed — URLs now live in the `remote` package's endpoint registry, overridable via `remote.SetURL`.
- `internal/tools.Download` and `internal/cache` are removed, absorbed into `remote.Download`/`remote.DataDir`.

### Added

#### Centralized network access: the `remote` package
- New public `github.com/TuSKan/astrogo/remote` package: a registry of every external endpoint astrogo can reach (`remote.Endpoints()`, `remote.Disable`, `remote.SetURL`, `remote.SetOffline`), an HTTP client with retry/backoff shared by every provider (`remote.Client`/`remote.NewClient`), a consent-gated file downloader (`remote.Download`, `remote.EnableDownloads`/`DisableDownloads`, `remote.SetPolicy`), and a configurable storage location for all downloaded data (`remote.SetDataDir`/`SetDataDirPath`/`DataDir`/`SubsystemDir`, built on `github.com/ungerik/go-fs` so a future blob/bucket backend can be registered without call-site changes)
- `ephemeris/jpl`: `Provider.AddKernelFile`, `RemoveKernel`, `UnloadAll`, `LoadedKernels` (kernel lifecycle management) and the package-level `Open(lskPath, spkPaths...)` for pure local, zero-network construction
- `ephemeris`: `Open(lskPath, spkPaths...)` passthrough to `jpl.Open`
- `iers`: `LoadFile`, `LoadFS`, `UseEmbedded`, `FetchNow` — the full local/explicit control set for Earth-orientation data, alongside the existing `FetchIfStale`/`RegisterModel`/`GetModel`/`Coverage`
- `catalog/openngc`: the `go:generate` source URLs are now pinned to a specific upstream OpenNGC commit SHA, so regeneration is reproducible

### Documentation
- `README.md`: new "Data downloads & offline usage" section (endpoint/size table, consent examples, offline setup); fixed the "No API keys, no downloads" claim to scope it to the SOFA quickstart
- `CLAUDE.md`: new "Network access & `remote`" section; `remote` added to the architecture diagram and layering rules
- 9 `examples/` programs that construct a JPL provider now call `remote.EnableDownloads` first (with a size comment); new `examples/19_offline_setup/` demonstrates `remote.SetOffline`, `jpl.Open`, and `iers.LoadFile`
- `iers/doc.go`, `catalog/openngc/doc.go`: updated for the lazy (non-`init()`) load and the pinned OpenNGC source SHA

### Fixed
- `iers`, `catalog/openngc`: embedded data is now parsed lazily (on first `GetModel()`/`New()` call) instead of in `init()`, removing a ~3.7 MB parse-on-import cost paid by every program that merely imports `iers` (transitively, via `coord`) whether or not EOP data is ever queried — also brings both packages into compliance with this project's own "no `init()` side effects" rule (see `CONTRIBUTING.md`/`CLAUDE.md`)
- `remote`: fixed a data race in `TestClientContextCancelNotRetried` (plain `int` counter written by the test's HTTP handler goroutine, read by the main test goroutine after a context-deadline return with no synchronization between them)
- `plan` (integration tests): `usnoGet` now bounds each USNO API request with an explicit `context.WithTimeout` raced via `select`, independent of `http.Client.Timeout` — a stalled TCP connect on a CI runner was observed to outlast the client's own 30s timeout, hanging the whole test binary until its 10-minute global alarm fired

## [0.3.0] — 2026-07-08

### Added

#### Catalog Providers: full `catalog/jpl` and `catalog/vizier` implementations
- `catalog/jpl`: `ResolveObject` now parses Horizons' free-text `result` field for all three recognized response shapes (verified against live Horizons traffic) instead of always returning `ErrNotImplemented`:
  - Ambiguous major-body matches (planets, satellites, spacecraft, barycenters) via a fixed-width table parser, ported from `ephemeris/jpl/spk`'s production-proven `parseHorizonsResult` and hardened with a COSPAR-designation regex (`cosparDesignationRe`) so a body name that overflows its nominal column width no longer corrupts the following Designation field
  - Ambiguous small-body matches (comets/asteroids) via a new parser for Horizons' structurally different JPL/DASTCOM "Small-body Index Search Results" table
  - Unambiguous single matches (major or small body) via Horizons' stable "Target body name: `<name>` (`<id-or-designation>`)" header line — deliberately not the orbital-elements printout body that follows, which has no stable, verified schema
  - A genuinely novel/unrecognized non-blank response shape still returns `ErrNotImplemented`, preserving the honest-error-over-fabricated-Target policy from the prior audit
  - Added the missing `cache.Set` call before yielding (every sibling provider does this; `catalog/jpl` previously never cached a result)
- `catalog/vizier`: `resolve.ConeRequest` gains a `Table` field selecting which VizieR table to query, backed by a new schema registry (`tables.go`) mapping table name → RA/Dec/designation column names + `resolve.Kind`. An empty `Table` preserves the exact previous behavior (2MASS `II/246/out`). A table not in the registry returns the new `ErrUnknownTable` rather than guessing column names. Registered today: `II/246/out` (2MASS, default), `I/239/hip_main` (Hipparcos), `I/355/gaiadr3` (Gaia DR3)
- `catalog/vizier`: the cache key now includes the table name (previously only ra/dec/radius/limit — two different tables queried over the same cone would have collided on one cache entry once table selection existed)
- `catalog/vizier`: `parseCSV` now tags each row with the queried table's `resolve.Kind` instead of always `resolve.KindStar`, and sets `Target.HasCoord = true` (previously never set despite `Coord` always being populated)

### Documentation
- `catalog/jpl/doc.go`, `catalog/vizier/doc.go`: rewritten to describe the now-real capability
- `README.md`, `docs/ROADMAP.md`: both v1.0.0-blocking catalog providers are now fully implemented; Implementation Status table updated, "Path to v1.0.0" section updated
- `CONTRIBUTING.md`: added guidance for contributors using AI coding tools to strip generated commit-message attribution/co-author trailers before submitting a PR

## [0.2.0] — 2026-07-07

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
- `atmosphere/atmosphere.go`: `RefractionApproximate`/`RefractionRigorous`'s low-altitude cutoff was −5.0°, past Bennett (1982)'s tangent-formula singularity at −4.4° — altitudes in [−5.0°, −4.4°) could return wildly wrong refraction (observed up to −711 arcmin in testing) instead of the documented zero; tightened to −4.0° (`lowAltitudeCutoffDeg`), clear of both Bennett's and Saemundsson's (−5.11°) singularities with margin
- `ephemeris/jpl/spk/reader.go`: `CacheDownload`'s auto-heal only checked file size and the DAF summary/directory records, leaving the bulk Chebyshev-coefficient data (most of the file) unverified; it now records a SHA-256 sidecar the first time a kernel is trusted and checks against it on every later open, since NAIF publishes no per-kernel checksum to verify against externally
- `lightpollution/lightpollution.go`: `Client.artificialBrightness` made a single unconditional HTTP request with no retry logic; it now retries transient failures and 429/5xx responses with bounded exponential backoff, matching `catalog/resolve.Client`'s policy
- `catalog/vizier`: `ConeSearch`'s CSV parser silently returned an empty result set on a successful response instead of parsing it; it now parses `designation`/`ra`/`dec` into real `resolve.Target`s
- `catalog/jpl`: `ResolveObject` fabricated a placeholder `Target` (with a caveat string baked into its `Name`) on every successful response instead of erroring; it now returns `ErrNotImplemented`, since Horizons' free-text result format has no stable, verified schema to parse (its table-header wording has been observed to differ across responses)
- `ephemeris/jpl/provider.go`: `Provider.AddKernel` mutated `Kernels`/`Index`/`ByTarget`/`ByTargetCoverage` with no locking, so adding a kernel after construction while `State`/`FindSegment`/`SupportedBodies` ran concurrently could race; `Provider` now guards this state with a `sync.RWMutex`
- `plan/plan.go`: `moonSepCache` was a single-entry cache keyed by exact epoch, thrashing to a near-0% hit rate whenever concurrent lookups (e.g. `Rank` scoring several targets, each at its own epoch) touched more than one epoch at a time; replaced with a bounded 32-entry LRU
- `plan/visibility.go`, `plan/plan.go`: `TransitEstimate`'s coarse-scan buffer and `Rank`'s ranked-results slice grew via unsized `append` despite having a known upper bound; both are now pre-sized
- `plan/satellite.go`: `Satellite.ApparentMagnitudeCtx` fetched the Sun's position from `s.provider` — but a bare SGP4/TLE provider (the documented construction via `eph.NewProvider(eph.Satellites, ...)`) tracks exactly one body and ignores the requested ID, so it silently echoed the satellite's own state back for `eph.Sun` too. This made the Sun→Satellite vector always zero, so `ApparentMagnitudeCtx` failed with `errDegenerateGeometry` on every call for any satellite built the documented way. The Sun's position is now always sourced from `eph.Default()` (the analytic SOFA provider), independent of whatever provider tracks the satellite
- `ephemeris/satellite/satellite.go`: `Satellite.State` ignored its `id` argument entirely, silently answering for the tracked satellite regardless of what body was actually requested — the root cause that made the `ApparentMagnitudeCtx` bug above possible, and a hazard for any other caller that might query the wrong ID against a single-body provider. `State` now returns `ErrUnexpectedID` for any `id` other than the documented `core.ID(0)`
- `catalog/mast`: `ConeSearch` was a no-op stub returning an empty-but-successful result despite the provider advertising `resolve.CapConeSearch`; now returns an explicit `ErrNotImplemented` instead of silently claiming "found nothing"
- `unit/quantity.go`: `Quantity.Equals` compared via a strict `math.Abs(v1-v2) < 1e-15*max(|v1|,|v2|)`, whose tolerance is exactly 0 when both values are 0 — so two physically-equal zero quantities in different (but compatible) units, e.g. `0m` vs `0km`, compared unequal. An exact-match check now short-circuits before the relative-tolerance comparison
- `angle/angle.go`: `DMSString`/`HMSString`'s 60-second carry correction only ran for `precision >= 0`, but the digit-writing branch rounds to a whole second for `precision <= 0` — so a negative `precision` could render an invalid sexagesimal string like `00'60"` instead of carrying to `01'00"`. The carry check now uses the same rounding rule as the digit-writing branch for every `precision <= 0`, not just `0`
- `catalog/gaia`, `catalog/sbdb`, `catalog/simbad`, `catalog/jpl`, `catalog/vizier`, `catalog/mast`: their `ConeSearch`/`ResolveObject` swallowed an `http.NewRequestWithContext` construction error into an empty-but-successful result instead of surfacing it; now returned as a wrapped error

### Documentation
- `ephemeris/jpl/spk/reader.go`: documented that `*Reader` is safe for concurrent use once constructed (previously true but unstated)
- `plan`: added regression tests confirming `ErrStepNotPositive`, `ErrStepTooLarge`, and `ErrFamilyNotImpl` are matchable via `errors.Is` from their public entry points (`ObservableWindows`, `VisibleIntervals`, `EventSolver.Find`) — these sentinels were declared and wrapped correctly but never verified reachable
- `catalog/jpl`, `catalog/vizier`: `doc.go` overstated current capability (claimed working name resolution / multi-catalog cone search) against what the code actually does post-fix (`ErrNotImplemented` / a single hardcoded 2MASS table); rewritten to match reality
- `plan`: added compile-time interface assertions (`var _ ConstraintCtx = ...`, `var _ Observable = ...`, etc.) for every built-in `Constraint`/`Observable`/`MovingBody`/`MagnitudeComputer`/`StaticMagnitude` implementer. Go's interface satisfaction is structural and silent — a method signature drift drops a type out of an interface with no compiler error (this is exactly how the `MoonSep.CheckCtx` bug fixed earlier this cycle happened) — these turn that regression class into a build failure instead of a runtime gap
- `vector/vector.go`: `DivScalar`'s doc comment claimed division by zero always produces "a NaN vector" — actually only true when the dividend is also zero; a nonzero component divided by zero is `±Inf`, not `NaN`. Doc corrected to describe the actual per-component behavior
- `unit/dimension.go`: documented that `Dimension.PowInt`'s `p` is silently truncated to `int8` range, matching `Dimension`'s own exponent field width
- `examples/17_equinox_prediction`: removed hardcoded `v0.1.3` version strings from doc comments and printed output (stale since the v0.1.3 release)
- `README.md`: the Quick Start and Satellite Tracking code samples had never been compiled — `ScoreObservable` was missing its `*coord.Context` argument, `ScheduledBlock.Start`/`.End` don't exist (it's `.Window.Start`/`.Window.End`), `satellite.NewFromGP` doesn't exist, `Satellite.PropagateECI`/`.SubSatellitePoint` are unexported, `SatellitePasses` was missing its `name` argument, and every printed `time.Time` used bare `%s` — which prints a raw Julian Date (`JD 2461147.37 (UTC)`) instead of a calendar date for any UTC-scale `Time`, since `Time.String()` only formats as a calendar string for a non-UTC location. Every example in the README is now copy-pasted from a program that was actually compiled, run, and its real output captured

### Tests
- `plan/phases_test.go` (new) — `MoonPhases`, `Seasons`, `Apsides`, `MoonIllumination`, `LunarEclipses`, `SolarEclipses` had zero coverage under default `go test ./...` (only exercised via `integration`-tagged USNO/NASA-eclipse/AstroPixels tests); now covered by fast, offline, deterministic unit tests
- `plan/moving_bodies_test.go`, `plan/satellite_test.go` (new) — `Asteroid`, `Comet`, `GenericBody`, and `Satellite` (constructors, `Position`, `GeocentricVec`, `GetDetails`, `ApparentMagnitude(Ctx)`, `LookAngle`, `SatellitePasses`) had zero coverage; now covered using a deterministic synthetic ephemeris provider and a real (offline) ISS TLE — the latter is what surfaced the `ApparentMagnitudeCtx` bug fixed above
- `plan/events_convenience_test.go` (new) — `Conjunctions`, `ConjunctionsEcliptic`, `Appulses`, `Oppositions`, `GreatestElongations`, `FullMoonOppositions`, `VisibilityEvents`, `NextNewMoon`, `NextFullMoon` (and the `EventFamilyIllumination` dispatch they exercise) had zero coverage; now covered against real planetary/lunar geometry
- `catalog/{simbad,gaia,jpl,mast,sbdb,vizier,fink}`, `ephemeris/jpl/validation`: every `network`-tagged test in the repo except `catalog/norad`'s lacked the documented reachability pre-check (TCP dial + `t.Skipf` on failure) — a transient external outage (this was caught live: SIMBAD timed out mid-run) would hard-fail the whole suite instead of skipping. All now follow the same pattern as `catalog/norad`'s existing `requireCelestrak`
- `magnitude/fink_test.go`: this file had **no build tag at all**, so its live network calls to the FINK/ZTF API ran under the default `go test ./...` — meaning CI's blocking `lint-and-test` (all 3 OSes) and `race-detection` jobs could fail on nothing but FINK API downtime (caught live: a 504 Gateway Timeout failed all three `TestFINK_*` tests in a single run). Tagged `//go:build integration`, matching `catalog/norad`'s established pattern for live-network tests actually wired into CI's non-blocking integration job; a 5xx response now also `t.Skipf`s instead of `t.Fatalf`s, since it signals external degradation rather than a bug in the request

### Removed
- `plan.EvalContext`, `NewEvalContext`, `NewEvalContextWith`, `plan.Slot`, `plan.Observation` — unused exported symbols with zero callers anywhere in the codebase
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

[Unreleased]: https://github.com/TuSKan/astrogo/compare/v0.6.0...HEAD
[0.6.0]: https://github.com/TuSKan/astrogo/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/TuSKan/astrogo/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/TuSKan/astrogo/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/TuSKan/astrogo/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/TuSKan/astrogo/compare/v0.1.5...v0.2.0
[0.1.5]: https://github.com/TuSKan/astrogo/releases/tag/v0.1.5
[0.1.4]: https://github.com/TuSKan/astrogo/releases/tag/v0.1.4
[0.1.3]: https://github.com/TuSKan/astrogo/releases/tag/v0.1.3
[0.1.2]: https://github.com/TuSKan/astrogo/releases/tag/v0.1.2
[0.1.1]: https://github.com/TuSKan/astrogo/releases/tag/v0.1.1
[0.1.0]: https://github.com/TuSKan/astrogo/releases/tag/v0.1.0
