# Sky Brightness V2 — Design

**Status:** Phase 0 design document. This is the internally-consistent architecture the
implementation phases below build against. It supersedes the v1 `skybrightness` package
entirely — there is no backward compatibility (see §15).

This document is the single source of truth for *why* the package is shaped the way it is.
Individual package doc comments explain *how* to use a given type; this file explains the
decisions that made that shape inevitable, so a future maintainer does not have to re-derive
them from source.

---

## 1. Scientific scope and non-scope

Sky Brightness V2 predicts ground-observed spectral sky radiance

```
L_λ(λ, altitude, azimuth, site, epoch)      W·m⁻²·sr⁻¹·nm⁻¹
```

for arbitrary terrestrial sites, arbitrary horizontal directions, point or all-sky queries,
spectral or passband-integrated output, decomposed into natural and anthropogenic components,
under clear, partly-cloudy, or overcast skies, in climatological, historical, nowcast, or
forecast atmospheric states, with uncertainty and full provenance attached to every result.

**Six quantities the package never treats as interchangeable**, each with its own named,
unit-tested conversion function (§3):

| Quantity | What it actually is | How V2 gets from it to sky brightness |
|---|---|---|
| Satellite upward radiance (VIIRS-DNB) | Radiance leaving the ground toward a satellite, one ~15″ pixel, one small ground patch | An `EmissionField` input to §9's clear/cloudy propagation — never a direct SB conversion (§9, §25) |
| V-band surface brightness | A photometric magnitude system, tied to the Johnson V passband | One of many `Passband` outputs of `IntegrateRadiance` (§3) — never the implicit default |
| SQM reading | An instrument measurement through a specific, imperfect device response curve | A `calib.Measurement` with an `Instrument` model (§10 Phase 7) — never assumed equal to Johnson V |
| Luminance | A photopically-weighted radiometric quantity (cd/m²) | `PhotopicLuminance`/`ScotopicLuminance` (§3) — a derived output, not an input unit |
| Horizontal irradiance | An integral of radiance over the visible hemisphere | `HorizontalIrradiance` (§3) — a derived output |
| Limiting magnitude | A function of sky background *and* an explicitly chosen visual/detector model | `LimitingMagModel.LimitingMagnitude(...)` (§5) — the model is always named in `Provenance` |

**Explicitly out of scope for this design**: a GUI (ROADMAP's own Non-Goals rule this out
project-wide); a from-scratch reimplementation of a validated commercial radiative-transfer
package (LPTRAN/SkyGlow); a neural-network runtime in the first surrogate pass (§10 Phase 6);
running MCARaTS or any external RT solver from Go (§13); claiming any accuracy target has been
*met* before real field data exists in this repo (§12).

---

## 2. Mathematical decomposition

Total spectral radiance is additive **in linear space**:

```
L_total(λ) = L_airglow(λ) + L_zodiacal(λ) + L_starlight(λ) + L_diffuse_galactic(λ)
           + L_moon_scattered(λ) + L_twilight(λ) + L_artificial(λ) + L_aurora(λ)
```

Magnitudes are logarithmic; summing them is a correctness bug (this was already stated in
v1's `doc.go` and remains true — the difference in V2 is that it now applies per-wavelength,
not just to one V-band scalar). Every `Component` (§5) returns `SpectralRadiance` on the
caller's `SpectralGrid`; the engine sums components into `Result.Total` before any
magnitude conversion happens, and `Result.Components` retains the individual linear-space
contributions for inspection.

The artificial component is itself a pipeline, not a single equation:

```
E(position, λ, upward_direction, t) = intensity(position, t)
                                     × spectral_mixture(position, λ, t)
                                     × angular_emission(position, upward_direction, t)

L_artificial(λ) = Propagate[E](λ, observer_direction, atmosphere.Atmosphere)
```

