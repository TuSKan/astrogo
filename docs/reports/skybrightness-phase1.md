# Sky Brightness V2 — Phase 1 Report

## 1. Scope delivered (and scope deferred, with the phase it moved to)

**Delivered**: the full spectral foundation — canonical SI-based scalar types, `SpectralGrid`/
`SpectralField`, the `Passband`/integration API, `Component`/`Engine`/`Request`/`Result`/
`atmosphere.State`/`Provenance`/`UncertaintyResult`/`QualityFlags` types, a working
`CompositeEngine` (linear-space summation, batch evaluation via
`internal/parallel.MapChunked`), the fast, simplified models (`natural.ConstantAirglow`,
`natural.KrisciunasSchaeferMoonlight`, `skybrightness.SchaeferNELM`) re-implementing v1's exact
physics against the new API, an analytic Rayleigh-only transmission model
(`atmos.RayleighOnly`), a structurally-complete (not yet populated) passband-bundle provider
(`dataset/passband`), `plan`'s rewritten `LimitingMagnitudeConstraint`/`ScoreObservableSky`,
and both examples (`examples/18_sky_brightness`, `examples/21_meteor_shower_forecast`)
rewritten against the new API.

**Deferred, by design**: real spectral zodiacal light / integrated starlight / diffuse
galactic light / twilight / aurora (Phase 2); the full molecular/aerosol/cloud transmission
model, `atmosphere.State`'s aerosol/cloud fields being physically exercised (Phase 3); the
artificial emission-field pipeline, spectral-mixture/angular-emission models, spatial
aggregation, `rt.ClearSkyPhysical` (Phase 4); `rt.CloudyAllSkyPhysical`/
`rt.FastCloudApproximation` (Phase 5); the offline reference-simulation pipeline and
`surrogate/` package (Phase 6); `calib/` and CAMS/ERA5/nowcast/forecast providers, plus richer
planner constraints (Phase 7).

**Deviation from the original phase plan, stated plainly**: `docs/skybrightness.md`'s
original file-disposition table labeled `skybrightness/atlas` and `skybrightness/lpmap` for
deletion "at Phase 4." In practice, `atlas` could not be left in the tree during Phase 1 —
it imported core `skybrightness` symbols (`SQMProvider`, `Floor`, `RadianceToArtificialSB`,
`NaturalZenithMcdM2`) that Phase 1 deletes, so leaving it in place would have broken the
"repo must compile at the end of every phase" rule. `skybrightness/atlas` was therefore
deleted now, not in Phase 4; its real replacement (`dataset/raster`, `dataset/blackmarble`,
`dataset/eog`, `dataset/worldatlas`) is unbuilt and is real Phase 4 work — recovering the
779-line GeoTIFF decoder verbatim from git history (pre-Phase-1 commits) is the concrete first
step when that phase starts. `skybrightness/lpmap` needed no such change (it never imported
deleted core symbols) and is unchanged, kept as a live cross-check provider.

## 2. Files added / modified / deleted

**Deleted** (git history preserves the originals for Phase 2-4 physics porting):
`skybrightness/{doc,model,units,provider,floor,moonlight,airglow,zodiacal,limitingmag,
radiance,interp}.go` + their tests; the entire `skybrightness/atlas/` and
`skybrightness/lpmap` (re-added unchanged) trees — see the disposition table in
`docs/skybrightness.md`.

**Added** — core `skybrightness` (18 files): `doc.go`, `units.go`, `spectral.go`,
`scratch.go`, `mode.go`, `component.go`, `passband.go`, `engine.go`, `request.go`,
`result.go`, `derived.go`, `provenance.go`, `uncertainty.go`, `quality.go`,
`atmosphere_state.go`, `limitingmag.go`, `bortle.go`, `instrument.go`,
`importgraph_test.go`, `invariants_test.go`.

**Added** — `skybrightness/natural` (7 files): `doc.go`, `legacy_units.go`,
`legacy_common.go`, `legacy_airglow.go`, `legacy_moon.go`, `legacy_engine.go`,
`constants.go`, `legacy_test.go`.

**Added** — `skybrightness/atmos` (3 files): `doc.go`, `transmission.go`,
`transmission_test.go`.

