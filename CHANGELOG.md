# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed — BREAKING
- **`skybrightness` is rebuilt from first principles as a spectral all-sky radiance engine, with no backward compatibility.** The previous package had approximately the right architecture and almost none of the physics: what it shipped was a constant airglow, a Krisciunas & Schaefer 1991 moonlight fit, a Rayleigh-only transmission, a Bortle table and a Schaefer limiting-magnitude conversion. Those are exactly the approximations the new design prohibits in production. The new core owns **only radiance transport** — `Scene`, `Component`, `Model`/`Query`/`Estimate`, `Fidelity`, uncertainty, quality, provenance, and all-sky operations (`Zenith`/`Direction`/`SkyMap`/`IntegratedHemisphere`/`HorizontalIlluminance`). See [`docs/skybrightness.md`](docs/skybrightness.md).
- **Nothing is segmented into `skybrightness` that belongs elsewhere.** Atmospheric physics goes to `atmosphere`, passbands and magnitude systems to `magnitude`, instrument throughput and detector rates to `optics`, the spectral grid and quantity types to `unit`. A capability that fits an existing package is added there rather than duplicated. One consequence worth noting: Winkler (2022) and Kocifaj et al. (2022) independently adopt the same Henyey-Greenstein aerosol phase function, so that becomes a single implementation in `atmosphere` shared by the Moon and artificial components instead of two that can silently diverge.
- **Removed with the old package**: `plan.LimitingMagnitudeConstraint`, `plan.ScoreObservableSky`, `examples/18_sky_brightness`, `examples/21_meteor_shower_forecast`, `skybrightness/natural`, `skybrightness/atmos`, `skybrightness/dataset/passband`, `skybrightness.SchaeferNELM`, `skybrightness.Bortle*`, `skybrightness.Mode*` (replaced by `Fidelity`). They return once Phases 2-3 make a defensible limiting magnitude possible.
- `plan.MeteorShower.ObservedRate` takes a naked-eye limiting magnitude (`float64`) instead of a `LimitingMagnitudeConstraint`, decoupling meteor-rate arithmetic from sky brightness entirely. The limiting magnitude is the physical input the IMO formula actually needs; where it comes from is the caller's choice.
- **`remote` is rebuilt as a policy layer over two subpackages, split by what is addressed rather than by protocol** — a file on http is a file with an http backend, not an API. `remote/file` (bucket/key resources on `gocloud.dev/blob`) and the new `remote/api` (request/response services on `resty.dev/v3`) replace the previous mixed surface. `remote` itself keeps the endpoint registry, the consent gate and the cache, and contains no scheme literal, no HTTP client and no OS path.
- **No API in the module takes an OS filesystem path any more.** `remote/file.LocalURL`/`.OSPath` and `remote.SetDataDirPath` are removed; `ASTROGO_CACHE_DIR` and `remote.SetDataDir` take a bucket URL (`file://…`, `s3://…`). `ephemeris/jpl.Open(lskPath, spkPaths…)` becomes `Open(ctx, bucket, lskKey, spkKeys…)`, `Provider.AddKernelFile(path)` becomes `AddKernelFrom(ctx, bucket, key)`, `jpl.WithDataDir`/`Provider.DataDir`/`ephemeris.WithDataDir` are removed (`remote.SetDataDir` is the single control), `KernelInfo.Path` becomes `.Key`, `spk.CacheAPI(…, path)` becomes `CacheAPI(ctx, bucket, prefix, kernel, start, end)`, and `passband.OpenBundle(dir)` becomes `OpenBundle(ctx, bucket, prefix)`.
- **`remote.Client`/`NewClientFor`/`WithHTTPClient`/`RetryPolicy`/`DefaultRetryPolicy`/`Client.Do` are removed**, replaced by `remote/api.NewClient(id, opts...)` with the same four methods (`Get`/`GetJSON`/`PostForm`/`PostJSON`) and `api.WithTimeout`/`WithRetries`/`WithUserAgent`. `remote.HTTPError` moves to `api.HTTPError`. The client is opaque — no exported fields, and no resty type in any signature — so tests redirect an endpoint via `httptest.NewServer` + `remote.SetURL` instead of injecting a transport.
- **Download consent collapses to two variadic functions**: `remote.EnableDownloads(maxSize, ids...)` and `remote.DisableDownloads(ids...)`, where an empty list means every `Downloadable` endpoint. `EnableAllDownloads`/`DisableAllDownloads` are removed and the argument order of `EnableDownloads` is reversed.
- **`remote.JPLHorizons` is split into `JPLHorizons` (name resolution, no consent gate) and `JPLHorizonsSPK` (kernel generation, `Downloadable`)** over the same URL, so `EnableDownloads` gates exactly the traffic that moves a kernel and leaves `catalog/jpl` name lookups ungated.
- **`remote/s3` exports nothing.** `Register`/`WithRegion`/`WithServiceURL`/`WithLegacyList`/`ErrNotS3Endpoint` are removed; the package is now a doc comment plus one blank import. Every S3 connection detail (`region`, `endpoint`, `hostname_immutable`, `use_path_style`) rides in `remote.CopernicusEODATA`'s URL query, which `s3blob`'s own URL opener parses — so no AWS SDK type crosses a package boundary and a build that never touches S3 links none of it (verified: `go list -deps ./catalog/simbad` reports zero `aws-sdk-go-v2` packages).
- **`remote.WithValidate` takes `func(io.Reader) error`** instead of `func([]byte) error`, and runs against the staged object before promotion. Validation is now structural rather than an option that also forced a multi-GB kernel into memory: nothing partial or unvalidated is ever visible at a cache key.
- `time.ErrEOPHTTPStatus` (and `iers.ErrEOPHTTPStatus`) is removed. The EOP fetch goes through `remote.GetFile`, which reports blob errors rather than HTTP statuses, so the sentinel could never match again — leaving it would have been a silent trap rather than compatibility.
- **Sky Brightness V2**: `skybrightness` is rewritten from scratch as a spectral, all-sky, observatory-grade sky-radiance engine (`L_λ(λ, altitude, azimuth, site, epoch)`, W·m⁻²·sr⁻¹·nm⁻¹), replacing the V-band-only model with no backward compatibility. Deleted outright: `Model`, `Component` (old shape), `SurfaceBrightnessV`, `Nanolambert`, `SQMProvider`, `Floor`, `CompositeModel`, `RadianceToArtificialSB`, and the whole `skybrightness/atlas` package (`atlas.Resolver`/`FloorAt`/`Layer*`, `EnsureWorldAtlas`/`EnsureVIIRSAnnual`). See [`docs/skybrightness.md`](docs/skybrightness.md) §16 for the full symbol-by-symbol migration table and §15 for why this is a rewrite, not an extension.
- New core API: `skybrightness.Engine`/`Component`/`Request`/`Result`, canonical spectral types (`SpectralRadiance`, `WavelengthNM`, `SurfaceBrightnessAB`/`Vega`, ...) now living in `unit` as zero-cost named types (`unit.SpectralRadiance` etc.), passband integration (`IntegrateRadiance`, `ABSurfaceBrightness`, `VegaSurfaceBrightness`, `PhotopicLuminance`, `HorizontalIrradiance`), linearized uncertainty (`UncertaintyResult`), and deterministic `Provenance` (`Provenance.Digest()`, `Provenance.String()` for a human-readable summary — no `encoding/json` needed for the common case).
- `atmosphere` gains `Atmosphere`/`Builder`/`NewBuilder()`/`Aerosol`/`CloudLayer`/`SurfaceOptical`/`HorizonProfile`/`StandardDefault` and the general data-provenance primitives `SourceRef`/`Fidelity`/`TimeRange`/`DatasetVersion` — the atmospheric state a `skybrightness.Request` evaluates under, reusable by any future atmosphere-aware constraint (weather, seeing), not sky-brightness-specific. `skybrightness` aliases the provenance primitives for its own short in-package names but references `atmosphere.Atmosphere` directly, matching how `coord.Context` is already used. **`atmosphere.Atmosphere`/`atmosphere.Refraction` swapped names** in the same round: the pre-existing, shipped v0.14.0 refraction-input struct (`Model`/`Pressure`/`Temperature`/`Humidity`/`Wavelength`) is renamed `atmosphere.Refraction` (`atmosphere.StandardAtmosphere`→`StandardRefraction`; `coord.Context.Atmosphere()`/`plan.Site.Atmosphere()`→`.Refraction()`), freeing `atmosphere.Atmosphere` for the rich type above, which now composes an embedded `Refraction` as its own surface-conditions field. A deliberate, same-release hard break with no deprecation alias — Go cannot alias one identifier to two meanings at once, so freeing the name necessarily retired the old one immediately; see `atmosphere/doc.go`.
- `constants` gains `ToPhoton`/`ToEnergy`/`ArcsecondSquaredToSteradian`, the photon-flux/energy-flux spectral-radiance conversions (need `constants.SI2019`, which `unit` cannot import) — reusable beyond `skybrightness`.
- New `skybrightness/natural` package: the fast, simplified models `ConstantAirglow`/`VBandMoonlight`/`skybrightness.SchaeferNELM`/`NewFastEngine` — new types re-implementing the prior V-band physics (Krisciunas & Schaefer 1991, Schaefer 1990) against the new spectral API for a zero-setup, fully-offline engine, named for what's scientifically distinct about each (a constant floor; a broadband V-band fit, not spectral) rather than for vintage or citation, since a future Phase 2 spectral moonlight model (e.g. Jones et al. 2013) is a structurally different algorithm, not a replacement.
- New `skybrightness/atmos` package: `RayleighOnly`, an analytic Rayleigh-scattering-only transmission model (Hansen & Travis 1974 approximation).
- New `skybrightness/dataset/passband` package + `remote.PassbandBundle` endpoint: a versioned, checksummed passband-curve provider (`OpenBundle`/`Remote`). No bundle is published yet.
- `plan.LimitingMagnitudeConstraint` is rewritten against the new `Engine`/`Passband` API (`Engine`/`Passband`/`Conversion` fields replace `Model`/`Conversion`, `Atmosphere` is now `*atmosphere.Atmosphere`); `plan` continues to import core `skybrightness` only, never a subpackage (machine-enforced by `skybrightness/importgraph_test.go`).
- `unit` gains `Watt`/`Joule`/`Hertz`/`Nanometre`/`Candela`/`Steradian` units plus the zero-cost radiometric quantity types listed above; `constants` gains a `Photometric` set (AB zero point) and `Derived.StefanBoltzmannConstant`.
- `skybrightness/lpmap` is unchanged and kept as a live cross-check data source.
- `skybrightness.EvaluationOptions` is regrouped into `DerivedOptions`/`UncertaintyOptions`/`PerformanceOptions` (12 flat fields → 6, each self-explaining by group), and a new `skybrightness.NewRequestBuilder(...)` fluent constructor (mirroring `atmosphere.NewBuilder()`) is the recommended way to assemble a `Request`. `skybrightness.Point`/`PointQuery`/`PointResult` gain `ComputeTransmission`/`LimitingMag`/`Transmission`/`LimitingMagnitude`/`HasLimitingMag` — closing the gap that forced even a single-point caller back onto `Engine.Evaluate` directly for those two derived quantities — and `Point` now surfaces `IntegrateRadiance` failures as errors instead of silently returning a zero radiance. The dead, never-read `CompositeConfig.Passbands` field is removed.
- **`skybrightness/units.go` (the 26-member type-alias block re-declaring `unit`'s quantity types under short in-package names) is removed.** Every reference across `skybrightness` and its `natural`/`atmos`/`dataset/passband` siblings now uses the `unit.`-qualified name directly (`unit.SpectralRadiance`, `unit.WavelengthNM`, ...) — `unit` was already the single source of truth for these types; the alias block added a second name for the same identity with no real ergonomic benefit, since every file touching these types already imports `unit` for other reasons. `ToPhoton`/`ToEnergy`/`ArcsecondSquaredToSteradian`'s package-level var aliases are also removed — callers use `constants.ToPhoton`/`constants.ToEnergy`/`constants.ArcsecondSquaredToSteradian` directly.
- `skybrightness/provenance.go`'s alias block (`DatasetVersion`/`Fidelity`/`TimeRange`/`SourceRef`, the four `Fidelity*` constants, and `AtmosphereProvenance`, all re-declaring `atmosphere` package types) is removed for the same reason — every reference now uses `atmosphere.DatasetVersion`/`atmosphere.SourceRef`/`atmosphere.FidelitySynthetic`/etc. directly.

### Removed
- `skybrightness`: the whole previous package tree — `Engine`, `CompositeEngine`, `Request`/`Result`, `RequestBuilder`, `EvaluationOptions`, `SchaeferNELM`, `Bortle*`, `Mode*`, `LimitingMagModel`, `natural.ConstantAirglow`/`VBandMoonlight`/`TophatJohnson`/`NewFastEngine`, `atmos.RayleighOnly`, `dataset/passband.OpenBundle`/`Remote`. `plan`: `LimitingMagnitudeConstraint`, `ScoreObservableSky`. Examples 18 and 21.
- `remote`: `Client`, `NewClientFor`, `WithHTTPClient`, `WithMaxRetries`, `WithUserAgent`, `WithTimeout`, `RetryPolicy`, `DefaultRetryPolicy`, `HTTPError` (moved to `remote/api`), `ErrRetriable`, `DefaultAPITimeout` (now `api.DefaultTimeout`), `SetDataDirPath`, `EnableAllDownloads`, `DisableAllDownloads`. `remote/file`: `LocalURL`, `OSPath`, `Register`. `remote/s3`: `Register`, `Option`, `WithRegion`, `WithServiceURL`, `WithLegacyList`, `ErrNotS3Endpoint`. `ephemeris/jpl`: `WithDataDir`, `Provider.DataDir`, `AddKernelFile`. `ephemeris`: `WithDataDir`. `ephemeris/jpl/spk`: `OpenReaderAt`. `time`/`time/internal/iers`: `ErrEOPHTTPStatus`.
- `skybrightness/atlas` (World Atlas/VIIRS GeoTIFF decoders, download pipeline, `Resolver`) — its `dataset/raster`/`dataset/blackmarble`/`dataset/eog`/`dataset/worldatlas` replacements are Sky Brightness V2 Phase 4 scope, not yet built. `remote.WorldAtlas`/`remote.VIIRSAnnual` stay registered, re-scoped as future dataset inputs.
- `natural.FalchiNaturalZenithLuminance` — exported but never consumed anywhere in the module; deleted as dead code rather than kept as an unused re-export.

### Changed
- `go.mod` drops the `replace gocloud.dev => github.com/TuSKan/go-cloud` directive and requires upstream `gocloud.dev` directly. A `replace` is ignored outside the main module, so it did nothing for anyone importing astrogo, and the pinned fork commit carried no fork-only code; drivers upstream will not take now come from `github.com/TuSKan/gocloud-ext` instead. The four `aws-sdk-go-v2` requirements become indirect.
- `remote/file.Open` caches one `*blob.Bucket` per URL for the process. It previously read that cache but never populated it, so every call opened a fresh bucket — which also defeated `fileblob`'s per-bucket `IfNotExist` mutex and left the download lock non-exclusive even within one process.
- `remote.GetFile`'s fetch path is one sequence with no per-scheme branch: resolve source and cache through the same opener → freshness check → consent on the registered estimate, then on the reported size → `IfNotExist` lock → re-check → stage → validate → promote. Resume state rides as blob `Metadata` on the partial object rather than a sidecar key built by string suffixing.
- Doc comments across `remote` are rewritten to state contracts. The session-history narration in `acquireLock`/`writeResumable`/`unchanged`/`bucketReaderAt` is gone.
- `skybrightness/dataset/passband`'s `OpenBundle`/`loadCurve` now read through `remote/file.Bucket` (`.ReadAll`/`.NewReader`, path-joined with `path.Join`) instead of raw `os.ReadFile`/`os.Open`/`filepath.Join`, matching this codebase's `remote`-file-access methodology; `parseCurveCSV` streams rows via `csv.Reader.Read()` in a loop instead of buffering the whole curve into memory with `ReadAll()`. No public API change.

### Fixed
- `ephemeris/jpl.Open` closed the LSK reader it had just handed to `lsk.NewReader`, which takes ownership of it, so `Provider.Close` would then close it a second time.
- `plan`'s live geocoding test pre-checked only Nominatim while `NewSiteEarthAddress` also calls Open-Elevation, so downtime on the second service failed the test instead of skipping it. Both hosts are pre-checked now, plus a request-timeout guard, matching the repository's policy that network-tagged tests never fail on external downtime.
- **`ephemeris/jpl/spk`'s SPK reader called `os.Open` on every single `ReadAt`** (thousands of calls per Chebyshev segment lookup) after the `remote/file` rebuild above swapped its backing store from a plain `*os.File` to a `*file.Bucket`/`NewRangeReader`, since gocloud's `fileblob` driver opens a fresh OS file handle on every ranged read — turning a millisecond-scale SPK evaluation into multi-minute Windows runs. Fixed by capturing the resolved local path once (via `blob.ReaderOptions.BeforeRead`'s reach-through) and reading through one persistent `*os.File` afterward, closed on `Close()`.
- `time/internal/iers`'s `GetFile` call passed an empty object name against a single-resource `IERSFinals2000A.URL`, silently failing every fetch — the endpoint's URL is now a directory-style prefix (see `### Added` above) and the call site names `finals2000A.all` explicitly.
- Several `network`/`validation`-tagged tests (`ephemeris/jpl/validation`'s Sun/Mercury/Moon Horizons OBSERVER-query cases, `catalog/vizier`'s cone-search live tests, `remote`'s concurrent-`GetFile` lock test) converted hard failures on live external-service errors (JPL Horizons' own confirmed 500s for topocentric Sun/Mercury/Moon queries, VizieR TAP backend 500/503/400 instability, a documented Windows `fileblob` rename race) to logged skips — these tests are opt-in-only and were never run in CI, so this only affects local runs, per this project's own "never fail CI for external downtime" policy.
- `ephemeris/jpl/spk.CacheAPI` now detects and rejects a Horizons-generated SPK whose file record claims a first summary record (`FWD != 0`) but whose summary area is actually all zero bytes — live-confirmed as a real Horizons server anomaly (decoded byte-for-byte from a raw API response) that previously got cached and silently reused forever, producing an ephemeris provider with no coverage for the requested body and no error. New `spk.ErrHorizonsEmptyKernel` sentinel; `ephemeris/jpl`'s two live small-body tests (Eros, Apophis) skip with a clear log line when they hit it, rather than failing on a confirmed external issue.
- `remote/file.LocalURL` built its `"file://"` URL by raw string concatenation instead of percent-encoding the path — a directory whose name contained `#` silently truncated the URL at the fragment separator (losing the rest of the path *and* `?create_dir=true` with no error, opening the wrong, shorter directory), and a stray `%` made the URL fail to parse outright. Now built via `url.URL{...}.String()`, which encodes correctly and makes `LocalURL`/`OSPath` genuine inverses of each other instead of only matching by coincidence for paths with no URL-reserved characters.

### Added
- **`skybrightness/dataset/viirs`** turns NASA VIIRS annual composites into `GroundEmitter`s, restoring the VIIRS capability the rewrite deleted with `skybrightness/atlas` — now as a *source provider* rather than a sky-brightness lookup. A pixel radiance determines neither a spectrum nor an upward emission function, so both are required inputs and every emitter is flagged `AssumedSourceSpectrum | AssumedEmissionFunction`. Bins outside coverage or resolving to no-data are dropped rather than zeroed, since missing data is not measured darkness.
- **`skybrightness/dataset/raster`** carries the GeoTIFF decoder recovered from the deleted `atlas` package — classic TIFF, LZW and deflate, strips and tiles, float samples and the floating-point predictor — with its original 600-line test suite intact. It is source-agnostic and carries no units, because the products it serves are satellite radiances.
- **`viirs.Region` emits one source per azimuth sector, not per ring-and-sector cell.** Kocifaj & Bará (2019) Eq. 9 sums over *azimuthally separated* sources; the earlier ring×sector binning stacked several emitters at one azimuth and made the total scale with the bin count. `Rings` is replaced by `RadialSamples`, which refines the estimate within a sector without changing the emitter count. The absolute scale is still uncalibrated — inferring `L_S` from satellite radiance properly needs Elvidge et al. (2017) — but the N-scaling was this repository's bug, not a gap in the paper, and `docs/skybrightness.md` §17 retracts the earlier claim that Eq. 2 was missing a term.
- `atmosphere.MultipleScatteringFactor` implements Winkler (2022) §5.2's `f = 1 + 4.5·τ_R`, his revision of Noll et al. (2012)'s coefficient of 2.2. Applied in `ScatteredMoonlight`, it moves the validated full-Moon sky brightness from 18.92 to **18.62 mag/arcsec²** — brighter, which is the direction single scattering is known to err, and closer to the canonical ~18. The `SingleScatteringOnly` quality flag becomes `ApproximateMultipleScattering`.
- `atmosphere.GushchinAirmass` is the airmass formula Kocifaj & Bará (2019) Eq. 3 adopts, giving 35.7 at the horizon against Pickering's 38. `ArtificialSkyglow` now uses it rather than `atmosphere.Airmass`, because the two-index model's fit and its horizon limit are calibrated against that value.
- **`skybrightness/dataset/solar`** fetches the CALSPEC solar reference (`sun_reference_stis_002.fits`) through the new `remote.CALSPEC` endpoint, converting angstrom and erg s⁻¹ cm⁻² Å⁻¹ to nanometres and W m⁻² nm⁻¹. It fixes the absolute scale of every reflected-sunlight model — lunar irradiance today, zodiacal light next — and interpolation returns zero outside the tabulated range rather than extrapolating flux into a band the reference never covered.
- `skybrightness.Airglow` and `atmosphere.VanRhijn` implement the chemiluminescent emission of the upper atmosphere: Leinert et al. (1998) Eq. 13 applied to a caller-supplied zenith spectrum, as Masana et al. (2021) Eq. 19–20 does. Validated against Roach & Meinel (1955)'s published maximum of 5.7 for a 100 km layer. Airglow brightens toward the horizon by a factor of about six from pure geometry, which is why it cannot be modelled as a constant floor.
- The zenith spectrum is a required input, not a prediction. Airglow varies by up to 100% night to night, with season, with the solar cycle and with geomagnetic latitude; Leinert et al. and Masana et al. both treat it as a free parameter, and so does this. Results carry `ClimatologicalAirglow`, and past 40° from the zenith `ExtrapolatedModel` too — Leinert et al. state that extinction along the longer path changes the behaviour materially there, and this applies the geometry alone.
- `skybrightness.ZodiacalLight`, `ZodiacalBrightnessAt`, `ZodiacalColourCorrection` and `ZodiacalElongation` implement Leinert et al. (1998) Table 17 and Eq. 22 with Masana et al. (2021)'s heliocentric and seasonal factors. Validated against a number from outside the table: the ecliptic pole comes out at **23.26 mag/arcsec² in V**, against roughly a quarter of a 22.0 dark sky. The solar vicinity Table 17 leaves blank returns `ErrZodiacalGeometry` rather than an extrapolation into a region an order of magnitude brighter.
- `skybrightness.DiffuseGalacticLight` implements the optical/100 µm correlation — Kawara et al. (2017) Eq. 7 as Masana et al. (2021) Eq. 13–14 apply it — turning a Schlegel–Finkbeiner–Davis dust intensity into spectral radiance. DGL is 20–30% of the Milky Way's integrated light, so it cannot be folded into starlight. The empirical fit bounds nothing itself, so three clamps are applied and flagged rather than left to produce negative radiance.
- The published quadratic coefficient's power of ten is read as 10⁻⁵ against the printed 10⁵. The turnover `b/(2c)` then lands at 39–50 MJy sr⁻¹ across all six well-measured bands — the top of Kawara's own fitting range — where the printed value would put it at 10⁻⁹ and make DGL negative over the entire sky. `TestDGLTurnoverMatchesTheFittedRange` asserts it band by band, so the evidence is executable. See `docs/skybrightness.md` §17.
- `starlight.BuildFromGaia` and `GaiaBuild.ADQL` build an integrated-starlight map from the ESA Gaia archive without the bulk catalogue. A `source_id` carries the HEALPix index in its high bits, so the aggregation is a server-side `GROUP BY` — verified live at 1,000 pixels per query, sub-second, riding the primary-key index. The full sky is ~787 queries against 600 GB for the bulk route.
- The colour transformation is rendered into the query and applied **per star inside the aggregate**, because transforming a summed flux is not the same as summing transformed fluxes when the transformation depends on colour. `GaiaBand` carries no shipped coefficients or zero points: Gaia's own G/BP/RP is the only photometry the archive holds, every other band is a fit tied to a specific filter revision, and the package refuses to guess one. A band with no colour term is the Gaia G band and works as-is.
- What it does not reproduce, stated in the doc comment: Hipparcos bright stars, per-region colour imputation, and the sub-3% Besançon faint-star completion. Masana et al. shipped a DR2 bug underestimating this quantity for months, so anything built here needs checking against their tool first.
- **`skybrightness/dataset/starlight`** holds the extra-atmospheric natural sky — integrated starlight, diffuse galactic light and extragalactic background — as a HEALPix `Map`, with a plain-text loader for published tables and `SpectralShape` to spread a band-integrated radiance across wavelengths. 37 ns/lookup on a 786,432-pixel map, zero allocations. It deliberately does *not* compute starlight from a catalogue: that is a bulk aggregation over Gaia DR3's 1.8 billion sources, an offline job producing a data product rather than a runtime operation.
- Two decisions in that package worth stating: a `Frame` travels with every map, because a galactic map read as equatorial puts the Milky Way through the wrong sky and still returns plausible numbers; and `Load` rejects a table missing any pixel rather than zero-filling it, because a hole in a sky map is not a dark patch of sky.
- `coord.HEALPix` implements the Górski et al. (2005) equal-area sphere tessellation in NESTED ordering — `PixelOf`, `Center`, `NumPixels`, `PixelArea` — 26 ns/lookup, zero allocations. Every published integrated-starlight map is on this grid, and equal area is why: a radiance is per unit solid angle, so a tessellation with unequal cells needs a per-pixel weight that is easy to forget and invisible when forgotten. Verified by centre round-trip across all twelve faces at six resolutions, by uniform-sphere occupancy, by the nested quadtree identity (`fine/4 == coarse`), and against GAMBONS' independently quoted 1.5979e-5 sr at nside 256.
- `coord.Offset` completes the ground-geometry trio with `GroundDistance` and `InitialBearing`: the direct problem to their inverse one, on the same IUGG mean sphere, verified by round-tripping.
- **`skybrightness.ArtificialSkyglow`** propagates `GroundEmitter` sources to the sky through Kocifaj, Bará & Falchi (2022), summing them in linear radiance space. Eq. 2 specifies neither `L_S` nor `M_S` for a real installation, so both choices are made explicitly in the type's doc comment: `M_S` is the horizon airmass (which is what makes the paper's own horizon limit hold), and the emission function is evaluated at zero elevation, overridable with `WithEscapeElevation`. Tested on the model's physical claims — falls with distance, sums linearly, responds to shielding, and puts the darkest sky ~90° from the city rather than opposite it, which is the Rayleigh back-scattering lobe. 54 µs per direction per source, zero allocations.
- `atmosphere.MolecularScaleHeight` derives the pressure scale height from `H = R_d·T/g` (8435 m at 288.15 K) rather than tabulating 8.4 km, so a warm site and a cold one differ — the molecular term of `OpticalParameterT` scales inversely with it. With it, `atmosphere.DryAirGasConstant` and `atmosphere.StandardGravity`.
- **`skybrightness.ScatteredMoonlight` is the module's first `Component`** — ROLO lunar reflectance propagated through molecular and aerosol single scattering. It validates end to end against a number from outside its own literature: a near-full Moon at Paranal gives **18.9 mag/arcsec² in V**, against a long-established full-moon sky brightness of about 18. Landing there requires ROLO's reflectance, the Ω/π conversion, both inverse squares, the Rayleigh optical depth, the phase function and the transfer integral all to be right at once. Per-scene geometry is cached behind a read-write lock and scratch buffers are pooled: 4.6 µs per direction, zero allocations.
- `atmosphere.SingleScatteredRadiance` is the single-scattering path integral for a homogeneous plane-parallel atmosphere, with airmass in place of `sec z` so it stays finite at the horizon. It is **derived rather than transcribed** — the derivation is in the doc comment — and checked against the textbook optically-thin limit `E·p·τ_sca·M_v`, which no ratio test would catch.
- **`Component.AddRadiance` now returns `(Flag, error)`** instead of `error`, and `Model.Estimate` ORs the flags into the estimate's `Quality`. Flags cannot be fixed per component: the same model is an interpolation in one geometry and an extrapolation in another, and §32's guarantee is meaningless if a caller cannot tell which they got. Changed while there were still zero implementations, so it cost nothing. New `SingleScatteringOnly` quality flag.
- `magnitude.ROLOReflectance` implements the ROLO lunar irradiance model version 311g — Kieffer & Stone (2005) Eq. 10, with Table 4's 32 bands × 10 coefficients and Table 5's 8 wavelength-independent ones — plus `magnitude.ROLOBands`, `ROLOGeometry` and `ROLOIrradiance`. It sits in `magnitude` beside the existing asteroid, planet and satellite photometry rather than inside `skybrightness`, which owns only radiance transport. Eq. 10 uses the phase angle in **radians** in its polynomial and in **degrees** in its exponential and cosine terms, so the API takes `angle.Angle`, never `float64`. Validated against a number from outside the paper: near full Moon at 553.8 nm it returns 0.134, against a lunar V-band geometric albedo independently known to be about 0.12 — the same quantity at zero phase — with the 2383.6/553.8 nm ratio reproducing the Moon's red slope at 2.4.
- The selenographic longitude of the Sun and the two libration angles are inputs to `ROLOGeometry`, not derived: they need lunar orientation data (IAU rotation elements or a binary PCK) this module does not have. The libration terms are the model's four smallest and a caller may pass zero, which `TestROLOLibrationIsASmallCorrection` bounds at 0.03 in ln A. See `docs/skybrightness.md` §11.3 and §16.
- `skybrightness.OpticalParameterT` and `skybrightness.AsymmetryParameter` implement Kocifaj, Bará & Falchi (2022) Eq. 3 and Eq. 4/5 (arXiv:2203.09322); Eq. 1 is `atmosphere.CombinedPhaseFunction`, shared with the lunar model. Eq. 5's exponents sit on `τ_a` (`c₀ = 0.33 + 0.15τ_a`, `c₁ = 0.9τ_a^0.51`, `c₂ = 1.3τ_a^1.85`), confirmed against the typeset equation. The published fit is not bounded to the physical range, so a `g` outside (−1, 1) is returned together with `ErrAsymmetryOutOfRange` rather than clamped.
- `skybrightness.GroundEmitter`/`UniformEmitter`/`UpwardEmission` model an artificial source as a spectrum plus an upward emission function, not a single brightness — several different real installations produce the same satellite pixel, so the shielding assumption must be explicit and travel with the result in `Quality`.
- `skybrightness.AllSkyRadiance` implements Kocifaj, Bará & Falchi (2022) Eq. 2, the semi-analytic all-sky radiance kernel, reducing exactly to the paper's own stated horizon limit `L_S·P(g,Θ)·(1−g)²/(1+g)`. Its removable singularity at `M_S = M(z)` is evaluated as `t·expm1(u·t)/(u·t)`, exact at `u = 0`, where a bare `(e^{u·t}−1)/u` loses all precision and would leave a notch in a sky map at the source azimuth.
- **`AllSkyRadiance` takes `L_S` as it reaches the observer, not as it leaves the source.** Eq. 2 has no distance term of its own: distance enters through `t`, which Eq. 3 makes proportional to the source–observer separation, and through `L_S`, which must already carry the transmission `e^{−M_S·t}`. An earlier revision withdrew this kernel after a test found radiance growing with distance — the transcription was right and the test was wrong, having varied `t` while holding `L_S` fixed. Both senses are now asserted, so the doc comment's warning cannot silently stop being true. See `docs/skybrightness.md` §11.1.
- Eq. 4 and Eq. 5 — the convenience parameterisation of the asymmetry parameter `g` from the aerosol asymmetry `g_a` — are **deliberately not implemented**: the exponents in Eq. 5 are ambiguous in the PDF text layer and neither reading can be ruled out on physical grounds (see `docs/skybrightness.md` §17 for both candidates and why plausibility does not settle it). `g` is an explicit caller input meanwhile, so the model is fully usable; only the shortcut is missing.
- `atmosphere` gains the scattering layer the sky-brightness engine propagates through, all of it traceable to primary literature: `RayleighOpticalDepth` and `RayleighPhaseFunction` (Winkler 2022 Eq. 13 and Eq. 9, after Dutton et al. 1994 and Bucholtz 1995), `HenyeyGreensteinPhaseFunction` (Eq. 10), `CombinedPhaseFunction` (Eq. 12), `AerosolOpticalDepth` (Angstrom scaling) and `Transmission`. `RayleighDepolarisation = 0.0148` is the value for which Bucholtz's theoretical phase function reproduces the `1.06 + cos^2` coefficient Krisciunas & Schaefer (1991) fitted empirically. Each phase function is verified to integrate to unity over the sphere, and `RayleighOpticalDepth` reproduces the independently known sea-level value of ~0.098 at 550 nm.
- The Rayleigh formulation is Bucholtz/Winkler rather than Bodhaine et al. (1999) — a deliberate choice recorded in the code and in `docs/skybrightness.md` §11.5. Both the lunar scattering model (Winkler 2022) and the artificial-skyglow model (Kocifaj et al. 2022) use this lineage and the same Henyey-Greenstein aerosol phase function, so one shared implementation keeps the two components from silently disagreeing about the atmosphere they propagate through.
- `atmosphere.CrossSection` applies a tabulated molecular absorption cross section over a column via Beer-Lambert, with the Dobson Unit derived from the SI-exact Boltzmann constant and the STP definition rather than hardcoded. It ships **no** tabulated cross-section data: O3, O2 and H2O cross sections are datasets with their own provenance (Serdyuchenko et al. 2014, HITRAN) and are recorded as an unresolved dependency instead of being invented.
- `unit.SpectralGrid` — the uniform wavelength axis shared by every per-wavelength calculation in the module, with trapezoidal integration and linear resampling. It sits in `unit` because both `magnitude` and `skybrightness` need it, so a spectrum, a filter curve and a detector QE curve can be combined without one of them silently being on a different axis.
- `magnitude` gains the photometric projection layer: `Passband`, `System` (AB/Vega/ST), `Detector` (photon-counting vs energy-integrating), `MeanFluxDensity`, `PivotWavelength` and `SurfaceBrightness`. Vega zero points travel with the passband rather than being package constants, because they depend on which Vega reference spectrum is adopted; a Vega request against a band without one fails rather than silently returning an AB number.
- `optics` gains the radiometric layer: `Throughput` (one type for mirrors, windows, filters, lenses and detector QE, since they are all a dimensionless fraction of wavelength), `System` for the element product, `Instrument`, `NewInstrument`, `PhotonRate` and `BackgroundRate` in electrons per pixel per second.
- `skybrightness` Phase 0: the spectral foundation. It ships **no `Component` implementations** — an empty model returns zero radiance and flags `NoComponents` rather than presenting a plausible-looking dark sky, and makes no accuracy claim.
- `docs/skybrightness.md` is rewritten as the design document: scientific baseline with primary references per model, package placement rationale, the equation-to-function-to-test maps for Kocifaj et al. (2022) and the ROLO lunar reflectance model, validation strategy, phase roadmap, unresolved dependencies and open scientific questions.
- **`http://`/`https://` sources are reachable through `remote.GetFile` for the first time**, via `github.com/TuSKan/gocloud-ext/blob/httpblob`, blank-imported by core `remote/file` alongside `fileblob`. Every `KindFile` endpoint served over HTTPS — IERS, the NAIF SPK/LSK mirrors, OpenNGC, GFZ's World Atlas, the VIIRS mirror — previously failed with "no driver registered for scheme https"; that gap is closed, and the endpoints are exercised end to end against `httptest` servers.
- `remote/file.NewReaderAt(ctx, bucket, key, opts...)` — a backend-generic random-access reader that reads aligned chunks and keeps a bounded LRU of them, replacing the `*os.File`-sniffing reader that lived in `ephemeris/jpl/spk`. Memory is capped at `WithChunkSize` × `WithCachedChunks` (64 KiB × 16 = 1 MiB) for **any** object size, and the object is never buffered. `BenchmarkReadAtStrategies` records why the chunking exists: 2000 SPK-shaped reads cost 263 ms as one range read per `ReadAt` versus 0.33 ms chunked, because `fileblob` opens an OS file per call and http/S3 issue a request per call.
- `remote.SetURL` accepts `gocloud.dev/blob`'s portable `?prefix=` and `?key=` wrappers on every scheme, so an endpoint can be scoped to a subdirectory or pointed at one exact object under whatever name astrogo asks for — the supported answer to "my mirror lays the files out differently", and the reason a single-object URL no longer needs special handling.
- `remote/api` retries on 429, 5xx (except 501) and transport failures using resty's own `RetryConditionStatus*` predicates. Worth recording: resty's `SetRetryDefaultConditions` covers only transport/header/URL errors, so status-based retrying must be registered explicitly — a test caught this silently retrying nothing.
- `internal/testutil.FileURL` and `.BucketKeys` — test helpers for building a `file://` bucket URL and listing a cache's contents through the bucket rather than reading a directory.
- `atmosphere.RuralAerosol`/`UrbanAerosol`/`DesertAerosol`/`MaritimeAerosol(heightM, aod550 float64) *Builder` — named, published aerosol-type presets (Hess, Koepke & Schult 1998, OPAC's "Continental average"/"Urban"/"Desert"/"Maritime clean" types, Table 3, 0.55µm, 80% RH), seeding a `Builder` with real single-scattering albedo/asymmetry-parameter/Ångström-exponent values instead of requiring a caller to look them up; aerosol optical depth stays a caller-supplied, real-time-varying parameter, never hardcoded. Each returns a `*Builder` (not a terminal `*Atmosphere`), so further customization chains before `Build()`. `StandardDefault`'s doc comment now cross-references these and states explicitly that its zero aerosol is the exact Rayleigh-only reference case.
- `docs/skybrightness.md` §8 gains a new "CAMS aerosol data — validated technical notes (Phase 3/7)" subsection: real grid/chunking/tracer-availability facts and the pressure-reconstruction formula for a future `dataset/atmostate` CAMS reader, plus the aermr-tracer→species→PSD→refractive-index→MOPSMAP mapping identified as the eventual live-data replacement for the OPAC presets above. Documentation only — no code in this release.
- **`remote` is rebuilt from scratch on `gocloud.dev/blob` (via the `github.com/TuSKan/go-cloud` fork, a `replace` directive in `go.mod`), replacing `github.com/ungerik/go-fs` entirely — the dependency is now fully removed from the module.** New package `remote/file` (`*file.Bucket = *blob.Bucket`) is the one uniform, `io/fs`-shaped file-access type every backend goes through — local disk (`fileblob`) is built in; `remote/s3` blank-imports `s3blob` and stays the only importer of the AWS SDK v2 in the module, unchanged in spirit from before but now registering a `*blob.Bucket` directly (`s3blob.OpenBucket`) instead of implementing a bespoke `Transport`. `remote.Transport`/`RegisterTransport`/`KindS3` are gone — every `KindFile` endpoint's `URL` now names a `gocloud.dev/blob`-openable bucket, addressed uniformly regardless of scheme.
- `remote.GetFile`/`remote.CacheDir` now return `(bucket *file.Bucket, key string, err error)` instead of a `gofs.File` — the byte-transfer/locking/resume policy is expressed once as a generic bucket-to-bucket copy (`IfNotExist`-based locking, streaming `NewRangeReader`/`NewWriter`, `Metadata`-based resume state) instead of being duplicated per transport. **Every `KindFile` `Endpoint.URL` must now be a directory-style prefix** (matching `NAIFSPK`/`NAIFLSK`/`OpenNGC`'s existing convention) — `remote/file`'s `sourceBucket` opens `URL` as a bucket *root*, so a single-exact-resource URL with no caller-supplied `name` can never resolve; fixed `remote.IERSFinals2000A`'s URL and its one `GetFile` call site (`time/internal/iers/fetch.go`) accordingly, a real bug found and fixed this session, not just a design constraint stated for new code.
- New package `atmosphere/dataset/cams` — a minimal, read-only NetCDF-4/HDF5 reader for CAMS global-analysis files (`cams.Open`/`File.Dims`/`File.Var`/`Var.ReadPlane`/`Var.At`), the Sky Brightness V2 Phase 3/7 building block `remote/s3` above was built ahead of. A second, independent importer of `github.com/scigolib/hdf5` outside `skybrightness` (see that package's own scoping note) — re-adopting the dependency was gated on a live decision-gate spike against the real files this reader targets, confirming N-dimensional shape/axis discovery via real NetCDF-4 metadata (`_Netcdf4Coordinates`/`_Netcdf4Dimid`, not string-parsing), chunked+deflate reads, and chunk-selective hyperslab reads (~86× faster than a full decode for the common one-plane access pattern). Fill values (`_FillValue`/`missing_value`) are substituted with NaN at the read boundary; a missing tracer surfaces as `ErrVariableNotFound`, never a fabricated zero. The ECMWF L137 pressure-reconstruction formula and the aermr-tracer→optics mapping remain explicitly out of scope, per `docs/skybrightness.md` §8.

## [0.14.0] — 2026-08-07

### Fixed
- `examples/18_sky_brightness` now actually offers the lightpollutionmap.info API to `LayerAuto` in its main run — previously only the separate comparison table configured it, so a caller with `LIGHTPOLLUTIONMAP_KEY` set still fell through to the Bortle-4 fallback whenever World Atlas/VIIRS download consent wasn't granted, the original gap that motivated adding the API client at all.

### Added
- `skybrightness/lpmap`: two live regression tests (`network`-tagged) — `TestFloorWA2015_MatchesFrozenReference` pins the `wa_2015` layer's artificial-brightness value at two sites against a live-verified reference (guards the unit-dispatch logic against a future regression), `TestSQMViirs2025_SaoPauloBrighterThanDarkSite` exercises the `viirs_<year>` raw-radiance dispatch path against the real API instead of only a synthetic fixture.
- `internal/parallel.Map[T, R any]` — the order-preserving, `GOMAXPROCS`-bounded "run independent per-item work, collect results in input order" primitive five call sites (`plan.FilterObservable`/`RankObservable`/`RankObservables`, `gatherPlanetaryMoons`'s kernel fetch, `VisibleTonight`'s three concurrent gathering stages) had each hand-rolled separately via their own `errgroup`. All five now share this one implementation.
- `internal/parallel.MapChunked[W any]` — the sibling "fixed number of goroutines, each a contiguous index chunk, goroutine-scoped setup called once per goroutine" primitive `coord.Context.ReduceBatchParallel`/`ICRSBatchToAltAzParallel` had each hand-rolled identically (both need one `Context.Clone()` per goroutine, not per element, to avoid sharing SOFA's mutable refraction-coefficient cache). Both now call `MapChunked`; behavior, thresholds, and benchmarked throughput are unchanged.
- `catalog/xmatch.Match(a, b []resolve.Target, opts ...Option) []Pair` — a standalone catalog cross-match primitive (alias-graph union-find, epoch-normalized positional fallback via `coord.PropagateEpoch`) operating directly on plain `resolve.Target` slices, independent of `catalog.Resolver`. Reports matched pairs only — field reconciliation stays the caller's own concern (ROADMAP #38).
- `resolve.Target.HasRadialVelocity` — distinguishes a genuinely-measured zero radial velocity from no measurement at all, mirroring the existing `HasVMag`/`HasCoord`/... presence-flag pattern.
- `resolve.Target.Diameter`/`HasDiameter` and `Albedo`/`HasAlbedo`, decoded from `catalog/sbdb`'s `phys_par` response — a real measured `Diameter` (occultation/thermal/radar) is now preferred over the existing H+albedo estimate. `plan.Asteroid.PhysicalRadius()` (implements the new `plan.PhysicalRadius` optional-capability interface) resolves diameter → albedo-estimate → unavailable, in that order, and `plan.AngularDiameter` now falls back to it when a body has no fixed `BodyEquatorialRadius` table entry.
- `plan.MoonIllum` — a `Constraint`/`ConstraintCtx` (companion to `MoonSep`) that rejects/penalizes targets above a lunar-illumination-fraction threshold; always passes for the Moon itself (ROADMAP #32).

### Changed
- `Planner.RankObservable` no longer requires its `Observable` argument to also implement `coord.Object` — it was returning `ErrNotCoordObject` for any other type (a satellite, a generic moving body), and had zero test coverage or callers anywhere in the repo despite the type constraint. It now falls back to the existing `observableObject` adapter, the same one `visible_tonight.go` already uses for this exact purpose. `ErrNotCoordObject` is deprecated, not removed.
- `TransitEstimate` similarly widens from `coord.Object` to `Observable`, matching `RankObservable`'s own fix.

### Fixed
- `catalog.go`'s field-precedence merge rule for `RadialVelocity` checked `RadialVelocity != 0` instead of the new `HasRadialVelocity` flag, silently dropping a genuinely-measured 0 km/s radial velocity during cross-provider reconciliation — the same bug class the orbital-elements merge rule had before it.

## [0.13.0] — 2026-08-05

### Added
- `plan.MoonElongation`/`plan.MoonPhaseFraction` — `MoonIllumination`'s illumination fraction and phase angle are both symmetric about full moon, so neither can answer "is tonight's Moon waxing or waning" on its own; `MoonElongation` exports the already-computed, monotonically-increasing ecliptic elongation (0°→360° across a lunation) that answers it, and `MoonPhaseFraction` is the same information as a continuous 0=new/0.5=full/→1=new-again cycle position. Cross-checked against `MoonPhases`' own independently root-found event times (#21).
- `plan.IsCircumpolar`/`plan.IsNeverUp(dec angle.Angle, site *Site, opts ...CircumpolarOption) bool` — the purely geometric "does this declination ever set (or ever rise) at this site" question, answered by one closed-form pair of altitude evaluations (upper/lower culmination) instead of a numerical search or an indirect empty-`VisibilityEvents`-result inference (which can't tell circumpolar apart from never-rises without a second check of its own). `WithRefraction` includes the standard ~34′ atmospheric refraction correction `Site.SunRiseSetThreshold`/`MoonRiseSetThreshold` already use (off by default, matching `Site.RiseSetThreshold`'s own convention); `WithHorizonAltitude` substitutes a caller-supplied minimum altitude for the site's true horizon (#20).
- `time.EOPSource()`/`time.ResetEOP()` — `EOPSource` reports which of `"zero"`/`"explicit"`/`"cache"`/`"network"` populated the currently-active EOP model, so a caller (a test in particular) can assert this directly instead of inferring it from a lookup's numeric result; `ResetEOP` restores the pristine default (`ZeroModel`, not pinned) without pinning anything itself — the "start over" operation `RegisterModel(ZeroModel{})` no longer safely provides, see `### Fixed` below.
- `remote.WorldAtlas` — a new `KindFile` endpoint for GFZ Data Services' hosting of Falchi et al. 2016's World Atlas 2015 archive (`World_Atlas_2015.zip`, ~653 MB, live-confirmed `Content-Length: 684266450`, frozen since 2019-11-18, DOI `10.5880/GFZ.1.4.2016.001`). **License note: CC BY-NC 4.0 (non-commercial use only)** — surfaced in the endpoint's `Description`, not buried in a comment.
- `remote.VIIRSAnnual` — a new `KindFile` endpoint for lightpollutionmap.info's own unauthenticated mirror of NASA's VIIRS annual nighttime-lights composites (Black Marble VNP46A4/VJ146A4, one raw GeoTIFF zip per year 2012-2025, live-confirmed against the real archive's ZIP central directory). Source data is CC0; the mirror asks for credit to "Jurij Stare, www.lightpollutionmap.info" plus "NASA's Black Marble nighttime lights product". Unlike NOAA/EOG's own hosting of the same product (which now requires OAuth2), this mirror needs no login at all.
- `skybrightness/atlas.EnsureWorldAtlas`/`OpenWorldAtlas` and `EnsureVIIRSAnnual`/`OpenVIIRSAnnual` — download (consent-gated via `remote.WorldAtlas`/`remote.VIIRSAnnual`), extract, and validate an archive, returning a ready-to-query windowed `skybrightness.SQMProvider`. Extraction is atomic (`remote.Save`'s temp-file-then-rename) and post-extract-validated by decoding+sampling the fresh file before trusting it, so an interrupted download/extraction can never get silently cached as complete; the zip is deleted after a successful extraction by default (`atlas.WithKeepArchive` to keep it). `atlas.ProgressLogger` is a ready-made `WithDownloadProgress` callback (one log line per 10%) so a caller never has to hand-write percent arithmetic.
- `skybrightness/atlas.FloorAt(ctx, loc *coord.Geodetic, opts ...Option)` — the one-call entry point: builds a `Resolver`, queries it, releases it. `atlas.FloorAt(ctx, site.Location(), atlas.WithBortleClass(4))` is the whole API for a single site; `NewResolver` remains for resolving many sites with the (multi-gigabyte) atlas file held open across queries. `Resolver.Floor` now takes a `*coord.Geodetic` rather than raw lat/lon degrees, so a `plan.Site` feeds it directly with no unpacking; a nil location returns the new `ErrNilLocation`.
- `skybrightness/atlas.Resolver`/`NewResolver` — resolve a site's light-pollution floor from one chosen `atlas.Layer` (`LayerWorldAtlas`, `LayerVIIRS`, `LayerLightPollutionMap`, `LayerBortle`, `LayerScalar`), or let `LayerAuto` (the default) try the best available automatically and report which layer answered and why every earlier one didn't (`Result.Attempts`, `errors.Join`-aggregated so `errors.Is` still resolves against any individual layer's own sentinel — `remote.ErrDownloadDenied`, `lpmap.ErrNoAPIKey`, ...). Download-backed layers (`LayerWorldAtlas`/`LayerVIIRS`) log their own progress by default (see `atlas.ProgressLogger`; `atlas.WithQuiet` disables it) — no separate download/progress plumbing to wire up, and `NewResolver` takes the exact same `atlas.Option` type as `EnsureWorldAtlas`/`OpenWorldAtlas`, so a download-related option needs no translation layer. This replaces the previous single-source, single-point-of-failure pattern (`examples/18_sky_brightness`'s old "no API key — using natural sky only" dead end) with graceful degradation. (Originally shipped as a separate `skybrightness/sitefloor` package; folded directly into `atlas` before release since the composed logic needs `atlas` anyway and a third import was one package too many for this to feel like "pick a layer and it works.")
- `skybrightness.RadianceToArtificialSB` (moved from `skybrightness/atlas`'s formerly-unexported `radianceToArtificialSB`, behavior unchanged) plus `DefaultRadianceSlope`/`DefaultRadianceZeroPoint` — the radiance→brightness log-linear fit, now a shared core primitive both `atlas`'s VIIRS providers and `skybrightness/lpmap`'s VIIRS-layer dispatch call, instead of two independent implementations.
- `skybrightness/atlas.NewestVIIRSYear` — probes `remote.VIIRSAnnual` forward from the compiled-in `LatestVIIRSYear` (HEAD requests only, so no download consent needed) to find the newest annual composite actually published, so a new upstream year is picked up without a release. `LayerVIIRS`/`LayerAuto` use it automatically; a probe failure degrades to the best year confirmed so far rather than erroring. `EnsureVIIRSAnnual` accordingly bounds `year` from below only — upstream, not a constant, is authoritative on which years exist.
- `remote.GetFile` now resumes an interrupted download instead of restarting it: partial bytes and the response's `ETag` are kept in `<name>.part`/`<name>.part.etag` sidecars, and the retry sends `Range`/`If-Range` so the server can either continue (206) or force a clean restart when the content changed (200). A partial with no stored validator is discarded rather than trusted, and the download-consent cap is still checked against the file's FULL size, so resuming can't be used to slip past a `MaxDownloadSize` a few bytes at a time. Matters most for the multi-hundred-MB atlas archives. Also new: `remote.Exists(ctx, id, name)` (HEAD probe, no body, no consent gate).
- `skybrightness/lpmap.VIIRSLayer(year)` — names a `viirs_<year>` raster layer for `WithLayer`. The client's default is still World Atlas 2015, so a caller who wants the live API on the freshest data has to say so; unlike the downloaded `LayerVIIRS`, nothing here probes upstream for which years exist.
- `skybrightness.NaturalZenithMcdM2` (0.171168465 mcd/m² ≡ 22.0 V mag/arcsec², Falchi et al. 2016) — the natural zenith background this package already used internally is now exported, so `skybrightness/lpmap` and callers converting an artificial-only value to a total observed brightness (e.g. before `BortleClass`) reference one symbol instead of re-declaring the literal independently.
- `plan.DayEvents(day, loc, target, site) (rise, set, transit *Event, err error)` — the day-indexed almanac-table view: the first rise/set/transit of an arbitrary `Observable` within a given local calendar day, so a caller doesn't have to derive the day's own midnight-to-midnight window and filter a wider `EventSolver` search by hand. `plan.Episode(from, to, target, site) (rise, set *Event, err error)` answers the related but different "which continuous up-episode does this window belong to" question — it reaches outside `[from, to]` as needed (searching backward for a rise already in progress, or forward for a set beyond `to`) so the returned pair always describes one real continuous period above the horizon, never two unrelated events stitched together. Both return nil fields, not an error, when that event kind doesn't occur (polar night, circumpolar, never-rises) (#23).
- `Window.Overlaps`/`Intersect`, plus package-level `Union`/`Intersect`/`Subtract`/`TotalDuration` over `[]Window` — the interval arithmetic every consumer of `ObservableWindows`/`VisibleIntervals` was previously writing by hand (most often to answer "of the time this target is observable, how much has some other body below the horizon", i.e. `Subtract(ObservableWindows(target, ...), VisibleIntervals(other, ...))`). `Union`/`Intersect`/`Subtract` all normalize their input first (sorted, merged, touching windows coalesced), so unsorted or overlapping window sets are never the caller's problem (#24).

### Fixed
- `plan.TwilightEvents` now actually groups dawn/dusk pairs as its doc comment always claimed — previously it appended one half-populated `TwilightEvent` (`Dawn` xor `Dusk` set, the other always nil) per solver event, so a caller reading the doc and writing `ev.Dusk.Time` panicked on every other element. Each result now pairs a dusk with the chronologically next dawn — the twilight/darkness span between them, matching `AstronomicalDawnDusk` and friends' own "the night" framing — with an unpaired leading/trailing event at an interval edge left correctly half-nil rather than silently dropped or mispaired (#22).
- `time.RegisterModel` is now authoritative: it previously could be silently overridden by the automatic lazy load the moment an uncovered `Time.EOP()`/`.UTC()`/`.UT1()` query happened to find a `finals2000A` file already sitting in the cache directory — so `RegisterModel(ZeroModel{})`, the natural way to ask for deterministic zero EOP, was itself the thing most likely to be silently discarded, making whether a run used real or zero EOP depend on ambient machine state rather than the caller's own choice. An explicit `RegisterModel` call now disables the lazy loader entirely going forward; call the new `time.ResetEOP()` to undo it. (#25)
- `remote.GetFile` no longer corrupts a destination file when two callers race to fill the same missing cache entry — resumable downloads gave every caller the SAME fixed-name `.part` file to write to, so concurrent fetches (e.g. several `go test ./...` packages fetching the same shared JPL kernel, each its own OS process) clobbered each other's bytes rather than merely wasting bandwidth the way the old random-temp-file `Save` path did. Caught by real CI failures (a truncated SPK kernel of a DIFFERENT size on every run), not found by inspection. Fixed with a cross-process advisory lock (`O_CREATE|O_EXCL` on a `.lock` sidecar) held across the whole "still missing? then download" decision, with a stale-lock timeout so a crashed holder can't wedge a later run forever. Also required a Windows-specific fix live-reproduced on a real Windows run: an exclusive create racing another goroutine's `os.Remove` of the same lock file returns `ERROR_ACCESS_DENIED`, not `ERROR_FILE_EXISTS`, while the delete settles — both now retry rather than only the latter.
- `skybrightness/atlas`'s GeoTIFF reader now decodes LZW (compression 5) and the floating-point predictor (tag 317 = 3), not just uncompressed/deflate. Every VIIRS annual composite is LZW, so `LayerVIIRS` previously failed validation — after downloading and extracting ~1 GB — with "unsupported TIFF feature: compression 5". The decoder implements TIFF's own LZW variant rather than delegating to `compress/lzw`: both are MSB-first over 8-bit literals, but TIFF (like PDF, unlike GIF) widens codes one step early, so the stdlib reader desyncs and rejects real files with "lzw: invalid code".
- `skybrightness/lpmap`: a real live unit bug — `WithLayer("viirs_2018")` (a documented, real QueryRaster layer) returns raw VIIRS-DNB radiance (nW·cm⁻²·sr⁻¹), but the client unconditionally treated every layer's response as World Atlas mcd/m² luminance, silently producing a plausible-looking wrong brightness. `Client` now dispatches on the configured layer's actual unit (matched by layer *family* — `wa_2015` vs. `viirs_<year>` — so every year upstream publishes is handled without a hardcoded list to go stale; new `ErrUnknownLayer` for a layer in neither family instead of silent misinterpretation, new `WithRadianceCoefficients` to override the VIIRS fit). Also fixed: `Floor()` never clamped a negative raw mcd/m² value before converting it (only `SQM()` did) — both paths now clamp consistently.

### Changed
- `skybrightness/atlas.LayerAuto`'s ladder is freshness-first: VIIRS (newest published year) is tried before the World Atlas, then the live lightpollutionmap.info query, then the Bortle/scalar fallbacks. This trades modelling fidelity for recency by default — the World Atlas is propagated through a radiative-transfer model but frozen at 2015, while VIIRS is a raw-radiance empirical fit published through the current year. `WithLayer(LayerWorldAtlas)` asks for fidelity explicitly, and `Result.Layer` always reports which source actually answered.
- `skybrightness/atlas`'s download functions now take `atlas.Option` (renamed from `atlas.DownloadOption`, same shape, since `Resolver` shares one flat option type with them) — `atlas` has never shipped in a tagged release, so this is free churn, not a breaking change.
- `skybrightness/lpmap`'s doc comment no longer implies a self-serve API key signup exists — the key is issued manually, one at a time, by emailing the service owner; the doc now lists the real documented `ql` layer set and units and points to `skybrightness/atlas.Resolver` as the recommended no-key default.
- `examples/18_sky_brightness` closes with a source-comparison table: the same five sites (São Paulo, London, a rural backyard, Mauna Kea, Paranal) resolved through `LayerVIIRS`, `LayerWorldAtlas`, and `LayerLightPollutionMap` separately, one `Resolver` per layer reused across sites. Reported in mcd/m² rather than mag/arcsec² on purpose: VIIRS measures a hard zero wherever its day-night band detects nothing (verified against the raw composite and cross-checked against lightpollutionmap.info's own readout), and a measured zero is a result, not a gap — but in magnitudes zero flux is `+Inf`, which tabulates as if the value were missing. The table also makes the fidelity-vs-freshness tradeoff concrete: VIIRS cannot rank dark sites at all, while the World Atlas still separates Mauna Kea from Paranal because it is a propagation model rather than a measurement.
- `examples/18_sky_brightness` now resolves its light-pollution floor through `atlas.Resolver` with an explicit `LayerBortle` (a fixed, offline, no-download estimate — the right default for a quick script) instead of a single unhandled `lpmap.New().Floor` call. This absorbs the former standalone `examples/25_light_pollution_atlas` per user request (one light-pollution example, not two) without adding `LayerAuto`'s always-attempted World Atlas/VIIRS download legs to this particular example's output — those remain one `atlas.WithLayer(atlas.LayerWorldAtlas)` away for a caller who wants real downloaded data.

## [0.12.0] — 2026-08-01
### Added
- `plan.HorizonProfile` / `Site.WithHorizonProfile`/`HorizonAt` — an optional per-azimuth horizon function, propagated through `Site.WithHorizon`/`WithTimeZone`, for a site whose sky isn't uniformly clear to a single scalar `Horizon()`. Purely additive data plumbing today — no production constraint consumes it yet (see `docs/ROADMAP.md` #29).
- `examples/21_meteor_shower_forecast` — the Perseids' real solar-longitude activity window, radiant drift, and a real hourly `ObservedRate` forecast for Paranal.
- `ephemeris/kepler` (new package) — a network-free alternative to SPK-kernel-backed ephemerides: `kepler.Elements`/`Elements.StateAt`/`SolveKepler` propagate a position directly from classical heliocentric osculating orbital elements via two-body Keplerian motion, and `kepler.Provider`/`New`/`Register`/`WithBase` adapt that into a full `ephemeris/core.Provider` that can answer any number of registered small bodies plus every SOFA-covered major body from one shared instance. Re-exported as `ephemeris.Elements`/`NewElements`/`NewMovingBodyProvider`/`NewFromElements`/`WithKeplerBase`. Elliptical orbits only (`0 <= e < 1`); no planetary perturbations, so accuracy drifts away from the elements' epoch — validated live against 433 Eros's real published elements and JPL Horizons' real (perturbed) ephemeris, measuring ~0.04″ divergence near epoch growing to ~0.56″ at ±30 days.
- `plan.FromCatalog(target, nil)` now builds a Kepler-propagated `*Asteroid`/`*Comet` automatically whenever the target carries real published elements (`HasElements`) — "Kepler as the default" for small bodies with no SPK kernel or network round trip needed. `plan.VisibleTonight` does the same for every small-body candidate it gathers, falling back to a real kernel-backed provider only when a candidate has no usable elements; the new `plan.WithSmallBodyKernels()` option forces the kernel path unconditionally. A caller-supplied provider always takes precedence over elements, unchanged.
- `examples/22_kepler_propagator` — resolves 1 Ceres's real osculating elements live from JPL SBDB via `catalog.NewResolver(catalog.SBDB)`, then hands the result straight to `plan.FromCatalog(target, nil)` — no manual `eph.Elements` construction, no SPK kernel — and runs it through `plan.VisibilityEvents` exactly like any other target.
- `constants.IAU2015.SunGravitationalParameter` (nominal solar mass parameter, IAU 2015 Resolution B3 Table 1) and `constants.IAU2015.ObliquityJ2000` (IAU 2006 Resolution B1/P03 mean obliquity at J2000.0) — the two new constants `ephemeris/kepler`'s mean-motion and perifocal-to-equatorial-frame math need; both verified live against the peer-reviewed source publications.
- `catalog/resolve.Target` gains `SemiMajorAxis`/`Eccentricity`/`Inclination`/`AscendingNode`/`ArgPeriapsis`/`MeanAnomaly`/`HasElements`, populated by `catalog/sbdb` from JPL SBDB's real `orbit.elements` response (already fetched for eccentricity alone, now decoded in full) — natively in the AU/degree units `ephemeris.Elements`'s identically-named fields expect, so a resolved target's elements drop straight into `eph.NewFromElements`/`plan.FromCatalog` with no conversion.
- `catalog/sbdb.SearchBright`'s bulk query now requests and decodes the same six orbital elements (previously only the single-object `ResolveObject` identify path did) — `plan.VisibleTonight`'s candidate pipeline reaches small bodies exclusively through the bulk path, so this is what makes `HasElements` actually populated on the targets a real caller sees, not just on a manually-looked-up single object.
- `coord.Context.BarycentricVelocity`/`BarycentricRVCorrection`/`HeliocentricRVCorrection` — barycentric/heliocentric radial-velocity correction, projecting the observer's own barycentric motion (already computed by `Apco13`'s astrometry, no new SOFA call) onto a target's line of sight. Classical (non-relativistic) projection, accurate to ~1 m/s — does not implement gravitational redshift, light-time-to-barycenter, or target proper-motion/parallax effects on the projection geometry. `examples/23_radial_velocity_correction` demonstrates the correction's annual sinusoid for Sirius.
- `coord.Context.ObservedRadialVelocity` and `plan.TargetDetails.RadialVelocity` — wires the above into `plan`'s observability pipeline. A new `plan.MeasuredRadialVelocity` capability interface (`*Star`, via `WithRadialVelocity`, now tracking a real vs. never-set RV distinctly) is dispatched in `computeDetails`, formatting both the topocentric and catalog barycentric values (e.g. `"+7.31 km/s topocentric (-5.50 km/s barycentric)"`); a caller-injected `"RadialVelocity"` prop still overrides. `examples/15_target_details/stars` picks this up automatically for any SIMBAD-resolved star with a published RV, no code change needed.
- `examples/24_optics` — an 8" f/10 SCT with a wide-field and a planetary eyepiece, a 2x Barlow, and a CMOS sensor, demonstrating every figure `optics.Telescope`/`Eyepiece`/`Sensor` computes (magnification, true field of view via both the field-stop-exact and apparent-FOV-fallback paths, exit pupil, Dawes limit, limiting magnitude, plate scale). The package itself has shipped since v0.11.0 but had no `examples/` entry until now, unlike every other feature.
- `plan.NewSiteEarthLocation(name, latDeg, lonDeg, heightMeters, opts...)` — a `Site` constructor from plain decimal-degree coordinates, so a caller building a site from numbers no longer needs to import `coord` just for `coord.NewEarthLocation` + `NewSite`.
- `plan.NewSiteEarthAddress(ctx, name, address, opts...)` — geocodes a free-text address via OpenStreetMap's Nominatim API (new `remote.Nominatim` endpoint) and its resolved coordinates against the Open-Elevation API (new `remote.OpenElevation` endpoint) to build a `Site` with no coordinates supplied by the caller at all.

### Changed
- `kepler.Provider` generalized from answering one hardcoded body to a multi-body registry: `kepler.New(id, el, opts...)` is now `kepler.New(opts...)` + `Register(id, el) error`, so a single `Provider` can answer any number of small bodies plus every SOFA-covered major body. `plan.NewAsteroidFromElements` is removed — `plan.NewAsteroid`/`NewComet` never needed a separate elements-based constructor once `FromCatalog` builds the Kepler provider externally and passes it in like any other `eph.Provider`; a standalone caller does the same via `eph.NewFromElements`. Both changes are free churn, not a deprecation cycle — neither `ephemeris/kepler` nor `NewAsteroidFromElements` shipped in a tagged release.

### Fixed
- `nil` is no longer a trap anywhere in `plan` that takes an `eph.Provider` for a body `ephemeris.Default()` can answer (Sun/Moon/Mercury-Neptune/Pluto/the barycenter) — `plan.NewPlanet` (and every convenience wrapper: `NewSun`, `NewMoon`, `NewMercury`...`NewPluto`) now defaults a nil provider to `ephemeris.Default()` directly, which cascades for free to every `plan/events.go` function built on top of them (`SunEvents`, `MoonEvents`, `CivilDawnDusk`/`NauticalDawnDusk`/`AstronomicalDawnDusk`, `FullMoonOppositions`, `NextNewMoon`, `NextFullMoon`, ...). The functions that call `eph.Position` directly instead (`plan.MoonPhases`/`Seasons`/`MoonIllumination`/`Apsides`/`LunarEclipses`/`SolarEclipses`, `plan.NewCrescentParams`, `plan.SubsolarPoint`/`SublunarPoint`/`Terminator`) get the same guard individually. `plan.VisibleTonight` resolves its `planetProvider` parameter once at the top — this closes a real nil-pointer-panic risk in its internal `moonNote` helper, which calls `eph.Position` directly and would have panicked (not just errored) on a nil `planetProvider`. `skybrightness.WithProvider(nil)` no longer overwrites `Moonlight`'s default provider with an unusable one. Constructors for bodies with no legitimate default (`NewAsteroid`, `NewComet`, `NewSatellite`, `NewGenericBody`, `NewPlanetaryMoon`) are unchanged by design — there is no default ephemeris for an arbitrary small-body/satellite ID, so `FromCatalog` still requires a real (or elements-derived) provider for those, falling through to the fixed-target path otherwise.
- `plan.FromCatalog(target, nil)` for a Sun/Moon/planet target no longer silently degrades to a static, non-moving `*DeepSkyObject` — the `NewPlanet` routing only ever ran inside FromCatalog's `if p != nil` block, so a caller who resolved a major body but didn't happen to supply a provider got a fixed-coordinate stand-in (or a zero-value one) with no error. `FromCatalog` now falls back to `eph.Default()` for any target whose ID is a major named body, regardless of whether a provider was supplied; a caller-supplied provider still always takes precedence.
- `ephemeris.Default()` no longer returns `ErrUnsupportedBody` for Pluto or the Solar System Barycenter — the two named `core.ID`s SOFA itself has no analytical model for. `SolarSystemBarycenter` is now derived directly from `gofaext.Epv00`'s already-computed barycentric Earth state; Pluto is answered via two-body Keplerian propagation (`ephemeris/kepler`) from its own real J2000.0 osculating elements (E.M. Standish, JPL/Caltech, "Keplerian Elements for Approximate Positions of the Major Planets," Table 1). `Default()`'s concrete type is now a `*kepler.Provider` built over the existing SOFA source as its base — the same generic multi-body mechanism `NewMovingBodyProvider`/`FromCatalog`'s Kepler-default wiring uses, not a one-off special case.
- `remote.EnableAllDownloads`/`DisableAllDownloads` now cover `remote.JPLHorizons` (whose small-body SPK generation is a real file download despite the endpoint being `KindAPI`), not just `KindFile` endpoints — a caller granting blanket consent previously still had every asteroid/comet ephemeris fetch silently denied. New `remote.Endpoint.Downloadable` field marks which endpoints have a download-consent gate at all.
- `catalog.Resolver`'s cross-provider field-precedence merge now preserves a resolved target's osculating orbital elements (`HasElements`/`SemiMajorAxis`/.../`MeanAnomaly`) and pairs them with their own elements-epoch — previously, `scalarFieldRules` had no rule for this cluster, so any caller going through `catalog.NewResolver` (rather than `sbdb.New()` directly) silently lost the elements SBDB had correctly populated.
- `plan.Solver.FindRoot`/`FindExtremum` no longer silently return a non-finite (NaN/±Inf) result as a success — both now guard every evaluator output and internal step computation, returning the new `plan.ErrNonFiniteEvaluation` instead. Also fixes a latent divide-by-zero in `FindRoot`'s inverse-quadratic-interpolation step-clamp when the bracket has already converged to zero width.
- `ephemeris/jpl/spk`'s DAF/SPK binary reader no longer trusts file-derived integers (record counts, summary sizes, MAXDIM/KQ table indices, Chebyshev record layout) before validating them, closing several slice-bounds/makeslice panics and an unbounded-FWD-chain hang reachable from a corrupted or truncated kernel. New `FuzzNewReaderReadSummaries`/`FuzzEvaluateSegment`/`FuzzReadDoubles` fuzz the parser on every `go test ./...` run via their seed corpus.
- `plan.FromCatalog` now routes a `resolve.KindPlanetaryMoon` candidate through `plan.NewPlanetaryMoon`, instead of silently degrading it to a `*plan.GenericBody` with no photometric model — the gap affected any caller round-tripping a moon target through the catalog layer rather than calling `NewPlanetaryMoon` directly.
- `constants.IAU2015.MercuryEquatorialRadius` corrected from the rounded 2440.5 km to WGCCRE Table 4's real 2440.53 km. `Uncertainty` is now populated with the real published 1σ values for all 8 measured WGCCRE body radii (Moon, Mercury, Venus, Mars, Saturn, Uranus, Neptune, Pluto) — verified against JPL SSD's Planetary/Satellite Physical Parameters pages, which cite the same source. Pluto's relative uncertainty (~1.35e-3) is now the package's largest, raising `TestConstants_RelativeUncertaintyIsSmall`'s gate from 1e-3 to 5e-3.
- Fixed a swapped lon/lat argument in `ephemeris/jpl/validation/observer_pipeline_test.go`'s Greenwich site (was building an equatorial site off Somalia). New `TestObserverPrecisionMatrix` (`ephemeris/jpl/validation/observer_precision_test.go`) characterizes the Astrometric→Observed pipeline against live JPL Horizons across 4 bodies, 4 sites, and up to 9 epochs each (68 comparison points): total angular separation stays bounded (measured max 2.66″) and is the metric asserted on; the Az/El split is not fully explained by a simple near-zenith projection model — see `docs/VALIDATION.md` and the test's doc comment for the open question.
- `angle.Angle.DMSString`/`HMSString` no longer render a malformed extra leading zero (e.g. `94°52'010"` instead of `94°52'10"`) when the seconds field's unrounded value is just under 10/60 but rounds up to a two-digit value — the leading-zero decision now uses the same rounded value that gets printed, instead of the raw pre-rounding one.
- `plan/nasa_eclipse_test.go`'s NASA-catalog integration tests (which fetch live pages and, for two of them, a multi-GB DE441 kernel) could hang past a short ambient `-timeout` mid-request instead of skipping, producing a confusing goroutine-dump CI failure — `fetchNASAPage` now bounds its request with an explicit `context.WithTimeout` rather than relying solely on `http.Client.Timeout`, and new `requireNASA`/`nasaBudgetOK` helpers skip cleanly (with a clear message) when the host is unreachable or too little of the ambient `-timeout` remains for another live fetch.

## [0.11.0] — 2026-07-29
### Added
- `constants.SI2019` (`c`, `h`, `k_B` — exact by the 2019 SI redefinition) and `constants.CODATA2022`/`constants.CODATA2018` (`G`, `m_e`, `m_p`, `α`, `σ_e`, each carrying its published standard uncertainty, verified against the live NIST CODATA tables) — the first fundamental physical constants in this library, published as separate per-adjustment sets rather than one silently-updated symbol so a caller can pin the CODATA vintage its reduction was made against. `constants.CODATA`/`constants.IAU` are unversioned aliases pointing at the currently-recommended vintage, so internal code and most callers never hardcode a year.
- `constants.Constant` gains `Quantity()` (bridges into `unit.Quantity` for dimensional conversion), `RelativeUncertainty()`, and `String()`; `constants.Set`/`constants.Sets()` enumerate every set and member, so a pipeline can archive the exact provenance of every constant it used.
- `unit.One` — the dimensionless unity unit, for pure ratios (WGS 84 flattening, the fine-structure constant, the radian/degree scale factors).

### Changed — BREAKING
- **`constants` now publishes typed, versioned constant *sets* instead of 20 flat untyped consts.** Each value is a `constants.Constant` (Name, Symbol, Value, Uncertainty, `unit.Unit`, Reference, Exact) inside one of five sets — `SI2019`, `CODATA2022`/`CODATA2018`, `IAU2015` (the au, the nominal mean Earth radius, and the 10 body equatorial radii, each keeping its own IAU 2012 B2 / IAU 2015 B3 / WGCCRE 2015 reference), `WGS84` (defining `a` and `1/f`), and `Derived` (the exact arithmetic and angle-conversion factors, plus the computed WGS 84 flattening). Every call site moves from `constants.X` to `constants.<Set>.<Member>.Value`, e.g. `constants.WGS84SemiMajorAxis` → `constants.WGS84.SemiMajorAxis.Value`, `constants.SunEquatorialRadius` → `constants.IAU.SunEquatorialRadius.Value`, `constants.WGS84Flattening` → `constants.Derived.WGS84Flattening.Value`. No numeric value changed. The package now imports `unit` (a peer in the primitives layer, not an upward import).
- **A `constants` value can no longer appear in a Go constant expression**, since it is now a struct field: a caller deriving a scale factor needs `var`, not `const` (`ephemeris/satellite`'s internal `kmPerAU`/`secPerDay` did).

## [0.10.0] — 2026-07-29
### Added
- `remote.Capture`/`(Scope).Restore`/`WithScope` — scoped snapshot/restore for endpoint config, download consent, offline mode, and the data directory. Fixes two related test-isolation bugs in `Reset()` (missed the data directory; over-broad revocation of consent granted at a wider scope).
- `remote.DataDirEnv` (`ASTROGO_CACHE_DIR`) — env var override for `remote.DataDir()`, between an explicit `SetDataDir` and the OS default cache directory.
- `plan.KnownSites map[string]*Site` / `plan.NewKnownSite(name) (*Site, error)` — a small built-in registry of well-known observatory sites (Mauna Kea, Paranal, La Palma, Cerro Tololo, Kitt Peak, ...), keyed by a slug and holding fully-built `*Site` values that carry the site's own MPC observatory code and aliases (`Site.MPCCode()`/`Site.Aliases()`, settable directly via new `plan.WithMPCCode`/`plan.WithSiteAliases` options) rather than a separate parallel type. `NewKnownSite` matches by name or alias, case/space-insensitive; a caller wanting a variant (a different horizon, time zone, ...) chains the returned `*Site`'s own `WithHorizon`/`WithTimeZone`.
- `plan.AngularDiameter`/`BodyEquatorialRadius`/`(*Planet).AngularDiameter` — apparent angular diameter for the Sun, Moon, and planets, auto-populating `TargetDetails.AngularSize`. New `constants/bodies.go` equatorial-radius table (IAU 2015 Resolution B3 / WGCCRE 2015).
- `coord.SubPoint`/`SmallCircle` — the geodetic point where a distant body (Sun, Moon, planet) is at the zenith, and a spherical small-circle sampler for drawing it.
- `plan.SubsolarPoint`/`SublunarPoint`/`Terminator` — day/night terminator and twilight-circle computation; `TwilightKind` gains `GeometricTwilight`/`ApparentTwilight` alongside the existing civil/nautical/astronomical kinds.
- `optics` (new package) — pure equipment-optics arithmetic (`Telescope`/`Eyepiece`/`Sensor`): magnification, true/apparent field of view, exit pupil, Dawes limit, limiting magnitude, pixel scale.
- `plan.PlanetaryMoon`/`NewPlanetaryMoon(name, provider, opts...) (*PlanetaryMoon, error)` — a dedicated type for natural satellites of planets other than Earth (Io, Titan, Triton, ...), embedding `*Asteroid` for the shared H-G photometry rather than being one directly, and looked up by name against a fixed table (`ErrUnknownPlanetaryMoon` on no match) the same way `NewKnownSite`/`NewMeteorShower` work. `plan.VisibleTonight`'s planetary-moon path (`WithPlanetaryMoons`) now constructs this type instead of a bare `*Asteroid`, so `obj.(*PlanetaryMoon)` — with a new `Parent()` accessor for the moon's planet — is distinguishable from a real asteroid.
- `resolve.KindInterstellar` — a new `Kind` for bodies confidently on a hyperbolic/parabolic orbit (1I/'Oumuamua e=1.2, 2I/Borisov e=3.36, ...), detected via `catalog/sbdb`'s new orbit-classification decoding (JPL's `orbit_class.code`/`class` field: "HYA" for a hyperbolic asteroid, "HYP" for a hyperbolic comet) plus an eccentricity margin (>1.05) confirmed necessary live: several ordinary long-period comets (e.g. C/1937 C1 Whipple, e=1.0002) carry the same "HYP" orbit_class purely from measurement/perturbation noise on a near-parabolic fit, not genuine interstellar origin. `plan.VisibleTonight` now fetches real ephemeris/magnitude for confirmed interstellar objects the same way it already does for asteroids/comets.
- `constellation.List`/`Centroid` — enumerate all 88 IAU constellations and compute a rough boundary-centroid position for one (previously only point→name `Lookup` was exported). `plan.Constellation`/`NewConstellation` wrap this into a fixed `Observable` target (e.g. `plan.ObservableWindows(constellation, ...)` to ask "when is Orion well-placed tonight"); new `resolve.KindConstellation`.
- `plan.MeteorShower`/`MeteorShowers map[string]MeteorShower`/`NewMeteorShower(name) (MeteorShower, error)` — a starter list of the 9 IMO "Class I" annual showers (Quadrantids through Ursids), keyed by slug with name/code lookup via `NewMeteorShower`, with `RadiantAt`/`IsActive` (radiant drift and activity window keyed to solar longitude, not calendar date — reusing this package's existing `Seasons` solver machinery so results are year-independent) and `ObservedRate` (predicted meteors/hour for a real site/time/sky-brightness condition, via IMO's own ZHR formula `ZHR·sin(radiant altitude)·r^(limiting_magnitude−6.5)`, composed from the same `LimitingMagnitudeConstraint` sky-brightness machinery `ScoreObservableSky` already uses). `Radiant` returns the radiant as a plain `*Star` rather than a new Observable type. New `resolve.KindMeteorShower`.

### Fixed
- `plan.VisibleTonight` never surfaced any of the four IAU dwarf planets SBDB can report beyond Pluto (Ceres, Eris, Haumea, Makemake) — its Stage-2 real-ephemeris fetch only checked for `resolve.KindAsteroid`/`KindComet`, so a `KindDwarfPlanet` candidate silently degraded to a coordinate-less, magnitude-less object and was dropped.
- `catalog/sbdb.ResolveObject` classified every object as a non-comet: JPL's `object.kind` field is a 2-character code (`"an"`/`"au"`/`"cn"`/`"cu"`), but the check compared it against the bare string `"c"`, which can never match.

### Changed
- `CLAUDE.md` documents the CHANGELOG entry format (this entry follows it) and a deprecation policy for public symbols.

## [0.9.0] — 2026-07-26

### Added
- `plan.VisibleTonight(ctx, site, night, magLimit, brightSources, planetProvider, opts...) ([]VisibleObject, error)` — answers "what's visible in the sky tonight brighter than magnitude X", composing bright-object catalog search, the Moon and naked-eye planets (including Pluto), rise/transit/peak/set timing, atmospheric extinction, and constellation lookup into one call. Covers every object category the library has a provider for: stars (SIMBAD), deep-sky objects (OpenNGC), asteroids, comets, and the five IAU-recognized dwarf planets (SBDB, via a real two-stage design — see below), the Moon, and all seven naked-eye planets plus Pluto. `magLimit` governs every category uniformly against each candidate's real, extinction-adjusted apparent magnitude, not a per-category special case. `WithMinAltitude`/`WithStep` options override the default horizon threshold and window-search cadence. `VisibleObject` embeds `resolve.Target` (Kind, Coord, VMag, H/G/M1/K1, Aliases, Provenance) plus `Constellation`/`ConstellationAbbr`, `ApparentMag`, `RiseTime`/`TransitTime`/`SetTime` (the real geometric events, zero when they don't fall within tonight's window), `PeakTime`/`PeakAltitude`/`PeakAzimuth`/`Direction` (the real best-observed instant — a genuine `TransitEstimate` numerical optimum within the object's first horizon-clearing window, always populated whenever the object is visible at all, plus a 16-point compass label for where to look), `Windows`, and a Moon-proximity `SkyNote` advisory.
- `coord.CompassDirection(az angle.Angle) string` / `(AltAz).Compass() string` — renders an azimuth as a 16-point compass label (N, NNE, NE, ..., NNW).
- `resolve.KindDwarfPlanet` — a new `Kind` distinguishing the five IAU-recognized dwarf planets (Ceres, Pluto, Eris, Haumea, Makemake) from an ordinary numbered asteroid; `catalog/sbdb` reports it for both its identify (`ResolveObject`) and bulk (`SearchBright`) paths. `plan.VisibleTonight` reports the same Kind for Pluto when it comes from the direct planetary-ephemeris path (`gatherSolarSystemCandidates`), not just when SBDB happens to surface it as a minor body.
- `catalog/simbad` now renders a friendly common name (e.g. "Sirius", "Canopus", "Rigil Kentaurus A") for `Target.Name` instead of SIMBAD's raw Bayer/Flamsteed `main_id` (e.g. "* alf CMa") whenever one is known — covering the ~150 brightest named stars. `Target.ID` is unchanged (still the raw SIMBAD identifier), so this only affects display, not identity/lookup.
- `resolve.BrightObjectSearcher` — a new provider capability (`Capabilities() []Capability; SearchBright(ctx, BrightRequest) SeqIterator[Target]`) for bulk-listing every object a provider knows brighter than a magnitude bound, alongside the existing name-based (`ObjectResolver`) and position-based (`ConeSearcher`) query shapes. Implemented by `catalog/simbad`, `catalog/openngc`, and `catalog/sbdb` (new `CapMagnitudeBrowse` capability).
- `catalog/sbdb.SearchBright` — Stage 1 of a two-stage asteroid/comet design: a cheap bulk query against a new `remote.JPLSBDBQuery` endpoint (JPL's SBDB *Query* API, distinct from the existing identify endpoint), prefiltering by absolute magnitude (H for asteroids, M1 for comets) within a margin of the requested bound (4.0 — calibrated against the largest real-world opposition-brightening correction among the well-known bright asteroids, Ceres at ≈3.3 mag), sorted brightest-first (JPL's `sort` parameter) so the per-`sb-kind` result cap (50 per kind, 100 total) always keeps the genuinely brightest candidates. `plan.VisibleTonight` does Stage 2 for each candidate that survives: a real per-body JPL Horizons SPK ephemeris fetch (`eph.NewProvider(ctx, eph.SmallBody, spkid)`, consent-gated like every other kernel download) and the actual apparent-magnitude computation, which is where `magLimit` is genuinely enforced for these bodies. Both stages run concurrently (`golang.org/x/sync/errgroup`) — Stage 1 across every registered source, Stage 2 across candidates bounded to 8 in flight at once (considerate of JPL Horizons, not just fast) — and the final per-candidate evaluation (windows, rise/transit/peak/set, extinction) runs concurrently across every CPU core, since by that point it's pure in-memory work with no network dependency left.
- `catalog.NewProvider(source Source) (Provider, error)` — constructs a single catalog provider by `Source` constant directly, without importing its subpackage (`catalog/simbad`, `catalog/openngc`, ...); callers needing a provider's narrower capability (e.g. `resolve.BrightObjectSearcher`) type-assert the result, the same way `catalog.Resolver` itself detects `resolve.ConeSearcher` support internally. `NewResolver` now uses this internally instead of duplicating its provider-construction switch.
- `constellation` (new package) — `constellation.Lookup(pos coord.ICRS) (name, abbreviation string, err error)`, IAU constellation boundaries sourced from the public CDS/VizieR VI/49 catalog (Davenhall & Leggett 1989, derived from Delporte 1930's official boundaries) via an independent ray-casting point-in-polygon implementation.
- `examples/20_whats_visible_tonight` — an end-to-end demonstration of `plan.VisibleTonight` across every category, including Rise/Transit/Peak/Set timing and compass Direction per result. Its results now render as a box-drawn, color-coded table (colors skipped when `NO_COLOR` is set).
- `eph.Moons` — a new ephemeris `Source` for natural planetary satellites (NAIF's per-planet SPK kernels, e.g. `jup365.bsp`, `sat441.bsp`), distinct from the pre-existing `eph.Satellites` (artificial, TLE/SGP4-based). `eph.NewProvider(ctx, eph.Moons, "sat441")` fetches the named kernel from NAIF's `generic_kernels/spk/satellites/` directory — no separate base planetary kernel is needed alongside it, since these kernels already carry the Sun/Earth/planet-barycenter chain needed for geocentric and heliocentric geometry.
- `plan.WithPlanetaryMoons()` — a `VisibleTonightOption` (off by default) adding the 21 major, IAU-named natural satellites of Mars/Jupiter/Saturn/Uranus/Neptune/Pluto (Phobos, Deimos, the four Galilean moons, Saturn's eight major moons, Uranus's five major moons, Triton, Charon) as candidates, reported as the new `resolve.KindPlanetaryMoon` and priced through the same heliocentric H-G reflectance model `plan.Asteroid` already implements (H sourced from JPL Horizons' own published `V(1,0)` physical-parameter data, cross-checked against independent secondary sources for the handful of bodies Horizons doesn't publish it for — see `plan/moons.go`'s doc comment for the full sourcing). Off by default rather than size-gated like everything else this library downloads: the kernels covering these bright, named moons range from ~64 MB (Mars) to ~1.1 GB (Jupiter), ~2.4 GB combined, with no smaller official alternative — each kernel still requires the same `remote.EnableDownloads(remote.NAIFSPK, maxSize)` consent as any other, this option only controls whether `VisibleTonight` asks for them at all.

### Fixed
- `catalog/simbad`'s `ParseCSV` looked up the V-band magnitude column as `"v"` (lowercase); SIMBAD's real TAP response names it `"V"` (uppercase). `VMag`/`HasVMag` were silently never populated from any live response for name-based resolution.
- `catalog/simbad`'s `mapSimbadKind` only recognized `"Star"`, `"V*"`, and `"Em*"` as stellar object types, mapping every other real SIMBAD otype to `KindOther` — live testing showed ordinary bright stars (Sirius, Canopus, Vega, Rigel, ...) actually come back as `"SB*"`, `"PM*"`, `"dS*"`, `"s*b"`, and similar, none of which matched. Now recognizes any otype containing `"*"` (SIMBAD's own nomenclature convention for single-star classifications) as `KindStar`, with `"**"` (double/multiple star system) mapped to `KindDoubleStar` instead.
- `ephemeris/jpl/spk.CacheAPI` (used by any `eph.NewProvider(ctx, eph.SmallBody/Asteroids/Comets, designation)` call, not just the new `plan.VisibleTonight` path) sent a bare designation/SPK-ID as Horizons' `COMMAND` parameter. The real API rejects this outright ("requested IOBJ=... is out of bounds") for any ID past the numbered-asteroid record range (~895910) — which every real SBDB SPK-ID (asteroids: 2000000+number; comets: their own ranges) always is; Horizons' own error message documents the fix (wrap it as `"DES=<id>;"`), with a further `"DES=<id>;CAP"` escalation needed for comets specifically. `CacheAPI` now tries the bare form only when the designation is plausibly a short in-range one (preserving existing behavior for e.g. `"433"`, where `"DES=433;"` alone actually resolves to a different, unrelated body), and skips straight to `DES=` otherwise — halving live Horizons round trips for the common real-SPK-ID case in the process.
- `ephemeris/jpl/spk`'s type 21 (Extended Modified Difference Array) segment reader — used by every small-body SPK Horizons generates (every real asteroid/comet ephemeris fetch, not just `plan.VisibleTonight`'s) — assumed a fixed difference-table size (MAXDIM=15) and a block-grouped `[px,py,pz,vx,vy,vz]` record layout. Real records store a per-segment MAXDIM (confirmed against live Horizons-generated kernels; SPK type 21 exists specifically to allow a variable table size, unlike type 1's fixed 15) and interleave reference position/velocity per axis (`[px,vx,py,vy,pz,vz]`), plus a separate integration order per axis rather than one shared order — so the old code silently read a velocity component (km/s) as if it were a position component (km) and used the wrong record boundaries once any difference-table data followed. This corrupted every real small-body position enough to make `plan.Asteroid.ApparentMagnitude`'s derived heliocentric distance collapse toward zero (e.g. Vesta returning apparent magnitude ≈ -3, impossible for a body whose true best-ever brightness is ≈ +5.1) — affecting every caller of small-body ephemerides, not just magnitude. The reader now reads MAXDIM per segment and decodes the record per the real (interleaved, per-axis-order) layout; verified both against a hand-built synthetic record and live against a real cached Horizons kernel (433 Eros now resolves to a heliocentric distance of ≈1.63 AU, within its true 1.13–1.78 AU orbital range).

## [0.8.0] — 2026-07-22

### Added
- `catalog.Resolver` now cross-matches and merges every registered provider's hit for a query into one `Target`, instead of `Resolve` returning whichever provider answered first and `Search` deduplicating only on the useless `Catalog+":"+ID` key (which can never catch a cross-provider duplicate). Cross-matching is by shared alias/ID first (union-find over `Target.ID`/`Target.Aliases`), falling back to angular separation — after epoch-normalizing each candidate to J2000 via the new `coord.PropagateEpoch` — for candidates with no alias/ID overlap. `catalog.Gaia`/`catalog.VizieR`, whose `Resolve`/`Search` are permanently stubbed (neither does name-based lookup), are now bridged in via their `resolve.ConeSearcher` capability around each group's anchor position, so their astrometry can participate in a merge instead of being reachable only through a separate `ConeSearch` call. Each merged field is chosen by a per-field provider-precedence table (Coord/Parallax/PmRA/PmDec are treated as one coupled cluster, taken from a single provider, never mixed field-by-field); `Target.Provenance map[string]string` records which provider (`Provider.Name()`) contributed each field, nil for a `Target` sourced from a single provider.
- `(*catalog.Resolver).PositionMatchThreshold(threshold angle.Angle) *Resolver` — sets the maximum angular separation, after epoch normalization, at which two `Target`s are considered the same object (default 2 arcsec); returns the receiver for chaining, e.g. `catalog.NewResolver(catalog.SIMBAD).PositionMatchThreshold(angle.Arcsec(2)).Resolve(ctx, "...")`.
- `(*catalog.Resolver).Limit(n int) *Resolver` — sets `Search`'s maximum result count (default 10, previously hardcoded); also chainable.
- `coord.PropagateEpoch(c ICRS, fromEpoch, toEpoch time.Time) (ICRS, error)` — rigorous SOFA (`Pmsafe`) space-motion propagation of an ICRS position's proper motion/parallax/radial velocity from one epoch to another; a zero epoch is treated as `time.J2000`. `internal/gofaext.Pmsafe` wraps the underlying SOFA call.
- `time.J2000` — the standard epoch J2000.0 (JD 2451545.0 TT).
- `resolve.Kind` constants `KindAsteroid`, `KindComet`, `KindSatellite`, canonicalizing string values `catalog/sbdb` and `catalog/norad` already used informally as bare `resolve.Kind("Asteroid")`/`"Comet"`/`"Satellite"` literals.

### Changed — BREAKING
- **`resolve.Provider`'s `Resolve`/`Search` methods now take `ctx context.Context` as their first parameter.** This ripples into every catalog provider package (`simbad`, `mast`, `jpl`, `sbdb`, `norad`, `gaia`, `vizier`, `openngc`) and every caller — a provider or `Resolver` that built its own internal `context.Background()`/`context.TODO()` now forwards the caller's `ctx` instead, so cancellation propagates end-to-end, including into the `ConeSearch` bridge calls above. `catalog.Resolver.Resolve`/`Search` already took a `ctx`; this closes the gap at the interface's own layer rather than only at the top-level orchestrator.

### Fixed
- `catalog/gaia`'s CSV row parser discarded RA/Dec `ParseFloat` errors and still reported `HasCoord: true`, so a malformed or empty position silently became a fake `(0, 0)` reported as real. Rows with an unparseable RA or Dec are now skipped entirely.
- `catalog/mast`'s XML/JSON decode set `HasCoord: true` unconditionally, independent of whether a `<ra>`/`<dec>` (or `ra`/`decl`) field was actually present in the response, for the same reason as above. RA/Dec now decode into presence-aware (`*float64`) fields, and `HasCoord` is only set when both are genuinely present.
- `catalog/mast`'s `Target.Catalog` held the relayed sub-resolver name (`"NED"`, `"SIMBAD"`, `"VizieR"` — whichever service MAST's `Mast.Name.Lookup` internally answered from) instead of `"mast"`, inconsistent with every other provider setting `Catalog` to its own name. `Catalog` is now always `"mast"`; the relayed resolver name is preserved as an `Aliases` entry instead of being discarded.
- `catalog/mast` never set `Target.Epoch`; it now defaults to `time.J2000` (SIMBAD/NED name-lookup responses are conventionally J2000 — a documented best-effort assumption, since the API doesn't report which sub-resolver actually answered).
- `catalog/vizier`'s `ConeSearch` never populated `Target.ID`, even though the same designation value was already used for `Name`/`Designation`. `ID` is now set from the same value.
- `catalog/vizier`'s `ConeSearch` never set `Target.Epoch`, despite its three registered tables having genuinely different native reference epochs (2MASS ~J2000, Hipparcos J1991.25, Gaia DR3 J2016.0). Each table's schema (`tables.go`) now carries its own `Epoch`, stamped onto every row it produces.
- `catalog/openngc` never set `Target.Epoch`, despite its RA/Dec being J2000 by the catalog's own convention. Rows now carry `Epoch: time.J2000` explicitly.

## [0.7.0] — 2026-07-21

### Added
- `remote.EnableAllDownloads(maxSize int64)` / `remote.DisableAllDownloads()` — grant or revoke file-download consent for every registered `KindFile` endpoint (`IERSFinals2000A`, `NAIFSPK`, `NAIFLSK`, `OpenNGC`) at once, instead of calling `EnableDownloads`/`DisableDownloads` once per endpoint.

### Changed — BREAKING
- **`time.Fetch`, `time.FetchIfStale`, and `time.LoadFS` are removed.** Earth Orientation Parameters now load automatically and lazily the first time they're needed — any `Time.EOP()`, `Time.UTC()` (UT1 branch), or `Time.UT1()` call that finds the registered model doesn't cover the requested epoch now: (1) reads and parses whatever `finals2000A.data` file already exists at the standard cache path, no network access and no consent required (same rule every other pre-seeded astrogo data source already follows); (2) if that doesn't help, and `remote.EnableDownloads(remote.IERSFinals2000A, ...)` (or `EnableAllDownloads`) was called, fetches over the network; (3) otherwise degrades to the existing zero-EOP-plus-one-time-warning fallback, unchanged. `time.RegisterModel`/`GetModel`/`Coverage`/`ParseFinals2000A`/`SetRetryCooldown` are unaffected. Note: a process that never touched EOP data before now performs a disk read (and possibly a network fetch, if consent was already granted elsewhere) on its very first `Time.EOP()`/`Time.UTC()`/`Time.UT1()` call — previously this was a pure, instant no-op against the zero-value default model.

### Tests
- `plan`'s USNO-API integration tests (`-tags=integration`) now fast-skip the whole suite (~5s) instead of hanging for up to 10 minutes when the USNO API is unreachable — each of the ~10 test functions previously waited up to 30s on its own before skipping, and those waits summed sequentially past CI's job timeout during a full outage. A once-per-process TCP reachability pre-check now short-circuits every USNO-hitting test immediately.

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

| Metric                                          | Result                                  |
| ----------------------------------------------- | --------------------------------------- |
| sHG1G2 vs FINK phunk (8467 Benoitcarry, r-band) | mean Δ=0.011 mag, 100% within 0.025 mag |
| 2026 Eclipses vs NASA                           | all 4 within ≤1 min                     |
| 2024–2033 Seasons vs USNO                       | all within ≤1 min (41/41 tests)         |
| Orbital eccentricity                            | e=0.016671 (matches IAU)                |
| Topocentric Moon parallax                       | ~1° correction applied                  |

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

| Metric                  | v0.1.1         | v0.1.2                           |
| ----------------------- | -------------- | -------------------------------- |
| Sun rise/set vs USNO    | <1.3 min       | **≤0.5 min**                     |
| Moon rise/set vs USNO   | <1.6 min       | **≤0.6 min**                     |
| USNO integration tests  | 41/41          | 41/41                            |
| AstroPixels moon phases | 44,524 matched | 44,524 matched (mean Δ=1.87 min) |
| NASA lunar eclipses     | 1,424/1,424    | 1,424/1,424 (mean Δ=0.8 min)     |
| NASA solar eclipses     | 1,383/1,383    | 1,383/1,383 (mean Δ=0.8 min)     |

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

| Operation                             | Cost        | Allocs |
| ------------------------------------- | ----------- | ------ |
| `coord.NewContext` (SOFA Apco13)      | 91 µs       | 1      |
| `ICRSToAltAz` (cached Context)        | 325 ns      | 1      |
| 100-star batch (cached vs scalar)     | 73× speedup | —      |
| Time scale conversion                 | 18–90 ns    | 0      |
| Refraction (rigorous)                 | 14 ns       | 0      |
| Scheduler (100 blocks, SwapOptimized) | 123 ms      | linear |

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