**VIIRS-DNB supplies `intensity` only.** It has one broad panchromatic band (~500–900 nm) with
essentially no spectral resolution and no view-angle diversity per pixel — it cannot constrain
`spectral_mixture` or `angular_emission`. Treating a VIIRS pixel as if it already encoded
brightness (v1's `RadianceToArtificialSB`, a single log-linear fit) conflates three physically
distinct factors into one empirical scalar. V2 keeps them as three pluggable, independently
uncertain models (§9) and reserves the actual radiative-transfer integral — the `Propagate[]`
step — for the RT engines in §9.

---

## 3. Units and canonical types

All internal computation is in SI (or SI-derived, documented) units. Every physical quantity
is a distinct Go type, declared in package `unit` (`unit/quantity_types.go`) and used with the
`unit.` prefix throughout `skybrightness` and its siblings — so a radiance can never be
assigned to an irradiance by accident:

| Type | Meaning | Unit |
|---|---|---|
| `unit.WavelengthNM` | vacuum wavelength (unless the source states air) | nm |
| `unit.SpectralRadiance` | the primary quantity of the whole engine | W·m⁻²·sr⁻¹·nm⁻¹ |
| `unit.PhotonSpectralRadiance` | photon-counting analogue | photon·s⁻¹·m⁻²·sr⁻¹·nm⁻¹ |
| `unit.SpectralIrradiance` | | W·m⁻²·nm⁻¹ |
| `unit.Radiance` | passband-integrated | W·m⁻²·sr⁻¹ |
| `unit.PhotonRadiance` | | photon·s⁻¹·m⁻²·sr⁻¹ |
| `unit.Irradiance` | | W·m⁻² |
| `unit.LuminanceCdM2` | photopic/scotopic derived output | cd·m⁻² |
| `unit.SurfaceBrightnessAB` | always paired with a `PassbandID` | AB mag·arcsec⁻² |
| `unit.SurfaceBrightnessVega` | always paired with a `PassbandID` + Vega zero-point version | Vega mag·arcsec⁻² |
| `unit.Transmission` | | [0, 1] |
| `unit.OpticalDepth`, `unit.AerosolOpticalDepth` | vertical unless documented as slant | ≥ 0 |
| `unit.SingleScatteringAlbedo` | | [0, 1] |
| `unit.AsymmetryParameter` | Henyey–Greenstein g | [−1, 1] |
| `unit.AngstromExponent` | | dimensionless |
| `unit.CloudFraction` | **sky cover** — not optical depth, not opacity | [0, 1] |
| `unit.CloudOpticalDepth` | at 550 nm unless stated | ≥ 0 |
| `unit.EffectiveRadiusUM`, `unit.OzoneColumnDU`, `unit.PrecipitableWaterMM`, `unit.PressureHPa`, `unit.TemperatureK`, `unit.AltitudeM`, `unit.SpectralAlbedo`, `unit.ElectronsPerPixelPerSecond` | as named | as named |

`unit.CloudFraction` and `unit.CloudOpticalDepth` are deliberately distinct types (§8) so the
mandate's "never collapse cloud state to one scalar" rule is enforced by the compiler, not by
convention.

**Revised, one release later**: these types used to be re-declared as a 26-member type-alias
block in `skybrightness/units.go` (`type SpectralRadiance = unit.SpectralRadiance`, and so on)
purely so in-package code could write the short name instead of the `unit.`-qualified one.
That file was removed: every one of these types already appears in nearly every function
signature across `skybrightness`, `skybrightness/natural`, `skybrightness/atmos`, and
`skybrightness/dataset/passband`, all of which already import `unit` directly for other
reasons, so the alias layer was pure indirection with no real ergonomic win, and it created a
second name for the same identity most readers only half-remembered. `unit` was already the
single source of truth for these types before the removal (`unit.Dimension`'s doc comment has
said so since before this round); the removal just stopped `skybrightness` from also
re-exporting them under its own name.

**The explicit integration path** — one exported function per arrow, nothing implicit:

```
spectral radiance ──ToPhoton──▶ photon radiance ──IntegrateRadiance──▶ passband radiance
        │                                                                     │
        │                                                          ABSurfaceBrightness
        │                                                                     ▼
        └─────────────────────────────────────────────────▶ AB mag/arcsec² ──▶ VegaSurfaceBrightness ──▶ Vega mag/arcsec²
                                                                                                              │
                                                                                              (Instrument, §5) ▼
                                                                                          detector background e⁻/s
```

`IntegrateRadiance`/`IntegratePhotonRadiance`/`BandMeanSpectralRadiance` resample a
`Passband.Response` onto the caller's `SpectralGrid` by linear interpolation; partial spectral
coverage sets `QualityFlagPassbandTruncated`, and coverage below 99% of the response integral
is a hard error — never a silent truncation. `ABToBandMean` is the round-trip inverse, tested
to < 1e-12 mag (§12).

**Where units live: a deliberate hybrid, not `unit.Quantity` throughout.** The bespoke named
`float64` types (the table above) carry type safety and appear in every hot loop — an all-sky
evaluation is 10⁴ directions × 10²–10³ wavelengths and must stay `[]float64`-shaped with zero
per-element indirection; `unit.Quantity` is a struct and does not fit that shape. These types
are declared once, in `unit/quantity_types.go` — not re-declared in `skybrightness` — alongside
`unit.Watt`/`Joule`/`Hertz`/`Nanometre`/`Steradian` and composed `unit.Irradiance`/`Radiance`/
`SpectralRadiance`/`SpectralIrradiance`/`Luminance` vars, which are used *only* for
documentation, provenance serialization, and boundary parsing — never in the numeric core. This
is stated plainly rather than implied: `unit.Dimension` has only the 7 SI base exponents and no
tag distinguishing a solid angle from a bare dimensionless ratio (`unit.Radian` and `unit.One`
already collide by the package's own documented design). Adding `Steradian` gives it **no**
type-level protection against a radiance silently cancelling into an irradiance — claiming that
protection in the docs would be a lie. `unit/doc.go` states this explicitly: "Steradian is
dimensionally identical to One, exactly as Radian is... Real radiometric type safety instead
comes from the zero-cost quantity TYPES declared in quantity_types.go — Radiance, Irradiance,
SpectralRadiance, LuminanceCdM2, and their neighbors... These live directly in this package
(not duplicated into a consumer package) specifically so a hot numeric loop — e.g.
skybrightness's spectral sky-radiance engine... — can use them with zero struct overhead."

**Photometric constants, split by `constants/doc.go`'s own scope rule** (a fixed single value
from a cited authority belongs in `constants`; anything model-dependent or series-valued does
not — the same test already used for `ObliquityJ2000`/`SunGravitationalParameter`):

| Value | Home | Why |
|---|---|---|
| AB zero point, 3631 Jy (Oke & Gunn 1983) | `constants.Photometric` (new set) | fixed defined value |
| Stefan–Boltzmann σ, Wien displacement constant | `constants.Derived` | exactly derivable from the SI2019 defining constants already in `constants` |
| Vega zero points | `skybrightness`, from the passband bundle | depends on *which* Vega spectrum (Hayes vs. CALSPEC alpha_lyr_stis) — series-valued, out of `constants` scope |
| Solar spectral irradiance | a dataset | a spectrum, not a constant |
| Falchi 2016 natural zenith, 0.171168465 mcd/m² | removed (2026-08-09) | carried forward from v1's `NaturalZenithMcdM2` as `natural.FalchiNaturalZenithLuminance`, but never gained a consumer — deleted as dead code rather than kept as an unused re-export; reintroduce at the point of use if a future natural-sky component needs it |
| Garstang nL↔mag constants (34.08, 20.7233) | `natural` package, unexported | only `ConstantAirglow`/`VBandMoonlight` may use them; not public API |

**Passbands, with zero `go:embed`.** CLAUDE.md forbids `go:embed` project-wide, and §5/§25
forbid undocumented embedded response curves regardless. Core defines `PassbandSet`
(`Get`/`List`/`Version`) plus `TopHat`/`Gaussian` analytic constructors — so unit tests never
need real data or a network call. Three real providers live in `skybrightness/dataset/passband`
and are the only place passband curves ever touch the filesystem or network:

- `passband.OpenBundle(path)` — a versioned, checksummed bundle directory (one CSV or FITS
  BinTable per curve, plus a `manifest.json` carrying SHA-256s, provenance, licence, and Vega
  zero points). Deterministic, offline. The primary path.
- `passband.Remote(ctx, opts)` — `remote.GetFile(remote.PassbandBundle)` (consent-gated,
  `KindFile`), checksum-verified, then `OpenBundle`.
- `passband.FromFITS(r io.ReaderAt)` — reads an SVO Filter Profile Service-style FITS BinTable
  using the existing `fits` package with zero changes to it.

Licensing (Gaia curves are ESA CC BY-SA and need attribution; SDSS/SVO terms) is verified
before any bundle ships — a Phase 0 checklist item, not assumed clear (§16).

---

## 4. Package architecture

```
skybrightness/                 PURE core: types, interfaces, composite engine, derived outputs
skybrightness/natural/         PURE: airglow, zodiacal, starlight, diffuse-galactic, moon,
                                twilight, aurora, constant_airglow, vband_moonlight
skybrightness/atmos/           PURE: molecular, aerosol, cloud, terrain, transmission
skybrightness/artificial/      PURE: emission, spectral_mixture, angular emission, aggregation
skybrightness/rt/              PURE: clear-sky, cloudy, fast-cloud approximation
skybrightness/surrogate/       PURE: reference-data format, inference, basis, domain checking
skybrightness/calib/           PURE: measurement schema, instrument models, fitting, splits
skybrightness/dataset/         IO tier: source metadata, download/extract/validate pattern
skybrightness/dataset/raster/  pure-Go GeoTIFF engine (carried from v1's atlas/{geotiff,grid,sample}.go)
skybrightness/dataset/granule/ HDF5 reader — the ONLY importer of github.com/scigolib/hdf5
skybrightness/dataset/blackmarble/  VNP46/VJ146 readers (NASA Black Marble)
skybrightness/dataset/eog/     EOG VIIRS annual/monthly readers
skybrightness/dataset/worldatlas/   Falchi 2016 — reference/validation dataset only
skybrightness/dataset/passband/     passband bundle provider (remote + fits + disk cache)
skybrightness/dataset/atmostate/    CAMS/ERA5/local atmospheric-state providers
skybrightness/lpmap/           live lightpollutionmap.info cross-check client (kept, re-scoped)
```

This widens v1's two-tier split (pure core / `atlas`+`lpmap` IO siblings) rather than
replacing it — `skybrightness/importgraph_test.go` already machine-enforces that core never
imports its siblings, only the reverse. V2 keeps that shape and adds four more rules, all
machine-enforced in the rewritten `importgraph_test.go`:

1. Core `skybrightness` imports only stdlib, `angle`, `vector`, `unit`, `constants`, `time`,
   `coord`, `ephemeris`, `atmosphere`, `internal/parallel`.
2. `natural`/`atmos`/`artificial`/`rt`/`surrogate`/`calib` import core and each other along a
   declared DAG (`rt` → `atmos` + `artificial`; nothing else imports `rt`). None import
   `dataset/*` or `lpmap`.
3. Only `dataset/...` and `lpmap` import `remote`, `fits`, `net/http`, `github.com/scigolib/hdf5`.
4. `plan` imports core `skybrightness` **only** — never `natural`, `atmos`, `rt`, `dataset/*`,
   or `lpmap`. Engines are assembled by the application (examples, user `main`) and injected
   into `plan`, exactly as `plan` never imported `atlas` in v1.
5. `constants`, `unit`, `atmosphere` never import anything under `skybrightness`.

Core stays the one dependency `plan` sees, preserving the CLAUDE.md rule that an HDF5-scale
dependency never reaches a `plan` user's build. **Revised from the original Phase 0 sketch**:
the rich atmospheric-state *data* type (`atmosphere.Atmosphere`, built via
`atmosphere.NewBuilder()`) lives in the peer `atmosphere/` package, not in core
`skybrightness` — general aerosol/cloud/surface-optical atmospheric state is not
sky-brightness-specific (a future weather or seeing constraint needs the identical type), and
canonicalizing it where a peer package already sits avoids the alternative of skybrightness
owning a concept it doesn't exclusively use. `atmos` still owns the *optics* (transmission
models consuming `atmosphere.Atmosphere`); `dataset/atmostate` still owns the *loaders*.
`skybrightness` references `atmosphere.Atmosphere` directly in `Request`/`EvalInput` — no
alias — matching how `coord.Context` is already referenced directly rather than aliased;
general data-provenance primitives (`SourceRef`/`Fidelity`/`TimeRange`/`DatasetVersion`) live
in `atmosphere` too, aliased into `skybrightness` only because they also appear pervasively in
`skybrightness`'s own `Provenance`/`ComponentProvenance`/`Passband` types.

