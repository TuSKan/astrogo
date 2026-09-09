# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

`astrogo` (`github.com/TuSKan/astrogo`) is an observatory-grade astronomy and observation-planning library for Go. Correctness, numerical reproducibility, and public-API stability rank above cosmetic style changes.

## Commands

```bash
# Quick local checks
go test ./...                       # all packages, default (unit) tests
go test ./coord/...                 # single package
go test -run TestApparentToObserved ./coord/...  # single test by name
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

## Fuzzing

Every parser that eats bytes from someone else's server or a user's disk is fuzzed. The property under test is "never panics or hangs on attacker-influenceable input," not correctness (that's covered by ordinary tests elsewhere). Seed corpora are `f.Add(...)` literals only — no checked-in binary fixtures — so they run as ordinary tests under `go test ./...` and are part of every CI run for free. Extended fuzzing beyond the seed corpus is a manual, periodic step, not a CI gate — run it when touching a parser.

Six packages carry targets:

| package | targets | what the bytes are |
| --- | --- | --- |
| `ephemeris/jpl/spk` | `FuzzNewReaderReadSummaries`, `FuzzEvaluateSegment`, `FuzzReadDoubles` | downloaded SPK kernels |
| `fits` | `FuzzParseCard`, `FuzzReadHeader`, `FuzzRead` | **arbitrary user files** — the only place astrogo opens one |
| `ephemeris/satellite` | `FuzzValidateTLE`, `FuzzNewFromTLE` | CelesTrak or user-supplied element sets |
| `catalog/norad` | `FuzzGPToTLE` | CelesTrak JSON, through `ToTLE` into SGP4 |
| `internal/votable` | `FuzzRead` | SIMBAD / VizieR / Gaia / MAST TAP responses |
| `time/internal/iers` | `FuzzParseFinals2000A` | the IERS bulletin |

**Always pass `-fuzzminimizetime`.** Its default is 60s *per interesting input*, and minimization executions are not counted in `execs`, so a target with expensive executions appears to freeze at a constant count while the engine is in fact busy. This cost #140 a long investigation; do not omit it.

How much it decides, measured on `fits`'s `FuzzReadHeader` with a fresh corpus and the same 60s budget each time:

| `-fuzzminimizetime` | executions | new interesting |
| ---: | ---: | ---: |
| 1s | 1,794,997 | 258 |
| 5s | 22,491 | 106 |
| 15s | 11,106 | 33 |

**160x, from one flag, on one target.** Which is also the reason not to read `execs` as a throughput figure: it is executions *minus* whatever minimization consumed, and minimization cost is set by that flag times how often the target finds something interesting.

```bash
go test -run=^$ -fuzz=FuzzNewReaderReadSummaries -fuzztime=60s -fuzzminimizetime=5s ./ephemeris/jpl/spk/
go test -run=^$ -fuzz=FuzzReadHeader            -fuzztime=60s -fuzzminimizetime=5s ./fits/
go test -run=^$ -fuzz=FuzzValidateTLE           -fuzztime=60s -fuzzminimizetime=5s ./ephemeris/satellite/
```

Executions vary by three orders of magnitude across these targets. Corpus-controlled — a fresh `-test.fuzzcachedir` per run, so each starts from the seed corpus only — at `-fuzztime=60s -fuzzminimizetime=5s`, two runs each:

| target | executions | spread |
| --- | ---: | ---: |
| `internal/votable` `FuzzRead` | 21.7M – 24.0M | 10% |
| `ephemeris/satellite` `FuzzValidateTLE` | 4.68M – 4.70M | 0.5% |
| `catalog/norad` `FuzzGPToTLE` | 4.22M – 4.26M | 0.9% |
| `fits` `FuzzParseCard` | 3.38M – 3.67M | 8% |
| `ephemeris/jpl/spk` `FuzzNewReaderReadSummaries` | 1.82M – 2.39M | 27% |
| `time/internal/iers` `FuzzParseFinals2000A` | 1.27M – 1.76M | 33% |
| `fits` `FuzzRead` | 48k – 54k | 11% |
| `fits` `FuzzReadHeader` | 22k – 35k | 43% |

Two things follow, and both matter more than the numbers.

**Never compare two runs that shared a corpus.** `go test -fuzz` carries interesting inputs forward in the build cache, so a second run starts deeper and spends its budget differently. Measured while investigating #199, the same command minutes apart gave 163,582 and 967,292 executions with no code change between them — a six-fold difference from corpus state alone. Use `-test.fuzzcachedir` pointed at a fresh directory (it goes *after* the package path).

**A single run is not a measurement.** Even corpus-controlled, the run-to-run spread reaches 43%. Read these as an order of magnitude, not a benchmark.

The low `fits` figures are minimization, not a slow parser: those two targets find ~110 interesting inputs from a fresh corpus and pay up to 5s minimizing each. `fits`'s `FuzzRead` measures ~800 exec/s here, against the "~4" this table claimed before it was measured this way — that figure was the default 60s minimize time, not the parser.

Any crasher Go writes to `testdata/fuzz/` should be committed — that's the one place a small binary fixture is legitimate here, since it's a regression test for a bug the fuzzer found, not a data source. Two are checked in: an out-of-range TLE epoch day that panicked inside the SGP4 backend, and a VOTable with rows but no field declarations.

## Performance

Three layers, and only one of them can fail a build. That split is deliberate.

**Allocation contracts are the gate** (`coord/allocs_test.go`, `time/allocs_test.go`, `atmosphere/allocs_test.go`). Plain `testing.AllocsPerRun` assertions on paths where zero-allocation is a design claim: a cached `Context` transform, an EOP-free scale conversion, a refraction evaluation. They assert exact `0` where zero is the claim and a bound where a bound is the honest statement — `NewContext` allocates 1 and is contracted at ≤ 8, because the risk is not that it costs one allocation but that it quietly starts costing many.

They are ordinary untagged tests, so they run in every CI job and block a merge. `allocs/op` is what makes that safe: it is deterministic across machines and — measured, not assumed — **unchanged under `-race`**, so no build tag and no short-mode skip are needed. `ns/op` on a shared runner is not, which is why nothing here gates on time.

Add a contract when you can state *why* a path must not allocate, not merely that it currently doesn't.

**The Benchmarks CI job reports, and never fails.** It runs on push to main and posts a `benchstat` delta against the previous successful run into the job summary. `-count=6`, not 3: with three samples benchstat cannot compute a confidence interval and prints `± ∞` for every row.

**Profiling is a manual, periodic step, not a CI gate** — the same rule as extended fuzzing above, for the same reason. A flame graph cannot be usefully diffed by a machine, and the finding that motivated all of this came from *reading* `pprof -top` and noticing that `io.ReadAll` had no business appearing in a scheduler. What automates is the scalar assertion you write afterwards, once you know what the number should be.

```bash
go test -run=^$ -bench=. -benchmem -cpuprofile=cpu.out -memprofile=mem.out -o pkg.test ./plan/
go tool pprof -top -nodecount=15 pkg.test cpu.out
go tool pprof -sample_index=alloc_space -top -nodecount=15 pkg.test mem.out
```

**Two ways to read a benchmark wrong, both of which happened here.** A low `-benchtime` charges a one-time lazy load to every iteration: at `-benchtime=1x`, `BenchmarkUTCToUT1` reported 8.6 ms and 15.6 MB for a conversion that actually costs 67 ns and allocates nothing — that was the EOP file loading once. The same illusion appears on any path that touches the network once. Before believing a per-op cost, raise the iteration count and check the *total* stays proportional; if total time is flat while ns/op falls, it is a fixed cost, not a per-call one.

And a benchmark's epoch is part of its subject. `plan`'s scheduler benchmarks ran at `time.ZeroTime()` — JD 0, 4713 BC — where no Earth Orientation Parameters exist, so 99.88% of their allocated bytes were EOP handling rather than scheduling (#246). Benchmark a realistic epoch, and a fixed one: a result that depends on the day it was run cannot be compared with the one before it.

## Embedded data

There is no `go:generate` step in this codebase — it was removed deliberately. No package uses `go:embed` either — every data source is obtained at runtime through `remote.GetFile`, on a lazy, on-first-query load: `catalog/openngc`'s two CSVs (see [catalog/openngc/openngc.go](catalog/openngc/openngc.go)) and `time`'s Earth Orientation Parameters (see [time/eop.go](time/eop.go) and the unexported `time/internal/iers`). A constructor performs no I/O; the first query that needs the data fetches it under that caller's context.

Never reintroduce a `go:generate`/download-tooling step, and never add `go:embed` to a new catalog provider — fetch through `remote` instead (see the caching primitives below).

## Network access & `remote`

`remote` is astrogo's I/O boundary and owns *policy*: the endpoint registry, the consent gate, and the cache. Moving bytes belongs to its two subpackages, split by **what is being addressed, not by protocol** — *a file on http is a file with an http backend, not an API*:

- **`remote/file`** — byte-addressable resources with a stable identity, size and range semantics (SPK kernels, IERS bulletins, catalog CSVs, GeoTIFF bundles), on `gocloud.dev/blob`.
- **`remote/api`** — request/response services whose returned document depends on the query (SIMBAD, VizieR, Gaia, MAST, CelesTrak, FINK, JPL SBDB/Horizons), on `resty.dev/v3`.

Every endpoint is registered in [remote/endpoint.go](remote/endpoint.go) as an `Endpoint`: `URL`, `Kind` (`KindAPI`|`KindFile`), `Subsystem`, `Timeout`/`DownloadTimeout`, `Mutable`, `Downloadable`. That struct is the single source of truth for how a package talks to a service, and `remote.URL(id)` is the single gate every call site passes through (offline mode, `Disable`, `SetURL`).

- **No scheme is ever hardcoded in `remote` or `remote/file`.** `gocloud.dev/blob` is already a scheme-dispatched registry populated by blank imports; do not build a second one (no `Backend`/`Source`/`Transport` interface, no `if scheme == …`, no per-scheme branch in the fetch path). An `Endpoint.URL` is the **exact string** handed to `blob.OpenBucket`. Core `remote/file` blank-imports `fileblob` (`file://`) and `gocloud-ext`'s `httpblob` (`http://`, `https://`); every other scheme is an opt-in subpackage that is *nothing but* a blank import — see [remote/s3](remote/s3/doc.go), four lines with zero exported symbols. Adding SFTP or GCS means adding one such package and editing nothing in `remote`.
- **Scheme-specific connection detail rides in the URL's query string, never in Go.** `s3blob`'s URL opener already parses `region`, `endpoint`, `hostname_immutable`, `use_path_style`; `remote.CopernicusEODATA`'s URL carries all four, so reaching a non-AWS S3 service needs no astrogo configuration API and no AWS SDK type in any signature. `blob.OpenBucket` also handles two portable wrappers on every scheme at once: `?prefix=sub/dir/` scopes a bucket, and `?key=exact/object.dat` serves one object under any name — the supported way to point an endpoint at a single file, since a bare single-object URL leaves no room for the caller's `name`.
- **Nothing is assumed local.** There is no `LocalURL`/`OSPath`/`SetDataDirPath`, and **no API anywhere takes a `path string` meaning an OS filesystem path**. Every signature is a `*file.Bucket` + key, or a bucket URL. Bucket keys are `/`-separated — use `path.Join`, never `filepath.Join`. Nothing needs `os.MkdirAll`: a bucket "directory" is only a key prefix. The single OS-path contact point in the module is the unexported default-cache-dir resolver in [remote/storage.go](remote/storage.go), which turns `os.UserCacheDir()` into a `file://` URL; `ASTROGO_CACHE_DIR` holds a URL, not a path. The deliberate `os.*` exceptions are `fits.Open` (an arbitrary user-supplied FITS file, not astrogo-managed data) and the `os.CreateTemp` staging in `catalog/fink` and `atmosphere/dataset/cams`, where a third-party decoder demands a real OS path — that staging is the consumer's problem at its own call site, never pushed back into `remote`'s signatures.
- **Bulk file downloads never happen without explicit consent.** A missing kernel makes `jpl.NewProvider`/`eph.NewProvider` (both take `ctx` first) fail with an actionable `remote.ErrDownloadDenied` unless the caller granted `remote.EnableDownloads(maxSize, ids...)` — with no ids it covers every `Downloadable` endpoint. Consent is checked twice per fetch: against the registered `ApproxSize` before any request, then against the size the source actually reports. Never add an implicit download; route it through `remote.GetFile`. The one deliberate *automatic* (not unconsented) behaviour is `time`'s EOP lazy load: `Time.EOP()`/`.UTC()`/`.UT1()` trigger `iers.EnsureLoaded`, which reads a pre-seeded cache object unconditionally but still gates its *network* step behind the same consent check.
- **`JPLHorizons` and `JPLHorizonsSPK` are two endpoints over one URL.** Name resolution (`catalog/jpl`) needs no consent; kernel generation (`ephemeris/jpl/spk.CacheAPI`) returns a whole SPK base64-encoded in a JSON body and is `Downloadable`. Splitting them is what lets `EnableDownloads` gate exactly the traffic it should.
- **All API access goes through `remote/api.NewClient(id, opts...)`** — never a raw `http.Client`. It defaults the timeout to the endpoint's registered `Timeout` (`api.DefaultTimeout` if zero). `Get`/`GetJSON` for GETs, `PostForm`/`PostJSON` for POSTs; all return `io.ReadCloser` or decode directly, never `*http.Response`, since a non-2xx becomes an `*api.HTTPError` before a caller sees a body. **No resty type escapes the package** and `Client` has no exported fields — tests redirect an endpoint with `httptest.NewServer` + `remote.SetURL` rather than injecting a transport. Retries use resty's own `RetryConditionStatus*` predicates (429, 5xx except 501, status 0); note that resty's `SetRetryDefaultConditions` covers only transport/header/URL errors, so status retrying **must** be added explicitly.
- **Reuse `remote.GetFile` instead of hand-rolling "ensure cached, then read/download".** `GetFile(ctx, id, name, opts...) (bucket *file.Bucket, key string, err error)` resolves source and cache through the same opener, reuses a cached object on existence alone for `Mutable: false` endpoints (JPL kernels) or after a recorded-source-ETag check for `Mutable: true` ones (IERS, OpenNGC), and on a miss downloads under a cross-process `IfNotExist` lock. It returns bucket+key, not a stream — the caller reads it however it needs. `WithCacheName` sets the cache key when it differs from the source name (IERS serves `finals2000A.all`, cached as `finals2000A.data`); `WithValidate(func(io.Reader) error)` runs against the **staged** object before promotion, so a corrupt fetch is never cached and a multi-GB kernel never has to fit in memory. `file.Save` is the generic write primitive for content that doesn't arrive via `GetFile` (a decoded API payload, a checksum sidecar).
- **`Endpoint.URL` is a directory-style prefix for every `KindFile` endpoint** — the bucket root the caller's `name` resolves within, never one exact resource. A single-resource URL cannot resolve a name and fails; `TestKindFileEndpointsAreDirectoryPrefixes` enforces this for the registry.
- **Random access uses `file.NewReaderAt(ctx, bucket, key)`** — an `io.ReaderAt` that reads aligned chunks and keeps a bounded LRU of them (64 KiB × 16 = **1 MiB resident for any object**, a 3 GB kernel included; it does not buffer the object). This is generic across backends, with no `*os.File` reach-through and no local-only fast path. It exists because a naive `NewRangeReader`-per-`ReadAt` costs one `os.Open` per call under `fileblob` and one HTTP request per call over http/S3 — measured at 263 ms vs 0.33 ms for 2000 SPK-shaped reads (`BenchmarkReadAtStrategies`). Do not "simplify" it back to a plain wrapper without re-running that benchmark.
- **A `KindFile` endpoint serving a small fixed manifest lists it in `Endpoint.Files`** — see `remote.OpenNGC`'s two CSVs. A consuming package reads `remote.Lookup(id).Files` rather than hardcoding the list. JPL kernels leave it nil since the caller names the kernel.