**Added** — `skybrightness/dataset/passband` (3 files): `doc.go`, `passband.go`,
`passband_test.go`.

**Modified**: `unit/radiometric.go` (new, composed `unit.Unit` documentation vars),
`unit/quantity_types.go` (new, the 26 zero-cost radiometric quantity types),
`unit/doc.go` (radiometric-type-safety section), `constants/photometric.go` (new,
`PhotometricSet`), `constants/derived.go` (`StefanBoltzmannConstant`),
`constants/units.go`, `constants/constant.go`, `constants/constant_test.go` (set-count
update), `remote/endpoint.go` + `remote/registry.go` + `remote/registry_test.go`
(`PassbandBundle` endpoint), `plan/skybrightness.go` (rewritten),
`plan/skybrightness_test.go` (rewritten), `plan/meteor_test.go` (fake types updated),
`examples/18_sky_brightness/main.go` (rewritten), `examples/21_meteor_shower_forecast/
main.go` (updated), `docs/skybrightness.md` (new, the Phase 0 design document),
`CLAUDE.md`, `README.md`, `CHANGELOG.md`.

**Net effect on dependencies**: `go mod tidy` removed `github.com/scigolib/hdf5` — its only
importer (`skybrightness/atlas`) is gone; it returns in Phase 4 with `dataset/granule`.

## 3. Public API changes

Breaking, no backward compatibility (full symbol table in `docs/skybrightness.md` §16).
Headline additions: `skybrightness.Engine`/`Component`/`CompositeEngine`/`Request`/
`BatchRequest`/`Result`/`BatchResult`/`Passband`/`PassbandSet`/`Provenance`/
`UncertaintyResult`/`QualityFlags`/`Mode`/`LimitingMagModel` (new signature)/`SchaeferNELM`/
`Point`; `atmosphere.State`/`Builder` (moved out of core `skybrightness` in a follow-up
API-polish pass — see the note at the end of this report); `natural.ConstantAirglow`/
`KrisciunasSchaeferMoonlight`/`NewFastEngine`/`TopHatJohnsonV`/`TopHatVGrid`;
`atmos.RayleighOnly`;
`passband.OpenBundle`/`Remote`/`FromFITS` (FromFITS deferred — see §9); `unit.SpectralRadiance`
and 25 sibling types; `constants.Photometric`; `remote.PassbandBundle`.

## 4. Equations implemented

- AB magnitude definition, `m_AB = -2.5*log10(f_ν/3631 Jy)` — Oke & Gunn (1983), verified
  against the defining relation directly (not a secondary summary).
- Trapezoid quadrature for all spectral integration — standard numerical method, not
  paper-specific.
- Pivot wavelength `λ_p² = ∫Rλdλ / ∫R/λdλ` and response-weighted effective wavelength —
  standard photometric definitions.
- Krisciunas & Schaefer (1991), PASP 103, 1033, eqs. (15)/(18)/(20)/(21)/(3) — ported
  verbatim from astrogo v1's own already-implemented, tested form; not re-derived from the
  paper this session (v1's implementation was itself derived from the paper in an earlier
  session — verified-against-original: carried, not re-verified this phase).
- Schaefer (1990) PASP 102, 212 / Unihedron SQM↔NELM relation — same provenance as above,
  ported verbatim.