**Revised again, one release later**: `atmosphere.Atmosphere` and `atmosphere.Refraction`
swapped names. The narrow, refraction-model-input struct (`Model`/`Pressure`/`Temperature`/
`Humidity`/`Wavelength`) used to be exported as `Atmosphere`; the rich, Builder-validated
aerosol/cloud/surface-optical/provenance type used to be exported as `State`. Both names now
match what a reader expects: `atmosphere.Atmosphere` is the rich type, `atmosphere.Refraction`
is the small one every hot `coord`/`plan` refraction call site still constructs as a cheap
literal. `atmosphere.Atmosphere` composes an embedded `atmosphere.Refraction` as its own
surface-conditions field (`Atmosphere.Surface()` returns Kelvin; `Atmosphere.Refraction()`
returns the embedded `Refraction` value directly, in `Refraction`'s own Celsius convention —
the one explicit unit conversion this composition needs, at the single boundary
`Builder.Surface` owns) rather than duplicating pressure/temperature as separate raw fields.
This was a deliberate, same-release hard break with no deprecation alias for the old
`atmosphere.Atmosphere` name — Go cannot alias one identifier to two different meanings at
once, so freeing "Atmosphere" for the richer type necessarily retired the refraction struct's
old name immediately, an explicit, stated exception to CLAUDE.md's normal 2-release
deprecation policy (see `atmosphere/doc.go` and the CHANGELOG's `### Changed — BREAKING`
entry). `coord.Context.Atmosphere()`/`plan.Site.Atmosphere()` were renamed to `.Refraction()`
to match — both have always returned the refraction-input struct, never the richer type.

---

## 5. API proposal

```go
type Engine interface {
    Algorithm() AlgorithmRef
    Evaluate(ctx context.Context, req Request) (Result, error)
    EvaluateBatch(ctx context.Context, req BatchRequest) (BatchResult, error)
}

type Request struct {
    Astro      *coord.Context      // ONE per epoch, reused — hard repo convention
    Directions []coord.AltAz
    Grid       SpectralGrid
    Passbands  []*Passband
    Mode       Mode
    Atmosphere *atmosphere.Atmosphere
    Selection  ComponentSelection
    Options    EvaluationOptions
}

type Result struct {
    Grid         SpectralGrid
    Directions   []coord.AltAz
    Total        SpectralField
    Components   ComponentResults
    Transmission SpectralField      // empty unless Options.ComputeTransmission
    Derived      DerivedQuantities
    Uncertainty  UncertaintyResult
    Provenance   Provenance
    Quality      QualityFlags
}
```