See the README's "Data downloads & offline usage" for the user-facing picture (sizes, `remote.SetOffline`, pre-seeding IERS EOP data).

## Architecture

Strictly layered, unidirectional imports (no cycles). Lower layers never import higher ones:

```
plan, catalog, fits/plan                                ← orchestration (observability, scheduling, events, resolvers, FITS↔plan bridge)
ephemeris, coord, atmosphere, fits, skybrightness        ← scientific engines
skybrightness/dataset/...                                ← sky-brightness IO tier (dataset providers; the only tier allowed I/O)
time, angle, vector, unit, constants, remote, optics, logging  ← primitives
```

- **`time`** is the sole gateway for Earth Orientation Parameters and epoch arithmetic. `time/internal/iers` (unexported — nothing outside `time/` can import it) fetches/parses IERS EOP data; `time` re-exports what's needed (`time.EOP`, `time.RegisterModel`/`GetModel`/`Coverage`/`SetRetryCooldown`) and adds `Time.EOP()`, `Time.MJD()`, `Time.GAST()`, `Time.JulianEpochYear()`, `Time.DayOfYear()`. EOP data loads automatically and lazily the first time `Time.EOP()`/`Time.UTC()`/`Time.UT1()` needs it — a pre-seeded on-disk cache file, then (if `remote.EnableDownloads(remote.IERSFinals2000A, ...)` was called) a network fetch, then a zero-EOP-plus-one-time-warning degradation — no explicit populate call needed. `coord` and every other package get EOP/epoch values through these `time` APIs — never by hand-rolling MJD/GAST arithmetic or importing EOP internals directly.
- **`coord`** is the transform core. `coord.Context` (in [coord/context.go](coord/context.go)) caches the expensive SOFA `Apco13` matrix computation (~91 µs) once per epoch so each subsequent transform is ~325 ns. **Hot paths must create one `Context` per epoch and reuse it** — never one per transform. The scheduler shares a single `Context` per time step across constraints via the `ConstraintCtx` interface; built-in `Altitude`/`Airmass` implement it.
- **`ephemeris`** provides Sun/Moon/planet positions (SOFA + JPL SPK). `ephemeris/jpl` is the multi-kernel SPK provider with on-demand Horizons fetching (`Provider.AddKernel`/`State`/`FindSegment`/`SupportedBodies` are guarded by an internal `sync.RWMutex` — safe to call concurrently); `ephemeris/satellite` is SGP4 (TEME→GCRS, ground track, look angles).
- **`plan`** is the planning/event engine: `Observable` targets, `Constraint`s, the Chandrupatla/Brent `Solver` (rise/set/transit, phases, seasons, eclipses, conjunctions), and the `Strategy`-based scheduler (`Greedy`/`Priority`/`SwapOptimized`). `plan` has no dependency on `fits` or Apache Arrow — the FITS↔plan bridge (`SiteFromFITS`/`TargetFromFITS`) lives in the separate `fits/plan` package.
- **`skybrightness`** predicts ground-observed spectral sky radiance (`L_λ(λ, direction, observer, time, atmosphere)`, W·m⁻²·sr⁻¹·nm⁻¹) and owns **only radiance transport**: `Scene`, `Component`, `Model`/`Query`/`Estimate`, uncertainty, quality, provenance, and all-sky operations (`Zenith`/`Direction`/`SkyMap`/`IntegratedHemisphere`/`HorizontalIlluminance`). **Nothing else is segmented into it.** Atmospheric physics (Rayleigh, aerosol, molecular absorption, transmission, vertical profiles, spherical airmass, cloud optical properties) belongs to `atmosphere`; passbands, AB/Vega/ST systems and `SurfaceBrightness` belong to `magnitude`; `Throughput`/`Instrument`/`PhotonRate`/`BackgroundRate` belong to `optics`; `SpectralGrid` and the spectral quantity types belong to `unit`. A capability that fits an existing package is added there, never duplicated here. Spectral radiance is the internal quantity and stays spectral until projection — `mag/arcsec²`, an SQM reading, luminance, a photon rate and an electron rate are all projections of the *same* stored spectrum, because a model can reproduce a correct V magnitude with a wrong spectrum and every instrument projection would then be wrong. Components sum in **linear radiance space**; summing magnitudes is a correctness bug. **Never fabricate a coefficient**: a component whose primary literature cannot be obtained is not implemented, and is recorded in `docs/skybrightness.md` §16 instead. Prohibited in production: a constant dark sky, KS91 as the production Moon, Bortle class as physics, VIIRS radiance read as sky brightness, geographic averaging as propagation, a universal cloud multiplier, a single extinction coefficient. Evaluation performs **no I/O and no network access** — datasets are resolved by a provider layer under `skybrightness/dataset/...` and handed in via `Scene`; this is enforced behaviourally by `TestEstimateWorksOffline` (identical output under `remote.SetOffline(true)`) and structurally by `importgraph_test.go`'s direct-import check. A *transitive* ban would be wrong rather than stricter, since `coord`/`ephemeris`/`time` legitimately reach `remote` for EOP and JPL kernels. Eight `Component` constructors are implemented over seven `ComponentID`s — `ArtificialSkyglow` and `CloudySkyglow` share `Artificial` and `NewModel` refuses a model holding both, since they compute the same contribution by different solutions — and together they are the whole sky this module models: `ScatteredMoonlight` (ROLO reflectance + `atmosphere.SingleScatteredRadiance`), validated end-to-end at 18.9 mag/arcsec² in V for a near-full Moon against an independently-known ~18; `ArtificialSkyglow` (Kocifaj 2022 over `GroundEmitter`), tested on the model's physical claims since validating it needs a real source inventory; `CloudySkyglow` (Kocifaj 2007 Eq. 27 with the 2025 extension), which resolves the vertical so a cloud deck can reflect — measured, an overcast deck amplifies the zenith 88× over a city and *screens* at 0.80× 60 km away; and the natural sky as `DiffuseGalacticLight` (Kawara 2017), `ZodiacalLight` (Leinert 1998), `ExtragalacticBackground`, `Airglow` (van Rhijn over a caller-supplied zenith spectrum) and `IntegratedStarlight` (a tabulated map, directly attenuated). `fullsky_test.go` runs six of them against one scene and is the proof they compose: a moonless night at Paranal comes out near 21.5 mag/arcsec². A component that takes a tabulated quantity per direction also takes the passband that quantity is averaged over — normalising a spectral shape by the sum of its samples instead ties the answer to the grid spacing, which stays positive and plausible while being wrong. Two things a caller must supply rather than have guessed: the airglow zenith spectrum and the starlight spectral shape, because integrated starlight is the summed light of stars of every type and no single blackbody is right. `Component.AddRadiance` returns `(Flag, error)` because the same model is an interpolation in one geometry and an extrapolation in another, and `Model.Estimate` ORs those flags into the estimate's `Quality`. See `docs/skybrightness.md` for the full design: scientific baseline with per-model primary references, the equation→function→test maps, validation strategy, phase roadmap, unresolved dependencies and open scientific questions.
- **`catalog`** + `catalog/resolve` expose unified `resolve.Provider` interfaces over SIMBAD/MAST/Gaia/VizieR/JPL/SBDB/OpenNGC/NORAD/FINK, with Apache Arrow columnar caching. All network access goes through `remote/api.Client`.
- **`internal/gofaext`** wraps [github.com/hebl/gofa](https://github.com/hebl/gofa) (SOFA-derived algorithms). All low-level SOFA calls go through here to keep public APIs clean and the backend swappable.
- **`internal/testutil`** holds float/error test helpers used across packages.
- **`logging`** holds the one `*slog.Logger` astrogo writes to, and imports nothing from astrogo so every layer can reach it — including `time`, which `remote` depends on and which therefore cannot depend on `remote`. `logging.Set(nil)` restores a default that passes `Warn` and above and drops `Info`: progress lines (a kernel downloading) are a library's business only if asked for, while the EOP-unavailable warning is the only notice a caller gets that accuracy silently degraded, since `Time.EOP` has no error return. No astrogo API takes a logger parameter, and no library file imports the standard `log` — `TestNoLibraryCodeWritesToTheGlobalLog` enforces the latter.
- **`optics`** is pure equipment-optics arithmetic (magnification, field of view, exit pupil, resolving power, pixel scale) for a `Telescope`/`Eyepiece`/`Sensor` combination — no astrometry, no ephemeris, no network access. A top-level primitive, not a `plan` subpackage.

## Conventions for this codebase

- **Git workflow**:
  - Never commit on `main` — for any code change, create a new branch first.
  - On a branch you created, you have write access: `git add`/`commit`/`push` freely, no need to ask each time.
  - Opening a pull request needs an explicit instruction — if the user hasn't clearly asked for one, ask before creating it.
  - Merging (a PR, or into `main`) only happens on explicit instruction — never merge on your own initiative.
  - Read-only `git status`/`diff`/`log` are always fine, including on `main`.
  - A commit fixing a filed issue must use a real GitHub closing keyword (`Closes #NN.`/`Fixes #NN.`) on its own line in the commit body — a bare `(#NN)` in the title (the CHANGELOG citation style) is not a closing keyword and will not auto-close the issue on merge. In a PR body/description referencing multiple issues, repeat the keyword per issue (`Closes #20, closes #21, ...`) — GitHub does not chain one keyword across a comma-separated list.
- **Named returns are intentional** for astronomical quantities (`ra`, `dec`, `jd`, `az`, `alt`, `dist`); short domain variable names (`r`, `t`, `jd`, `tt`, `ut1`) are idiomatic here. `nonamedreturns`/`varnamelen` are disabled deliberately.
- **"Magic numbers" are physical constants, coefficients, and NAIF IDs** — `mnd`/`goconst` are off. Do not abstract constants out of published formulas; keep algorithms readable against their reference paper/SOFA routine/Horizons fixture rather than splitting into many helpers.
- **Errors**: prefer static sentinels wrapped with `%w` over dynamic `fmt.Errorf` strings. No hidden global mutation or `init()` side effects.
- **`//nolint` only when locally scoped with a documented reason.** Do not downgrade `.golangci.yml` or remove linters to pass CI.
- **Cross-platform**: tests must pass on Linux, macOS (ARM64), and Windows. Use tolerance-based float comparisons (account for FMA/atan2 rounding); prefer inequality bounds near precision limits; document any tolerance you relax.
- **Tests**: add a regression test for bug fixes; prefer known reference values, explicit tolerances, deterministic fixtures, and edge cases (poles, horizon, angle wrap 0→360, epoch boundaries, circumpolar/never-rising targets).
- **CHANGELOG.md entries** (forward-only — never rewrite already-released entries): 1–3 lines, ending with a `[#NN]` PR link. Deep forensic detail (root cause, before/after numbers, live-test confirmation) belongs in the PR description or the code's own doc comment, not the changelog — the changelog is an index, not the record. Use the full [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) vocabulary the file header already claims adherence to: `### Added`, `### Changed`, `### Deprecated`, `### Removed`, `### Fixed`, `### Security` — not just `Added`/`Fixed`. Keep the established `### Changed — BREAKING` variant for public API breaks.
- **Deprecating a public symbol**: mark it with a trailing `// Deprecated: <use X instead>.` doc-comment paragraph (the form `staticcheck`/`gopls`/pkg.go.dev recognize). Pre-1.0, a deprecated symbol must survive at least 2 minor releases before removal; post-1.0, it survives the entire major cycle. A mark lands as a `### Deprecated` CHANGELOG entry in that release; a removal lands as `### Removed`. Enforcement is already automatic: `.golangci.yml`'s `default: all` runs `staticcheck`'s `SA1019`, so deprecating a symbol requires migrating every internal caller in the same PR (or a locally-scoped, documented `//nolint:staticcheck`).