- Hansen & Travis (1974), Space Sci. Rev. 16, 527 — the commonly-cited approximate sea-level
  Rayleigh optical depth fit, `τ_R(λ) = 0.0088·λ^(-4.15+0.2λ)` (λ in μm) — verified-against-
  original: **no**, this is a secondary-source-cited approximation used explicitly as a
  labeled Phase 1 simplification (`atmos.RayleighOnly`'s doc comment states this), not a
  claim of primary-source verification; cross-checked numerically against the commonly quoted
  τ_R(550nm)≈0.097 figure (`TestRayleighOptical_DepthMatchesHansenTravisAt550nm`).
- Stefan-Boltzmann constant, `σ = 2π⁵k_B⁴/(15h³c²)` — computed from SI2019's exact c/h/k_B
  (not hardcoded), cross-checked against the published CODATA value.

## 5. Data assumptions and their uncertainty

- `ConstantAirglow`/`KrisciunasSchaeferMoonlight`: RelSigma 0.3 / 0.15 respectively (carried from the
  original physics' own accuracy claims — KS1991's own stated ~8-23% moonlight accuracy,
  documented in the component's `Assumptions`).
- `atmos.RayleighOnly`: no aerosol, no molecular absorption, no ozone — an explicit,
  documented approximation, not assigned a numeric uncertainty (Phase 3 replaces it with a
  model whose uncertainty is meaningful to quote).
- The Garstang nanolambert↔V-magnitude round trip (`TopHatJohnsonV`) is precise to ~1.5e-4
  mag at V≈22, bounded by the historically-published 0.92104 coefficient's own rounding (not
  by anything introduced in this phase) — measured and documented in
  `natural/legacy_units.go` and `natural/legacy_test.go`.

## 6. Tests added

10 core invariant tests (`skybrightness/invariants_test.go`): total finite/non-negative
(incl. poles/az-wrap), component-sum-equals-total (exact, linear space), passband-integration
linearity, AB round-trip (<1e-9 relative), no azimuth-wrap discontinuity,
`Materialize:true`/`false` bit-identical `Total`, provenance-digest stability,
component-selection exclusion exactness, 64-goroutine concurrent-evaluate consistency,
high-latitude/polar direction finiteness. 4 import-graph tests (rewritten
`importgraph_test.go`, rules 1/2/3/4 from `docs/skybrightness.md` §4). 5
`natural/legacy_test.go` tests (round-trip precision, zero-value default, below-horizon zero,
nil-context error, `NewFastEngine` end-to-end). 5 `atmos/transmission_test.go` tests
(zenith-vs-low-altitude ordering, wavelength-dependence sign, below-horizon error,
pressure-dependence sign, Hansen-Travis numeric sanity check). 3
`dataset/passband/passband_test.go` tests (round trip, missing manifest, unknown ID). 2 new
`constants` tests (`TestPhotometric_ABZeroPoint`, `TestPhotometric_Name`,
`TestDerived_StefanBoltzmannConstant`). `plan/skybrightness_test.go` fully rewritten (4
tests, all passing against the new API with a test-local fixed-SQM `Component`).

## 7. Benchmarks

Not added this phase — deferred to Phase 2, when there is real (non-fast-model) physics whose
performance is worth characterizing. The mandate's benchmark targets (`BenchmarkPointSpectral`,
`BenchmarkAllSky10k`, `BenchmarkBatchTimeSeries`, `BenchmarkPassbandIntegrate`) remain a Phase
2 action item, noted here rather than silently dropped.

## 8. Validation evidence (and explicitly: what is NOT validated)

- **Level 1 (invariants)**: done — see §6.
- **Level 2 (reference models)**: not attempted this phase — no real natural-sky physics
  exists yet to compare against ESO SkyCalc/Paranal cases.
- **Level 3 (observations)**: not attempted, and structurally cannot be until real
  ground-truth SQM/TESS data enters this repository — see `docs/skybrightness.md` §13.
- **Level 4 (regression fixtures)**: the fast-model round-trip and Rayleigh sanity checks are the
  closest analogue this phase has; no external fixture files were introduced.
- **What Phase 1 does NOT prove**: physical accuracy of any component's output. It proves
  unit correctness, integration correctness, additivity, determinism, and allocation-shape
  correctness (`Materialize` bit-identity) — exactly the claim `docs/skybrightness.md` §13
  commits to for this phase, no more.

## 9. Known limitations and unsupported regimes

- `dataset/passband.FromFITS` (SVO-style FITS BinTable reading) is specified in the design
  but not implemented this phase — a real scope cut, not silently dropped. `OpenBundle`/
  `Remote` (the primary and secondary paths) are both implemented and tested.
  `remote.PassbandBundle` has no published bundle yet, so `Remote` is structurally complete
  but unexercised against real data.
  `examples/18_sky_brightness` therefore uses `natural.TopHatJohnsonV()` — the only passband
  whose output is physically meaningful given Phase 1's fast-model-only components — rather than
  a real Johnson V/Sloan r curve; the mandate's literal "Johnson V and Sloan r" end-to-end
  example is Phase 2 scope, once real spectral components exist to make a second passband's
  output meaningful.