`Component` is the unit of physics: `ID()`, `Algorithm()`,
`Eval(ctx, EvalInput, out SpectralField) (ComponentReport, error)` — implementations must be
concurrency-safe after construction, must not retain any input slice, and must not read `out`
(the engine pre-zeroes it). `ComponentSelection.Materialize` controls memory: when false,
components accumulate straight into `Total` and their own `SpectralField`s are never
allocated — proven bit-identical to `Materialize: true` in a Phase-1 invariant (§12). `Result`
uses fixed-size, `ComponentID`-indexed arrays (`ComponentResults`), never a map, in the return
path — maps and interface dispatch are reserved for setup, not the inner numeric loop.

Batch evaluation reuses `internal/parallel.MapChunked`, which constructs a worker (one
`Scratch` + one cloned `coord.Context`) **once per goroutine**, not once per direction —
exactly the primitive `coord.Context.ReduceBatchParallel` already uses for the same reason:

```go
parallel.MapChunked(len(req.Astro), workers,
    func() *Scratch { return NewScratch(nDir, nLambda) },
    func(s *Scratch, i int) error { out[i], err = e.evaluateOne(ctx, req.at(i), s); return err })
```

A convenience point API covers the "simple" half of the dual requirement without forcing every
caller through the full batch machinery: `Point(ctx, Engine, PointQuery) (PointResult, error)`,
where `PointQuery{Astro, Direction, Passband, Mode, Atmosphere, Grid, Components,
ComputeTransmission, LimitingMag}` and `PointResult{AB, Vega, Radiance, Luminance, Sigma,
AnthroRatio, Quality, Provenance, Components, Transmission, LimitingMagnitude,
HasLimitingMag}`. **Revised, one release later**: `ComputeTransmission` and `LimitingMag` were
added to close the gap that used to force even a single-point caller back onto `Evaluate`
directly whenever it wanted transmission or a limiting magnitude — the two derived quantities
`examples/18_sky_brightness` actually needs. `Point` now builds its own `Request` via
`RequestBuilder` (below) internally, rather than a second, parallel construction path, and
surfaces `IntegrateRadiance` failures as errors instead of silently returning a zero radiance.

**`EvaluationOptions` construction — `RequestBuilder`, one release later**: `EvaluationOptions`
is grouped into three cohesive sub-structs — `DerivedOptions{Mask, LimitingMag, Instrument}`,
`UncertaintyOptions{Mode, Samples, Seed}`, `PerformanceOptions{Parallelism, ScatteringOrders,
Buffers}` — cutting the flat option surface from 12 fields to 6, each self-explaining by group
name. `NewRequestBuilder(astro, directions, grid) *RequestBuilder` is the recommended,
documented way to assemble a `Request` (mirroring `atmosphere.NewBuilder()`'s established
pattern) via chained methods (`.Atmosphere(...)`, `.Mode(...)`, `.Derive(...)`,
`.LimitingMag(...)`, `.Uncertainty(...)`, `.Performance(...)`, ...) ending in `.Build() (Request,
error)`, which runs `Request.Validate()` internally. `Request{...}` struct-literal construction
stays fully valid — no field is hidden — `RequestBuilder` is additive, not a gate.

**Buffer ownership contract**: `SpectralField` is the only container ever used for spectral
output (a flat `[nDir×nLambda]` direction-major `[]SpectralRadiance`, never `[][]float64`).
`Scratch` is per-goroutine, caller-constructed via `NewScratch`, never shared across
goroutines, never returned to a caller. `EvaluationOptions.Performance.Buffers *BufferPool`
lets a caller reuse allocations across repeated calls (the `plan` scoring loop's actual use
case).

---

## 6. Data flow

```
external dataset (VIIRS/Black Marble/CAMS/...)
        │  remote.GetFile (consent-gated, checksummed)
        ▼
dataset/{blackmarble,eog,atmostate,passband,worldatlas}   ← reader returns raw values +
        │                                                    quality mask + SourceRef, never
        │  wrapped as an EmissionField / AtmosphereProvider  interpolates missing pixels
        ▼
immutable state (atmosphere.Atmosphere, EmissionField snapshot, PassbandSet)
        │
        ▼
Component.Eval / TransmissionModel.LineOfSight   ← pure, deterministic, given the same state
        │
        ▼
CompositeEngine.Evaluate → Result{Total, Components, Derived, Uncertainty, Provenance}
```