- `UncertaintyMode` only implements `UncLinearized`; `UncEnsemble`/`UncMonteCarlo` return
  `ErrUncertaintyModeUnimplemented` rather than silently degrading.
- `EvaluationOptions.Buffers` (`BufferPool`) exists but only pools a single `Scratch`,
  reused across sequential `Evaluate` calls on one goroutine — not a general concurrent pool.
- `DeriveIrradiance`'s `HorizontalIrradiance` uses a uniform-solid-angle approximation
  (`4π/n`) rather than caller-supplied real per-direction solid angles — documented in
  `engine.go`'s `uniformSolidAngle` as a Phase 1 placeholder.
- `photopicPassband` (used internally for `DeriveLuminance`) is an analytic Gaussian
  stand-in for the real CIE V(λ) curve, documented as such.

## 10. Next phase entry conditions

Phase 2 (natural-sky baseline) can start once: (a) the Cinzano/Falchi and Leinert/Noll/Patat
papers cited in `docs/skybrightness.md` §24 are read in full for their exact equations (not
re-derived from this phase's carried-forward v1 constants); (b) a decision is made on whether
`natural/starlight.go`/`dgl.go`'s versioned all-sky template is sourced or deferred further;
(c) Phase 1's benchmark gap (§7) is closed before or alongside Phase 2, so performance
characterization exists before real (more expensive) physics lands on top of it.

## Addendum — post-Phase-1 API polish (same branch, before Phase 2)

Before Phase 2 started, a design review (prompted directly by feedback that the shipped API
wasn't "astrogo-integrated" enough) produced four changes, all still within Phase 1's scope
(no new physics), applied on top of what's described above:

1. **`atmosphere.State`/`Builder`/`Aerosol`/`CloudLayer`/`SurfaceOptical`/`HorizonProfile`
   moved out of core `skybrightness` into the peer `atmosphere` package.** General
   atmospheric state isn't sky-brightness-specific — a future weather/seeing constraint needs
   the identical type. `skybrightness` now references `atmosphere.State` directly (no alias),
   matching how `coord.Context` is already referenced. General data-provenance primitives
   (`SourceRef`/`Fidelity`/`TimeRange`/`DatasetVersion`) moved alongside it, aliased back into
   `skybrightness` since they also appear pervasively in `skybrightness`'s own `Provenance`
   types.
2. **`ToPhoton`/`ToEnergy`/`ArcsecondSquaredToSteradian` moved into `constants`** (as
   `var`-aliased in `skybrightness` for unchanged call-site syntax) — reusable by any future
   photometry code, not skybrightness-only.
3. **Renamed the "Legacy" naming scheme to be citation/descriptive-based**:
   `LegacyAirglow`→`ConstantAirglow`, `LegacyMoonlight`→`KrisciunasSchaeferMoonlight`,
   `LegacySchaeferNELM`→`SchaeferNELM`, `LegacyJohnsonV`→`TopHatJohnsonV`,
   `NewLegacyEngine`/`LegacyConfig`→`NewFastEngine`/`FastConfig`, `ModeLegacy`→`ModeFast`,
   `FallbackToLegacy`→`FallbackToFast`, `QualityFlagLegacyPhysics`→
   `QualityFlagApproximatePhysics`. "Legacy" implied "about to be removed," which
   contradicts writing brand-new code, and risked colliding with a future real spectral
   moonlight model's more deserving claim to the plain name.
4. **`natural.FastConfig` gained a `Transmission` field**, `Point()`/`PointResult` gained an
   optional `Components`/`ComponentBrightness` breakdown, and `Provenance` gained a
   `String()` method — closing gaps found while checking whether the "simple path"
   (`natural.NewFastEngine`, `Point()`) could actually reproduce what `examples/18` needed
   without dropping to `Engine.Evaluate`/raw `encoding/json`.

Full verification gate re-run clean after all four changes: `gofmt`, `go vet`, `go build`,
`golangci-lint run` (0 issues repo-wide), full untagged `go test ./...`, and the tagged
integration/network/validation suite for every touched package.