Every dataset that enters the pipeline attaches a `SourceRef{Name, Version, Acquired
TimeRange, Retrieved, Checksum, Licence, Endpoint, Fidelity}` at the point it is opened;
`Provenance.Datasets` is the concatenation of every `SourceRef` that contributed to a given
`Result`. Raster/granule readers (`dataset/raster`, `dataset/granule`) cache decoded blocks in
a bounded LRU (carried from v1's `geotiff.go`); the observer-centric aggregation grid (§9) is
cached in a separate bounded LRU keyed by `(site, dataset version, r_max, refinement)`. Neither
cache is unbounded, and both bounds are documented constants.

---

## 7. Runtime modes

```go
type Mode uint8
const (
    ModeClimatology Mode = iota  // versioned, deterministic, fully offline baseline
    ModeHistorical                // dataset-backed state for a specified past epoch
    ModeNowcast                   // recent state; carries issue time + age
    ModeForecast                  // init time + lead time + ensemble uncertainty
    ModeUserSupplied              // caller provides every physical state directly
    ModeFast                      // v1-equivalent empirical physics, re-implemented (§15)
)
```

**Fallback between modes defaults to forbidden.** `EvaluationOptions.Fallback` is
`FallbackForbidden` unless a caller explicitly opts into `FallbackToClimatology` or
`FallbackToFast` — and every fallback that does occur is recorded as a `FallbackRecord{From,
To, Reason, At}` in `Provenance.Fallbacks`. This directly replaces v1's `atlas.LayerAuto`
freshness-first ladder, which silently tried five data tiers in sequence with no caller-visible
record of which one actually answered beyond a post-hoc `Result.Attempts` field — exactly the
silent-fallback behavior the mandate forbids. A caller who wants ladder-like behavior in V2
builds it explicitly, one `Mode` at a time, and gets a `Provenance.Fallbacks` entry for every
step that was actually taken.

---

## 8. Atmosphere model

```go
// Lives in package atmosphere, not core skybrightness — general
// atmospheric state, not sky-brightness-specific (§4 above).
type State struct { /* immutable; built only via atmosphere.Builder */ }

type CloudLayer struct {
    Fraction      CloudFraction       // sky cover [0,1] — a DIFFERENT TYPE from OpticalDepth
    BaseAlt, TopAlt AltitudeM
    OpticalDepth  CloudOpticalDepth   // at 550nm; independent of Fraction
    Phase         CloudPhase          // Liquid | Ice | Mixed
    EffRadius     EffectiveRadiusUM
    Albedo        SpectralAlbedo
    Asymmetry     AsymmetryParameter
    Morphology    CloudMorphology     // Stratiform | Cumuliform | Cirriform | Unknown
    Uncertainty   CloudUncertainty
    Source        SourceRef
}
```

`Fraction`, `OpticalDepth`, sky cover, and explicit layer geometry are four separate concepts
in the mandate and four separate Go fields here — never collapsed into one scalar "cloudiness"
number. `CloudFraction` and `CloudOpticalDepth` being *distinct types* (not both bare
`float64`) means a caller cannot accidentally pass one where the other belongs; the compiler
enforces the mandate's "never collapse to one scalar" rule rather than a code-review
convention having to catch it. `atmosphere.Atmosphere` also carries surface pressure/temperature, a
vertical profile, precipitable water vapour, ozone column, aerosol optical depth + Ångström
exponent + wavelength-dependent single-scattering albedo + phase-function asymmetry + vertical
profile, boundary-layer height, surface spectral albedo/BRDF, snow state, and a horizon
profile. `AtmosphereProvider` is the interface future CAMS/ERA5/local-instrument providers
implement; the physical core (`atmos/`) depends only on the immutable `atmosphere.Atmosphere` type,
never on a specific provider.

### CAMS aerosol data — validated technical notes (Phase 3/7, not yet built)

Investigated live against real CAMS GLOBAL EODATA NetCDF files (2026-08, via a separate agent
session with real Atmosphere Data Store access) as the eventual `dataset/atmostate` CAMS
provider's data-format contract. These are the modern, per-location, per-time operational
counterpart to the static named-climatology presets (`atmosphere.RuralAerosol`/`UrbanAerosol`/
`DesertAerosol`/`MaritimeAerosol`, OPAC-sourced, §3) — the two tiers coexist rather than one
superseding the other: the OPAC presets are the pure, offline, zero-dependency default for a
caller with no live atmospheric data; a CAMS provider is the live, geographically-resolved
tier for a caller with real-time access. Recorded here for the Phase 3/7 implementer, not
implemented now — the practical blockers (an ADS API key, a NetCDF4/HDF5 reader, and the
species-to-optics mapping below) make this real, scoped, later work, not a quick swap-in.

**Grid and file layout.** CAMS analysis files use a global regular 0.4° grid — 900 longitude ×
451 latitude points (longitude 0..359.6° east, latitude 90..-90°; 451 points at 0.4° spacing
from +90° to -90° is an exact pole-to-pole match, `(90-(-90))/0.4 = 450` intervals), 137 ECMWF
model levels (the IFS L137 hybrid sigma-pressure vertical coordinate — the same vertical
discretization used across ECMWF's reanalysis and forecast products), one timestamp per file.
Aerosol mixing-ratio fields (`aermr01`…`aermr18`) are `float64`, dimensioned
`(time, level, latitude, longitude)`, units kg·kg⁻¹.

**Chunk layout is bulk-friendly, point-query-hostile.** The aerosol NetCDF-4/HDF5 chunking is
`[1, 1, 451, 900]` — one whole global horizontal plane per `(time, level)` pair, deflate
level 1. Excellent for bulk/global preprocessing; poor for a runtime "give me the vertical
column above this observatory" query, since answering one column touches up to 137 chunks,
each a full global plane. Keep CAMS NetCDF as the authoritative source format and derive a
runtime representation reorganized around geographic tiles/vertical columns rather than
point-reading the original files — the same "authoritative source format ≠ runtime access
pattern" split this repo already applied to VIIRS/GeoTIFF windowed reads (Phase 4 quality
work) and to the passband bundle's checksummed-manifest-over-raw-curve-files split (§3).

**Deriving mass concentration and true pressure/altitude.** `den(time,level,lat,lon)` is
atmospheric density in kg·m⁻³; `lnsp(time,lat,lon)` is the log of surface pressure (no level
dimension). Aerosol mass concentration is therefore `C_i = aermr_i × den`. The 137 model-level
indices are **not** altitude or pressure directly — real pressure at each level must be
reconstructed from ECMWF's L137 hybrid A/B half-level coefficients and
`ps = exp(lnsp)` via the standard hybrid sigma-pressure formula
(`p_half(k) = A(k) + B(k)·ps`, layer values from adjacent half-levels) — never assume level
index is a proxy for altitude.

**Tracer availability is dataset/version-specific.** The investigated EODATA snapshot exposes
`aermr01`…`aermr11` and `aermr16`…`aermr18` — `aermr12`…`aermr15` are absent in this
particular product/version and must not be assumed present; any future `dataset/atmostate`
CAMS reader has to keep dataset/version differences explicit (matching `Provenance`'s existing
`DatasetVersion` field, §3) rather than silently treating a missing tracer as zero.

**The real next scientific task, and what NOT to do.** Do not invent aerosol microphysical
constants to bridge CAMS tracers to optical properties — that would repeat exactly the mistake
§15 already rejected the log-linear VIIRS→SB shortcut for. The correct next step is a real,
versioned mapping, built from authoritative CAMS/IFS-AER literature and documentation (not
guessed): `CAMS aermr tracer → species/bin → particle size distribution → particle density →
hygroscopic growth model → wavelength-dependent complex refractive index → particle shape →
MOPSMAP inputs` (Gasteiger & Wiegner's "Modeled optical properties of ensembles of aerosol
particles" tool, the standard way to turn aerosol microphysics into extinction/single-
scattering-albedo/phase-function). This mapping is the eventual, live-data-driven replacement
for the OPAC named-type tables — not a constant-swap, a genuine physical pipeline with its own
citations at every stage, following the same never-invent-a-constant discipline already
established for `atmosphere.RuralAerosol`/`UrbanAerosol`/`DesertAerosol`/`MaritimeAerosol`.
`float64` stays the type throughout this reference/scientific pipeline; premature `float32`
optimization or hand-tuned lookup tables come only after the physical mapping itself is
established and validated, never before.

---

## 9. Artificial emission and spatial aggregation

The v1 package converted one VIIRS-DNB pixel directly to sky brightness via a single
empirical log-linear fit (`RadianceToArtificialSB`, Sánchez de Miguel 2020 ISS-HDR
coefficients). Its own 50-line doc comment, preserved here verbatim because it is the primary
evidence motivating everything in this section, recorded a real, live-measured finding from
this project's own investigation (2026-08-06):

> Raw satellite-nadir radiance at one ~15-arcsec VIIRS pixel is not the physical quantity
> zenith skyglow is. A single pixel measures upward radiance from one small patch of ground;
> zenith skyglow is an additive flux integral over scattered light from sources up to ~300 km
> away. Live neighbourhood-pixel sampling (±1 to ±10 pixels, ~460 m–4.6 km) at a
> moderate-brightness site showed raw VIIRS-DNB 2025 radiance swinging from 0 to over
> 6.5 nW·cm⁻²·sr⁻¹ within that radius — and the live lightpollutionmap.info API showed the
> *identical* zero/nonzero pixel pattern at the same offsets, ruling out a decode bug and
> confirming the ~3× disagreement against World Atlas 2015 at that site is real VIIRS-DNB
> per-pixel spatial noise, not a coefficient-calibration problem. Falchi et al. 2016's
> atmospheric propagation model exists specifically because scattered light reaching an
> observer's zenith integrates contributions from sources up to ~300 km away — a single-pixel
> log-linear fit cannot reproduce that integral by construction, regardless of how well its
> coefficients are calibrated.

V2's artificial component is therefore a real emission-to-propagation pipeline (§2), and the
propagation step needs contributions from a wide neighbourhood around every observer, not one
pixel. Iterating every native-resolution source pixel within 300 km for every pointing is
computationally prohibitive at all-sky scale, so V2 builds an **observer-centric log-polar
geodesic grid**: radial rings at `r_i = r₀·q^i` out to a configurable `r_max` (default 300 km,
justified by the same atmospheric-scattering-geometry argument the quote above makes), each
ring divided into azimuthal sectors sized to keep every cell's solid angle at the observer
bounded, cell boundaries snapped to a deterministic global index so aggregation order is
reproducible byte-for-byte, and elevation-aware splitting when the DEM elevation spread inside
a cell exceeds a threshold. The built grid is cached in a bounded LRU keyed by `(site, dataset
version, r_max, refinement params)`.

**Rejected: HEALPix.** Equal-area tessellation of the sphere is exactly right for *storage and
output* (and is adopted for the surrogate's angular basis and offline global-map generation,
§10 Phase 6) but wrong for *observer-centric refinement* — it gives no natural way to be fine
near the observer and coarse 200 km away. **Rejected: a spherical quadtree.** Better
refinement behavior than HEALPix, but geodesic distance/bearing at the cell level becomes
awkward and deterministic ordering is harder to guarantee across refinement levels.

The emission field itself is three independently-uncertain, pluggable models, per §2:
`EmissionField.Intensity` returns the dataset's own native units and quality mask and never
interpolates across a missing pixel; `SpectralMixtureModel` reconstructs a spectral shape from
regional priors, user-supplied spectra, or local lighting inventories, with a default ≥25%
relative uncertainty in the 400–500 nm band specifically because VIIRS-DNB has almost no
sensitivity there and cannot meaningfully constrain that part of the spectrum;
`AngularEmissionModel` supplies an upward emission function from configurable priors (Garstang
1986, Cinzano & Falchi arXiv:1209.2031, Kocifaj) or local calibration, each carrying a prior
covariance rather than being treated as exact.

---

## 10. Uncertainty strategy

Nine covariance groups, matching the mandate's uncertainty-source list one-to-one:
`EmissionIntensity`, `SourceSpectrum`, `AngularEmission`, `Aerosol`, `Cloud`, `Natural`,
`Surrogate`, `Calibration`, `InputAge`. Each contribution is tagged `Aleatoric`, `Epistemic`,
or `Measurement`.

**Combination rule** (Phase 1 implements `UncLinearized`; `UncEnsemble`/`UncMonteCarlo` are
later phases of the same interface): within one covariance group, contributions add
**linearly** — they are treated as fully correlated. Across groups, they add **in
quadrature**. Naive all-quadrature combination is explicitly rejected, with the reason stated
in the doc comment: VIIRS intensity error is spatially correlated across an entire city (the
same calibration/sensor bias affects every pixel of that city's emission field together), not
an independent per-pixel draw — treating it as independent would understate the true
uncertainty on any aggregate quantity. `UncertaintyResult` reports `P05/P50/P95` spectral
fields, a per-wavelength relative sigma, per-group and per-component variance shares, and a
list of `DomainWarning`s — including, once §10 Phase 6's surrogate exists, an explicit warning
whenever a requested atmospheric state falls outside the surrogate's trained domain rather
than a silent extrapolation.

---

## 11. Validation matrix

| Component / claim | Level 1 (invariants) | Level 2 (reference models) | Level 3 (observations) | Level 4 (regression fixtures) |
|---|---|---|---|---|
| Unit algebra, passband integration | **done**, Phase 1 | analytic single-scattering cases planned | n/a | done, Phase 1 |
| Zodiacal, airglow, moonlight, twilight | planned, Phase 2 | ESO SkyCalc/Cerro Paranal comparison, planned | **blocked — no data**, see §13 | planned, Phase 2 |
| Clear-sky artificial propagation | planned, Phase 4 | Cinzano & Falchi analytic single-scattering, planned | **blocked — no data** | planned, Phase 4 |
| Cloudy-sky propagation | planned, Phase 5 | PNAS 122,e2508001122 published scenarios, planned (licensing TBC) | **blocked — no data** | planned, Phase 5 |
| Surrogate | planned, Phase 6 | held-out locked-test MCARaTS split, planned | **blocked — no data** | planned, Phase 6 |
| Calibration fit | n/a | n/a | **blocked — no data**; framework unit-testable on synthetic data only | n/a |

A cell marked *blocked — no data* is a structural fact about this repository today (§13), not
a scheduling gap — it stays blocked until real calibrated field data is acquired.

---

## 12. Accuracy targets

Engineering goals, not present-tense claims:

- Clear photometric conditions: median passband error ≤ 0.15 mag/arcsec², 90th-percentile
  ≤ 0.30 mag/arcsec².
- Partly cloudy conditions: median ≤ 0.30 mag/arcsec², 90th-percentile ≤ 0.60 mag/arcsec²,
  with calibrated empirical interval coverage.
- Synthetic passband integration: ≤ 0.01 mag numerical error at the supported spectral
  resolution — Phase 1's actual round-trip error is ~1e-12 mag, roughly 10⁹× inside this
  target, which is a statement about numerical correctness, not physical accuracy.

If real observation ever shows these targets are unrealistic, the target is revised with the
evidence stated plainly — never met by loosening a test's tolerance without a matching, cited
reason.

---

## 13. Non-goals and honest blockers for this repository

- **Zero ground-truth SQM/TESS field measurements exist in this repo today.** Level-3
  observational validation cannot be performed until real data arrives; the §12 accuracy
  targets are unverifiable until then, and claiming them met before that would be a
  fabrication this project explicitly forbids (§25). Phase 7's calibration framework can be
  built and unit-tested against synthetic fixtures but cannot be *fitted*. Acquiring roughly
  50 nights of calibrated SQM/TESS data from 5+ sites spanning different regimes is the
  single highest-value unblocking action for this whole effort, and it is not a coding task.
- **NOAA/EOG's VIIRS hosting requires OAuth2**, confirmed live this session.
  `remote.EOGAnnualV2` ships `Downloadable: false`; `dataset/eog` supports local files only.
  `remote.VIIRSAnnual` (lightpollutionmap.info's unauthenticated Black Marble mirror) is the
  only unauthenticated global DNB source found — a third-party mirror with no SLA, documented
  as a single point of failure, not hidden behind a generic "data source" label.
- **NASA LAADS DAAC requires an App Key** for the full multi-SDS VNP46A4/VJ146A4 product
  (which is what carries the QA/StdDev/Snow/CloudMask bands §9's emission-field design wants).
  `dataset/blackmarble` exposes `WithAppKey`/`OpenDir`; no key ships in the repo, and no key is
  prompted for or required by any test.
- **No MCARaTS or other external radiative-transfer solver is invoked from Go, by design.**
  The offline reference-simulation pipeline lives entirely outside the Go module
  (`tools/skybrightness-refsim/`, no `.go` files participate in `go build ./...`), stays
  optional, and is not built by Phases 0–5. Phase 6's `surrogate/` package can ship its
  interchange format, inference code, and out-of-domain detection tested against synthetic
  fixtures, but the surrogate itself is scientifically empty — not physically meaningful —
  until real reference simulation output exists to train it on.
- **World Atlas 2015/2016 is a model output, not an observation**, and can never serve as
  Level-3 ground truth; it is used only as a Level-2 reference-model comparison target.
- **Digitized published-figure fixtures may not be redistributable.** Where a PNAS/MNRAS
  scenario's underlying data cannot legally be committed to this repository, the comparison
  stays manual and is documented as manual — never silently skipped or claimed automated.
- **Passband curve licensing needs verification before publication** — Gaia (ESA CC BY-SA,
  needs attribution), SDSS, and SVO Filter Profile Service redistribution terms are checked in
  Phase 0/1, not assumed.

**What Phase 0–1 can actually prove and ship**, given the above: a compiling, documented,
benchmarked, `-race`-clean spectral foundation with correct unit algebra, round-trip-exact
passband integration, enforced linear-space additivity, deterministic provenance digests, a
working `plan` integration, and two runnable examples — with **zero physical-accuracy claims**.
How bright the sky actually is stays aspirational until Phase 2 and beyond; how accurate any
prediction is stays aspirational until real observational data enters this repository.

---

## 14. Staged implementation plan

| Phase | Scope | Entry condition | Exit condition |
|---|---|---|---|
| 0 | This document | Mandate received | Internally consistent, no open contradictions |
| 1 | Spectral foundation: canonical types, `Engine`/`Request`/`Result`, passband integration, provenance/uncertainty/quality types, fast simplified-model components, `plan` + example rewrite | Phase 0 complete | Repo compiles, 10 invariant tests + benchmarks pass, `plan` works end-to-end |
| 2 | Natural-sky baseline: spectral zodiacal/airglow/moonlight, twilight guard, starlight/DGL interface | Phase 1 complete | Comparison fixtures + monotonicity/symmetry tests pass |
| 3 | Atmosphere: `atmosphere.Atmosphere`, molecular/aerosol/cloud optics, terrain, local provider, `dataset/atmostate` CAMS reader (see §8's CAMS notes for the validated grid/tracer/pressure-reconstruction contract) | Phase 2 complete | Zero-aerosol/zero-cloud limits proven exact |
| 4 | Artificial clear sky: emission-field providers, spectral mixture/angular models, log-polar aggregation, `ClearSkyPhysical` | Phase 3 complete | Zero-emission ⇒ zero artificial radiance (bitwise); aggregation-invariance test passes |
| 5 | Clouds: `CloudyAllSkyPhysical`, `FastCloudApproximation` | Phase 4 complete | Clear-sky limit proven as `Fraction→0` |
| 6 | Reference simulation + surrogate | Phase 5 complete | `.sbsur` format + OOD detection tested on synthetic data; pipeline documented as unbuilt/optional |
| 7 | Calibration + operating modes + planner integration | Phase 6 complete | `calib/` unit-tested on synthetic data; `plan` constraint family complete |

Each phase leaves the repository compiling, documented, and tested, and closes with a written
report at `docs/reports/skybrightness-phase<N>.md` covering: scope delivered/deferred; files
added/modified/deleted; public API changes; equations implemented (source, equation number,
verified-against-original yes/no); data assumptions with a numeric relative sigma each; tests
added and what each proves; benchmarks; validation evidence and what is explicitly not
validated; known limitations; next-phase entry conditions.

---

## 15. Rejected alternatives

**Why not extend the current V-band package instead of rewriting it?** The v1 contract is a
scalar `SurfaceBrightnessV` returned by a one-method interface,
`Model.SurfaceBrightness(altaz, ctx) (SurfaceBrightnessV, error)`. Every V2 requirement —
spectral output, per-component decomposition, uncertainty, provenance, mode selection,
allocation-conscious batch buffers — is a change to that one signature. There is no
evolutionary path from a scalar return to a spectral field; extending it would mean either
breaking the signature anyway (making "extension" a rewrite in disguise) or bolting a second,
parallel API onto the first and carrying both forever. The user explicitly authorized the
clean break (§6), so V2 takes the honest option: delete the old symbols, rewrite the one
production consumer and both examples in the same change, and document the migration as a
symbol table (§16) rather than a compatibility shim.

**Why not one universal log-linear VIIRS→SB equation?** See §9's quoted finding — a ~3×
measured disagreement at a real site, root-caused to VIIRS-DNB per-pixel spatial noise that no
single-pixel coefficient fit can average away, because the physical quantity it needs to
approximate is a ~300 km spatial integral, not a point sample.

**Why bespoke typed floats instead of `unit.Quantity` throughout?** Hot-loop allocation and
indirection cost at all-sky scale, plus the honest admission (§3) that `unit.Dimension` cannot
distinguish a steradian from a bare dimensionless ratio — adding `Steradian` to `unit` gives
real documentation value but zero type-level protection, so the protection has to live where
it can actually be enforced: in `skybrightness`'s own named types.

**Why log-polar observer-centric aggregation instead of HEALPix or a spherical quadtree?**
See §9 — HEALPix is right for equal-area storage/output (and is used for exactly that in
Phase 6), wrong for observer-centric fine/coarse refinement; a quadtree makes geodesic
distance/bearing and deterministic cell ordering harder to guarantee.

**Why no neural-network surrogate in v1?** The mandate's own admission bar — "unless it
demonstrably improves held-out accuracy" — cannot be met without held-out validation data,
and this repo has none (§13). A neural surrogate is not ruled out permanently, only until that
bar can honestly be cleared.

**Why is `Bortle` output-only in V2, with no `BortleToBrightness`?** The Bortle 1–9 scale is a
lossy, human-authored, non-invertible qualitative descriptor. v1 already documented
`BortleClass` as non-round-trippable; V2 keeps it strictly as a reporting helper
(`BortleFromLuminance`) and removes the reverse direction entirely, since using it as a model
*input* would silently discard precision the rest of the engine worked to preserve.

**Why is v1's `atlas.Resolver` freshness-first fallback ladder gone?** It tried five data
tiers in sequence with no caller-visible record of which one actually answered beyond a
post-hoc `Attempts` field — precisely the silent mode-fallback behavior §16 forbids. V2
replaces it with an explicit `Mode` per request, a `FallbackPolicy` that defaults to
forbidden, and a `Provenance.Fallbacks` record for every fallback step actually taken.

---

## 16. Migration note — there is no backward compatibility

Every symbol below is **deleted**, not deprecated, not aliased. There is no shim package and
none will be added. The "replaced by" column is the adaptation recipe.

| Old symbol (`skybrightness` v1) | Replaced by | Adaptation |
|---|---|---|
| `Model` interface | `Engine` (§5) | Build a `CompositeEngine` from `Component`s instead of implementing one method |
| `Component` interface | `Component` (new shape, §5) | `Radiance(altaz, ctx) (Nanolambert, error)` → `Eval(ctx, EvalInput, out SpectralField) (ComponentReport, error)` |
| `SurfaceBrightnessV` | `SurfaceBrightnessAB` / `SurfaceBrightnessVega`, always with a `PassbandID` | Name the passband explicitly — there is no unqualified "V magnitude" any more |
| `Nanolambert` | `SpectralRadiance` (per-wavelength) or `Radiance` (passband-integrated) | Work in linear spectral radiance, integrate through a named `Passband` |
| `SQMProvider` | `calib.Measurement` + `Instrument` (Phase 7) | An SQM reading is an observation through a device response, not a model input |
| `Floor` | `atmosphere.Atmosphere` + an `EmissionField`-backed `Component` (Phase 4) | There is no standalone "floor" concept; light pollution is one component of the full decomposition |
| `CompositeModel`/`NewCompositeModel` | `CompositeEngine`/`NewCompositeEngine` (§5) | Same idea (sum components), spectral instead of scalar |
| `RadianceToArtificialSB` | `EmissionField` → `SpectralMixtureModel` → `AngularEmissionModel` → `rt.ClearSkyPhysical`/`CloudyAllSkyPhysical` (§9) | There is no direct pixel-to-brightness function any more, by design |
| `atlas.Resolver`/`LayerAuto` | Explicit `Mode` + `FallbackPolicy` + `Provenance.Fallbacks` (§7) | Choose a mode explicitly; opt into fallback if you want it, and it will be recorded |
| `LimitingMagModel` | `LimitingMagModel` (new signature, §5) | Now takes a `LimitingMagInput{Passband, SkyVega, SkyAB, Airmass}` instead of a bare `(SurfaceBrightnessV, airmass)` pair |
| `plan.LimitingMagnitudeConstraint` | `plan.LimitingMagnitudeConstraint` (rewritten) + `plan.SkyConditions` | Construct a `SkyConditions{Engine, Passband, ...}` once and pass it in; the constraint itself keeps its below-horizon short-circuit |

---

## 17. Data licensing inventory

| Dataset | Licence | Redistribution | Auth required | Approx. size | Mutable |
|---|---|---|---|---|---|
| World Atlas 2015 (Falchi et al. 2016) | CC BY-NC 4.0 (non-commercial) | Not redistributed; downloaded per-user via `remote.WorldAtlas` | none | ~684 MB zip, ~2.8 GB extracted | no (frozen 2019-11-18) |
| VIIRS annual composites (lightpollutionmap.info mirror of Black Marble) | source data CC0; mirror asks credit to Jurij Stare + NASA Black Marble | Not redistributed; downloaded per-user via `remote.VIIRSAnnual` | none | ~700 MB–1 GB per year | yes (past years occasionally reprocessed) |
| NASA Black Marble VNP46A3/A4, VJ146A3/A4 (LAADS DAAC, full multi-SDS) | NASA open data | Not redistributed | LAADS App Key | per-granule | yes |
| EOG VIIRS annual v2 | EOG terms | Not redistributed | OAuth2 bearer token | ~6 GB | yes |
| CAMS EAC4 reanalysis | Copernicus licence | Not redistributed | ADS API key | per-request | yes |
| ERA5 single levels | Copernicus licence | Not redistributed | CDS API key | per-request | yes |
| Passband curves (Johnson-Cousins, Sloan, Gaia, CIE, SQM) | Gaia: ESA CC BY-SA (attribution required); others: verify per-source at Phase 1 implementation time | Redistributed in `remote.PassbandBundle`, checksummed | none | ~2 MB | no (pinned by semver + SHA-256) |
| lightpollutionmap.info live API (`lpmap`) | provider terms, manual API key issuance | Not redistributed; live queries only | manual-issue API key | n/a | n/a (live) |

This table is the authoritative pre-publication checklist for §3's passband bundle and every
Phase 4+ dataset provider — nothing in that list ships until its row here is verified against
the primary source, not a secondary summary.
