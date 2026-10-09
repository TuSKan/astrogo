# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.20.0] — 2026-10-09

### Added
- **A tripwire for astrogo's two leap-second sources.**
`TestLeapSecondSourcesAgree` compares NAIF's kernel against the table compiled
into gofa in both directions — every kernel entry, plus an independent
1972–2035 sweep that is the half able to see an entry beyond gofa's last one.
They agree today; when they stop, the pinned table is stale (#143).
- **The leap-second table is now validated, not just cross-checked.**
The complete 28-entry published ΔAT record is pinned and asserted against
gofa's table, including the half-open boundary convention at every step. A new
`validation`-tagged suite re-verifies it against the IERS timescale service —
the one reference that does not share ancestry with gofa, NAIF and finals2000A. (#148)
- **A guard that makes the LSK prove its own coverage.** Every assignment in the
kernel's data block must be one the parser models, and every constant the
parser claims must be non-zero — so an unrecognized keyword in a future kernel
revision fails a test instead of being silently dropped, which is how the
relativistic constants went unread for a release. (#148)
- **Tests for the catalog error contract.** `resolve.Drain` is covered, and the
Resolver's semantics are pinned directly: a provider failure is never reported
as `ErrNotFound`, a canceled context is caught before any provider is
consulted, `ErrUnsupported` is an answer rather than an incident, and one
broken provider neither denies an answer another gave nor suppresses partial
`Search` results. (#151)
- **A guard that compiles the README's Go blocks.** `docsguard` proved a cited
name is *declared*; it could not prove a program *builds* — which is how
`coord.NewContext(epoch, observer, atmosphere.Atmosphere{})` survived a release
under the sentence claiming every sample was compiled and run. All 17 blocks
are now type-checked, and four broken samples are fixed. (#155)
- **The third altitude pipeline is now compared against the other two.** A
satellite joins the constraint-versus-details guard — the body class with the
most parallax of all — and the events pipeline is pinned against its own
convention, which turned out to differ by event kind: `Event.Altitude` is
geometric at rise and set and refracted at transit (#156).
- **A ΔAT table can now be registered, so a new leap second no longer needs a
release.** `time.RegisterLeapSeconds` installs a published table process-wide;
`LeapSecondSource`/`ResetLeapSeconds` mirror the EOP registry. Registration is
superset-only — a table that contradicts the built-in record below its last
step is refused, which is what makes registering late safe. `jpl.NewProvider`
registers its kernel's `DELTA_AT` block, so `time`'s scale conversions and the
ET the SPK is evaluated at follow one source (#143).
- **A `logging` package replaces the three production writes to the global `log`
package**, whose output went wherever `log.SetOutput` last pointed. It is the
only package in astrogo that imports `log/slog`; everything else calls
`logging.Info`/`logging.Warn`, which name no slog type. Progress lines are
`Info` and discarded by default; the EOP-unavailable message is `Warn` and
still emitted, because `Time.EOP` has no error return and that line is the only
notice a caller gets that topocentric accuracy silently dropped, by up to 13.5 arcsec.
`logging.Set(nil)` restores the default, `slog.DiscardHandler` silences
everything (#108).
- **CI now re-checks every open pull request when `main` moves.** A PR's own
checks test it merged into its base, but nothing re-runs them when the base
advances — so a PR can sit green while the branch it would merge into changes
underneath it. #169 did exactly that: it merged with no textual conflict and
did not compile, because #165 had meanwhile rewritten the function it touched.
The new job trial-merges each open PR into the pushed commit and builds the
result (#169).
- **SGP4 is now verified against Vallado's reference vectors** (AIAA 2006-6753),
checked in and run by the `validation` tier: 588 states over 30 element sets,
reported as a distribution. 22 cases agree to p50 35 m / max 289 m; 8 diverge by
0.6–3440 km, asserted from both sides so neither a regression nor a fix passes unnoticed. (#181)
- A guard keeps the six hand-written `go-version` pins in the CI workflows on the same
minor version as `go.mod`. They are deliberately decoupled from `go-version-file`
(#109), which is safe but invisible: the day `go.mod` moves past 1.25, every job would
silently start downloading a toolchain again while CI stayed green. (#183)
- A guard holds `time`'s exported package-level vars to a documented inventory of eight,
each recording why Go leaves no alternative — six error sentinels, `LocationUTC` (which
wraps the standard library's own mutable `time.UTC`) and `J2000` (#113).
- **Every parser that reads bytes astrogo did not produce is now fuzzed** — `fits`
(3 targets), `ephemeris/satellite` (2), `catalog/norad`, `internal/votable` and
`time/internal/iers`, joining the existing SPK targets. Seeds are literals, so the
corpora run in ordinary CI; two crashers they found are checked in as regressions (#139).
- Tests for four exported symbols that had no reference anywhere in the module — not a
caller, not a test, not an example: `plan.EventAnyPhase` (a documented four-phase
wildcard nothing had ever passed), `plan.NewEarth`, `plan.WithStep` and
`time.FileEOPLoader` (no longer exists; `time.FSEOPLoader` replaced it in #509), the recommended no-dependencies EOP path (#106).
- Runnable `Example` functions for `time`, `coord`, `ephemeris`, `atmosphere`, `magnitude`
and `catalog` — the task each package exists for, on pkg.go.dev's front page. All but
`catalog`'s carry an `// Output:` comment, so they are executed and diffed by every
`go test` rather than merely compiled (#141).
- **`time.GPST` — GPS system time, the scale a satellite user most often actually holds.**
A receiver timestamp had no way to say what it was; calling it UTC is 18 s wrong today,
which is 138 km of ISS track. GPST is TAI − 19 s exactly, so the conversion is arithmetic
and cannot fail. Galileo shares it; BeiDou and GLONASS do not (#145).
- Tests for a **negative** leap second — permitted since 1972, never yet observed, and
projected for about 2030. They cover ΔAT stepping *down* through both the TAI and TT
branches, UTC↔TAI staying inverse across it, and that widening the record check from
"+1" to "|step| = 1" did not widen it into nothing (#147).
- Offline tests for `skybrightness/dataset/solar`'s CALSPEC parse path, taking it from 22.9%
to 71.4% — the unit conversion (Å→nm, erg s⁻¹ cm⁻² Å⁻¹→W m⁻² nm⁻¹, derived independently),
the row filters, the negative-flux clamp, both float widths, and the padded column names a
real CALSPEC file actually carries (#122).
- **`remote.RetryPolicy` — the extension point `remote.ErrRetriable` had been documenting for
months without it existing.** A per-client `func(remote.Attempt) bool`, with
`remote.DefaultRetryPolicy` exported so a custom one can defer to it rather than restate the
429 / 5xx-except-501 / no-response rule. (#195)
- Offline tests for `skybrightness/dataset/dust`'s I/O paths, taking it from 37.5%
to 96.5% — the IRSA fetch and its cache (a second session asks nothing, a cell is
asked once, a run cut off keeps what it paid for, a corrupt line costs one
sightline) and every reason `SFD.Open` refuses a hemisphere, against synthetic
SFD-shaped FITS built in the test (#122).
- `time.Date` reports a `23:59:59` that a registered ΔAT record says was removed
by a negative leap second — the mirror of the `23:59:60` warning, on the same
terms. None has ever been announced, which is why it is written now: Levine,
Tavella & Milton (2023) project one by about 2030 and warn that never having
happened is what makes errors near-certain when it does (#147).
- `time.Time.LeapSmearWindow` reports whether an epoch falls within a day of a
leap second and names the step. An NTP-disciplined host may be deliberately
wrong by up to 0.5 s for up to 24 hours around one — 3.8 km of ISS ground track
— and no library can detect it, so this says where the question arises rather
than answering it (#146).
- `time.BDT`, the BeiDou system time scale, and `Time.BDT()`. TAI − 33 s exactly,
so BDT − UTC is 4 s today and BDT − GPST a permanent 14 s. Four seconds reads as
a rounding difference and is 30 km of ISS ground track, which is the argument
for a name rather than an offset the caller subtracts. GPST and BDT also join
the scale round-trip matrix, which covered neither (#145).
- CI runs `apidiff` between a pull request's head and its base, and fails when the
exported API changes incompatibly with no fragment declaring it. Nineteen minor
releases in eight weeks is fine pre-1.0 only if the breaking diff is
machine-reported rather than found by a downstream build (#121).
- Offline tests for `catalog/fink`'s SSOFT bulk-table path, taking the package
from 30.7% to 87.0% — the parquet load and the fit/status filter on it, the four
ways the download is not the table, the JSON coercions the single-object
endpoint needs, and absence against failure in `Search` and `ResolveObject`
(#122).
- Offline tests for `skybrightness/plan`, 39.4% to 90.9% — the request reaching
the model, each failing step naming itself, the band being the sky's own, and
depth deepening with exposure, all against a sky assembled from synthetic inputs
through the real `skybrightness.Model` (#122).
- Four `plan` constraints: `SunSep` (the companion to `MoonSep` for the other
bright source), `GalacticLatitude` (distance from the plane, unsigned so it
serves both the surveys that avoid it and those that want it), `TimeWindow` (a
coordination window or a deadline, the constraint with nothing to do with the
sky), and `Horizon`, which is the first consumer of `WithHorizonProfile` — the
per-azimuth terrain limit `Altitude` cannot express (#129).
- `plan.NewMPCSite(ctx, code)` and `plan.MPCObservatories(ctx)` resolve the IAU
Minor Planet Center's ~2,700 observatory codes to a `*Site`, recovering each
position from the published parallax constants. `MPCObservatory.ResolutionM`
reports how finely a row was published — from ±3 m to ±3.2 km — because the
register mixes both and a recovered height does not say which it is. New
`remote.MPCObsCodes` endpoint, download-gated like every other bulk fetch
(#125).
- `catalog/mpcorb` reads the Minor Planet Center's own orbital-element files —
`MPCORB.DAT` and its NEA/Distant/PHA/Unusual cuts — as a streaming
`iter.Seq2[resolve.Target, error]`, at the full published precision SBDB
rounds to three significant figures. gzip is detected from the stream, the
packed epoch is decoded through the exported `ParseEpoch`, and a Target feeds
`plan.FromCatalog` unchanged. New `remote.MPCORB` endpoint (#128).
- `coord.FK4`, `coord.FK4ToICRS` and `coord.ICRSToFK4` read and write B1950
positions, so a catalogue built on the Palomar or ESO/SERC surveys can be
pointed at — treating one as J2000 misses by about 0.7°. Two constructors,
because "the catalogue recorded no proper motion" and "the proper motion is
zero" convert to places 0.04″–0.25″ apart: FK4's equinox drifts, so a star at
rest in it is moving in FK5 (#126).
- `internal/leakcheck` fails a package's tests when Go 1.27's `goroutineleak`
profile proves a goroutine leaked, naming each survivor and where it started.
Installed in the two packages that fan out — `internal/parallel` and `catalog`
— both of which measure clean, including against the live services (#234).
- Allocation contracts for `coord`, `time` and `atmosphere`'s hot paths, as
ordinary tests that fail a build — `allocs/op` is deterministic where `ns/op`
on a shared runner is not. The Benchmarks CI job now reports a `benchstat`
delta in its job summary instead of uploading numbers nothing reads (#249).
- A guard reporting exported symbols in `internal/` packages that nothing in the
module names — narrow on purpose: the broad version #106 proposed reports 173
of 581 exported functions, almost all of them public API (#252).
- `ephemeris.AstrometricState` returns the astrometric place — the target
retarded by light time with the observer left where it is — which is the
quantity JPL Horizons publishes as quantity 1 and the one a consumer applying
its own aberration needs (#254).
- The topocentric cross-track bias is localised to Earth rotation. Against USNO —
an independent NOVAS implementation — geocentric apparent declination has a
signed mean of −0.027″ while GHA has +0.662″, and declination cannot see Earth
rotation, so the apparent-place chain is clean and 0.66″ is 44 ms of UT1 (#254).
- A geocentric astrometric comparison against Horizons, with no Earth Orientation
Parameter in the path: agreement is zero to the ~3.6 µas Horizons prints, which
places the ~0.52″ topocentric cross-track bias downstream of the ephemeris and
the light-time solution rather than in them (#254).
- An offline tier that isolates the Earth-orientation *coupling* from the
Earth-orientation *data*: varying UT1 and the pole by known amounts and
asserting astrogo responds by the amount physics requires. Modeled on
Skyfield's pinned-input NOVAS comparison, but checked against the sidereal
rate rather than a second implementation (#254).
- `core.ID` and `jpl.NAIFFor` now document which bodies are system barycenters
rather than planets — the giant planets are, and the gap is 0.03–0.05″ against
a reference that defaults to the body center. A test pins the numbers and the
center-versus-barycenter split (#258).
- `docsguard` now catches a dependency bumped in the root module but not mirrored
into `examples/`. The examples module carries the library's dependencies as
indirect requirements, and a stale copy makes `go build ./...` inside it refuse
outright — previously visible only after a push, from CI (#259).
- Every geocentric stage of the observed-place pipeline is now excluded by
measurement — ephemeris, light time, apparent RA and Dec, Earth rotation, polar
motion, all within 0.05″ of Horizons — leaving the topocentric step itself as
the sole remaining suspect for the ~0.5″ azimuth residual (#260).
- `coord/sofareference_test.go` pins astrogo's topocentric reduction against
SOFA's `iauAtco13` over 210 site/direction/epoch combinations, offline, at a
measured maximum of 0.000″ against a 1 µas contract. It is what excludes the
last stage of the observed-place pipeline as the source of the ~0.5″ azimuth
residual (#260).
- Offline tests for the apparent-place failure paths #263 added: each of the four
provider fetches is made to fail on its own and asserted to report *which* one
did, and the three degenerate deflection geometries are pinned as returning the
place undeflected rather than NaN (#265).
- `coord.Reduction`'s fields now say what they are and how they relate. Two are
positions and two are directions, and since #262 put diurnal aberration on the
direction path, rotating `Topocentric` by hand no longer reproduces `Geometric`
— they differ by up to 0.32″, which is now measured by a test rather than left
to be discovered (#267).
- **`fits.WCS.CUnit` exposes the axis units the header declares**, which astrogo
did not read at all. `PixelToWorld` returns CRVAL plus a linear offset for any
non-celestial axis, so its value was correct in a unit no part of the API could
name — meters or Angstrom for a spectral axis, seconds or days for a time one.
Both transforms now also document what they return: degrees for celestial axes,
each other axis in its own `CUNITi` (#178).
- **GCS, Azure Blob Storage and SFTP join S3 as opt-in bucket backends** —
`remote/file/{gcs,azure,sftp}`, each a blank import with zero exported symbols registering
`gs://`, `azblob://` and `sftp://`. Every connection detail rides in the endpoint URL, so
`remote.SetDataDir("gs://my-bucket")` is the whole configuration. Verified: each pulls only
its own SDK, and a build that opens none links none of them. (#274)
- **`remote.Client` scopes I/O policy to a component instead of a process.** Offline mode,
download consent, endpoint URLs and the cache location are now a value: `remote.NewClient()`,
configure it, hand it to the component that owns it. An HTTP handler can be offline-only
while a background prefetcher downloads, in one binary. The package-level functions operate
on `remote.Default()` — the `http.DefaultClient` analogue — so nothing changes for a program
with a single policy. Closes #114. (#274)
- **`remote.HTTPError` is reachable again**, along with `RetryPolicy`, `Attempt`,
`DefaultRetryPolicy` and `ErrRetriable`. #195 removed the old `remote.HTTPError` because
it was a second, identically shaped type and `errors.As` against the wrong one failed
silently; these are aliases for the subpackage's own types, so there is one type with two
names and nothing to pick wrongly. (#274)
- **The FITS conventions real files use, in both directions.** HIERARCH keywords and CONTINUE
long strings (previously mangled and truncated on *read*, not merely unwritten); unsigned
images through BZERO; DATASUM and CHECKSUM on every HDU, satisfying the sum-to-all-ones test
cfitsio and astropy apply; TNULL and NaN so a missing table value stays missing; vector
columns, which used to decode as nulls and discard every value; and ASCII tables, whose
reader consumed the payload without decoding it (#127).
- **`fits.Write` — the package writes FITS as well as reading it.** Images at every BITPIX
(uint8, int16/32/64, float32/64) and binary tables built from an Arrow batch, to any
`io.Writer`. Structural keywords are derived from the data rather than copied from the
stored header, so a filtered table or a replaced image cannot produce a file whose header
describes something it does not contain. A card that will not fit the 80-byte record is an
error rather than a truncation, since an over-long card shifts every card after it. The primary header carries `EXTEND` when extensions follow, and a non-finite `BSCALE`/`BZERO` is refused rather than written as text no reader can parse (#127).
- **`time.TCG` and `time.TCB` — the two coordinate time scales.** TCG is the geocentric
frame's coordinate time (TT rescaled by L_G, 22 ms/year) and TCB the barycentric frame's
(TDB rescaled by L_B, 0.49 s/year), which is what relativistic geodesy, orbit integration
and pulsar timing are quoted in. Both convert to and from every other scale and are exact
in both directions; verified against SOFA's own published values for `iauTttcg`,
`iauTcgtt`, `iauTdbtcb` and `iauTcbtdb` (#126).
- `coord.FK5`, `coord.FK5ToICRS` and `coord.ICRSToFK5` read and write FK5 J2000
positions, and `coord.FK4ToFK5`/`coord.FK5ToFK4` expose the classic B1950 ↔
J2000 conversion that was previously buried inside `FK4ToICRS`. A catalogue
that says "J2000" is usually FK5, not ICRS, and the two differ by the ~20 mas
frame bias plus an epoch-dependent spin — so, like FK4, each direction has a
position-only route and a six-element one (#126).
- `coord.SkyOffset` is a frame centered on a target, so positions near it can be
given as offsets — dither and mosaic patterns, offset guide stars, slit
layouts, finder charts. It is a rotation of the sphere rather than a projection
onto a plane, which is the difference between it and subtracting coordinates: a
point one degree due east of a target at δ = 80° differs from it by 5.7° of
right ascension and by three arcminutes of declination (#126).
- `coord.LSRCorrection` refers a barycentric radial velocity to the Local
Standard of Rest, which is the frame Galactic work is quoted in — the Sun's own
18 km/s through its neighbourhood otherwise sits in every measurement. It takes
an `LSRKind` rather than choosing: Schönrich, Binney & Dehnen (2010) and
Delhaye (1965) disagree by 2.1 km/s, so a v_LSR quoted without naming its
convention carries that much ambiguity. `coord.LSRApex` reports the direction
and speed each one implies (#126).
- `coord.Context.ICRSToITRS` and `ITRSToICRS` expose the rotation between the
celestial frame and the rotating Earth — station coordinates, ground tracks,
anything Earth-fixed. The matrix was already built and cached for every horizon
transform, so this is a matrix multiply rather than a second implementation,
and a test now pins the cached factored form as bit-identical to SOFA's
one-call `C2t06a` (#126).
- `coord.TETE` is the apparent place referred to the true equator and true
equinox of date — what almanacs and most telescope control systems mean by
"apparent RA and Dec". `coord.Context.CIRSToTETE` converts to it from
`coord.CIRS`, which measures right ascension from the Celestial Intermediate
Origin instead. The two are apart by the equation of the
origins: **20.3 arcminutes in 2026**, growing by 46 arcseconds a year (#126).
- `coord.Supergalactic` puts the flattened sheet of nearby bright galaxies on the
equator — the Local Supercluster, with the Virgo cluster near its center — the
way Galactic coordinates do for the Milky Way's disc. `ICRSToSupergalactic` and
`SupergalacticToICRS` convert. The de Vaucouleurs pole at Galactic l = 47.37°,
b = +6.32° is only 6.32° off the Galactic plane, so the two planes are nearly
perpendicular: the supercluster is cut in half by the zone of avoidance (#126).
- `coord.SunBarycentric`, `BarycentricToHeliocentric` and
`HeliocentricToBarycentric` move a position between the solar system
barycenter and the center of the Sun — the HCRS frame, which keeps the ICRS
axes and shifts only the origin. That shift reaches 0.009 AU, nearly two solar
radii, so it is half a degree seen from one AU and nine milliarcseconds seen
from a parsec. Unlike every other frame here it is a translation rather than a
rotation, so it takes a position vector rather than a direction (#126).
- `docs/sgp4.md` — the design for a from-scratch SGP4 at
`ephemeris/satellite/sgp4`, and the measurement behind it: the divergences
astrogo's Vallado suite has been reporting are one transcription error in the
current dependency, `128.0` where the algorithm says `120.0` in the s⁴ drag
coefficient. [#309] (#310)
- `constants.WGS72` — the World Geodetic System 1972 realization that every
two-line element set is expressed in, as its own `Set` alongside `WGS84`.
`WGS84Set` also gains the `GeocentricGravitationalConstant` its own comment
named as the fourth defining parameter without carrying it, so the two sets
can be compared member for member. (#311)
- `spk.ErrHorizonsInternalFault` and `spk.TransientHorizonsFault` separate a JPL
Horizons outage from a Horizons refusal. Both arrive as HTTP 200 with a
well-formed body and mean opposite things: one resolves itself, the other never
will. Retry logic and astrogo's own live-network tests now branch on the
difference instead of treating every refusal alike. [#312] (#313)
- `ephemeris/satellite/sgp4` — a public SGP4 package written from Vallado's
published algorithm, starting with its element-set layer: `Elements`,
`ParseTLE`/`ParseTLEName`, `VerifyTLEChecksums` and the `Gravity` models.
Checksum verification is a separate call from parsing, which is what makes
Vallado's three deliberately-bad-checksum cases readable and takes astrogo's
coverage of its own reference suite from 30 of 33 to 33 of 33. No propagation
yet — see `docs/sgp4.md`. [#310] (#316)
- `ephemeris/satellite/sgp4` gains the near-Earth propagator: `New`, `At`
(minutes from epoch as a float), `AtTime`, and the model's own branch
predicates. Measured against Vallado's reference states it agrees to a maximum
of **9 nanometers** across 158 states — and the three near-Earth cases the
current dependency misses by up to 3438 km agree to 6 nanometers, confirming
the diagnosis in [#309]. Deep space (SDP4) is next. [#310] (#317)
- `ephemeris/satellite/sgp4` gains the deep-space (SDP4) path — lunisolar
periodics, geopotential resonance, and the Lyddane formulation. All **33** of
Vallado's reference cases now run, 666 states, agreeing to a maximum of 4.1e-06
km. Every one of the seven divergences astrogo has been reporting against the
current dependency is gone, including the one [#309] predicted would survive.
Vallado's three error-return cases are exercised for the first time. [#310] (#319)
- `docs/storage.md` — the design for replacing `gocloud.dev/blob` with the
standard library's `io/fs`, plus three small interfaces for the things `io/fs`
lacks (streaming writes, delete, and a context). The measurement behind it:
importing `astrogo/plan` links 433 packages, 107 of them gRPC, protobuf and
OpenTelemetry that `gocloud.dev/blob` pulls in unconditionally so it can emit
traces nobody consumes. `time`, `coord` and `ephemeris` link none. (#321)
- `remote/file` gains an `io/fs`-based storage core: `File` (`fs.File` +
`io.ReaderAt` + `io.Seeker`), the `CreateFS`/`RemoveFS`/`ContextFS` extension
interfaces, a scheme registry, and a `file://` backend. It sits alongside the
gocloud path for now. The new backend fixes [#315]: staging is named from the
process id and an atomic counter rather than a clock that does not advance on
Windows, and happens inside the tree so the rename cannot cross a volume. (#322)
- `remote/file`'s `io/fs` core gains `http`/`https` and `mem://` backends. The
HTTP one is read-only by design — no astrogo source accepts a write — and keeps
one body open across sequential reads while each `ReadAt` takes its own range.
`mem://` exists for the write path that `fstest.MapFS` cannot cover, keyed by
URL host so two opens can share a store or deliberately not. (#324)
- **Round-trip tests over all six elements, for every conversion between the
frames that carry kinematics.** ICRS, FK5 and FK4 give six directed
conversions; each is now exercised over a grid of eight sky positions crossed
with five kinematic profiles — 240 cases — asserting position, both proper
motion components, parallax and radial velocity. The existing tests compared
positions, which is how [#278] shipped: the proper motion was wrong by
0.6–0.9 mas/yr while the position closed to 19 µas. The matrix also turned up
[#331], where a conversion silently needs a non-zero parallax. (#332)
- `coord.LSRKinematic` is the kinematic Local Standard of Rest — what radio
spectroscopy means by "LSR", and what a spectral line's velocity is quoted
against unless a paper says otherwise. It joins the two dynamical kinds in
`LSRCorrection` and `LSRApex`. Unlike them it is published as an apex rather
than as Galactic components — 20 km/s toward RA 270°, Dec +30°, **B1900
equinox** (Gordon 1975) — so astrogo derives the ICRS vector from that
statement rather than copying a converted one. The B1900→B1950 step uses IAU
1976 precession where Newcomb's is correct, which is measured rather than
assumed: 0.55″ of direction and 5.4 cm/s against Astropy's independent
realization of the same definition. Closes [#295]. (#333)
- `coord.Galactocentric` and `coord.GalactocentricFrame` express a position in
the right-handed Cartesian frame centered on the Galactic center, in parsecs —
the frame a rotation curve, a disc scale height or a stellar stream is actually
written in. `GalactocentricFrame.FromICRS` takes the distance as an explicit
argument rather than reading `ICRS.Dist`, whose unit depends on the subsystem
that filled it in, and `coord.ParallaxDistance` converts a catalogue parallax
into the parsecs it wants. Measured parameters are arguments (R₀ = 8178 pc,
GRAVITY Collaboration 2019; z☉ = 20.8 pc, Bennett & Bovy 2019); the orientation
is not, so the axes come from the IAU Galactic frame this package already
implements rather than from a second definition that could drift from it.
Verified against Astropy's independent parameterisation of the same frame,
which astrogo never writes down: 0.33″ in the Galactic-center direction and
0.12″ in the roll, both of which are Astropy's rounding of the shared
convention. Positions only — astrogo has no space-velocity type, so a
Galactocentric *velocity* is not yet expressible. This was the last frame on
the [#126] checklist; the general transform graph on it remains open. (#334)
- `coord.SpaceVelocity` returns a target's velocity with respect to the solar
system barycenter in km/s, as Cartesian components on the ICRS axes, and
`coord.SpaceSpeed` its magnitude. Proper motion is an angular rate and radial
velocity is a linear one, so neither can be compared with the other; this is the
combination that Galactic UVW velocities, cluster membership tests and orbit
integrations all start from, and astrogo had no way to compute it. Validated
against Barnard's Star at 142.5 km/s, a figure published independently of this
library, and internally against the classical v = 4.74047·μ·d identity. Both
report a bool rather than a velocity when the target records no kinematics or no
usable parallax: unlike a frame conversion, where the distance divides out and
#331's fix exploits that, turning an angular rate into km/s genuinely needs the
distance — the same 150 mas/yr is 7 km/s at 10 pc and 700 at a kiloparsec — so
there is nothing to return rather than a plausible figure built on a distance
nobody supplied. This is the first of the three pieces [#335] needs before
`Galactocentric` can carry velocities; the frame-specific parts follow
separately. (#343)
- `coord.Galactocentric` now carries a velocity as well as a position.
`GalactocentricFrame.FromICRS` attaches one in km/s whenever the target's
kinematics can supply it, `Galactocentric.Velocity` reports whether it did, and
`ToICRS` reconstructs catalogue proper motion, parallax and radial velocity on
the way back, so the pair is a real inverse rather than one that silently drops
half the state. The Sun's velocity is a third measured frame parameter beside R₀
and z☉, and `coord.SolarVelocityFromSgrA` derives it rather than copying a
triple: the rotational component is the Sun's distance times the apparent proper
motion of Sgr A* (6.379 ± 0.024 mas/yr, Reid & Brunthaler 2004), which is the
reflex of the Sun's own orbit, while the radial and vertical components are its
peculiar motion with respect to the LSR (Schönrich, Binney & Dehnen 2010, already
cited here for `LSRDynamical`). Deriving it means the rotational component tracks
whatever R₀ the frame was built with, so a frame cannot mix one paper's distance
with another's velocity — and it reconstructs Astropy's own number exactly:
evaluated at their R₀ of 8122 pc it gives 245.6049 km/s against their published
245.6, because their V *is* R₀ × μ. Closes [#335]. (#344)
- `remote.ErrNotServingData` lets a caller tell "the archive is down" from "the
archive sent nonsense". Archives serve their own failures — a maintenance
notice, a load shedder, a login wall — as an HTML page with a **200**, so
nothing before the parser can tell, and both cases used to arrive as an opaque
error string. They want opposite handling: a web page should be retried later, a
malformed payload should not be retried at all, and without the distinction the
honest options were to retry everything or to retry nothing. `catalog/gaia`,
`catalog/simbad`, `catalog/vizier` and `skybrightness/dataset/starlight` now
report it, and `remote.LooksLikeHTML` is the shared leading-bytes check they use
— deliberately not a content-type sniffer, since a VOTable is XML too and
telling those apart is the whole point. The sentinel lives in `remote` rather
than beside a parser because the condition is a statement about what the
endpoint did, not about the payload, and because `skybrightness` cannot import
`catalog` without inverting the layering — a sentinel on `catalog/resolve` would
have served the providers and forced the dataset tier to invent a second name
for the same thing. Detection stays with the parsers, which are the only code
that knows what the payload should have looked like. Closes [#300]. (#347)
- `unit.Length` and `unit.Velocity` are named `float64` types carrying meters and
meters per second, with constructors and accessors for every unit astrogo
speaks — `unit.AU`, `Km`, `Pc`, `Meters`, `KmPerSec`, `AUPerDay` and the
readers that match. They are the pattern `angle.Angle` already uses, extended
to the two dimensions the API was passing as bare `float64`. Measured, a named
`float64` is indistinguishable from the `float64` it replaces (1.88 ns against
1.88 ns scalar; 1887 ns against 1888 ns over a 1000-element batch; 8 bytes
either way), while `unit.Quantity` is 56 bytes and 14.3× slower over that batch
— neither allocates, so the difference is width and cache pressure rather than
the heap. New allocation contracts in `unit` hold that claim. The constructors
read their scale factors from this package's own `Unit` table rather than from
constants of their own, so a `Length` and a `Quantity` cannot come to disagree
about how long an astronomical unit is; `Length.Quantity` and `unit.LengthFrom`
bridge the two when a value has to compose dimensionally. (#353)
- **`unit.Duration`**, an elapsed time stored in seconds, with `Seconds`,
`Minutes`, `Hours`, `Days` and `JulianYears` constructors and matching
accessors. `unit.JulianYear` joins the unit table as 365.25 days exactly.
It is the third named scalar after `Length` and `Velocity`, and the one they
implied: `Velocity` is declared as `Meter.Div(Second)`, so the package has been
doing time arithmetic since it was written without a type for it.
`time.ToGoDuration` and `time.FromGoDuration` convert to and from the standard
library's `time.Duration`, which remains what a timeout, a ticker or a sleep is
measured in. `ToGoDuration` reports whether the value fit, since an int64
nanosecond count stops just past ±292 years. (#363)
- `Time.UT1Using(dut1)` converts to UT1 with a UT1−UTC the caller already holds,
for code that caches Earth orientation parameters; `Time.UT1` is built on it. Adding
DUT1 to a UTC Julian Date by hand is wrong by up to a second on a day that ends
in a leap second, and only `time` knows which days those are. (#381)
- **Every planet's magnitude is pinned to JPL Horizons.**
`TestPlanetMagnitudesAgreeWithHorizons` holds Mercury, Venus, Jupiter, Uranus
and Neptune to 0.02 mag at 29 dates across their phase ranges (measured: 0.007
at worst). Mars is held to 0.1 mag, because the rotation and orbital-longitude
corrections of Mallama & Hilton (2018) are not yet applied (#389).
- **Comets on open orbits propagate from the catalogs.** `resolve.Target` carries
the comet form of its elements (`PerihelionDistance`, `PerihelionTime`), SBDB
decodes it, and `plan.FromCatalog` builds an orbit with e >= 1 from it instead
of dropping it to the kernel path. `mpcorb.Read` and `mpcorb.Open` read the
MPC's `CometEls.txt` too, all 959 comets, 118 of them on open orbits (#374).
- **Parabolic and hyperbolic orbits in `ephemeris/kepler`.**
`kepler.FromPerihelion` (and `eph.ElementsFromPerihelion`) builds elements
from perihelion time, distance and eccentricity, the MPC's comet form, for
any e >= 0, and propagates them by universal variables. It agrees with
Horizons' own conversion of 1I, 2I, 2P and C/2023 A3 to 1e-11 AU.
`Elements.PerihelionDistance` reports q for either form (#374).
- **`magnitude.PlutoCharonApparent`**: V magnitudes of Pluto, Charon and the
pair from Buie et al.'s (2010) Hubble light curves — rotation, nonlinear Hapke
phase curves, fluxes combined — reproducing the paper's 2002–2003 photometry to
0.005 mag on average. A calibration of that epoch: in the 2020s it runs
0.16–0.36 mag fainter than `PlanetApparent`'s historical law, and which is
nearer is not yet established (#410).
- **`plan.EclipseEvent` carries the eclipse's `Kind`** — penumbral, partial,
total, annular or hybrid — **and its `Magnitude`** (and, for the Moon,
`PenumbralMagnitude`), decided as NASA's Five Millennium Canons decide them:
every kind agrees over six centuries of the canon, hybrids included, and
magnitudes are within 0.0004. Callers no longer have to guess the kind from
ecliptic latitude, which called the partial eclipse of 2026-08-28 total (#405).
- **`plan.CrescentVisibility`** evaluates a young crescent on one evening from a
real sunset, moonset and best time, giving each criterion the quantities its
author defined: geocentric for Yallop, topocentric for Odeh (#496).
- **`CrescentParams.AlrefayOpticalAid`**, Alrefay et al.'s (2018) criterion for
optically aided sighting, beside their naked-eye one (#503).
- **Civil twilight is now checked against USNO.** The USNO comparison fetched
Begin and End Civil Twilight with every response and discarded them. All 18
events across three sites and dates agree within 0.5 min, USNO's own rounding. (#534)
- **Greatest elongations are now checked against Skyfield.** All seven of 2026 on DE440s agree within 0.58 s and to four decimals in elongation, with the east/west side right each time. (#559)
- **Satellite passes are now checked against Skyfield.** Six ISS passes over two days agree in rise and set within 0.38 s, culmination within 0.49 s and highest elevation within 0.005°. (#561)
- **Nautical and astronomical twilight are now checked against Skyfield.** 30 crossings agree within 0.26 s, at La Silla and on solstice nights at 48.56°N whose darkness, as short as 5.6 minutes, falls wholly between the solver's 15-minute samples. (#567)
- **Constellation lookup is now checked against Roman (1987)'s boundary table.** 199,999 of 200,000 random positions agree, and a test holds the 64 lying within about 22″ of a boundary. astropy's `get_constellation` is not used as the reference, because it carries 18–22″ of annual aberration into the lookup. (#571)
- **Appulses are now checked against Skyfield.** On DE440s, four pairs agree on the instant of minimum separation within 0.44 s and on the separation to 10⁻⁶°. (#580)
- **The η-Aquariids and Orionids are now checked against observed returns.** Against Egal et al.'s (2020) re-analysis of IMO's visual data for 2001–2019, each shower's maximum falls 0.10° from the median return, its rate is within twice the returns' scatter, and its activity profile is within ×1.8 of the observed average down to a tenth of the maximum. (#587)
- **`plan.WithHorizonRefraction` refracts a site's rise and set horizon through the air it names**, in place of the almanacs' fixed 34′: 33.85′ at 10 °C and 1010 hPa, 38.6′ at −20 °C and 1030 hPa. The default stays 34′. No model predicts a real rise or set better than about 2 minutes, and the docs say so. (#590)
- **`atmosphere.Atmosphere.Extinction` gives a site's extinction coefficient from its own air**: Rayleigh scattering from the surface pressure, ozone from its column, and aerosol from its optical depth. It reproduces Paranal's measured extinction curve within 0.01 mag/airmass, and Mauna Kea's published decomposition term by term. (#593)
- **`plan.WithAtmosphere` names the air `VisibleTonight` dims every object through**, such as tonight's measured aerosol and ozone, in place of the default clean night at the site's height. (#594)
- `atmosphere.Atmosphere.OzoneOpticalDepth`, the vertical optical depth of an air's ozone column, and `atmosphere.OzoneAirmass`, the airmass of the ozone layer 20 km up: the two terms `Atmosphere.ExtinctionToward` already used for ozone. (#649)
- **A release every week, and a GitHub Release for every tag.** `docs/RELEASING.md` sets the cadence (every Monday when anything merged; security fixes and regressions within a day), reads the version off the changelog fragments, and routes the release commit through a pull request. A tag push now publishes a GitHub Release with that version's changelog section as its notes. (#657)
- **`coord.HEALPix`'s NESTED indexing is held to an independent implementation.** `TestHEALPixMatchesAstropyHEALPix` checks `PixelOf` against astropy-healpix 2.0.1 at 32 directions across all twelve base faces, at nside 256 and 4096. The existing tests were self-consistency checks, which a scheme with its x and y bits interleaved the other way passes in full. (#664)
- **Sidereal time is held to USNO's.** `TestUSNO_SiderealTime` compares `Time.GAST` and `Site.LocalSiderealTime` with USNO's sidereal-time service (NOVAS) at six epochs from 1990 to 2049, east and west of Greenwich, to 0.2 ms; every residual is within USNO's own 0.05 ms rounding. Before, nothing held sidereal time tighter than 0.5°, and the test of that name checked only that the result lay in [0°, 360°). (#673)
- `plan.EventSpec.UpperLimb` applies `Threshold` to a moving body's upper limb, raising its altitude by half its `AngularDiameter` at every instant the solver evaluates; `Validate` returns `plan.ErrUpperLimbNeedsBody` for a target that is not a `MovingBody`. (#703)

### Changed — BREAKING
- **`resolve.Provider` returns errors instead of a bool.** `Resolve` is now
`(Target, error)` and `Search` is `([]Target, error)`, so a transport failure,
a canceled context or an unreachable service is no longer reported as
"target not found". `errors.Is(err, context.Canceled)` works through the
catalog layer for the first time. Adds `resolve.ErrUnsupported` for
cone-search-only providers, and `catalog/fink` and `catalog/fits` now take a
`context.Context`. (#151)
- `skybrightness/plan.Spec.Sky` is now the four-method `plan.Sky` interface rather
than `*dataset.Sky`. A `*dataset.Sky` satisfies it, so every caller is unchanged
— what it buys is that `LimitingMagnitudeAt` can be evaluated without a network
and 145 MB of reference data, which is why it went uncovered on every commit
(#122).
- **`ephemeris`'s kernel sources now need one blank import.** `import _
"github.com/TuSKan/astrogo/ephemeris/jpl"` registers the backend `Planets`,
`SmallBody`, `Asteroids`, `Comets` and `Moons` use; without it they return an
error naming it. Asking SOFA where Mars is went from 13.9 MB and 424 packages to
4.7 MB and 224, with gRPC, OpenTelemetry, protobuf and `gocloud.dev` at zero —
64 packages of gRPC were arriving for an error-code enum. `eph.JPL` is removed;
name `jpl.Provider` (#112).
- `fits.WCS`'s getters lose their `Get` prefix (`GetCRVAL` is now `CRVAL`), and the
five setters with a length invariant return `error` — a short CRPIX was a panic
reachable from `PixelToWorld` and a long one a silent disagreement about axis
count, both accepted without a word. New `NAxis` reports the invariant (#178).
- `Observable.GetDetails` takes a `plan.DetailOverrides` struct instead of
`props ...string` read two at a time. That form had three silent failures: an
odd count dropped the last argument, a misspelled key landed in `ExtraProps`
while the field it meant to override kept its computed value, and a key and
value could be swapped with nothing to notice (#116).
- `plan.ScoreObservable` no longer exists; scoring is `plan.Scorer{...}.Score(obj, t)`.
It took six parameters, two of them nilable pointers that were nil at nearly
every call site including the README's — and two adjacent nils of different
types can be transposed without the compiler noticing (#116).
- **`remote.APIClient` no longer exists; `remote.Client` carries the request methods.** `Get`, `GetJSON`,
`PostForm` and `PostJSON` are methods on the policy that decides whether the request may happen
at all, so a caller holds one object instead of two that each answered half the question.
`remote.Default()` is the process-wide one; a component wanting its own timeout, pacing or token
takes `remote.Default().Clone()` and calls `SetAPIOptions`. Transports are built per endpoint with
that endpoint's registered `Timeout`, which fixes a latent defect: one client used for two
endpoints previously applied whichever timeout was named at construction to both. (#274)
- **IERS Earth-orientation data now needs `import _ "github.com/TuSKan/astrogo/remote/eop"`.**
The loader moved out of `remote` because it needs `remote.GetFile` and so cannot live in
the package that would have to import it back. Without the blank import, `Time.EOP`/`.UTC`/
`.UT1` report zero DUT1 and polar motion and log one warning — costing about an arcsecond
of topocentric position, measured on the Horizons corpus as a p50 of 1.4 arcsec becoming a
max of 8.8 arcsec. (#274)
- **`remote/file` and `remote/api` are internal to `remote`.** Everything they do is
reachable from `remote` itself — `Bucket` (an alias for `blob.Bucket`), `OpenBucket`,
`Save`, `ReaderAt`/`NewReaderAt`/`WithChunkSize`/`WithCachedChunks`, `APIClient`/
`NewAPIClient` with the `With*` options, `HTTPError`, `RetryPolicy`/`Attempt`/
`DefaultRetryPolicy`, `DefaultAPITimeout` — and importing either package from outside
`remote/` now fails `TestSubpackagesAreNotImportedDirectly`. Going around the front door
skipped the endpoint registry, offline mode and the consent gate for that one call site,
silently. (#274)
- **`remote/s3` moved to `remote/file/s3`.** S3 is a file backend, so the opt-in blank
import now sits where the file code does: `import _ "github.com/TuSKan/astrogo/remote/file/s3"`.
Still four lines and zero exported symbols, and still the module's only importer of the
AWS SDK. (#274)
- Two exported symbols are gone, and neither appeared in a tagged release, so
**no released version is affected**.
`Satellite.Verified` no longer exists: there is no longer a regime for it to
flag. Use `Satellite.Propagator` with `sgp4.Propagator`'s `SimplifiedDrag`,
`DeepSpace` and `PerigeeAltitude`, which describe the orbit rather than
astrogo's coverage.
`sgp4.ErrDeepSpace` no longer exists either — it was scaffolding while SDP4 was
being written, and deep space propagates now, so nothing raises it and the
branch handling it can go.
Recorded because a caller pinned to a `main` commit between those changes gets a
compile error, which is the one audience a release-to-release note would miss.
[#310] (#320)
- **`remote` is built on `io/fs`, and `gocloud.dev` is gone.** A storage container
is now an `fs.FS` and an open object a `remote.File` (`fs.File` + `io.ReaderAt` +
`io.Seeker`). `remote.Bucket` no longer exists: it is `remote.FS` now, with
`OpenBucket` → `OpenFS`, `NewReaderAt` → `Open`, `Save` → `WriteFile`, and
`IsNotFound` deleted in favour of `errors.Is(err, fs.ErrNotExist)`. Linked packages for a consumer of
`remote` fall from 406 to 219, with gRPC and OpenTelemetry — 98 packages
astrogo never configured — going to zero. (#325)
- **`coord.Apparent` no longer exists — it is `coord.CIRS` now, with its
transforms.** The name said the one thing the type is not: "apparent place" has meant the equinox-based
place — true equator, true equinox of date — for two centuries, and this is the
CIRS place, which measures right ascension from the Celestial Intermediate
Origin. They are apart by the equation of the origins: **20.3 arcminutes in
2026**, growing 46 arcseconds a year, in right ascension only — so a caller
comparing against an almanac sees a pure RA offset and reaches for a
sidereal-time bug. `coord.TETE` is the equinox-based place and keeps the word
everyone else means by it.
Migration is mechanical: `Apparent` → `CIRS`, `NewApparent` → `NewCIRS`,
`AstrometricToApparent` → `AstrometricToCIRS`, `ApparentToObserved` →
`CIRSToObserved`, `ApparentToTETE` → `CIRSToTETE`, `TETEToApparent` →
`TETEToCIRS`. `Name()` now returns `"CIRS"` and `String()` is prefixed `CIRS`.
A program that stays inside the pipeline is otherwise unaffected — the numbers
do not change. Closes [#298]. (#328)
- **`coord.NewGalactocentricFrame` takes the Sun's velocity as a third argument.**
It is a measured frame parameter beside R₀ and z☉ — the frame cannot place a
target's velocity without knowing its own — so it is supplied the same way they
are rather than being a hidden default. A caller passes
`coord.SolarVelocityFromSgrA(sunDistance)` for the derived value, the same
distance as the first argument so the two stay consistent, or their own
`vector.Vec3` in km/s to reproduce another frame:
```go
// before
f := coord.NewGalactocentricFrame(8178, 20.8)
// after
f := coord.NewGalactocentricFrame(8178, 20.8, coord.SolarVelocityFromSgrA(8178))
```
`coord.DefaultGalactocentricFrame` is unchanged and already does this, so code
using it needs no edit. The function being reshaped was added in [#334] and has
not appeared in a release. (#344)
- **`time.J2000` is now a function, `time.J2000()`.** It was the last symbol
astrogo itself exported as a mutable package-level variable, so any package
anywhere in the import graph could reassign the standard epoch and change every
epoch calculation in the process — a supply-chain footgun in the package that
underpins the rest of the library. Go cannot declare a struct value immutable,
so a function returning a copy of an unexported one is the only construction
that removes it; the value is computed once and the call costs a struct copy.
Callers write `time.J2000()` wherever they wrote `time.J2000`, and the compiler
finds every site. `time.LocationUTC` remains a var and is now the only one that
is not a sentinel error: the standard library declares `var UTC *Location`, so
wrapping it would hand back the same reassignable pointer and remove nothing.
README's claim is corrected to say that rather than to imply nothing is
reassignable at all, and `internal/docsguard`'s inventory of remaining vars
records why each survives. Closes [#113]. (#346)
- **The `Dimension` type and its seventeen values move from `unit` into a new
`unit/dim` package**, so they are now `dim.Dimension`, `dim.Length`,
`dim.Velocity` and so on. `unit.Unit` keeps its `Dimension` field, retyped to
`dim.Dimension`.
The names collided: the dimension of a length held the name a caller wants for
the *quantity* a signature carries, which is what `unit.Length` and
`unit.Velocity` now are. Prefixing the dimensions instead would have read as
`DimVolume.Div(DimMass).Div(DimTime.PowInt(2))` at the places that actually use
them — `constants/units.go` composes six units that way — and the dimensionless
one stuttered. A package is how Go namespaces, so those lines now say
`dim.Volume.Div(dim.Mass).Div(dim.Time.PowInt(2))`.
Dimensions are also the more primitive idea: a unit is a scale on a dimension,
so `unit` imports `dim` and not the reverse. Fifteen call sites outside `unit`
were affected, all in `constants`. (#353)
- **`coord`'s distances and speeds are now `unit.Length` and `unit.Velocity`
instead of `float64`.** Every signature that carried a length or a speed changed:
`ICRS`/`AltAz`/`Galactic`/`Ecliptic`'s `Dist`/`SetDist`, `Astrometric.RV`/`SetRV`,
`NewICRSWithKinematics`, `NewFK4WithProperMotion`, `NewFK5WithProperMotion` and
`FK4.RV`/`FK5.RV`, `Geodetic.Height`/`NewGeodetic`/`MustGeodetic`, `Ellipsoid.A`,
`NewObserversLocation`/`SetHeight`, `GroundDistance`, `Offset`,
`ParallaxDistance`, `SpaceSpeed`, `LSRCorrection`, `LSRApex`, the
`GalactocentricFrame` parameters and `FromICRS`/`ToICRS`, `Galactocentric`'s
`X`/`Y`/`Z`/`Distance`/`Radius` and its two constructors, and `Context`'s five
radial-velocity methods. `ephemeris/satellite.Satellite.Altitude` follows, since
it returns a `Geodetic` height.
`ICRS.Dist` is the reason: it held astronomical units on an ephemeris path and
kilometers on a satellite one, and nothing in the signature said which. Callers
read the unit they want — `d.Km()`, `d.AU()`, `d.Pc()` — and write the unit they
mean — `unit.KmPerSec(-7.6)`. Named `float64`s, so this costs nothing: measured
at 1.88 ns and zero allocations against the same for a bare `float64`.
`NewEarthLocation` deliberately keeps plain `float64` degrees and meters, since
its whole purpose is to accept numbers copied off a GPS or a map service.
Part of #130; `plan`, `ephemeris`, `atmosphere`, `magnitude` and `optics` still
take bare `float64` and convert at the `coord` boundary. (#355)
- **`plan`'s distances and speeds are now `unit.Length` and `unit.Velocity`**,
following `coord` in #355. Two optional-capability interfaces changed, so a
target type outside this repo that implements either needs its signature
updated: `MeasuredRadialVelocity() (unit.Velocity, bool)` and
`PhysicalRadius() (unit.Length, bool)`.
Also retyped: `WithRadialVelocity`, `WithDSORadialVelocity`, `WithDiameter`,
`RadialVelocity`, `BodyEquatorialRadius`, `TargetDetails.Distance`,
`ApsisEvent.Distance`, `PassEvent.Range` and `MeteorShower.VelocityKmS` — the
last renamed to `Velocity`, since the unit is no longer part of the name.
`Site.Height` is new and returns a `unit.Length`. It replaces
`Site.HeightMeters`, which #533 removed.
`TargetDetails` is the one worth reading about. Its `Distance float64` meant
parsecs for a star, au for a planet and kilometers for a satellite, and the
neighbouring `DistanceUnit string` was the only record of which — so a caller
reading `Distance` without reading `DistanceUnit` alongside it got a number
three different scales could produce. `Distance` now carries its own unit and
`DistanceUnit` only decides which one `String` prints.
`NewSiteEarthLocation` deliberately keeps plain `float64` degrees and meters,
for the reason `coord.NewEarthLocation` does. (#357)
- **`atmosphere`'s heights and scale heights are now `unit.Length`**, following
`coord` in #355 and `plan` in #357. Retyped: `AtAltitude`, `HorizonDip`,
`StandardDefault`, `Builder.SurfaceAtAltitude`, `Builder.AerosolScaleHeight`,
`VanRhijn`, `MolecularScaleHeight`'s return, `ExtendedSourceOpticalDepth`'s
observer height, `ExponentialExtinction`/`ExponentialDepth`, the eight OPAC
aerosol preset constructors, and `CloudLayer.BaseAlt`/`TopAlt` and
`Aerosol.ScaleHeight`. In `skybrightness`: `AirglowRadiance`, `NewAirglow`,
`OpticalParameterT` and `dataset.AerosolPreset`.
`ContinentalScaleHeightM`, `DesertScaleHeightM`, `MaritimeScaleHeightM` and
`AirglowLayerHeightM` lose the `M` and become typed `unit.Length` constants,
since the unit is no longer part of the name.
`skybrightness.OpticalParameterT` declared its two scale heights as
`unit.OpticalDepth`. They are lengths, as its own arithmetic shows: it divides
an optical depth by one to get an extinction per unit length. The numbers were
right and the type was a lie; both scale heights and the separation are now
`unit.Length`, and no computed value changes.
`MolecularScaleHeightM` and `AerosolScaleHeightM` in `transfer.go` keep their
names and stay `float64`: they appear only inside
`ExtendedSourceOpticalDepth`'s own arithmetic and are named after the symbols
in Masana et al. (2021), rather than crossing the API the way the preset scale
heights do. (#358)
- **`optics` and `magnitude` now carry lengths as `unit.Length`**, finishing the
mechanical half of #130 after `coord` (#355), `plan` (#357) and `atmosphere`
(#358).
`optics` loses its `MM` suffixes: `NewTelescope`, `NewEyepiece` and
`WithFieldStop` take `unit.Length`; `Telescope.ApertureMM`/`FocalLengthMM`
become `Aperture`/`FocalLength`, `Eyepiece.FocalLengthMM`/`FieldStopMM` become
`FocalLength`/`FieldStop`, `Telescope.ExitPupil` returns a `unit.Length`, and
`Sensor`'s `WidthMM`/`HeightMM`/`PixelMicrons` become `Width`/`Height`/
`PixelPitch`.
`magnitude.SatelliteApparent` takes an `observerRange unit.Length` in place of
`rangeKm`, and `ROLOIrradiance` takes typed sun and moon distances.
`ROLOStandardDistanceKM` becomes `ROLOStandardDistance`, a typed constant.
**`Telescope.PixelScale` moves by 9.4e-7 of itself.** It computed
`206265·pixelPitch(µm)/focalLength(mm)`, where the constant is a rounded
radian-to-arcsecond conversion (206264.806…) and the 1000 reconciled microns
against millimeters. Between two `unit.Length` values the small-angle relation
is the ratio itself and `angle.Angle` converts exactly, so both factors are
gone. For a 3.76 µm pixel at 1000 mm that is 0.7755557″ against the previous
0.7755564″ — below any plate solve's precision, and the only computed value in
this change that moves at all. (#361)
- **The orbital semi-major axis and three `catalog/resolve.Target` fields are now
typed**, closing the part of #130 that spans `ephemeris/kepler` and `catalog`
together.
`kepler.NewElements` takes a `unit.Length` semi-major axis and
`Elements.SemiMajorAxis` returns one; `ephemeris.NewElements` follows, since it
re-exports it. The element set was already half-typed — `Inclination`,
`AscendingNode`, `ArgPeriapsis` and `MeanAnomaly` are `angle.Angle` — and the
semi-major axis was the one member still carrying its unit in a doc comment.
`resolve.Target.SemiMajorAxis` and `.Diameter` become `unit.Length`, and
`.RadialVelocity` becomes `unit.Velocity`. The providers name the unit where
they decode it: SBDB's `phys_par` diameter in kilometers, SIMBAD's
`rvz_radvel` in km/s, MPCORB's semi-major axis in astronomical units.
`Elements.WithPeriod`/`Period` stay `float64` days: a duration, and `unit` has
no type for one. (#362)
- **`Time.Add` and `Time.Sub` take and return a `unit.Duration`**, replacing four
methods with two: `Add(time.Duration)`/`AddDays(float64)` and
`Sub() time.Duration`/`SubDays() float64` are gone.
**`Sub` no longer has a ceiling.** It returned an int64 nanosecond count, which
saturates just past ±292 years — well inside the range this library supports —
and `SubDays` existed alongside it solely because a saturated maximum is not an
answer. A float64 second count has neither problem, so there is one method.
Two consequences worth knowing:
- `Sub` no longer quantises to whole nanoseconds. A scale round trip leaves a
  few picoseconds of residue that integer rounding used to absorb; that
  rounding was an artifact of the type, not a measurement. Tests asserting
  exact equality on a round trip need a tolerance.
- Durations render as `1 s` and `60 min` rather than `1s` and `1h0m0s`, since
  `unit` puts a space before the symbol. A value a hair under a threshold shows
  in the smaller unit — an interval a few picoseconds under an hour is
  `60 min`, because `String` picks its unit from the value it holds rather than
  the one it rounds to.
Solver parameters follow the same type, because they are search intervals over
an ephemeris that meet an epoch directly: `Solver.Tolerance`,
`EventSolver.Step`, `NewEventSolver`, `ObservableWindows`' step and
`plan.WithStep`.
Scheduling and observing quantities keep `time.Duration` — `Block.Duration`,
`SetupTime`, `FilterChangePenalty`, `Cadence.MinInterval`,
`SatellitePass.Duration`, `Window.Duration` — since they are sub-day, written
as `30*time.Second`, and formatted by the standard library. (#363)
- **Five exported names lose their British spelling**, which is the whole
exported surface that had one:
| package | before | after |
| --- | --- | --- |
| `ephemeris`, `ephemeris/core` | `CenterGeocentre` | `CenterGeocenter` |
| `ephemeris`, `ephemeris/core` | `CenterBarycentre` | `CenterBarycenter` |
| `ephemeris`, `ephemeris/core` | `CenterHeliocentre` | `CenterHeliocenter` |
| `magnitude` | `JohnsonCousinsColourTerm` | `JohnsonCousinsColorTerm` |
| `skybrightness` | `ZodiacalColourCorrection` | `ZodiacalColorCorrection` |
The three `Center` constants spelled it both ways inside one identifier — an
American prefix on a British suffix. astrogo's convention is American and is
already stated in code: `unit/units.go` declares `Name: "meter"`.
`Center.String()` follows, so it now returns `"geocenter"`, `"barycenter"` and
`"heliocenter"`. A caller matching on those strings needs updating; nothing in
this repository parses them. (#364)
- **Five more exported names lose their British spelling** — the ones #364 missed,
because the scan it relied on looked for `Metre` only at the start of a word:
| package | before | after |
| --- | --- | --- |
| `unit` | `Nanometre` | `Nanometer` |
| `skybrightness/dataset/crosssection` | `Nanometre` | `Nanometer` |
| `skybrightness/dataset/starlight` | `ColourTerm` | `ColorTerm` |
| `skybrightness/dataset/starlight` | `BrightStarCatalogueRadius` | `BrightStarCatalogRadius` |
| `atmosphere` | `SourceRef.Licence` | `SourceRef.License` |
The nanometer unit's printed name follows, from "nanometre" to "nanometer". (#383)
- **`remote/file.AcquireLock` takes a `staleAfter` duration**, the age past which
a lock is taken for a crashed holder's; zero keeps the 30-minute default.
`remote/file` is internal to `remote` — docsguard rejects importing it from
anywhere else — so only `remote` itself calls it (#445).
- **`remote/file.ResumePoint` returns an error** beside the offset, for a partial
whose sidecar or size cannot be read. `remote/file` is internal to `remote` —
docsguard rejects importing it from anywhere else — so only `remote` itself
calls it (#452).
- **`plan.CrescentResult` carries every criterion's answer with the quantities it
read** (`CrescentVerdict`, `CrescentZone.Params`). `EvaluateAll`, the shared
`Params`, `Fatoohi` and `Gautschy` are gone, the last two having no traceable
source; `Schaefer` is `Fatoohi1998` and `Ilyas1984` is `Ilyas1983` (#503).
- **`time.FileEOPLoader`, which took an OS path, no longer exists; it is replaced by
`time.FSEOPLoader{FS, Name}`.** Write `time.FSEOPLoader{FS: os.DirFS(dir),
Name: "finals2000A.data"}` where you wrote the old loader with a path. A
bulletin that is there but unreadable is now reported as itself rather than as
`time.ErrNoEOPData`. (#519)
- **`magnitude.GaiaGToJohnsonV`, `GaiaGToJohnsonB`, `GaiaGToJohnsonR` and
`GaiaGToCousinsI` now return `(mag, ok)`**, with `ok` false outside the BP−RP
interval each relation was fitted over. `catalog/gaia` reports no V outside it
instead of an extrapolation — an L dwarf at BP−RP = 6 came back at V = 16.6 for
G = 12. Callers that used the single value take the first result and check `ok`. (#531)
- **`magnitude.CometNuclearApparent` now takes the phase coefficient and phase angle**, `(M2, k2, pc, r, delta, phase)`, and `plan.WithNuclearMagnitude` takes `pc`.
JPL's nuclear magnitude includes PC·β, which was missing and made 13P/Olbers's nucleus 0.21 mag bright against Horizons; SBDB's `PC` is now parsed into `resolve.Target.PC`.
Callers pass SBDB's PC, or zero where it publishes none. (#552)
- **A star rises and sets on the almanac horizon.** `Site.RiseSetThreshold` is −(34′ + dip), not the dip alone, so `VisibilityEvents`, `DayEvents`, `Episode`, `IsCircumpolar` and `IsNeverUp` agree with USNO and Skyfield, which put a star's horizon 34′ down; rises were 2.4 to 7 minutes late. `plan.WithRefraction` no longer exists, since `IsCircumpolar`'s default now includes the refraction it added; `WithHorizonAltitude(0)` gives the geometric horizon. (#576)
- **`atmosphere.StandardRefraction` and `kepler.PlutoElements` are functions.** As exported vars, one assignment anywhere changed them for every caller in the process; call them as `atmosphere.StandardRefraction()` and `kepler.PlutoElements()`. The `unit` and `dim` vars stay, documented read-only rather than immutable, and a module-wide guard requires a stated reason for any exported var that is not a sentinel error. (#577)
- **`atmosphere.CleanMountainAOD550` is 0.027, Paranal's measured median aerosol (Patat et al. 2011)**, in place of an unsourced 0.03 whose comment placed Mauna Kea inside a range its own median (0.016) falls below. Anything built on it carries 10% less aerosol; pass 0.03 explicitly to keep the old value. (#593)
- **`magnitude.StarApparent` takes the extinction coefficient as a required argument**: `StarApparent(catMag, airmass, k)`. Its hidden default of 0.20 mag/airmass is gone; pass `air.Extinction(λ)` for the band the magnitude is in. (#594)
- **`coord.Context.SetTime` replaces `AtTime` and `Clone`**: it moves a Context in place without allocating, and rebuilds it an hour from its epoch, so its error stays ≲0.1″ for every caller. `AtTime` copied 704 bytes per instant, 72% of what plan's event solver allocated. A copy is `c := *ctx`. `plan.TransitionContext`, since renamed `plan.Transition`, carries both targets' observed alt/az instead of `ContextAt`; `plan.NewTransition` builds one (#675).
- **`time.DeltaT` and `time.DeltaTUncertainty` take a `time.Time`, and ΔT is one value everywhere**: the Espenak & Meeus model before 1960, the IERS bulletin's measured value where it covers, and that value held past its end, where the conversions used to pin UT1 to UTC. `DeltaT` followed the model at every epoch, 6.2 s from the conversions in 2026. Past the bulletin, `Time.UT1` answers instead of failing, and the uncertainty grows from the bulletin's end (#696).

### Changed
- **CI selects Go by minor version instead of reading go.mod.** The `go 1.25.8`
directive — inherited from gocloud-ext, not needed by astrogo (#109) — made
`setup-go` demand that exact patch and download a toolchain before every job,
which hung for 25 minutes on one run. `go-version: '1.25'` resolves to whatever
1.25.x the runner already has. (#152)
- **The SOFA/DE440 planet comparison samples 1800–2100 again**, the interval
SOFA's own table is quoted over. It was restricted to 1972+ to dodge a
pre-1972 timekeeping term; that term is gone, and the guard at the boundary
now asserts its absence rather than its presence. (#158)
- **The astronomical unit was written out in four production files and the
light-time constant in two**, all agreeing with `constants` and with each other
— which is the problem, since a future revision in `constants` would leave five
copies silently behind. All now derive from `constants`.
`jpl.KMPerAU` no longer exists: it was exported, letting a downstream caller
pin the stale value, and nothing outside its own file referenced it.
`TestCanonicalConstantsAreNotWrittenOut` scans the module so the literals
cannot come back (#137).
- **`docs/VALIDATION.md` and the `satellite` package doc now state that SGP4 is wrong
for low-perigee, deep-space and decaying orbits**, measured for the first time by the
suite above. Ordinary orbits are unaffected; `Satellite.Verified` says which side a
given element set falls on. (#181)
- **The `time` package doc and `docs/VALIDATION.md` now record that an epoch read from a
smeared host clock is not UTC.** Around a leap second, NTP providers spread the step over
as much as 24 hours — each differently, none announcing which — so `NowUTC` can be off by
0.5 s with nothing to detect it: 0.3″ of lunar motion, 3.8 km of ISS track (#146).
- **The `±0.9 s` bound on UT1−UTC is now stated as borrowed rather than intrinsic.** It
holds only because leap seconds keep it there, and CGPM Resolution 4 (2022) ends them by
2035 — after which the zero-EOP degradation grows without bound. The `time` package doc
and the EOP warning both say so; the data path is unaffected (#147).
- **`openngc.New` no longer fetches the catalog.** It did about 7 MB of I/O under
`context.Background()`, so `catalog.NewResolver(SIMBAD, OpenNGC)` blocked for two
seconds against a warm cache with nothing above it able to cancel. The load now
happens on the first query, under that caller's context, and a failed load is
retried rather than replayed for the life of the provider. (#197)
- **`plan.SatellitePasses` is about twice as fast** — 201 ms to 96 ms over a
six-hour window at 30 s sampling. It rebuilt a full `coord.Context` per sample,
which was 61% of each one; it now derives them with `Context.AtTime` from a base
rebuilt hourly, costing ≲0.1″ against SGP4's own kilometer-scale error. (#201)
- **`catalog.Resolver` queries its providers concurrently.** `Resolve` and `Search`
looped over them one at a time, so a resolver over SIMBAD and OpenNGC cost a CDS
round-trip *plus* a local lookup per query instead of the slower of the two.
Provider-registration order, merging and error reporting are unchanged. (#202)
- The README's "Implementation Status" table is now a "Package Map" without the
Status column. It marked all twenty-two packages "Stable" while the CHANGELOG
carried twelve `Changed — BREAKING` sections across twenty-six pre-1.0 releases,
so the one column that claimed to say something said the same false thing on
every row (#119).
- The `network` tier now runs nightly rather than weekly, in its own job; the
`validation` and `integration` tiers stay weekly. Only the network tier notices
a SIMBAD schema change or a Horizons format change — `go vet -tags` cannot, the
code still compiles — and a week is a long time to be wrong about that. Still
off the PR path (#123).
- The README is 749 lines, down from 1,068. The 361-line dump of Quick Start and
five inline demos is a table linking to the runnable `examples/` they were
copied from, "Astropy-level capabilities" is replaced by what astrogo actually
is and an honest list of what it is not, and the `atime` import alias is stated
before the first code block rather than discovered at a compile error (#124).
- `examples/` is now its own Go module, so its 32 demo programs no longer appear
in astrogo's package listing — they were 32 of 84 rows. Run one with
`go -C examples run ./<name>`; a `replace ../` keeps them building against the
working tree, and CI compiles and lints them separately (#124).
- `ephemeris.ApparentState` iterates light time to convergence instead of a flat
five passes — measured, nothing in the solar system needs more than three, and
the Moon settles in one. That halves the cost of the apparent-place path this
release puts on the scheduler's hot path (#117).
- astrogo now requires Go 1.27. The module directive is `go 1.27` — a minor
version, not the patch-level `1.25.8` it inherited from a dependency — so the
forced per-job toolchain download that cost PR #151 twenty-five minutes cannot
recur from our own go.mod (#109).
- Five `sync.WaitGroup.Add`/`Done` pairs become `wg.Go`, removing the class of
bug where the two get out of step, and eleven `sort.Strings`/`Float64s` calls
plus the data-driven `sort.Slice` sites become `slices.Sort`/`SortFunc` —
measured 1.7× faster with no allocation. `Planner.RankObservable` also stops
mis-ordering a ranking that contains a non-finite score. (#232)
- Every JSON API response now decodes through `encoding/json/v2` — measured on an
SBDB-shaped body at 24.5 µs against 37.4 µs, 12.1 KB against 28.8 KB, and 9
allocations against 20. A response repeating an object name is now an error
rather than silently taking the last occurrence; case-insensitive field
matching and tolerance of invalid UTF-8 are deliberately kept, because exact
matching would leave a renamed coordinate at RA 0, Dec 0 (#126-adjacent, see
`plan.ErrNoCoordinates`). (#233)
- `resty.dev/v3` moves to `rc.4`, and `go.mod` now records why `gocloud.dev`
cannot leave its pseudo-version: `gocloud-ext`'s httpblob driver implements
`blob/driver.DeleteOptions`, which was added after v0.46.0 (#259).
- **An API client no longer resolves endpoints; `remote` does, per request.** The client
underneath takes the base URL its request goes to, and `remote.Client` resolves `id`
through its own `URL(id)` immediately before each call. That removed the import cycle that
kept `remote` from fronting its own API client, and makes `SetOffline`, `Disable` and
`SetURL` apply to a client that already exists rather than to the next one built. (#274)
- **Only `remote/file` names the storage library now.** `remote.IsNotFound` no longer exists.
It replaced `gcerrors.Code(err) == gcerrors.NotFound` at its three call sites and
`internal/testutil.BucketKeys` takes an `fs.FS`, so `gocloud.dev` appears in no import
outside `remote/file` — enforced by `TestGocloudStaysInsideRemoteFile`, since deleted
along with the library it guarded. The driver has
been swapped once already; what made that expensive was every package that had an opinion
about it. (#274)
- **Dependencies.** Update Apache Arrow Go to v18.8.0, HDF5 to v0.14.1, and `golang.org/x/sync` to v0.23.0 in the library and examples modules, retaining the previously merged security updates. (#292)
- `ephemeris/satellite` reads its gravity constants from `constants.WGS72` instead
of holding a private copy — the copy is how the package came to use WGS-84's
values while the propagator was handed WGS-72's. `constants.WGS72` gains `J2`,
one of that standard's four defining parameters, which its own doc comment
already named. (#314)
- `ephemeris/satellite` propagates through astrogo's own
`ephemeris/satellite/sgp4` instead of `github.com/joshuaferrara/go-satellite`,
which is **removed from `go.mod`**. Measured end to end through the public
wrapper against Vallado's reference states: 588 states, worst **4.1e-06 km**,
against 0.0031 km before — and the seven cases astrogo could not reproduce at
all, by up to 3438 km, now agree to nanometers. [#310] (#320)
- `satellite.ValidateTLE` accepts trailing whitespace and anything appended past
column 69, where it used to require exactly 69 characters. Feeds emit CRLF and
padding constantly and neither loses data; a line *shorter* than 69 does, and is
still refused. The old behavior was a side effect of a length equality check
rather than a decision. [#310] (#320)
- `docs/VALIDATION.md`'s generated accuracy table is regenerated from a full
`validation` + `network` collection. It was dated 2026-08-30 and had drifted in
three ways: ten suites had grown their corpora (the seven SOFA planets from
N=516 to 1204, the two time-scale round trips from 120 to 432 and 180 to 540),
four suites were being measured and never published, and the Horizons-referenced
rows had moved. `coord.topocentric.vs_sofa.stepwise` and `.collapsed` now appear
at 0.000″ across all four statistics over 210 combinations, and
`ephemeris.astrometric.geocentric` and `.apparent.geocentric` are cited by suite
name rather than by test file. Every row is still ✅ verified and inside its
contract. The status table's claim that the two SOFA-comparison suites were "not
yet in the generated table" is no longer true and is corrected. (#351)
- **`testutil.SkipOnUpstreamFailure` now answers the whole question**, consulting
`Unreachable` for the cases it did not cover — a DNS failure, a refused or
unroutable dial. A test no longer has to call both to find out whether a failure
was somebody else's, which is friction that helped make a skip-on-any-error the
convenient thing to write. A caller's own canceled context still does not skip. (#372)
- **The Horizons reference corpus is refreshed**, after establishing what moved and
why: Horizons changed something below its own emission resolution, and 94 of 300
values sat near enough to a rounding boundary to tip. Every change is one unit in
the last digit Horizons prints — 1e-06 deg for azimuth and elevation, 1e-14 AU at
Jupiter and 1e-13 at Saturn for range — while RA/Dec and the geocentric vectors
did not move at all. Two new tests keep the question answerable: one asks
Horizons the same query twice to tell a recomputation from a re-rounding, and one
checks the manifest's record of the query against what the code would ask today,
since a moved observatory reports as changed values rather than a changed query.
The generator's diff summary now groups by field instead of reporting a single
maximum. (#373)
- **American English throughout.** Doc comments, test names, and the text of a
few error and log messages now spell "meter", "center", "color", "behavior",
"labeled" and the -ize forms, matching the exported names #364 and #383
already renamed. Sentinel errors are unchanged, so `errors.Is` still matches;
only the message text differs (#382, #384, #385, #386, #387, #356).
- **`plan.Oppositions` samples once a day rather than every 6 hours**: it finds
the same instants, to a second, at a quarter of the cost (#432).
- **`coord.NewContext` is 34% faster** (138 → 91 µs), with bit-identical
results: it reuses the precession-nutation matrix `Apco13` already built
instead of evaluating the series again (#473).
- **`plan.SatellitePasses` does half the precession-nutation work**, with
identical results: each satellite state evaluates the series once instead of
twice, and culminations are sampled through the pass search's Context cache
rather than a full Context per sample (#476).
- **`VisibleIntervals`, `TransitEstimate`, `ObservableWindows` and `Find` are
6–40× faster**: they built a full `coord.Context` per sample, and now derive
each from an hourly one as the event solver does (≲0.1″; #480). (#482)
- **`plan`'s test suite runs its 26 slowest serial tests in parallel**, cutting
the package's race-detector run from 261 s to 174 s locally, against CI's
600 s timeout (#490).
- **Network tests no longer fail on a degraded VizieR**, which answers every
query with a 400: a 4xx now skips only when the service also rejects a
control query that cannot be wrong, so a genuinely bad query still fails
(#492).
- **A slewing schedule runs 6.6× faster**: `BasicTransitionModel` built a full
SOFA Context per instant of every slew, and the built-in strategies now observe
both ends through the Context they evaluate constraints with (#485).
- **UTC from 1960 to 1971 is read as SOFA reads it**: TT through SOFA's TAI−UTC
rather than ΔT, and the days UTC jumped by a fraction of a second stretched as
iauDtf2d stretches them. `TT()` and `TAI().TT()` now agree there (#479).
Also fixes a UTC label an ulp below a leap-second midnight converting to TAI a
second early (#499).
- **`satellite.Satellite.Altitude` is about 85 times faster** (50 µs to 0.6 µs)
and no longer looks up UT1 or loads EOP. It turned the position Earth-fixed by
GAST, the wrong sidereal time for TEME, to take a height no rotation about the
axis changes. Heights are unchanged. (#514)

### Removed
- **`fits.Write` no longer exists — it always returned `ErrUnimplemented`.** An
exported function that only ever fails, in a package the README marked Stable,
is a runtime surprise for anyone who type-checks against it; an absent one is a
compile error at the call site, which is the honest signal. Its signature could
not have been implemented as written either — a filename and a flat
`[]float64`, with no dimensions or header. `ErrUnimplemented` goes with it, and
the README now labels `fits` read-only, pointing at #127 for the real writer.
Nothing in the module called it (#135).
- **42 `//nolint:gochecknoglobals` directives suppressed a linter `.golangci.yml`
disables** — 29% of every suppression in the tree, each carrying a documented
reason for silencing nothing, which is what made the live suppressions hard to
pick out. All removed. `TestNoNolintForADisabledLinter` now cross-checks every
directive against the config, since a dead one is invisible to golangci-lint
itself: `nolintlint` reports an *unused* directive, but one naming a disabled
linter is simply skipped (#136).
- `remote.WorldAtlas` no longer exists, and neither does `remote.LightPollution`;
both were deprecated in 0.15.0 and are past the two minor releases the policy
requires. Nothing read either: one was a non-commercially-licensed model output
that cannot validate `skybrightness` and must not be served as its answer, the
other read satellite radiance as sky brightness, which is on that module's
prohibited list (#119).
- **The `s3://`, `gs://`, `azblob://` and `sftp://` schemes no longer resolve.**
They were gocloud.dev drivers and went with it; replacements were built,
measured and dropped — `docs/storage.md` §10 records the two silent-wrong-answer
defects found in the obvious library and why writing one directly is not worth
it for a single optional endpoint. `remote.CopernicusEODATA` is that endpoint:
it stays registered and now fails early, naming the schemes that are registered,
and `cams.RegistrationAdvice` says so rather than naming a package that does not
exist. (#326)
- **`unit.AltitudeM` no longer exists.** Use `unit.Length`, which stores meters,
so every value that type held is already correct: a conversion becomes
`unit.Meters(2635)` and a declaration changes type name only.
It named one quantity in one unit, which is the pattern `unit.Length` exists
to replace — a caller holding one could not ask for it in kilometers, and a
length crossing into `coord` or `plan` needed a cast at every boundary.
`atmosphere` was the only package using it. (#358)
- **`NewCrescentParams` is gone from `plan`**: use `CrescentVisibility`, which
finds the evening's sunset and moonset itself. `NewCrescentParams` gave every
criterion one set of parameters, with a constant lunar semi-diameter and a lag
estimated from the Moon's altitude (#496).
- **`ephemeris.Altitude` no longer exists.** It returned geocentric distance less
a 6371 km mean radius, as a `float64` in km: 6 to 7 km off the WGS84 height
for the ISS, and for a planet not an altitude at all. A satellite provider is a
`*satellite.Satellite`, whose `Altitude` returns the WGS84 height as a
`unit.Length`; for any other geocentric state,
`coord.FromECEF(ctx.ICRSToITRS(pos), coord.WGS84())` with a `coord.Context` at
the epoch. (#517)
- **The last `Deprecated` symbols are gone.** The mutable maps
`plan.KnownSites` and `plan.MeteorShowers` (since removed; use `KnownSiteNames`/`NewKnownSite`, `MeteorShowerNames`/`NewMeteorShower`),
`plan.TwilightThresholds` and `jpl.BodyIDToNAIF` (since removed; use `TwilightThreshold`, `NAIFFor`/`NAIFBodies`),
and the sentinel `plan.ErrNotCoordObject`, which nothing returned (since removed). (#536)
- **`ephemeris`'s `Body`, `Kind` and body table are gone** (since removed: `core.Body`, `core.Kind` and its constants, `core.SunBody` … `core.NeptuneBody`, `core.Bodies`, and their `ephemeris` re-exports).
Nothing took or returned a `Body`; use `core.ID`, whose `String` gives the name.
Eleven of them were reassignable globals, so one importer could redefine the Sun for the whole process. (#539)
- **`remote/file.StagingSuffixes` (since removed) is unexported**; `IsStagingName` is how a caller recognizes a staging object.
The guard against exported package-level maps now covers slices and arrays too, which any importer could also edit in place, and this was one it found. (#544)
- **`atmosphere.RefractionApproximate` no longer exists, and neither does `atmosphere.RefractionRigorous`.** They were the same two formulas, Saemundsson's and Bennett's, and "Rigorous" integrated nothing. `atmosphere.RefractionBennett` replaces both: Bennett's formula as refitted to the Nautical Almanac's tables, which it reproduces within 0.12′, inverted for the forward direction so the round trip is exact. (#589)
- **`magnitude.ExtinctionV`, `ExtinctionB`, `ExtinctionU`, `ExtinctionR` and `ExtinctionI` were since removed, and `magnitude.ExtinctionAtAltitude` no longer exists.** The coefficients had no source, and the altitude scaling thinned ozone and aerosol as if they were air molecules. Use `atmosphere.Atmosphere.Extinction` for a site's own air at the band's wavelength. (#594)

### Fixed
- **The JPL tests still hung when NAIF stalled.** #95's skip could never fire,
because `remote.NAIFSPK` allows a 30-minute download and the test binary dies
at ten. The fetch is now bounded and attempted once per package, so a stalled
NAIF skips in seconds instead of failing the build. (#99)
- **The JPL provider computes ET from the kernel again, and now completely.**
`lsk.Reader` parses the Moyer (1981) constants it previously ignored
(`DELTA_T_A`, `K`, `EB`, `M`), so the conversion applies the full relativistic
model the LSK defines rather than a leap-second offset alone — matching the
convention Horizons uses, while keeping the scale normalization that fixed the
69.184 s reinterpretation. (#148)
- **A truncated leap-second kernel parsed into a short table.** `lsk.NewReader`
never checked `scanner.Err()`, so a read that failed part-way kept the entries
seen so far and returned successfully — the same shape as the dropped-2017-entry
bug, reached by a short download instead of a parsing slip. (#148)
- **The kernel-driven ET no longer swallows historical ΔT.** The kernel-driven conversion now
delegates to `time` for epochs before the `DELTA_AT` table's first entry
(1972-01-01), where leap seconds do not apply and the offset is the Espenak &
Meeus (2006) ΔT instead — worth 175 minutes at year 1, which surfaced as ~180
minute errors across the AstroPixels year-0001 lunar phases. (#148)
- **Two ephemeris providers silently reinterpreted the caller's time scale.**
SGP4 read the calendar fields raw, putting the ISS 530 km out for a TT input;
the JPL provider treated anything but TDB as UTC, worth 40 arcsec of lunar
motion. Both now normalize at the entry point, and a new contract test asserts
every provider returns the same state however the instant is labeled. (#148)
- **The kernel-driven conversion returned TT, not TDB.** Its formula omitted the TDB−TT
periodic term (~1.7 ms amplitude — 1.7 m of lunar motion, 85 m for Mars). The
conversion is now delegated to `time.Time.TDB`, which owns leap seconds for the
library; the function's `*lsk.Reader` parameter is retained for compatibility
and is no longer read. (#148)
- **Corrected the stated cost of the UTC-for-UT1 fallback.** `Time.GAST()` and
`satellite.subSatellitePoint` both described it as "a few hundred ms of error
at worst"; it is bounded by the leap-second system at 0.9 s — about 13.5
arcsec, or 420 m of sub-satellite ground position — and that bound disappears
when leap seconds end in 2035. (#148)
- **`coord.Context.GeocentricToObserved` returned altitudes of thousands of
degrees near the horizon.** Its refraction branch wrote out
`Refa·tan(z) + Refb·tan³(z)` with no clamp, so the series diverged just below
the horizon (+7028° at −0.076°) and canceled to zero just above it (0.000°
where the stellar path applied 0.16°); it also omitted the Newton-Raphson
correction. It now reproduces SOFA's `Atioq` exactly, so the two pipelines
agree to milliarcseconds. (#153)
- **The scheduler treated the Moon as a star at infinity.** Every constraint,
score and visibility check called `ICRSToAltAz` on a geocentric position,
discarding the observer's offset from the geocenter — up to **0.95°** for the
Moon, so an `Altitude{Threshold: 0}` constraint reported it up about four
minutes before `MoonEvents` said it rose. Crescent visibility was affected
worst, its own comment claiming "topocentric" while the code was not. (#154)
- **ET was quantised to ~40 microseconds.** The Julian-date conversion pair
summed the two-part Julian Date before subtracting J2000, and one ULP at a
modern Julian Date is 40 µs — 4 cm of lunar motion, at the level of the 33 mm
claimed against Horizons. The new `lsk.UTCToET` removes the epoch first and
resolves **0.128 µs**, a 256-fold improvement. `UTCToTDB` and `TDBToET` are
removed: they held the same value in a container that could not represent it,
and nothing in production called them. (#158)
- **`Time.Sub` between two UTC epochs was short by every leap second between
them** — 27 s across 1972-2026, or 207 km of ISS track. It unified scales only
when they *differed*, so mixing scales gave the right answer and being
consistent gave the wrong one. Both operands now go through TT unless they
share a uniform scale (TAI/TT/TDB), where label arithmetic already is elapsed
time. `Sub` also saturates instead of wrapping past ±292 years — year 1 to 2026
used to return a negative duration — and rounds to the nearest nanosecond
rather than truncating (#149).
- **Following the documented `RefractionModel` API panicked.** Every constructor
leaves `Refraction.Model` nil, so `env.Model.RefractFromTrue(...)` was a nil
dereference. `Refraction` now answers for itself via `RefractFromTrue`,
`RefractFromApparent` and `EffectiveModel`, which resolve nil to the new
`RefractionSOFA` when a pressure is set and to `RefractionNone` otherwise —
moving the "nil means SOFA" convention out of `coord` and into the package that
owns the type. `coord.Reducer.Disperse` consequently stops reporting zero
dispersion for an environment `Reduce` had just refracted through (#118).
- **`RefractionRigorous` and `RefractionApproximate` returned negative refraction
near the zenith** — −0.114″ and −0.080″, crossing zero at 89.89° and 89.92°,
because the term that stabilises each fit near the horizon carries its tangent
argument past 90° at the top. Both now return zero above the crossing, which is
the physical limit and costs at most 0.001″ against a fit quoting 6″. The
known-values test also stops taking `math.Abs` before comparing, which had made
its bracket blind to the sign of every row, not just the zenith one (#162).
- **`plan.LookAngle` discarded the `coord.Context` it was handed** and had
`coord.Reducer` build a second one, repeating the Apco13 solve the first
already held. Measured: 242 → 95 µs per call, and `SatellitePasses` 318 → 200
ms over a six-hour window, since it paid the solve twice per 30-second sample.
Everything the Reducer computed is already on `Context`, so nothing is rebuilt.
Using the given Context is also what the signature promised — one derived by
`Context.AtTime` used to be silently replaced with a full rebuild (#111).
- **`Event.Altitude` was geometric at rise/set and refracted at transit**, with
nothing in the type saying so, and `Event.GeometricAltitude` was assigned the
identical value at both construction sites — so one field told a caller nothing
the other did, and comparing a rise altitude against a transit altitude
compared two different quantities. `Altitude` is now the refracted altitude at
every event kind, matching `IsObservable` and `GetDetails`; `GeometricAltitude`
is the unrefracted one. `Value` stays geometric at rise/set, so event times are
unchanged (#156).
- **`plan.Episode` searched up to 366 days to discover a target never rises.**
For a fixed target that answer is two arcsines — declination does not change,
so upper and lower culmination bound the whole window — and `IsNeverUp` /
`IsCircumpolar` already computed it while having no caller in the library.
Measured: 2.84 s → 67 ns for the never-rises case, and the `plan` package
18.6 s → 12.7 s. A one-degree margin keeps anything refraction or parallax
could decide on the search path, and moving bodies always search (#110).
- **Six test files asserted astronomical results at whatever instant the suite
happened to run.** A test at `time.NowUTC()` drifts across the IERS
measured/predicted EOP boundary — which moves every week — and eventually off
the end of the file, failing on a future date with no code change. Those now
use a fixed past epoch, which is final and cannot drift.
`TestNoUndeclaredWallClockTests` scans the module and requires any remaining
wall-clock test to declare why the present is its subject; twelve do, and a
stale declaration fails too (#142).
- **Three places reported a real failure as a legitimate absence.** The CAMS
HDF5 reader treated *any* `ReadAttribute` error as "attribute missing", so a
corrupt file reported every attribute as absent and the reader carried on with
defaults; it now asks which attributes exist first, which separates absence
from an unreadable header. `IntegratedStarlight` read `err != nil || value <= 0`
and reported a map that could not answer — a band it does not carry — as an
uncovered direction; the two are now distinguished by the new
`skybrightness.ErrNoCoverage` (#172).
- **Seven predicates could not report a failure, so an error read as "the answer
is no".** The scheduler's and visibility solver's bisection predicates returned
a bare `false` for a constraint that *could not be evaluated*, so a target
silently vanished from a schedule; `ObservableWindows` swallowed a failure
during refinement, moving a rise/set boundary rather than dropping it; and
`DiffuseGalacticLight.capFactor` treated a star map that could not answer as a
sightline with no starlight, quietly dropping the Toller cap. All now
propagate (#177).
**`plan.VisibleTonight` reports an incomplete result instead of only logging
it.** It still skips a candidate it cannot evaluate rather than failing the
night, but now returns its results alongside an error wrapping the new
`plan.ErrIncomplete`, naming every catalogue source, target, small body, moon
kernel and candidate that was dropped and why. A caller who ignores that error
gets the previous behavior; one who checks it can finally tell a quiet sky
from an unreachable JPL (#177).
**`satellite.ValidateTLE` now checks that every numeric field is numeric, which
prevents the SGP4 backend from calling `os.Exit` on the caller's process.**
`joshuaferrara/go-satellite` parses the twelve numeric TLE fields through
helpers that `log.Fatal` on a parse error — no error, no panic, nothing to
recover. A TLE's modulo-10 checksum cannot catch this, since letters and spaces
contribute nothing to the sum, so a field replaced by text can still check out.
`NewFromTLE` now refuses such a set with `ErrMalformedTLE` naming the field, and
`Satellite.MeanMotion` comes from that same parse rather than a separate one
that returned a silent `0`.
**`cams.isDimensionScale` reported an unreadable object header as "not a
dimension scale".** A corrupt file then filed its axes as data variables, so
the reader indexed a shape the file does not have and the failure surfaced much
later as a missing dimension on a variable whose dimensions are all present. It
was the one attribute reader #172 left behind. (#180)
- **Satellite positions carried up to a second of orbital motion of error — 5.94 km
for Vallado's reference case, worst at the element epoch where it should be exact.**
The SGP4 backend truncates the element epoch to a whole second as well as the query,
so the sub-second correction must be `frac(t) - frac(epoch)`, not `frac(t)`. (#181)
- **`time` re-exported fifteen standard-library functions as reassignable package-level
`var`s, and its six layout strings as a `var` block.** Any package in the import graph
could reassign `time.Parse`, `time.Now` or `time.RFC3339` process-wide, in the package
every epoch calculation goes through. They are functions and constants now — identical
at every call site, so nothing outside had to change (#113).
- **A TLE with a day-of-year past the end of the year panicked inside the SGP4 backend**,
reachable from `satellite.NewFromTLE` with an element set that is 69 columns,
checksum-valid and numeric in every field. `days2mdhms` guards a twelve-element month
array with `i < 22`. `ValidateTLE` now range-checks the epoch day (#139).
- **`votable.Read` returned rows for a document declaring no columns**, so a response that
was not a VOTable at all became an empty result set with a nil error — indistinguishable
from a query that matched nothing. It now returns the new `ErrNoFields` (#139).
- **Four places where the documentation contradicted the code** (#119): the ROADMAP said
`LimitingMagnitudeConstraint` was removed while a differently-shaped one exists;
`plan/events.go` credited SOFA for a rise/set threshold that hardcodes the conventional
34′; `.golangci.yml` still described a go-cloud fork `replace` that `go.mod` has not
carried since the remote rebuild; and the `cams`, `kepler` and `xmatch` package synopses
were paragraphs, so pkg.go.dev rendered their directory listings as walls. The ROADMAP also still listed limiting magnitude as unbuilt while `skybrightness/plan.Imaging` implements it. (#188)
- **`catalog.ErrNotFound` was a different error value from `resolve.ErrNotFound`, with
identical text.** `catalog.Provider` is an alias for `resolve.Provider`, whose contract is
written in terms of the latter — so a caller following it and testing
`errors.Is(err, resolve.ErrNotFound)` got false for an object that simply does not exist,
and fell into their "the service is down" branch. Both are now the same value (#141).
- **`remote.ErrRetriable` was produced by nothing.** A 503 that survived every retry and a
404 were the same error, so a caller could not tell "the service was busy and we gave up"
from "you asked for something that is not there". `APIClient.Get` now wraps the final
`*remote.HTTPError` with it whenever the retry policy would have retried that status. (#195)
- **`openngc.Provider.SearchBright` reported an unreachable catalog as an empty
sky.** It was the one query that never consulted the recorded load failure, so a
denied download or a dead endpoint came back as "no objects brighter than that"
rather than as an error. It now yields the failure through the iterator. (#197)
- **A leap second passed to `time.Date` was aliased silently.** `23:59:60` has no
representation in a two-part JD whose day is 86400 seconds long, so it collapsed
onto the following midnight — one second away, and converted with the wrong ΔAT
(37 rather than 36). It still does; it now says so through `logging` at WARN,
distinguishing a real leap second from a second that never existed. See #144 for
the representation question, which stays open. (#198)
- **`fits.ReadHeader` allocated about five times the file it was reading.** Its
failsafe bounded blocks read, not cards retained, so 10,000 blocks of distinct
keywords was 360,000 retained cards — measured at 144.1 MB from a 28.8 MB input.
A second bound on retained cards holds it at 8.5 MB and, more to the point, flat
as the input grows. (#199)
- **Five network tests failed the build on somebody else's outage and discarded
the error while doing it** — `Failed to resolve ISS` was the whole diagnostic
when CI throttled. They now route through `testutil.SkipOnUpstreamFailure` and
carry the error, and an `internal/docsguard` guard keeps the next one from
reintroducing it. (#204)
- **A 403 failed the build instead of skipping the test.** CelesTrak answers a
burst of requests with "Forbidden: Access is denied" and serves the same query
normally a minute later, which `testutil.SkipOnUpstreamFailure` classified as
astrogo sending a bad request. To a caller that sent no credential there is
nothing to correct, so it now skips; 401 still fails, and a new registry test
keeps the invariant that argument rests on (#206).
- **`catalog/fink` reported an asteroid it had never heard of as an outage.** FINK
answers an unknown identifier with a `RemoteException`, which read as a failure
and was joined into the returned error, so `Resolve` never produced
`ErrNotFound` — #102's inversion. The bulk SSOFT table loading and not holding
the object now settles it, and the exception's own message is carried instead of
discarded (#122).
- CLAUDE.md's fuzz throughput figures were unsupported and one was wrong by two
orders of magnitude: `fits.Read`'s "~4 exec/s" was the default 60 s minimize
time, not the parser, which measures ~800. Replaced with a corpus-controlled
table, the run-to-run spread that makes a single run not a measurement, and the
160× that `-fuzzminimizetime` alone decides (#200).
- **`fits.WCS` could be rewritten without going through a setter, in both
directions.** The getters returned the internal slice and the setters kept the
caller's, so reading `CRVAL` to inspect it, or holding the slice you passed in,
let you move an image on the sky at a distance. Both now copy, including the SIP
and TPV coefficient maps (#178).
- **Planets, asteroids, comets and generic bodies were placed geometrically, not
apparently.** Neither light time nor annual aberration reached the scheduler's
alt/az: measured at Paranal across 2026, Mars was out by up to 38.7″, Venus
44.8″ and the Sun 20.5″. All four target types now return the apparent place
(#117).
- **A kernel that could not be read was deleted as corrupt.** "Access is denied"
and a checksum mismatch shared one branch, so a process losing a race to a file
lock deleted the shared 32 MB kernel out from under every other process — the
observed cause of intermittent Windows CI failures. Only a proven content
failure is destructive now; an I/O failure leaves the file for the next open to
retry (#227).
- **The USNO comparison suite had been silently skipping.** A single-shot TCP
probe cached its failure in a `sync.Once`, so one dropped packet retired all
fourteen `TestUSNO_*` functions for the rest of the binary — as SKIP, which
reads as a pass. The probe now goes through `testutil.Reachable` (which retries
over IPv4) and remembers only success; a new docsguard check stops the next
hand-rolled probe (#225).
- `ephemeris/jpl` no longer rejects every comet fetched by its Horizons SPK-ID as
a substituted body. Those IDs sit outside NAIF's numbered-asteroid block, where
`core.SmallBodyID` reports 0, and the substitution guard was comparing the
loaded bodies against that zero rather than against the comet (#237).
- `plan`'s tests no longer revoke the download consent their own `TestMain`
grants: the four blanket `remote.Reset` cleanups are now scoped
`remote.Capture(...).Restore`, and a new guard in `internal/docsguard` fails
any consent-granting package that reintroduces one (#240).
- `plan`'s `network` and `validation` test tiers run on their own again: the
`TestMain` registering the kernel backend and granting download consent was
gated to `integration`, so `go test -tags=network ./plan/` reported "this build
has no kernel backend" for 24 checks. A guard now requires a `TestMain` to
cover every tier its package has tests in (#243).
- An EOP lookup outside the IERS bulletin's coverage no longer re-reads and
re-parses the whole file on every call — 71 ns for a covered epoch against
11 ms and 15.6 MB for one just outside. Scheduling more than a year ahead and
historical work before 1973 were both on that path (#247).
- `remote`'s download lock is now genuinely exclusive within a process. It relied
on `fileblob`'s `IfNotExist` being mutex-guarded, which the pinned driver does
not do — measured, 8 goroutines on one bucket produced 51 rounds in 200 with
two or more simultaneous holders (#250).
- `ephemeris/jpl/spk`'s Horizons status sentinels now wrap the HTTP error rather
than replacing it, so a 503 is recognizable as upstream downtime by anything
matching `HTTPStatus() int` — previously a service outage was indistinguishable
from a bad request (#251).
- The Horizons state comparison no longer claims both sides evaluate the same JPL
integration. Horizons serves DE441 and astrogo reads DE440, and the two differ
by 2.42 m at the Moon — most of that test's measured residual is the kernel gap
rather than astrogo, which the tolerance's derivation now accounts for (#257).
- **`Context.GeocentricToObserved` omitted diurnal aberration**, so the vector
reduction route and the stellar route disagreed by up to 0.32″ for the same
target — 0.3150″ at the equator, 0.1966″ at Greenwich — while applying
refraction, the other half of what "observed" means. `Reducer.Reduce`,
`ReduceBatch` and every `plan` caller shared the defect (#261).
- **`ApparentState` applied no gravitational light deflection**, so the apparent
place was short by a term that reached 0.62″ for Jupiter near conjunction.
Against Horizons over 2026 the worst case per body drops from 0.08–0.67″ to a
flat 0.05–0.06″, and the new `ephemeris.apparent.geocentric` suite pins it
(#263).
- **Concurrent cache writes of one object name collided on Windows.** `fileblob`
stages every write through a temp file named from a clock that does not advance
there — 2000 consecutive `UnixNano` reads returned one distinct value — and puts
it in `os.TempDir`, so writers renamed each other's staging file away, 59 times
in 320. Cache buckets now stage inside themselves, which also avoids re-copying
a multi-gigabyte kernel across volumes, and `remote.Save` (since renamed) serialises
writers of one key (#241).
- `docs/VALIDATION.md` said a smeared clock was untracked when
`time.Time.LeapSmearWindow` already handled it, and carried an SGP4 limitation
whose shape had changed. A document that tells readers a safety signal does not
exist while shipping it is worse than one that says nothing (#268).
- **Staging inside the cache bucket broke concurrent processes on Windows**, which
#266 shipped and CI then failed on: two `go test` binaries sharing one cache
collided on a staging path they could not retry past, failing every test in
`ephemeris/jpl` for three minutes. The parameter is gone. The in-process write
lock is keyed on the key's basename instead — the collision domain the staging
path actually has — which covers the separate-bucket case the parameter was
added for (#241).
- **A body id above `MaxInt32` was converted rather than refused.** `core.ID` is
unsigned and a NAIF id is signed 32-bit, so the top half of the range wrapped to
a *negative* id — which is meaningful, since that is how NAIF numbers
spacecraft. `core.ID(0xFFFFFFFF)` was looked up as −1 and answered for a body
the caller never named. `jpl.Provider.State` now returns `ErrBodyIDOutOfRange`,
and `plan`'s designation parser bounds to 31 bits (#273).
- **A cache fetch that lost a cross-process race failed instead of returning the object.**
The download lock is exclusive within a process and only mostly so across them, so two
`go test` binaries fetching one kernel could both reach the download; the loser died on
`Access is denied` renaming its staging file while the winner wrote a complete kernel.
`GetFile` now re-runs its freshness check before reporting a fetch failure, so the caller
gets what they asked for when someone else has just produced it (#241).
- **A null table value read as 0.0.** `fits.BintableHDU`'s float-column accessor — and a
second copy of the same logic in `skybrightness/dataset/solar` — returned zero for an absent
value, so a missing flux became a flux of nothing and a missing magnitude became magnitude 0,
which is a very bright star. Nulls read as NaN now, which is what every consumer already
screens for. (#275)
- **Download staging on Windows.** Serialize staging writes with other writes sharing the same basename, preventing temporary-file collisions across cache keys and buckets. (#292)
- **Documentation: the CIRS place says so.** The type now called `coord.CIRS`
had a doc comment saying only "the true geocentric position of an object",
which reads as the equinox-based apparent place it is not. A caller comparing
its `RA()` against an almanac's apparent right ascension was 20 arcminutes out
with nothing in the type saying why. The comment now names the system and
points at `TETE` (#126). It was renamed from `Apparent` in the same release
(#298), which is the other half of the same fix. (#299)
- **An archive's error page no longer reads as a corrupt result set.** When a TAP
service answers a query with HTML and a 200 — a maintenance notice, a load
shedder, a login wall — the VOTable reader now reports that rather than
surfacing whatever the page eventually fails to parse on. ESA's Gaia archive did
this during a tagged run and the suite failed on `XML syntax error on line 161:
unexpected end element </div>`, which sends the reader looking for a parser bug
instead of at somebody else's outage; `TestArchivesAgree` now skips, as it
already did when a front end accepts a connection and stops answering. (#301)
- **The FINK validation test now says why it could not compare.** "No valid
r-band observations" covered two conditions that want opposite outcomes — FINK
holding no r-band photometry for the object right now, which is absence, and
FINK renaming a column, which is a schema change astrogo must notice. The test
now skips for the first and fails for the second, naming the columns and the
counts either way. (#302)
- **`coord`'s package documentation reads in a sensible order again.** Six frame
sections landed in six separate pull requests, each inserted wherever it would
not collide with the others, so the result was ordered by merge mechanics rather
than by topic — and the `Reducer` paragraph ended up orphaned inside the Local
Standard of Rest section, where it has nothing to do with anything around it.
Frames are now grouped, the two mechanism sections sit at the end, and the
`Reducer` paragraph is back under Transformations. No text changed. (#305)
- **The README's frame list was three frames out of date.** It named ITRS, TETE
and LSR as missing while all three were on `main`, which is the worst kind of
stale documentation — the honest-limitations paragraph is the one a reader
trusts most. It now lists what arrived and what did not, and says why LSRK has
not: its apex is published in the B1900 equinox that `coord.FK4` cannot express.
The `coord` row of the package map enumerates the frames the way the `time` row
already enumerates its scales. (#305)
- **Proper motions from SIMBAD and Gaia were applied short by a factor of
cos(dec).** Both catalogues publish μα\* — the on-sky rate — and every consumer
in `coord` handed it to SOFA, which wants dRA/dt. A star propagated twenty years
moved 20·cos δ arcseconds instead of 20: 30% short at δ = 45° and 83% short at
δ = 80°, putting Kapteyn's Star 49″ from where it is. `coord` now converts at
the one boundary where SOFA is called, so a catalogue row travels to a position
unchanged at every layer between (#281).
- **The cross-process download lock collided in its own staging.** `AcquireLock`
creates its lock object through the same `fileblob` writer everything else
uses, and that writer stages through a temporary file named from a clock which
does not advance on Windows — so two writers of one lock key picked the same
staging path, and the loser's rename found its source already gone.
`TestStagingAndPartialWritesAcrossBuckets` failed on every run because of it (since renamed).
The write is now serialised within the process the way `Save` already was, and
the `NotFound` that a losing writer raises across processes is recognized as
contention rather than returned as an error (#241).
- **Satellite positions were 93× further from the reference than they needed to
be.** A TLE carries mean elements fitted by Space-Track *through SGP4 with
WGS-72 constants*, so feeding them back through a propagator configured for
WGS-84 asks a different model to interpret numbers this one produced.
Measured against Vallado's own verification suite, that single constant moved
the agreement from p50 0.0346 / max 0.2889 km to **p50 0.0000 / max 0.0031 km**.
Both sets passed the 1 km contract; the regression detector at 0.4 km had ten
times more slack than the fault it was watching for, and is now 0.01 km. (#308)
- **The cross-process download lock is now exact.** It was gocloud's
`WriterOptions.IfNotExist`, which under `fileblob` was a Stat followed by a
Rename and could admit a second holder ([#241]). It is `O_CREATE|O_EXCL` inside
an `os.Root`, which the kernel makes indivisible, so a losing writer has exactly
one way to lose and it is `fs.ErrExist` — the three-code classifier and the
staging lock the lock needed around its own write are both gone. (#325)
- **A directory listing's entry metadata now matches a stat of the same name.**
On Windows a directory entry's cached `LastWriteTime` lags the child's own,
so an `fs.FS` returning those entries while serving `Stat` from a real stat
disagreed with itself — measured at 15 failures in 40 for `os.DirFS` as well,
so it is the standard library's behavior there rather than astrogo's.
`remote/file`'s local backend reads each entry's metadata when it is asked for
instead, which `fs.DirEntry.Info` explicitly contemplates, and now passes
`fstest.TestFS` unfiltered: 0 failures in 40. Closes [#323]. (#327)
- **JPL being unreachable no longer turns a build red.** `TestSmallBodyEros` and
`TestSmallBodyMultiMatch` are untagged and hit the live network, and their own
doc comment said they must not fail for somebody else's downtime — but they
recognized only the two ways Horizons answers 200 and still cannot serve a
kernel. A CI run timed out dialling NAIF's file server, which is a different
host, and the build failed. `internal/testutil.Unreachable` is the predicate
they were missing: a timeout, a DNS failure or a dial that never reached a
service, distinguished from any status a server actually returned. (#327)
- **`ICRSToFK4` no longer invents proper motion for a star declared at rest.**
It chose its conversion route by testing every kinematic field for zero, which
cannot tell "no proper motion recorded" from "measured as zero" — different
claims about a star that convert differently — and answered the first when
asked the second. A star declared at rest in ICRS came back from
`ICRS → FK4 → ICRS` carrying **0.6 to 0.9 mas/yr** it never had, while its
position closed to 19 µas, which is what kept it invisible. `coord.ICRS` now
records whether kinematics were supplied, as `coord.FK4` has always done
through its two constructors, so both routes are chosen rather than inferred
and both round trips close. `ICRSToFK5` gets the same treatment. Closes [#278]. (#329)
- **The six-element frame conversions no longer need a parallax.** `ICRSToFK5`,
`FK5ToICRS`, `ICRSToFK4` and `FK4ToICRS` route through SOFA's `iauH2fk5` /
`iauFk52h`, which build a space-motion pv-vector and therefore need a distance.
A parallax below `PXMIN` was replaced by one putting the star at 10 Mpc, where
any real proper motion exceeds `VMAX = 0.5c` and the space velocity is set to
**zero** — and because those SOFA routines are `void`, the status saying so was
discarded before astrogo could see it. A star with 150 mas/yr and no recorded
parallax — most of any pre-Hipparcos catalogue — came back from a round trip
with no motion at all, and a star declared *at rest* in FK4 came back with
2.4 mas/yr in each component and 0.34 km/s it never had. `internal/gofaext` now
dispatches on `iauStarpv`'s own status and falls back to a formulation in which
the distance cancels exactly: the transformation is linear in velocity and
orthogonal in position, so dividing through by the distance leaves the proper
motion transforming on its own, with parallax and radial velocity untouched.
That is the exact limit rather than an approximation, and SOFA's route is still
taken whenever it can answer, since it carries relativistic and light-time terms
that no distance-free formulation can. Verified against SOFA in the overlapping
regime: the two agree to 8.9e-17 mas/yr for a star at rest and diverge only with
radial velocity, reaching 5.6e-05 mas/yr at 20 km/s. Closes [#331]. (#338)
- **The six-element frame conversions no longer label their output with an epoch
it is not at.** `ICRSToFK4` and `FK5ToFK4` answer at B1950.0 through SOFA's
`Fk524`, and `ICRSToFK5` answers at J2000.0 through `H2fk5`; none of those SOFA
routines takes an epoch, by design, because a star with a recorded proper motion
has its state stated at the catalogue equinox and moving it is a separate
operation. The conversions nonetheless stored the caller's epoch in the returned
struct, so `ICRSToFK4(star, 1975)` returned B1950 numbers reporting
`Epoch() == 1975` — the numbers right and the label wrong, which is the worse
way round, since a wrong epoch propagates into `FK4ToFK5`'s position-only route
and into anything reading `FK4.Epoch`. They now report `B1950` and `J2000Epoch`,
and the doc comments say plainly that the argument applies to the position-only
route only, why propagating instead would mean inventing a convention SOFA
declines to define (the E-terms of aberration would be evaluated at a different
epoch on each branch), and that `PropagateEpoch` is the rigorous way to move a
star to another epoch. The same defect was present in `ICRSToFK5`, which [#330]
recorded only for FK4. Closes [#330]. (#340)
- **FK4 and FK5 round trips no longer drift away from the catalogue equinox.** A
star with no recorded proper motion still moves in both frames, because each
drifts against the inertial one, and `ICRSToFK4`, `FK5ToFK4` and `ICRSToFK5`
hand that fictitious motion back rather than pretending the star is at rest.
They marked it as a *recorded* motion, which sent the inverse down the
six-element branch — SOFA's `Fk425` for FK4, `Fk52h` for FK5 — and both of those
take no epoch and assume the catalogue equinox, while the position they were
handed is at the caller's epoch. The error was linear in the distance from the
equinox and exactly zero at it, which is why every existing round-trip test
missed it: `ICRS → FK4 → ICRS` lost 0.474″ at B2050 and `ICRS → FK5 → ICRS`
0.096″ at J2100, at 4.7 and 0.96 mas per year respectively. `FK4` and `FK5` now
record whether a motion was measured or supplied by the frame — the same
distinction #278 gave `ICRS` — and dispatch on it. Both round trips close at
every epoch, the FK4 one to the 0.000023″ floor SOFA's own E-term iteration
leaves and the FK5 one exactly; `TestAstrogoMatchesRawSOFAAtEveryEpoch` now pins
astrogo to within a nanoarcsecond of the raw `Fk54z` → `Fk45z` pair, which always
closed and so localised the defect to astrogo's dispatch rather than to SOFA.
Closes [#341]. (#342)
- **A network failure is no longer swallowed by the deadline it caused.**
`internal/testutil.Unreachable` excluded `context.Canceled` and
`context.DeadlineExceeded` before every other check, so an unreachable host
reported as reachable whenever the endpoint's own download timeout fired
alongside the dial failure — which is how `ephemeris/jpl`'s kernel tests went
red on NAIF's downtime with their skip guard in place and not firing. The
exclusion now sits ahead of the check that genuinely cannot tell a network
timeout from a caller's deadline, and behind the three that can: a DNS failure,
a failed dial and `ECONNREFUSED` are not things a context error produces, so a
deadline alongside them decides nothing. Four `plan` tests that fetch a DE44x
kernel before testing AstroPixels, NASA or USNO gained the guard they never had,
and `newEph` now skips rather than substituting the analytic ephemeris when NAIF
is unreachable — that substitution made `TestUSNODecomposesTheTopocentricBias`
report a −0.456″ declination bias as evidence of a precession-nutation defect
that does not exist. Closes [#348]. (#349)
- **`coord`'s au-per-Julian-year constant was wrong in its ninth digit.** It
shipped as the literal `4.740470446`, a decimal repeated widely in the
literature, while its own doc comment described it as "the au divided by the
Julian year" — which is `4.740470463533`, from 149597870700 m over
365.25 × 86400 s, both exact by definition since IAU 2012 Resolution B2. The
constant is now computed from the au in `constants` rather than
written down, so it cannot drift from the au again. The error was 3.7e-09
relative and nothing observable moved — the Sun's rotational velocity in
`SolarVelocityFromSgrA` shifts by 9.2e-07 km/s — but a constant whose
documentation describes a different number than it holds is a trap for the next
reader. `coord/spacevelocity_test.go` held the same wrong decimal, which is why
no test caught it; it now asserts the value independently. (#354)
- **A network test no longer fails when the service's own TLS certificate does.**
An expired certificate, or one signed by an untrusted authority, now skips like
any other upstream outage — it passed the TCP pre-check and matched no network
predicate, so `api.open-elevation.com`'s lapsed renewal made a permanent red
build. A certificate valid for a *different* name stays fatal: that means
astrogo asked for the wrong host, which is the defect these tests exist to
catch. (#365)
- **Ten network-tagged test suites no longer fail when a service is merely having
a bad day.** They checked a socket was open and then treated any answer as a
verdict on astrogo, so a Horizons 503 saying *temporary overload/maintenance*
failed the build. `ephemeris/kepler`'s Horizons fetch also now reports a non-200
as a status rather than as an unparseable body, which is what made its outages
invisible to the classifier. (#368)
- **37 tests that skipped on any error, and so could never fail, now identify it
first.** Each named a cause the code had not established — "SkyCalc did not
answer", "(network issue?)" — and three were not about a service at all: one
skipped when IMCCE's document failed to decode into a struct declared in this
repository, so a schema change would have read green indefinitely. A new
`docsguard` check fails any skip that is the first statement inside a bare
`err != nil` guard, with an allowlist for genuine environment preconditions. (#369)
- **The last eight tests that skipped on an unidentified error now name the
condition they mean**, and the allowlist that exempted them is gone — each was
the same defect one level down. The symlink pair shows why no list: Windows
returns `ERROR_PRIVILEGE_NOT_HELD`, which does *not* satisfy
`errors.Is(err, fs.ErrPermission)`, so the natural predicate would have skipped
on every symlink failure while appearing to check for one. Two tests that
skipped when a loopback listener could not be bound are now fatal, including the
only test that exercises `Unreachable` against a socket the OS really refused. (#370)
- **FK4 conversions no longer invent a radial velocity for a star whose parallax
describes no distance.** A star declared at rest came back from
`FK4 → ICRS → FK4` with a velocity proportional to 1/parallax — 38.9 km/s at
1e-9 arcsec. `iauFk524` builds its pv-vector with a radius of 1.0, so the
geometry never needed the distance: measured, the direction and proper motion
are bit-identical across nine decades of parallax, and only the final
`rv = rd/(px·VF)` division was at fault. The same change closes a band where
`iauFk52h` reported complete success while implying a star moving at 0.43c.
Where the parallax is real, SOFA's answer is kept unchanged. (#376)
- **UT1 is no longer up to a second wrong on the day before a leap second.** The
IERS series was interpolated straight across the whole-second jump in UT1−UTC,
so on each such day it ramped from one side of the step to the other: measured
on 2016-12-31, half a second out at noon and a full second — 15 arcsec of Earth
rotation — just before midnight, on every leap-second day from 1973 to 2016. The
step is now removed before interpolating. (#378)
- **Horizons outages no longer fail the validation suite.** The Horizons fetchers
did not read the HTTP status, so the 503 JPL serves under load reached callers
as a sentinel no classifier could see; the corpus generator also turned a
partial fetch into a reported "corpus would change". Each fetcher now surfaces
the status, the generator skips on an outage instead of diffing half a fetch,
and four unclassified kernel fetches were found by auditing per call site
rather than per file. (#379)
- **Six more tests that could not fail now can.** The skip guard caught a skip only
when it was the first statement after `if err != nil` and spelled `Skip`, so a
skip left after a classifier, and `metrology.NotVerified` — which skips after
recording — both got past it. Four accuracy suites recorded NOT VERIFIED on any
provider error, so a regression that broke the provider would have been
published as an outage. The guard now judges a skip by its enclosing block,
counts skipping helpers as skips, and flags a helper that swallows the error it
is handed; `testutil.UpstreamFailure` is exported so a suite can record before
it stops. (#380)
- **A leap second is now an instant of its own.** `Date(2016, 12, 31, 23, 59, 60, …)`
used to land on the following midnight and convert with ΔAT 37, where the inserted
second carries 36 — a full second wrong. UTC Julian Dates now follow SOFA's
convention, the fraction of a day ending in a leap second being of 86401 seconds,
restated against astrogo's own leap-second table so a registered one is honoured.
Ordinary days are unchanged to the bit. **Do not subtract two UTC Julian Dates
across such a day**: the difference is neither the labels nor the elapsed time;
use `Time.Sub`, or `ToGo` for labels. Four places in this repository did, and are
fixed: UT1 in `coord.Context`, SGP4's `AtTime` and epoch, and `lsk.UTCToET`. (#381)
- **Saturn is no longer too faint when the south face of its rings is lit.**
`magnitude.PlanetApparent` gave the ring inclination a sign, so the ring terms
dimmed Saturn instead of brightening it: 1.9 mag too faint at the 2002
opposition, 0.49 mag at 2026's. It now agrees with JPL Horizons to 0.001 mag
on both faces (#375).
- **SBDB's bright-object query returns elements at full precision.** It rounded
them to four significant figures, as the single-object lookup rounded to three
before it asked for `full-prec`: C/1937 C1's e = 1.000162271 came back as
1.0002, and its perihelion time to a hundredth of a day. (#392)
- **SBDB values written in E-notation are read whole.** The decoder kept the
digits before the `E`, so a mean anomaly of `-2.593805408851336E-5` degrees
was read as −2.59°: every element set near perihelion was off by orders of
magnitude. (#392)
- **`kepler.NewElements` reports a comet's open orbit as `ErrUnsupportedOrbit`.**
Given a = q/(1−e), which is negative or infinite for e >= 1, it used to return
`ErrInvalidElements`, indistinguishable from bad data (#374).
- **The API Diff check judges a pull request by its own changes.** It compared
the pull request merged into today's main with main as it was when the pull
request was opened, so every break merged in between was blamed on it: #392,
which only adds symbols, was told to declare #383's renames (#394).
- **49 test loops that moved on from any error now say which error they expect,
or fail.** A `continue` past whatever an iteration returned could not fail on
it, and passed outright when every iteration errored: an airmass audit passed a
function that errored on every input, and the NASA and AstroPixels comparisons
logged solver errors as skips. `TestLoopsDoNotPassOverAnUnidentifiedError`
enforces it, with no exemption list (#393).
- **Kepler positions are no longer tilted 0.042″ about the equinox.**
`ephemeris/kepler` rotated J2000-ecliptic elements into the equator by the
IAU 2006 obliquity, where JPL defines their ecliptic with IAU 1976's: 33 km
for C/2023 A3 at 2.5 AU. 433 Eros now matches Horizons to 0.000″ at its epoch
of osculation, where it read 0.04″ (#391).
- **Mars's magnitude now includes Mallama & Hilton's rotation and seasonal
corrections**, which Skyfield omits and JPL Horizons applies. It was up to
0.076 mag from Horizons; it now agrees at 11 dates 2003–2026 to 0.0005 mag and
reproduces the authors' own test data (#389).
- **The NASA eclipse validation compares every catalog row.** It had dropped 78
eclipses whose type codes its parser did not list; with them, solar is
1433/1433 and lunar 1451/1452, and the ΔT cross-validation now asserts a bound
instead of logging (#398). The lunar miss and 173 over-reported eclipses are
#401. (#402)
- **`plan.LunarEclipses` and `plan.SolarEclipses` decide an eclipse by the
shadow, not a fixed 1.58° latitude.** Over six centuries of NASA's Five
Millennium Canons they reported 173 eclipses that do not happen and missed one
that does; they now agree on every eclipse, with greatest eclipse 0.16 minutes
from the canon's on average instead of 0.8. `Gamma` is measured against the
real limit, and a provider error mid-search is returned (#401).
- **`constants.IAU2015.SunEquatorialRadius` is IAU 2015 B3's 695,700 km, not
696,000 km**, so `plan.AngularDiameter` agrees with JPL Horizons for the Sun
instead of running 0.85″ large. `MeanEarthRadius` keeps its 6,371 km but no
longer cites B3, which defines no mean radius, or claims to be exact (#403).
- **`magnitude.PlanetApparent` computes Pluto**, which it listed as supported
and refused: the Explanatory Supplement's historical law for Pluto and Charon
together, V(1,0) = −1.01 and 0.041 mag/°, documented as an approximation that
can be tenths of a magnitude off, not a prediction. `plan.VisibleTonight`,
which dropped that error and so never listed Pluto at any limit, now reports a
body or planetary moon it cannot evaluate through `ErrIncomplete` (#407).
- **`fits` reads the D exponent the FITS standard allows** (`1.5D-05`), and a
keyword that is present but does not parse is an error, never its default.
Before, `ExtractWCS` silently read such a header as reference point (0, 0),
unit scale and no distortion, and `ReadImage` dropped a `BZERO = 3.2768D+04`,
leaving 16-bit unsigned pixels 32768 low. Adds `fits.ErrWCSMalformedKeyword`
(#409).
- **`plan.MoonPhases` samples to the end of its window**, so a phase — and an
eclipse at that syzygy, through `LunarEclipses` and `SolarEclipses` — in the
window's last partial six-hour step is no longer lost, and a refinement that
fails is returned instead of silently dropping the phase (#419).
- **A search interval that ends before it starts is `plan.ErrReversedInterval`**
from every interval search — `EventSolver.Find` and the helpers on it,
`MoonPhases`, the eclipse searches, `SatellitePasses`, `TransitEstimate`,
`Episode` and `Find` — where it used to panic inside a sampler (#418).
- **`plan.EventOpposition` — and `Oppositions` and `FullMoonOppositions` with
it — is solved on geocentric ecliptic longitude**, as oppositions are defined,
instead of right ascension, which put the oppositions of Mars up to 51 hours
late and Full Moons up to 3.25 hours off; they now agree with JPL Horizons to
a minute with DE440s (#413).
- **`plan.Seasons` includes nutation in longitude**, which SOFA's `Eqec06` does
not apply, and takes the Sun's apparent place from the ephemeris: equinoxes and
solstices that were up to 8.5 minutes off now agree with Skyfield to under a
second and with USNO to its rounding (#414).
- **`plan.MeteorShower`'s `IsActive` and `RadiantAt` compare IMO's solar longitudes
with the Sun's longitude for the equinox J2000.0**, as IMO tabulates them, not the
longitude of date: windows and peaks were about nine hours early in 2026, and
drifting 20 minutes further each year (#415).
- **`plan.MoonIllumination` returns the Moon's phase angle**, the Sun–Moon–Earth
angle, where it returned the elongation, and the fraction (1 + cos i)/2, up to
0.0014 closer to Skyfield's. Lunar-phase events carry that fraction as their
`Value`, so a quarter Moon is 50.13% lit, not exactly half (#416).
- **`EventSolver` finds a meridian transit wherever the hour angle rises through
zero**, not only beside a sampled altitude maximum: transits at high latitude,
in a window's first step, or with a fine step are no longer dropped, and the
Sun's are no longer 1.3 s early from aberration applied twice (#417).
- **`time.Time.ToGo` keeps the nanosecond**: it summed both Julian-date parts
into one float64 count of seconds since 1970 and lost up to 119 ns, so
`FromGo(t).ToGo()` was not the identity for most instants (#420).
- **A `plan.Window` whose End is before its Start is empty**: it overlaps
nothing, and `Union`, `Intersect`, `Subtract` and `TotalDuration` leave it
out, where `Subtract` returned it whole from under a window covering it,
`TotalDuration` counted it as negative time, and `Union` returned it beside a
window it overlapped (#421).
- **`plan.Episode` probes the horizon in the same geometric atmosphere as its
solver**: with refraction the probe called a target up just before a rise or
just after a set, and `Episode` returned the episode that had already ended
(#422).
- **TDB − TT is the leading 37 terms of Fairhead & Bretagnon (1990)**, within
0.8 µs of SOFA's `iauDtdb` over 1900–2100, where five terms of which only one
was FB90's were 54 µs off against a documented ±1 µs (#423).
- **`remote.Client.Reset`'s doc says what it restores**: endpoints, consent,
offline mode and policy, leaving the data directory and API options, where it
promised all of `NewClient`'s state. A `catalog/mpcorb` test that trusted it
broke the tagged suite, and a docsguard check now catches the pattern (#443).
- **`remote.GetFile` checks download consent before waiting for another
process's download lock**: a caller who could never download was held by the
lock for up to half an hour, and with it every lazy EOP lookup in the process
(#424).
- **`EventSolver` refines a crossing from the two samples that found it**, rather
than evaluating the ends again: a sample within 1e-7° of the threshold could
read differently the second time, and the whole search failed with
`ErrBracketingViolated` instead of returning its events (#425).
- **Rise/set and twilight helpers find a crossing pair that falls between two
samples**: when a body's daily extreme only just passes the threshold — the
Sun at Mawson in midwinter, a short astronomical night at Paris — both
crossings fit in one 15-minute step, and the helpers returned nothing (#426).
- **`time.Time.FormatJulian` rounds to the nearest second**, and
`DateJulianCal` keeps its day and time of day as separate Julian-date parts:
half of all whole-second Julian-calendar times printed a second early, and
`Format` did the same for years outside 0–9999 (#439).
- **`plan.MoonPhases` measures the elongation between the apparent Sun and
Moon**, as published phases are defined: geometric longitudes put every phase
about 40 s late, and up to 43 s from `FullMoonOppositions` (#430).
- **`EventSolver` returns provider and root-finder failures it used to drop**:
a rise, set, transit or lunar phase went missing, and a greatest elongation's
side was decided from a zero position, each with a nil error (#437).
- **A download lock left by a crashed process goes stale after twice the
download timeout** rather than always 30 minutes: a minute for the IERS
bulletin, whose lazy load, with consent, held every EOP lookup behind the
wait (#445).
- **`plan.Seasons` returns a failed refinement** rather than skipping that
season, which left a year missing an equinox or solstice with a nil error
(#453).
- **A partial download is no longer thrown away when its ETag sidecar cannot be
read** — a virus scanner holding the fresh file open on Windows read as no
ETag — and the read is retried before it is given up on (#452).
- **`plan.SatellitePasses` returns propagation failures** rather than dropping
the pass, leaving out its culmination, or filling a pass event with zeros,
each of which it did with a nil error (#454).
- **`fits.Read` rejects unusable structural keywords** instead of trusting
them: a missing or malformed `NAXIS2` no longer reads as an empty table, and a
negative axis, an overflowing axis product or `NAXIS` above 999 is an error
rather than a panic or an empty image (#460).
- **The scheduler returns a transition overhead it cannot compute** instead of
skipping the candidate, so a block no longer goes unplaced with no reason, or
placed with no setup time when greedy's refined estimate failed (#454).
- **CLAUDE.md's table of fuzz targets left out `ephemeris/satellite/sgp4`**, so
its two targets were outside the extended-fuzzing step. `internal/docsguard`
now holds the table to the code in both directions, and the package count
above it too. (#465)
- **`fits.Read` allocates the data that arrives, not the size a header claims.**
A 2880-byte file declaring a 400 MB image allocated all of it before reaching
EOF; it now costs what it holds and ends in `io.ErrUnexpectedEOF` (#461).
- **`skybrightness.ZodiacalColorCorrection` gives one direction one answer**:
an elongation written as 340° took the 90° slope while −20°, the same
direction, took the 30° one (#467).
- **`fits.WCS` works at and near the celestial poles.** `WorldToPixel` failed
for most pixels in frames centered above 89°, a pixel at the pole deprojected
to NaN, and the pole itself could not be located (#469).
- **A slow NAIF skips `plan`'s integration tests instead of timing out the
package.** Their kernel fetches are bounded by the test binary's own deadline
and an exhausted budget is read as the upstream's failure (#471).
- **A `coord.Context` before 1972 rotated stars and the Moon by different
Earths**, up to 11.8″ apart on the days SOFA stretches for a TAI−UTC step:
its astrometry used SOFA's TT and UT1, the rest astrogo's. Every part of a
Context now uses astrogo's, unchanged from 1972 on (#474).
- **`Planner.RankObservable` scored the Moon and satellites as stars at
infinity**: its adapter dropped `GeocentricVec`, so the Moon ranked up to 0.78°
high and the ISS at 64° while below the horizon (#483).
- **The scheduler scored a block with the Earth turned to the nearest
constraint step**, not to the block's midpoint, whenever the block was not an
even number of steps long. It also now evaluates through one hourly `Context`,
which makes the built-in strategies 13–59× faster (#481).
- **An endpoint or data dir reached through `?prefix=` or `?key=` could never
download**: both wrappers hid the backend's write, lock and cancel interfaces.
They now carry them, and `OpenFS` refuses a backend whose capabilities a
wrapper cannot keep (#487).
- **A `coord.Context` stepped across a leap second was 13″ off**: it reused the
base Context's DUT1, which jumps by a second there. Each instant now takes its
own DUT1, as `NewContext` does (#489), and `coord.Context.SetTime` keeps it. (#491)
- **`Satellite.GetDetails` reported every satellite at 0 km**: it read the
distance from `coord.Context.GeocentricToObserved`, which set none. The
transform now carries the topocentric distance, and the details keep the range
they computed (#495).
- **`CrescentParams.MABIMS1995` dropped the criterion's 8-hour age alternative**
("2-3-8"). It now reads the new `Age` field (#496).
- **The crescent criteria read their own publications**: Fotheringham's
`12 − 0.008·DAZ²` was linear, Maunder's DAZ coefficient ten times too small,
Qureshi's cubic added rather than subtracted, Bruin's and Ilyas's curves and
Caldwell & Laney's lines were not theirs, and each now reads its author's
frame and instant (#503).
- **A network test failed instead of skipping when the server dropped the
connection before answering**: `Unreachable` now counts an `EOF` from
`http.Client.Do`, and a reset on Windows, where `WSAECONNRESET` never matched
`syscall.ECONNRESET`. A body cut short by a server that answered still fails
(#505).
- **59 Go doc links rendered as plain text**, most behind a removed or renamed
symbol, and `remote`'s and `ephemeris/satellite`'s package docs described APIs
that no longer exist. Both docs are rewritten, and `internal/docsguard` now
fails on a doc link that does not resolve. (#510)
- **`coord.SubPoint`, and with it `plan.SubsolarPoint`, `SublunarPoint` and
`Terminator`, ignored precession and nutation.** A GCRS direction was rotated
by GAST alone, putting the subsolar point 0.38° (42 km) off in 2026 and
growing 50″ a year. It now uses the full IAU 2006/2000A celestial-to-terrestrial
matrix, and the plan functions pass the apparent place. (#513)
- **The EOP-unavailable warning's remedy now names `remote/eop`**, without which
`remote.EnableDownloads` changes nothing, and docs that still described the
gocloud storage layer — `s3://` and `sftp://` cache examples, a `no_tmp_dir`
parameter the `file://` backend ignores, binary sizes measured against gocloud —
describe the current one. (#516)
- **A degraded VizieR could still fail the network tests instead of skipping
them.** The control query that tells a service outage from an astrogo bug
named no column, so it passed while VizieR refused column names as
"unresolved identifiers"; it now selects a column the table has always carried. (#521)
- **`time.FromGoDuration` no longer claims to be exact in every case.** A
`unit.Duration` is float64 seconds, which keeps the nanosecond up to about 48.5
days and rounds by up to 4 ns at a year. `Weekday`, `JulianCalendar`, `AddDate`
and `FromGoDuration` are now tested against dates of record. (#522)
- **A rotated `CDi_j` matrix with ordinary sky parity was read rotated the wrong
way.** `fits.ExtractWCS` split CD into CDELT and PC by columns, which flips the
cross terms whenever the axes have opposite signs: 324″ out at 900 pixels for a
30° turn. It now splits by rows, and the WCS is held to WCSLIB through a
checked-in Astropy fixture, to 4e-10″. (#525)
- **`coord.FromECEF` lost height accuracy with altitude** — 1.5 mm at the ISS, 31
cm at geostationary orbit — because Bowring's single step is exact only near
the ground. It now uses SOFA's `iauGc2gde`, which holds 2e-8 m at
geostationary orbit, and is twice as fast. `coord.ErrInvalidEllipsoid` reports an
ellipsoid SOFA refuses instead of returning NaNs. (#528)
- **`atmosphere.RefractionRigorous` (since removed, #588) dispersed light 16 times too little**, through
an unsourced 0.005/µm wavelength factor: 0.155″ between 0.40 and 0.70 µm at 30°
where SOFA gives 2.47″. `StandardRefraction` uses this model, so
`coord.Reducer.Disperse` inherited it. It now scales by the IAG (1999) dry-air
refractivity SOFA's `iauRefco` uses, and disperses like SOFA's model to 0.1%. (#529)
- **`angle.ParseDMS` and `ParseHMS` no longer return the opposite sign for a typeset minus.**
A U+2212 minus, as journals and SIMBAD print negative declinations, was skipped like a separator, so a southern declination parsed as northern.
It is now a sign; text the parsers do not recognize, such as a hemisphere letter, is `ErrSeparator`, and a fourth field is `ErrTooManyFields`. (#541)
- **`plan.TransitEstimate` and `MaxAltitudeInWindow` missed a peak on the window's edge.**
The window's end was never sampled unless it was a whole number of 10-minute steps, so a target still rising at the end was reported up to 10 min early and 1.45° low.
`RankObservable` and `VisibleTonight` inherit the fix. (#543)
- **`plan.Conjunctions` now finds conjunctions in right ascension of date**, as the almanacs define them.
It compared right ascension along the J2000 equator, which put conjunctions up to 2.5 min off; all four checked against Skyfield on DE440s now agree within a second. (#546)
- **`plan.SwapOptimizedStrategy` could schedule a block before the slew to it had finished.**
Its swap and gap-insertion moves checked the transition into the block they moved but not the one out of it, and left stale `SetupTime`s; every move is now re-timed against its real neighbours. (#549)
- **`plan.VisibleTonight` no longer fails the whole night over a target that peaks just below 0°.**
From an elevated site its windows reach the dipped horizon, below the astronomical one, where airmass is undefined; such a peak now takes the horizon's airmass, a lower bound on its extinction. (#553)
- **`plan.VisibleIntervals`, `Find` and `ObservableWindows` now sample the end of their range.**
They stopped at the last whole step before it, so a target setting after that step was reported up until the end, and one rising after it had no final window at all. (#555)
- **`plan.Scorer`'s urgency no longer makes a rising target look about to set.**
The hours until set were interpolated from now rather than from the last probe the target was up at, so a star 15 minutes after rising came out 1.6 h from setting against a true 7.1 h. (#556)
- **Satellite standard magnitudes now reproduce themselves.** `magnitude.SatelliteApparent` measured both conventions' phase correction from 90° and converted Molczan magnitudes to McCants ones, so a McCants magnitude came out up to 1.24 mag too bright and a Molczan one 1.45 mag too bright at their own reference geometry. Each convention is now applied at its own reference phase, full phase for McCants and 90° for Molczan, and a Molczan-based prediction stays about 0.7 mag fainter, as the convention's definition says. (#563)
- **A satellite in Earth's shadow no longer gets a magnitude.** `Satellite.ApparentMagnitudeCtx` returns the new `plan.ErrSatelliteEclipsed` instead; it gave the ISS −3.9 on a pass spent entirely in the shadow. `LimitingMagnitudeConstraint` rejects an eclipsed satellite rather than falling back to its standard magnitude. Shadow entries and exits agree with Skyfield within 0.35 ms. (#564)
- **The horizon dip is documented as what it is.** `Site.HorizonDip` returns the apparent dip, 1.76′√h with terrestrial refraction (0.82° at 786 m), but its doc called it geometric and quoted the geometric 0.90°. Seven other places used the same label, and `VisibilityEvents` claimed 34′ of refraction its threshold does not add. A test now pins the dip to its doc. (#569)
- **The meteor shower table follows IMO's 2027 Working List.** The Southern δ-Aquariids peaked three days early (λ☉ 125° instead of 128°), several ZHRs, population indices and radiants were out of date, every maximum was rounded to the whole degree (17 h for the Ursids), and activity windows were short of IMO's dates. Every value now comes from the IMO Meteor Shower Calendar 2027, and each activity window is the Sun's J2000 longitude on IMO's dates. (#574)
- **`MeteorShower.ObservedRate` follows the date.** It applied the peak ZHR on every night of the year, predicting 97 Perseids an hour on 1 March. The new `MeteorShower.ZHRAt` gives the rate at a time: IMO's maximum shaped by a new `plan.ActivityProfile`, Jenniskens (1994)'s fit to each stream, and zero outside the activity window. A full-turn window, [0°, 360°], no longer collapses to a single instant. (#575)
- **`plan.SublunarPoint` puts the Moon at the zenith.** `coord.SubPoint` took the point whose ellipsoid normal is parallel to a body's direction, which treats the Moon as infinitely far: the Moon sat up to 11.7″ from the zenith there, and the sublunar latitude was 4.3″ from Skyfield's. It now takes the foot of the normal through the body's position, which also gives a satellite's sub-satellite point; the Sun and planets are unchanged. (#581)
- **`fits/plan` reads the FITS standard's keywords.** `SiteFromFITS` takes the standard's `OBSGEO-X/Y/Z` and `OBSGEO-B/L/H`, and sexagesimal `SITELAT`/`SITELONG`, all of which it reported as missing. `TargetFromFITS` evaluates the header's own WCS at the frame centre, honouring `CTYPE` and `RADESYS`. It had returned `CRVAL` as RA/Dec, 0.57° off for a corner reference pixel and the Galactic Centre as RA 0, Dec 0. A frame it does not convert is the new `ErrUnsupportedFrame`. (#583)
- **Zero EOP no longer arrives in silence.** With nothing loaded, as in a program that never imported `remote/eop`, `Time.EOP`, the UT1↔UTC fallback and `Time.UT1` now emit the one-time EOP warning, with its cause. Before, they returned zero DUT1 and polar motion without it. `RegisterModel(ZeroModel{})` stays silent. `Time.UT1` still returns no error there, and its doc no longer promises one. (#584)
- **The FINK integration tests no longer fail on FINK's bad moments.** A 200 holding nothing usable is retried, and then checked against a control query: an empty control skips the test as a degraded service, and a live one fails it as wrong data. Each distinct query is fetched once per run, two requests where there were five. (#585)
- **The default refraction now reaches the horizon.** `atmosphere.RefractionSOFA`, which every `Site` uses, was SOFA's series clamped at 2.87°: 10.3′ on the horizon against the almanacs' 34′. It hands over to Bennett-NA below 10°, within 13″ of Hohenkerk & Sinclair's ray tracing on the horizon, and is unchanged above. A line of sight more than about 4° below the horizon is no longer refracted. (#589)
- **`atmosphere.Builder.Ozone` accepted a negative or non-finite column**, which would make ozone emit rather than absorb. `Build` now refuses one. (#593)
- **`plan.VisibleTonight` applied 0.20 mag/airmass of V extinction at every site**, about twice what Mauna Kea measures. It now uses the site's own air at V's pivot, 547.8 nm. By default that is a clean night at the site's height: 0.162 at sea level and 0.132 at Paranal, where 0.129–0.131 was measured. (#594)
- **`catalog/gaia`'s `TestArchivesAgree` failed on ESA's own query timeout** (HTTP 408, "Job timeout/aborted"). It now skips on every upstream failure `testutil.SkipOnUpstreamFailure` recognizes, as the other network tests do. (#597)
- **`catalog/fink` read every asteroid number in FINK's SSOFT bulk table as 0**, because the table stores `sso_number` as a string. The index held one entry, `Count()` returned 1, and whenever the single-object endpoint failed, `Resolve` reported a numbered asteroid as not found. All 148,922 usable rows are indexed now. (#598)
- **`catalog/openngc` dropped OpenNGC's associations of stars and emission nebulae**, 72 rows whose type codes (`*Ass`, `EmN`) its parser did not know, M24 and Brocchi's Cluster among them. Every type OpenNGC defines is now mapped or deliberately skipped. (#600)
- **`catalog/simbad` classified objects by string-matching their type codes**, so candidate stars (Aldebaran among them), most galaxies (M33, M77) and most nebulae came back as `KindOther`, and open and globular clusters as a generic cluster. Kinds now follow SIMBAD's own type hierarchy, its `otypedef` table. (#602)
- **`plan.VisibleTonight` only ever considered SIMBAD's 100 brightest objects**, down to V 2.46, whatever its magnitude limit, because `simbad`'s `SearchBright` took `TOP 100` for a request with no limit. It now returns every object, up to SIMBAD's 50,000-row default. Past that it reports the new `resolve.ErrTruncated` rather than a short list. At Paranal, at limit 4.5, that is 632 objects where it was 86. (#604)
- **`catalog/gaia` and `catalog/vizier` cone searches returned an arbitrary subset of a crowded cone**, since `TOP n` came with no ordering. Now a capped result is the `n` sources nearest the center, nearest first. TOP 5 of a 5° cone around the Trapezium had returned stars 4.6° to 5° out. (#606)
- **Four `plan` Skyfield integration tests failed the build when NAIF was slow to deliver DE440s**, where every other kernel-backed test skips. They now skip through `requireKernel` on an upstream failure and stay fatal on anything else. (#608)
- **`catalog/norad`'s `GP.ToTLE` lost the last digit of eccentricity and B\* to floating-point scaling**, and wrote a zero exponent as `-0` where CelesTrak writes `+0`. It now writes these fields from the GP value's decimal digits, and reproduces CelesTrak's published TLE for 11,345 of 11,357 satellites on line 1 and all of them on line 2. (#610)
- **Gaia DR3's reference epoch J2016.0 was written as JD 2457388.5**, half a day early, in `catalog/gaia` and `catalog/vizier`. SIMBAD's J2000 was built on the UTC scale. Both now match their definitions; the effect on any propagated position is under 0.015″. (#614)
- **`catalog/mast` stored the name of the service it relayed a lookup to ("SIMBAD", "NED") as an alias**, so `xmatch` and `catalog.Resolver` matched unrelated MAST results as one object (M31 with M33 and Vega). The name is no longer recorded. (#615)
- **`catalog.Resolver` never matched one provider's ID against another provider's alias**, though its alias match is documented to. It indexed IDs and aliases in separate namespaces, so MAST's `"M  31"` and SIMBAD's M31, whose aliases include `"M 31"`, came back from `Search` as two objects when neither had a position. IDs and aliases now share one normalized key, as in `catalog/xmatch`. (#617)
- **`catalog/jpl` resolved common names to the wrong body, or to none**: ISS to Larissa, Moon to the Earth-Moon barycenter, Voyager 1 to the ID `"spacecraft"`, and Halley to nothing. `Search` now ranks Horizons' whole answer by name before capping it, both match tables are read by their columns, a single match's ID is its last parenthetical (or a comet's record number), and a DASTCOM record number is no longer stored as `SPKID`. (#619)
- **`catalog/sbdb` and `catalog/norad` reported "no such object" as a failure**, so a `catalog.Resolver` with either could never return `ErrNotFound`. SBDB's "specified object was not found" is now `resolve.ErrNotFound`, and a name several objects match is `resolve.ErrAmbiguous` unless one of them has the query as its designation. CelesTrak's 404 "No GP data found" is now an empty result. (#622)
- **`docs/VALIDATION.md` claimed ±1 min for rise and set at 8849 m against "internal consistency"**, for a test that checked only that the events moved at least 3 minutes from sea level. The Sun's and Moon's rises and sets on Everest's summit are now held to Skyfield on the site's own threshold, within 0.09 s and 0.31 s, and the row says the dip is the almanac's sea-horizon convention. (#624)
- **`docs/VALIDATION.md` claimed 1e-4 for `atmosphere.Airmass` against "analytical"**, for a test that bounded the horizon value between 35 and 42. It is now held to Pickering's published formula and to pvlib's independent implementation to 1e-12, and its doc says it is the molecular airmass. (#626)
- **`catalog.Resolver` never applied proper motion when cross-matching by position**, so a star faster than about 0.125″/yr never matched its own Gaia row: HD 189733, tau Cet and Barnard's star took nothing from Gaia. Positions now move by the Target's own kinematics, and `resolve.ConeRequest` gains `Epoch` so Gaia and VizieR search around the center moved to their own catalog's epoch. (#629)
- **`plan.VisibleTonight` charged ozone the molecular airmass**, three times the airmass of its thin shell 20 km up at the horizon, so a low target was dimmed 0.63 mag too much at 0° and 0.34 at 1° on the default air. New `atmosphere.Atmosphere.ExtinctionToward` dims each term of the air through its own airmass, and VisibleTonight uses it at each target's peak. (#631)
- **`remote.SetOffline` made `remote.GetFile` refuse even an object already in the cache**, so the README's air-gapped recipe failed: with DE440s pre-seeded, `eph.NewProvider` worked online and returned "offline mode enabled" offline. In offline mode a cached object is now served, Mutable or not, and only a miss fails with `ErrOffline`. (#637)
- **`plan.FromCatalog` ignored a catalog star's epoch**, so a star whose position was given at another epoch (Gaia DR3's J2016.0, a VizieR table's own) was moved as if from J2000. Barnard's star built from Gaia's row was 166″ from the same star built from SIMBAD's. `FromCatalog` now moves the position to J2000 with the star's own proper motion, parallax and radial velocity first, and the two rows agree within 0.5″. (#639)
- **`plan.MoonSep` measured separation from the Moon's geocentric position**, which the site sees up to 0.95° away. A star the observer saw 30.005° from the Moon passed a 30.5° threshold at 30.757°. `MoonSep`, the `Scorer`'s Moon merit and `VisibleTonight`'s Moon advisory now measure from the site, and agree with Skyfield within 0.005°. (#641)
- **`plan.FromCatalog` read `catalog/jpl`'s NAIF IDs as astrogo body IDs**, which number the bodies differently: the JPL Sun (NAIF 10) became astrogo's body 10, the Moon, and the Moon (301) and Mars (499) failed. A NAIF major body now becomes the planet, Sun or Moon it names, and one astrogo cannot place is `ErrNoCoordinates`. (#643)
- **`plan.NewConstellation` named Eridanus "Fornax" and Serpens "Ophiuchus"**, the constellations their boundary centroids fall in. A target is now named for the constellation asked for. `constellation`'s centroid round-trip test no longer skips Ursa Minor, whose centroid round-trips, and it now checks its exceptions too. (#645)
- **Five functions carried another function's doc comment**, among them `plan`'s `raOfDateDifference`, which opened with `wrap180`'s, and `skybrightness/dataset/starlight`'s `parquetRows.Has`, documented as reading a column as a float. Each doc is back on its own function, and docsguard's `TestNoFuncDocOpensWithAnotherFunc` fails when a function's doc opens with the name of another function in the same file. (#647)
- **No `skybrightness` component applied ozone absorption**, and the scene's ozone column was read by nothing. Starlight, zodiacal and diffuse galactic light, the extragalactic background and airglow now cross the ozone layer along the line of sight, and moonlight along the Moon's beam; artificial skyglow, made below the layer, is unchanged. At Paranal with 258 DU a dark sky's V is 0.022 mag fainter at the zenith and 0.083 mag at 10°. (#649)
- **`skybrightness.ScatteredMoonlight` placed the Moon from the Earth's center**, up to its horizontal parallax, about 0.95°, above where the site sees it: a Moon Skyfield puts at 9.43° was taken at 10.39° and charged too little air, and one just below the site's horizon lit the sky. It now places the Moon, and takes its phase angle, as the site sees it. Observatory's moonlight moves by 0.7 to 1.9 per cent. (#651)
- **`catalog.Resolver` dropped FINK's H, G1, G2, spin and oblateness**, even from a result FINK alone found, so `plan.FromCatalog` had no H to build an asteroid from. The physical-parameter cluster now takes SBDB's V-band photometry first and FINK's whole sHG1G2 fit where SBDB has none, never mixing the two. (#652)
- **Four planetary moons' absolute magnitudes had drifted from the Horizons `V(1,0)` `plan/moons.go` cites**: Enceladus, Tethys, Titan and Hyperion, by up to 0.17 mag, so `VisibleTonight` filtered each as fainter than it is. They are Horizons' values again, and `TestMoonAbsoluteMagnitudesAreHorizons` (network) holds every moon the table sources from Horizons to its live `V(1,0)`. (#653)
- **`catalog/vizier` stamped every 2MASS row J2000**, though each position is where its source was on the night 2MASS observed it, between 1997 and 2001. Each row now carries its own observation date from the table's `JD` column, so `catalog.Resolver` can match a fast star's 2MASS row; and a merged Target's `Epoch` now comes with its `Coord`, from the provider whose position won. (#655)
- **`catalog/jpl` resolved "Eros" to Pluto's moon Kerberos**, "Hebe" to Jupiter's Thebe and "Iris" to OSIRIS-REx, because Horizons searches its major bodies first and matches a name inside another. When no target carries the query as a whole word, `Search` now asks Horizons' small bodies with a trailing semicolon, and keeps that answer when it has one. (#656)
- **The documented sky-brightness figures were not what the tests measure.** CLAUDE.md, the README, the design document and VALIDATION.md quoted a near-full Moon at 18.9 mag/arcsec², the single-scattering figure from before Winkler's multiple-scattering factor; `TestScatteredMoonlightFullMoonSkyBrightness` measures 18.6. CLAUDE.md's full-sky Paranal figure, 21.5, predates integrated starlight; the scene now comes out at 21.3. The VALIDATION row now cites the test that produces its figure. (#658)
- **The SFD dust map could not be downloaded**, and the nightly network tier failed on it from late September. Dataverse now redirects to a pre-signed S3 URL that refuses the HEAD `remote`'s file probe made; a 403 there now falls back to a ranged GET. Behind it, `remote.SFDDustMap`'s and `remote.OpenNGC`'s `ApproxSize` were below their real files, so a grant of `ApproxSize` was refused; both are now measured, and a network test holds every listed file to its endpoint's budget. (#661)
- **The weekly validation tier had timed out in the GAMBONS comparisons every week since 31 August**, so it reported nothing. The `skybrightness` package spent 22.5 minutes asking IRSA, two seconds apart, for the same few hundred dust values every run, because a runner starts with an empty cache. The run now seeds that cache with IRSA's own answers, kept in the repository, and stays within its 15 minutes. `TestSFDMatchesIRSA`, which reads the same cache and had skipped in CI on every run, now runs. (#662)
- **The README said the natural sky was validated to 0.05 mag against GAMBONS**, and `VALIDATION.md` gave three rows a tolerance that nothing asserts: 0.05 mag beside a suite held to 1 mag, 1e-7 deg beside Horizons at 3 arcsec, and 1e-12 d beside round trips at 1e-6 s and 5 s. Each row now states the bound its suites assert, the README states what is measured (up to 0.28 mag per altitude band with airglow included), and `internal/docsguard` holds every row to its cited suite's contract. (#667)
- **`docs/skybrightness.md`'s whole-sky GAMBONS comparison described a model gone since 22 August.** Its table, and the two mechanisms it blamed (unextinguished airglow, no scattered-in light), predated slant extinction for airglow and the `GAMBONSWeb` preset's κ. The all-sky test printed the same account weekly beside numbers contradicting it. The section now carries the current measurement: the airglow-free sky agrees to 0.04 mag, and the residual is the airglow normalization. The test prints only what it computes, and the airglow provenance no longer says scattered-in light is ignored. (#669)
- **`atmosphere.HorizonDip`'s doc misstated the refraction its formula carries.** The Nautical Almanac's 1.76′√h is 8.6 per cent below the geometric dip of 1.926′√h, a refraction coefficient of 0.165; the doc said 0.13 and a reduction of a seventh. The computed dip is unchanged. (#671)
- **`coord.ICRSToEcliptic` and `EclipticToICRS` read the caller's time scale as TT.** A UTC instant was taken 69 seconds early, about 3e-8 degrees; both now convert to TT. The Galactic and ecliptic transforms are also anchored to their defining constants, the Hipparcos frame angles to 1e-9 degrees and the IAU 2006 obliquity to 0.05″, where the anchors had been held to 0.01 degrees. (#676)
- **`VALIDATION.md` ticked CAMS aerosol optical depth as validated, but every test behind it skips.** The orientation test needs an `s3://` backend, which astrogo has not had since gocloud.dev's removal, and the ground-truth tests need licensed files absent from CI. The row now says it is not run and dates its figures, and the skip message no longer says the backend is being rebuilt. (#683)
- **Rise, set and transit are now held to 1 minute of USNO**, for the Sun and the Moon at every site from the poles to Everest: USNO's half-minute rounding plus half a minute. Rise and set were held to 2 minutes for the Sun and 3 for the Moon, 5 near the poles, loose enough that the Moon rising on its center instead of its upper limb passed; the worst measured is 0.65 min. A USNO event astrogo misses, or one it reports that USNO does not list, now fails. Three `VALIDATION.md` rows that gave a measurement as their tolerance, the two rise and set rows and ΔT, now state the asserted bound. (#684)
- **The USNO celestial-navigation tests compared refracted with airless altitudes and counted the Sun's aberration twice**, inside tolerances of 0.1° to 1.5°, so the "0.002°" quoted for Sirius was its refraction at 83°. `TestUSNO_CelNav` is now an offline fixture asking astrogo USNO's own question: airless and geocentric, at UT1. The Sun and Sirius agree within 0.67″ at five places, held to 2″. The library was right; only the test was not. (#686)
- **The nightly network tier failed on `TestGenerateCorpus` from 2026-10-09**: JPL Horizons' answers moved by one printed digit in 76 of the corpus's 300 entries (ranges by up to 1.5 cm, five angles by 1e-6°). The corpus takes the new values; nothing astrogo computes changed. (#687)
- **The README said every magnitude model was "validated 100% within 0.025 mag against the FINK/ZTF production pipeline"**, which FINK/ZTF validates for sHG1G2 alone. The capability table now names each model's reference, planets and comets to JPL Horizons, and points to `VALIDATION.md`. (#688)
- **The missing-EOP warning said topocentric accuracy drops to ~1 arcsec**, beside a UT1 error of 0.9 s that is 13.5 arcsec of Earth rotation (8.8 measured on the Horizons corpus). It now states that bound. The README's EOP table had the same understatement, and claimed <0.01 arcsec with EOP where astrogo measures 0.4 (p50) and 2.1 (max) against Horizons. (#691)
- **Three README figures predated the work that changed them**: seasons "2–4 min vs USNO" (now within 0.74 min since #414), TDB−TT "single-term … ±3 µs" (37 terms, within 0.79 µs over 1900–2100 since #423), and horizon refraction "within 13″" (13.4″ at worst). Each now matches `VALIDATION.md` and its test. (#692)
- **`Time.DecimalYear` ran backward within a month**: it added the day's fraction as if it were the month's and dropped the day of the month, so 2026-03-01 23:59 read later in the year than 2026-03-31. It now gives the fraction of the year elapsed, and ΔT before 1960 is no longer read up to half a month off (#697).
- **The FINK residual tests called FINK's ephemeris outages wrong data and failed**: when FINK's ephemeris service fails inside a request, FINK returns every residual null, or a 400 naming Miriade, and the control query could not see either. They now skip, naming the condition; an absent residual column still fails (#700).
- **Moonrise and moonset now put the Moon's upper limb on the horizon at its semi-diameter at that instant**, not a fixed mean of 15.5′. The Moon's runs from 14.7′ to 16.8′, which put events up to 12 s off at London and 28 s at 60°N, and at high latitude reported a set and rise the limb never made: at Alta on 2026-06-18, a twelve-minute set that does not happen. (#703)
- **`simbad.Provider.Search` answered HTTP 400 on every call** since v0.19.0: its query ordered by a qualified column, which SIMBAD's TAP parser rejects. It now orders by `main_id`, and a live test runs `Search`, which none did. (#706)

### Security
- **SIMBAD and VizieR are now addressed over HTTPS.** Both were registered as
cleartext `http://`, shipping catalogue queries and the positions they return
where anything on the path could read or rewrite them. Verified against the
live services first: both answer HTTPS byte-identically to HTTP. `www.cvrl.org`
stays HTTP because it runs no TLS listener at all — port 443 refuses the
connection — and now says so in a new `Endpoint.InsecureReason`, which
`TestNoUndeclaredCleartextEndpoints` requires of any cleartext URL in the
registry (#107).
- **Dependency security updates.** Update Apache Thrift, gRPC, and YAML v2 to the versions selected by Dependabot, and synchronize dependency checksums in both Go modules. (#280)
- **SFTP dependency security update.** Upgrade `golang.org/x/crypto` to v0.56.0,
fixing SSH channel deadlock denial-of-service vulnerabilities GO-2026-6354 and
GO-2026-6355 reachable through the SFTP blob driver. (#293)
- **`golang.org/x/net` v0.60.0, for five HTTP/2 advisories published 2026-10-08**: GO-2026-6617, -6612, -6611, -6610 and -6603 (CVE-2026-97032, -78663, -78669, -78660, -78659), covering an HPACK encoder race, flow-control and window-update abuse, malformed framing headers and Trailer memory exhaustion. `govulncheck` found them reachable from astrogo's HTTP clients. (#682)

## [0.19.0] — 2026-09-02

### Added
- **CI now requires a changelog fragment naming the pull request.** Both `CONTRIBUTING.md` and `docs/PULL_REQUESTS.md` asked for one and nothing checked, so #80 and #81 merged without and would have been missing from their release. Label a pull request `no-changelog` when it genuinely warrants no entry. (#82)
- **Radial velocity now works for every target kind.** `plan.RadialVelocity` returns it for a solar-system body, computed from the ephemeris and agreeing with JPL Horizons to under 6 m/s, and `*DeepSkyObject` carries the catalog value SIMBAD publishes for galaxies. Adds `coord.Context.TopocentricRadialVelocity` and `constants.WGS84.AngularVelocity`. (#90)
- **Radial velocity now carries the observer's own clock.** `coord.Context.ObserverFrameShift` adds the second-order Doppler shift, the Sun's gravitational potential at the observer and Earth's own — about 4.65 m/s. Against Astropy over 175 cases the disagreement falls from **4.66 m/s to 0.5 mm/s**. `BarycentricRadialVelocity` and `ObservedRadialVelocity` now return an error. (#98)

### Changed — BREAKING
- **`plan.FromCatalog` returns `(Observable, error)`** and refuses a fixed target with no position instead of placing it at RA 0, Dec 0 — a real point in Pisces that rises, sets and schedules without complaint. Returns `plan.ErrNoCoordinates`. (#87)
- **`coord.Context.ObservedRadialVelocity` returns `(float64, error)`.** It now applies the observer's frame shift, which needs `Epv00` and so can fail — the same reason `HeliocentricRVCorrection` has always returned one. (#98)

### Changed
- **The `time` package's own tests no longer alias it.** Five external test files carried `atime`, `astrotime` and `gotime` — the package that exists to end the two-spellings-of-time problem was the last place still having it. The guard now applies its alias rule inside `time/` too. (#79)
- **The six known scientific limitations are gathered into one checklist** in `docs/ROADMAP.md`, with what each blocks and what would unblock it. They were spread across `VALIDATION.md`, `skybrightness.md` §16 and a roadmap checkbox, so answering "what is still open" meant reading three documents. (#80)

### Removed
- **`assets/old.png`**, the 8.2 MB mascot the hero banner replaced. Nothing referenced it; it remains in git history if it is ever wanted. (#94)

### Fixed
- **CI never compiled the build-tagged test files.** 59 of them — network, validation and integration — were built only by the weekly Validation workflow, so a tagged file that did not compile reached `main` green, and one did. `go vet` with all three tags now runs on every pull request. (#79)
- **Radial velocities composed by adding a correction instead of multiplying redshifts.** `coord.Context.BarycentricRadialVelocity` is the correct conversion and `ObservedRadialVelocity` is now its exact inverse; the dropped `rv*corr/c` term reached 4.66 m/s at a target velocity of 46.6 km/s and 30 m/s for a halo star. (#81)
- **Ten doc comments and documents named tests that do not exist**, including `scatterKernel`'s claim that a named test held its rearrangement to the functions it rearranges — there was no test of the kernel at all. That test and `TestCharonNeedsTheSystemParameter` are written here, the rest repointed, and `internal/docsguard` now fails on a citation that resolves to nothing. (#83)
- **Roadmap item 39 said "Not Started" with all five boxes ticked** — it shipped in #74. `internal/docsguard` now checks every item's status against its own checkboxes, in both directions; a box beginning "Optional" is excluded so a deliberately untaken extension does not force an item out of Done. (#84)
- **SIMBAD name resolution returned the wrong object for most deep-sky names.** `Resolve("M87")` gave a source 70° away in Cassiopeia, `Resolve("M31")` a nova inside the galaxy. The substring `LIKE '%name%'` is now an exact match against the spellings SIMBAD stores, and an unknown name returns not-found rather than the least-wrong of ten. (#88)
- **`Resolve("ISS")` returned the wrong satellite.** CelesTrak's NAME query is a substring match, so it gave UME (ISS) — NORAD 8709 — instead of the station, and a bare catalog number like `25544` found nothing because it was sent as a name. Numbers now use CATNR, and name matches are ranked. (#89)
- **CI has been failing to start since #82.** The changelog-fragment job's shell carried a literal newline inside a `printf` format string, which ended the YAML block scalar early and made the whole workflow unparseable — so every run since that merge failed before any job began. (#91)
- **The README banner's alt text still described the old mascot.** It now describes the scene the hero image actually shows, which is what a screen reader or a reader with images disabled gets in its place. (#92)
- **`coord.Context.TopocentricRadialVelocity` shipped with no offline test.** Its only cover was a `network`-tagged comparison against JPL Horizons, invisible to an ordinary coverage run — it was at 0%, and it is at 100% now, checked against the geometry rather than a service. (#93)
- **A slow NAIF turned into a red build.** `ephemeris/jpl`'s untagged tests fetch a 32 MB kernel and treated a download timeout as a test failure — four of them sat at 120 s each and took the package past its limit. An upstream failure now skips, as this repository's policy says it must. (#95)
- **Documents named functions that no longer exist.** `internal/docsguard` now checks every backticked, package-qualified symbol in every document against the code — 174 of them — and `declaredIn` learned to see entries inside grouped `const`/`var`/`type` blocks, which it had been blind to. (#96)
- **The documentation guards matched symbols with a regular expression**, which could not tell a method from a same-named function, nor `plan` from `skybrightness/plan`. They now read the module with Go's own parser, and both guards share one index. Two more stale citations found and fixed. (#97)

## [0.18.0] — 2026-08-30

### Added
- **`internal/docsguard` now checks roadmap checkboxes against the code**, in both directions: an unchecked box whose symbol is already declared, and a tick whose symbol has been deleted, both fail the build. It found `resolve.Target.HasRadialVelocity` sitting unchecked while being declared, populated by `catalog/simbad`, preserved through the multi-provider merge and consumed by `plan` — finished work left looking open, which sends the next contributor to build it twice. (#70)
- **`constants.DE440` — the JPL ephemeris mass parameters**, the first of the five pieces roadmap item 39 needs for central-body propagation of planetary moons. IAU 2015 B3 publishes nominal mass parameters for only the Sun, Earth and Jupiter, so anything needing Mars or Uranus had nowhere to look. Each planet appears twice, system and body: using one where the other belongs is a silent error the size of the satellite system, which for Pluto and Charon is 12%. Every value is checked against NAIF's own kernel by a network test rather than trusted to transcription. Adds `remote.NAIFPCK`. (#71)
- **Central-body two-body propagation in `ephemeris/kepler`.** `Elements.WithCentralBody` refers an element set to a planet instead of the Sun, `CentralBodyFor` supplies the parent's *body* mass parameter so the 12%-wrong system value cannot be picked by mistake, and the provider composes a satellite through its parent. Verified against Kepler's third law — 421,800 km about Jupiter gives 1.7699 days, Io's sidereal period, from a distance and a mass parameter alone. This is the machinery and the constants, not a satellite ephemeris: Laplace-plane frames and J₂ precession are still open. (#72)
- **`kepler.LaplacePlane` — the reference plane published satellite mean elements actually use.** `Elements.WithLaplacePlane` reads the angles against the tabulated pole instead of the J2000 ecliptic; without it Io lands 16,500 km from where it belongs — 4% of its orbital radius, 5 arcsec from Earth against a 1.2 arcsec disc — with the period still correct. Measured against Horizons, the remaining two-body drift is at most 5,900 km over ten days, and it is concentrated in Io and Europa: their periods are off by 0.41% and 0.76% while Ganymede's and Callisto's agree to one part in 100,000. (#73)
- **Planetary-satellite propagation is complete.** `kepler.PlutoElements` lets the default base answer Pluto, so a Charon orbit can be placed at all — before, it failed at the parent rather than the satellite. `Elements.WithPeriod` and `kepler.SecularPrecession` apply the published anomalistic period and apsidal precession as a pair, recovering the sidereal period (Io: 1.769137 d, from the table's own columns) and cutting the six-month error against Horizons from 125,000 km to 74,000. Applied singly, either is an order of magnitude worse than neither; the tabulated *node* rate is measured and deliberately not applied. (#74)
- **Measured accuracy for every offline ephemeris.** New metrology suites put SOFA's seven planets and all three Kepler paths — Pluto, small bodies and the Galilean satellites — against DE440 and Horizons-generated SPK kernels, and the Sun and Moon rows grow from 3 samples to 516. (#78)

### Changed
- **`astrogo/time` is now the module's only import of the standard library's `time`.** A guard test enforces it, and `time` gained `GoTime`, `GoDate`, `LocationUTC`, `Now`, the sub-second units and the timer constructors so no call site has to reach past it. (#75)
- **Benchmarks use `testing.B.Loop` instead of `b.N`.** 19 loops across 15 files, and the 20 now-dead `b.ResetTimer()` calls that preceded them — `b.Loop` resets the timer on its first call. (#76)
- **`ephemeris.Default()` now documents what it is worth, per body.** The package doc and README carry the measured per-body accuracy against DE440 beside each body's apparent diameter, and say plainly that it is a planning-grade provider — with Pluto called out as roughly thirty times worse than the worst planet. (#78)

### Fixed
- **Corrects a documented claim that was backwards.** #71 and #72 said the *system* mass parameter was "wrong by 12% at Pluto" for a satellite orbit. Two-body relative motion is governed by G(M_primary + M_satellite), and Charon is 12% of Pluto — so the system value is the right one there. Charon's published 6.3872-day period comes out at 6.3871 with it and 6.7648 with Pluto's own. (#74)
- **A validation test was passing because of a bug, and started failing when the bug was fixed.** It rendered a TDB epoch with `Time.Format`, which since #50 correctly produces the *UTC* calendar string, then told Horizons to read it as TDB — a 69.18-second shift, applied to both the elements and the vectors compared against them. Eros's divergence read 4.1 arcsec against a 2.0 tolerance; sending the epoch as a Julian Date restores the 0.04-to-0.56 arcsec the test's own comment describes. (#74)
- **`catalog/sbdb` returned orbital elements rounded to three significant figures.** SBDB rounds unless asked not to, so Eros resolved with a = 1.46 rather than 1.458243716; two-body propagation from those elements was 690,000 km out at their own epoch of osculation, where they are exact by construction. (#78)

## [0.17.0] — 2026-08-29

### Added
- **`time.EOPLoader`, `time.RegisterEOPLoader` and `time.FileEOPLoader`** — the seam above, plus a standard-library-only loader for a deployment that pre-seeds `finals2000A.data` and wants no cloud-storage dependency (#58).
- **Small-body SPK evaluation is now validated against Horizons.** The only assertion on a small-body position was `0.1 AU < |r| < 5.0 AU` — a bound spanning two orders of magnitude, guarding the hand-rolled SPK **Type 21** decoder where a defect once corrupted positions. Astrogo agrees with Horizons to **33 mm** across four bodies from 1 to 5 AU and eccentricity 0.08 to 0.89. (#60)
- **`docs/VALIDATION.md`'s status table cites its evidence, and `internal/docsguard` checks the citations resolve.** 52 of its 53 rows said "validated" — undated, with no link to what establishes them — in the same document as generated rows carrying a contract, a measured distribution and a commit stamp. Each row now names a test file or a generated suite; a renamed file, a suite that stops being produced, or a measured suite no row points at fails the build. That last check immediately found small-body and SOFA-analytical ephemerides being measured with no row at all. (#64)
- **Tests for the `ephemeris` front door**, which had none: every constructor and every option — `NewProvider`, `NewFromElements`, `NewElements`, `NewMovingBodyProvider`, `WithKernel`, `WithTLE`, `WithTimeInterval`, `WithKeplerBase` — sat at 0% while the arithmetic beneath them was well covered. The package goes from 31.3% to 81.2%, with no exported function left uncovered. (#65)
- **`docs/PULL_REQUESTS.md`**, written from the failures this repository actually had rather than generic advice: why the linter must be run twice, why reachability is not health, why a tolerance is derived from the smallest fault worth catching, and the two git habits that stop a branch looking like it deleted work it never touched. Adds a pull request template, and moves `changelog.d/` under `docs/` to sit with the rest of the written record. (#66)

### Changed — BREAKING
- **`Provider.SupportedBodies` reports small bodies as `core.SmallBodyID(n)`**, not the bare number: `SmallBodyID(433)` rather than `core.ID(433)`. A bare `core.ID(433)` still resolves in `State`, so only code that matches against the enumerated list needs updating. Adds `core.SmallBodyID`, `core.SmallBodyBase` and `ID.SmallBodyNumber`. (#61)

### Changed
- **`time` no longer imports `remote`**, so importing it to compute a Julian date no longer links gocloud, gRPC and protobuf: a binary doing exactly that goes from **19.4 MB to 2.5 MB**, and `time`'s external dependency count from 163 packages to 5. EOP bytes now arrive through a registered `time.EOPLoader`, which importing `astrogo/remote` installs automatically — and since download consent can only be granted through `remote.EnableDownloads`, every program that could fetch EOP data already imports it, so nothing changes for existing callers (#58).
- **Changelog entries now live one-per-file in `changelog.d/`**, assembled into `CHANGELOG.md` at release time. `CHANGELOG.md` was the only file five of eight pull requests conflicted on in a single batch — never the code — and resolving one textually can silently re-file an entry under the wrong heading, since merge markers carry no section information. (#59)
- **The Horizons query helpers take the COMMAND payload as a string** rather than a NAIF ID as an int, because a small body needs Horizons' designation syntax (`433;`) and an int cannot express it. `StateVector.NaifID`, which was written and never read, becomes `StateVector.Command` and records what was actually queried. (#60)

### Fixed
- **CI now runs on every pull request, not only those targeting `main`.** A stacked pull request matched no branch filter and so ran no checks at all, which reads as "nothing to report" rather than "nothing was run". (#59)
- **Ceres, Pallas, Juno and Vesta could not be loaded at all.** A small body was identified by its bare number, so asteroid 4 Vesta and Mars were both `core.ID(4)` — as were Ceres and Mercury, Pallas and Venus, and every asteroid numbered up to 12. A provider holding a planetary kernel resolved the planet and dropped the asteroid as a duplicate, and `ErrNoSmallBodyKernel` then blamed the designation, which was never the cause. Small bodies now keep NAIF's own `20000000+` identifier via `core.SmallBodyID`. (#61)
- **A Horizons refusal arrived as success.** Asked to generate an SPK for 101955 Bennu, Horizons answers "SPK creation is not available for pre-computed objects in the major body index" — a complete explanation, which `spk.CacheAPI` never parsed. The caller got an empty kernel list and a nil error, then a misleading complaint about designation syntax that was never wrong. The service's own sentence now reaches the caller as `spk.ErrHorizonsRefused`. (#62)
- **A bare number could load a different object entirely.** `NewProvider(ctx, core.SmallBody, "1")` returned comet 1000036, not 1 Ceres, because a bare number resolves against Horizons' major-body and comet indices before the numbered-asteroid record — and every mechanical check passed, so nothing said so. A numbered asteroid must now arrive as `core.SmallBodyID(n)` or the load fails with `jpl.ErrWrongSmallBody`. (#62)
- **The Horizons corpus sampled epochs whose reference values were still moving.** Its regular span ran to 2027-01-01, so the last epoch depended on *predicted* Earth orientation and shifted by up to 8.4e-05 degrees after generation — `TestGenerateCorpus` then reported a dirty diff on every run, for a corpus nobody had touched. The span now ends well inside the settled past, and `TestCorpusEpochsAreSettled` enforces it. 255 entries become 300, all with final Earth orientation. (#63)
- **Assembled changelog entries carried no link to the change behind them.** The fragment's `pr` field was read for ordering and then dropped, so v0.17.0 shipped eleven entries with no citation beside two hand-written ones that had theirs. A changelog is an index, and an index entry with no pointer is decoration. The v0.17.0 section is repaired from the fragments themselves rather than by hand. (#68)

## [0.16.0] — 2026-08-29

### Added
- **`time.scale.roundtrip.arithmetic` and `.modelled` convert every scale to every other and back, across twelve epochs chosen at the seams** — both sides of the 2017 and 2015 leap seconds, the 1972 UTC epoch, the pre-1972 rubber-rate era, the start of SOFA's ΔAT table, J2000, 1900, 1600, AD 33, the present, and past the end of measured Earth-orientation data. All four defects above would have failed it. Split into two suites because averaging them describes neither: the arithmetic scales round-trip to **9.6 picoseconds**, while UT1 and pre-1972 UTC are bounded by tabulated data at **0.94 s** (#50).
- **Radial-velocity correction is cross-checked against Astropy**, closing the last ⚠️ row in `docs/VALIDATION.md`. 175 cases — 5 named sites × 5 epochs × 7 target directions — agree to **0.7 mm/s** against a 1 m/s bound. The reference table is generated by a locked `uv` project in `coord/testdata/rvfixture/` and checked in as JSON, so ordinary `go test` needs no Python, no network and no `uv`; it is the only Python in the repository, added because this is the one reference astrogo validates against with no Go client. The comparison is **like for like**: Astropy's barycentric value is relativistic while astrogo is classical and documents that, so the test compares against the plain projection of Astropy's own observer velocity. The 4.66 m/s gap between the two is asserted rather than ignored, against 4.649 predicted from the terms astrogo names as omitted (second-order Doppler 1.481 + solar redshift 2.959 + Earth redshift 0.209). Recorded as a consistency check, not independent validation: both sides reach the observer's barycentric velocity through SOFA's `epv00` (#49).
- **`ephemeris.State` now says which frame and origin its vectors are in.** The contract was a comment reading `// Geocentric position in AU (ICRS-like)` — hedged, on a struct several providers fill in differently: `jpl`, `kepler` and the SOFA analytical provider produce ICRS, while `ephemeris/satellite` converts SGP4's TEME output to **GCRS** and produces that. Those differ by frame bias, about 23 mas, and nothing in the type distinguished them, so a value from one used where the other was meant stayed mathematically valid and became physically wrong. `State.Frame` and `State.Center` are one byte each, read at boundaries rather than in loops; units stay contractual because putting those in the type would cost the hot path more than it is worth. Both zero values are `Unspecified` and assert nothing, so an unlabelled state says so rather than claiming ICRS geocentric, and `State.Require(frame, center)` checks against a *wrong* label rather than demanding every producer carry one — which would break third-party providers for a change of astrogo's own (#53).
- **Read-only accessors for the four exported registries**, so enumerating or looking one up no longer requires reaching into process-wide mutable state: `plan.KnownSiteNames`, `plan.MeteorShowerNames`, `plan.TwilightThreshold`, and `jpl.NAIFFor`/`jpl.NAIFBodies`. The concern is not races but reproducibility and test isolation — one package deleting an entry, or redefining what "astronomical twilight" means, changes what every other caller in the binary sees, silently (#54).

### Fixed
- **`Time.ToGo` reinterpreted any scale as UTC, turning a representation difference into an instant difference.** It subtracted the Unix epoch from `jd1`/`jd2` directly, so one instant produced different `time.Time` values depending on which scale the caller happened to hold — measured, `utc.ToGo()` and `utc.TT().ToGo()` were **69.184 s** apart. Since `ToGo` backs `Time.String`, `Time.Format` and every formatted event time in `plan`, a caller who had converted a scale anywhere upstream got a wrong clock reading with nothing to indicate it. It now converts to UTC first (#50).
- **`Time.ApplyDeltaT` returned `TDB` where its own documentation said `TT`** — three times, including "the only reliable UT → TT bridge". A flat contradiction between contract and implementation, now returning TT (#50).
- **Historical UTC ↔ TT was not invertible.** `TT()` used the ΔT polynomial before 1972 while `UTC()` went through ΔAT with no such gate, so a 1600 round trip came back **85.7 s** late. The reverse direction now mirrors the same epoch gate, and iterates ΔT once so it is evaluated at the UT year both ways — which also removed a 0.097 s residual at AD 33 (#50).
- **`TAI ↔ TDB` routed through UTC**, dragging ΔAT and, before 1972, ΔT into a conversion that is pure arithmetic. `TAI→TDB→TAI` came back **10,170 s** — the whole of ΔT — late in AD 33. It now goes via TT (#50).
- **`testutil.Reachable` reported live services as unreachable**, silently skipping every suite that depended on them. Go's dual-stack dialer spent the whole probe budget on an AAAA lookup for a host with no IPv6 record: `irsa.ipac.caltech.edu` timed out on `tcp` after five seconds while `tcp4` reached the same address in 250 ms. Every IRSA-dependent suite — the SFD dust map and the three GAMBONS comparisons — had been skipping on a service that was answering, and a skip reads as a pass. Now falls back to IPv4 (#50).
- **A timing-ratio test asserted the star-map cache was used by requiring the cached call to be 2× faster than the first.** The first call is only slow when the on-disk cache is cold, so once any earlier run had warmed it both came back in ~3 ms and the test failed on a cache working correctly. Replaced with an absolute ceiling: a real Gaia query cannot complete in under 100 ms (#50).
- **The README advertised `v0.5.0` at v0.15.1** — ten minor releases stale — and went on summarising that release's contents as though current; "Known Limitations" was pinned to v0.5.0 too, as was `docs/ROADMAP.md`. The cost is not the wrong number but that a reader who finds one stale claim cannot tell which of the accuracy figures beside it is also stale. All three now name no version and point at the tags and CHANGELOG instead, and `internal/docsguard` fails the build if a current-version claim reappears — historical statements like "shipped in v0.1.0" are deliberately not matched (#52).
- **Examples and README snippets discarded 45 and 28 errors respectively**, teaching `site, _ := plan.NewSiteEarthLocation(...)` for a library whose errors mean missing EOP, an out-of-range ephemeris, an unreachable service or a solver that did not converge. All now handled: `log.Fatalf` at the top level of a program, `continue` inside a loop over many objects where skipping one is the behaviour the snippet illustrates. Two remaining `_` are not errors at all — `Calendar()` and `JulianCalendar()` return a day fraction — and are left alone (#52).
- **A small-body provider that loaded nothing was indistinguishable from one that worked.** `spk.CacheAPI` returns an empty slice rather than an error when Horizons matches no small body, so `jpl.NewProvider(ctx, core.SmallBody, …)` returned a provider with a **nil error** for `"Ceres"`, for `"101955"` and for an invented string alike; `SupportedBodies` then listed the eleven bodies of the planetary base kernel, which is exactly what a working provider looks like from outside. The only symptom was `ErrNoSegment` much later, far from the mistake. It now fails with `ErrNoSmallBodyKernel`, naming the designation and the syntax Horizons wants. The check is on whether a *body* was added rather than whether a file arrived, because `"1;"` and `"4;"` both return a non-empty kernel carrying no small-body segment (#51).
- **A known site's own name could not be passed back to `NewKnownSite`.** The lookup matched the map key and the aliases but never the site's display name, which works for every site whose name normalises to its key and fails for the one that does not: "Cerro Pachón" normalises to `cerro_pachón` against a key of `cerro_pachon`, so the name astrogo prints was a name astrogo could not resolve. Found by `KnownSiteNames`, which made the round trip testable for the first time (#54).
- **An outage at an external service failed the build instead of being recorded as not verified.** A reachability probe answers whether a socket opens, not whether the service works, so a CelesTrak 500 — and later an AstroPixels connection reset — failed pull requests that had not touched the network at all. `testutil.SkipOnUpstreamFailure` classifies the error instead: 5xx, 429, 408, timeouts and dropped connections are the upstream's problem, while a 400 or a 404 still fails because it means astrogo built a bad request (#56, #57).

### Changed — BREAKING
- **`Solver.FindRoot` and `Solver.FindExtremum` now return `ErrNoConvergence` when they exhaust `MaxIter` without reaching `Tolerance`**, instead of returning the best estimate with a nil error. The estimate still comes back alongside the error, so a caller content with an approximation can check the sentinel and keep it — what is no longer possible is mistaking one for the other. For an interactive display best-effort was defensible; for anything that points a telescope it was not. All four internal call sites converge with the default 64 iterations, so no behaviour changes in practice (#50).

### Deprecated
- **`plan.KnownSites`, `plan.MeteorShowers`, `plan.TwilightThresholds` and `jpl.BodyIDToNAIF`** in favour of the accessors above. They remain for the two minor releases this repository's own policy gives a deprecated symbol (#54).

## [0.15.1] — 2026-08-28

### Fixed
- **`skybrightness/dataset/starlight`'s synchronous Gaia aggregation was broken against its own default endpoint.** It requested `FORMAT=csv` and parsed the reply as CSV, but the default is Gaia@AIP, which answers VOTable for `FORMAT=csv`, `RESPONSEFORMAT=csv` and `text/csv` alike — so every synchronous build failed with `parse error on line 1, column 15: bare " in non-quoted-field`, which is byte 15 of an XML declaration and names neither the format nor the service. The payload is now sniffed rather than assumed, as `catalog/gaia` already does, through the `internal/votable` reader that existed for this and had never been adopted here. A truncated result (`QUERY_STATUS=OVERFLOW`) and an empty body are now refused rather than accumulated: the query is a `GROUP BY`, so a server-side row limit returns pixels whose sums are short by an unknown amount rather than fewer pixels. Five tests were failing on this, none of which runs in ordinary CI (#48).
- **`ephemeris/jpl/lsk` dropped the last leap second, putting every UTC epoch after 2017-01-01 one second early.** `naif0012.tls` closes its `DELTET/DELTA_AT` block on the same line as its final entry, and the parser cleared its in-block flag before testing it, so `37, @2017-JAN-1` matched neither arm of the guard and was discarded without an error. Measured: `lsk.UTCToTDB` ran 1.0000 s behind `time.TDB()` for every date after the boundary and agreed to 1e-4 s before it, putting the geocentric Sun about 30 km — one second of Earth's orbital motion — from DE440. Every `jpl.Provider.State` result for a UTC-scale time in that range moves; the regression test uses inline fixtures for both terminator shapes, so it needs no kernel and guards the next leap second too (#47).

### Changed
- **The metrology renderer no longer acts on a normal test run, and its own fixtures no longer pollute a real collection.** `TestRenderAccuracyReport` failed whenever the rendered table differed from the file, which cannot distinguish a stale document from a run that collected only some suites — and `go test -tags=validation ./...` with the output directory set is both at once, since the renderer runs inside the invocation still producing what it reads. It now acts only when passed `-update-accuracy`. Separately, the package's own test fixtures inherited the ambient output directory and wrote four spurious rows (`test.suite`, `x`, `ephemeris.example`, `ephemeris.passing`) into the published table (#48).
- **Two flaky tests in `remote` are made deterministic**, each failing about one run in fifteen — measured at 2 failures in 180 runs before and 0 in 240 after. `TestGetFileMutableHeadProbeChanged` wrote two same-length versions, and since `fileblob` derives an ETag from `(ModTime, Size)` two writes either side of a local file copy can share a Windows clock tick, making the edit invisible in the source's own metadata; `TestGetFileWithDownloadTimeoutOverridesEndpointDefault` raced a 1 ns timer against a local file read, where a non-positive duration is cancelled immediately by `context.WithDeadline` instead (#48).
- **The `Validation` workflow runs weekly instead of on manual dispatch only, and no longer tracks a hardcoded Go version.** Its three suites run under `if: always()`, so a failing one cannot hide the others, and results upload as artifacts. `validation` and `network` had been executing only when somebody remembered (#47).
- **The DE440-against-SOFA tolerances are derived from the reference routines rather than from astrogo's own last measurement.** Both were wrong, in opposite directions: 1e-6 AU for the Sun is thirteen times `Epv00`'s published worst case, while 1e-7 AU for the Moon was less than half `Moon98`'s 31.7 km, demanding closer agreement than SOFA documents its own routine to achieve. The suite now reports measured p-values beside its contract, and its epochs are fixed — one had been `time.NowUTC()`, so any failure was unreproducible (#47).
- **`ephemeris/jpl/validation`'s Horizons corpus grows from 3 entries to 255**, across five named sites and three sampling classes (regular, leap-second and J2000 boundaries, and the polar and equatorial geometries `plan.KnownSites` cannot supply), with a manifest recording the query shape, the astrogo commit and the sampling design. `TestGenerateCorpus` now reports what a regeneration would change and writes nothing without `-update-corpus`; it previously overwrote the reference data unconditionally (#47).

### Added
- **`docs/VALIDATION.md`'s hand-written status table is corrected where this work disproved it, and now discloses shared ancestry.** The ephemeris row published a 1e-7 AU tolerance as though it were the achieved accuracy — measured, the agreement is 5.5e-14 AU, about two million times tighter. The apparent/observed row documented its 3″ bound as chosen from a 2.66″ measurement; the value is unchanged and its derivation is not. The time-scale row did not cover `ephemeris/jpl/lsk`, which is where the leap-second bug lived, and the starlight row asserted a validated map while all five of its tests were failing. Five rows cite `gofa` as their reference while astrogo computes through gofa, which is now stated rather than left to be inferred (#48).
- **`docs/VALIDATION.md` records that radial-velocity correction has no external reference.** `BarycentricRVCorrection`/`HeliocentricRVCorrection` are public API validated by invariants only — the annual sinusoid measures 59.86 km/s peak-to-peak against twice Earth's mean orbital speed of 59.56 — while the Astropy cross-check in `coord/radialvelocity_fixture_test.go` has never run for want of a fixture table. A table of ticks with no row for a public function reads as coverage rather than as a gap (#48).
- **`docs/VALIDATION.md` carries a generated accuracy table** — distribution, contract, status and the date and commit each row was verified at — rewritten from the suites' own machine-readable results. The surrounding prose stays hand-written. A suite that could not run renders as `NOT VERIFIED` rather than vanishing, since an absent row reads exactly like one that never existed (#47).
- **Four more suites report through `internal/metrology`**, taking the generated table from five rows to nine: the DE440-against-SOFA comparison for the Sun and Moon, and the Horizons position and velocity comparison. All four are marked as consistency checks rather than independent validation — two share SOFA ancestry with astrogo, two share the JPL integration — which is the disclosure the Independence column exists for (#48).
- **`internal/metrology`** separates the accuracy *contract* from the *measured* accuracy: a bound that carries the reason it has its value, and a distribution recorded beside it. `NewContract` refuses a bound with no rationale or source, `Reference.SharedAncestor` marks a comparison that shares ancestry with astrogo rather than being independent of it, and `Baseline` flags accuracy regressions that stay inside contract. The topocentric and GAMBONS suites are retrofitted onto it, reproducing their previous numbers (#47).

## [0.15.0] — 2026-08-28

### Added
- **`skybrightness` reaches an observing plan again, and `plan` imports none of it.** `plan.SkyDepth` is a one-method interface `plan` declares for itself, and `plan.LimitingMagnitudeConstraint` returns on top of it — soft by default (a logistic ramp over the margin through `ScoreMultiplier`, since half a magnitude of moonlight makes a target harder rather than impossible), `Boolean` for a hard cutoff, `ConstraintCtx` implemented so the scheduler keeps its shared-`coord.Context` fast path. The previous version imported the sky engine directly and needed a bespoke import test to stop that spreading; there is nothing to police now, because there is no import. The new `skybrightness/plan` bridge supplies an implementation, mirroring `fits/plan`.
- **`optics` gains the detection half of a limiting magnitude**: `Instrument.SNR` and `Instrument.LimitingSignal`, the CCD equation of Merline & Howell (1995) and its closed-form inverse, over new `Instrument.ReadNoiseElectrons` and `DarkCurrentEPerSec` and the new `unit.ElectronsPerSecond`. That unit is separate from `ElectronsPerPixelPerSecond` deliberately: a point source's rate is a total and a sky background's is a density, and the SNR needs both at once. `LimitingSignal` returns a rate rather than a magnitude, because a zero point belongs to `magnitude` and `optics` has no business holding a photometric system.
- **The magnitude comes out without a zero point at all**, because the sky supplies one: the same estimate yields a surface brightness and a detector background through the same instrument and the same stored spectrum, so `m = SB - 2.5log(Ω) + 2.5log(B/S)`. A second copy of a zero point, free to disagree with the one the estimate used, would be worse than none. Two caveats are documented rather than buried: the answer assumes the source has the sky's colour, and an instrument declaring no filter in its `Throughput` gets broadband electrons on a named magnitude scale — measured at Paranal, such a setup lost only **0.09 mag** of depth between the zenith and ten degrees where the V-band sky brightened by **0.69**.
- Visual limiting magnitude is **not** included. It needs a human contrast model (Crumey 2014), and the tempting shortcut is the one to refuse: Schaefer's (1990) SQM→NELM conversion consumes a single V-band scalar, so routing a spectrum through it discards the spectrum in the first step. Recorded in `docs/skybrightness.md` §16.
- `cams.RegistrationAdvice` is appended to a failed Copernicus fetch, naming the free self-service registration at https://dataspace.copernicus.eu and the four steps after it. Credentials are issued per user and cannot be supplied on a caller's behalf — sharing them breaches the terms they were issued under, and anything done with a shared key traces back to whoever registered it — so the URL is the whole of what this package can usefully offer, and it is pinned by a test rather than left to drift out of an error nobody reads until they need it.
- **`dataset.LiveAerosol` builds an aerosol description from the optical depth actually over a site**, fetching it through `cams.AOD550` and handing it to an OPAC constructor. Deliberately opt-in rather than default: it downloads, needs consent, and reaches Copernicus over S3 with credentials the AWS chain resolves — none of which can be assumed of somebody who has just typed a preset name. The scale height is **required**, not defaulted, because the OPAC constructors set none and an atmosphere carrying zero builds cleanly and is then refused by `ArtificialSkyglow` and `CloudySkyglow` at evaluation.
- `dataset.Sky` and `LiveAerosol` have tests, including the consent-refusal message. That message is the whole mitigation for keeping consent outside the package, and nothing had ever executed it.
- **`dataset.Sky` assembles a preset, its data and its transfer into one thing you evaluate.** `Open` gathers everything, `Sky.Scene` manufactures scenes with the preset's kappa and scattering order already applied, and `Direction`/`Zenith`/`SkyMap` supply the preset's own fidelity and grid. It makes scenes rather than holding one, so a service answering for many instants is not forced back into transcribing the transfer by hand. Escape hatches (`Model`, `Grid`, `Band`, `Inputs`) mean nothing is walled off. `WithMagSystem` is an option rather than a `Spec` field because `magnitude.AB` is the zero value — an unset field would silently mean AB where sky brightness is quoted in Vega. `Spec` carries no observer at all: nothing it gathers depends on one, since the star and dust maps are all-sky and the grid and passband are spectral, so one `Sky` answers for any number of sites and the observer arrives with the scene. The airglow spectrum is the exception, and is chosen by name through `Spec.Observatory` — SkyCalc models Paranal at three altitudes rather than an arbitrary site, so that dependence cannot be resolved from coordinates, only chosen.
- **`Estimate.Composition` reports what a sky is made of**, brightest term first, as `ComponentShare{Component, Brightness, Fraction}`. `Fraction` is the share of band radiance and is deliberately the linear one: radiance adds and magnitudes do not, so the shares account for the whole sky while the magnitudes beside them cannot be combined by any arithmetic a reader might try. It works at `Reference` fidelity too — the Eq. 11 term is added into each component's own buffer precisely so a breakdown still attributes scattered light.
- **`Preset.Transfer` applies a preset's transfer to an atmosphere under construction.** The guard added earlier rejects a mismatched scene; this is the other half, and `TestPresetTransferProducesAnAcceptedScene` round-trips all four presets through it into a real evaluation — if the two ever disagree, a caller doing exactly what the library says would be refused by the library.
- **`atmosphere.SurfaceAtAltitude`** sets surface pressure and temperature from the ICAO standard profile at a site's elevation. The number is not free to guess: pressure sets the Rayleigh optical depth, so a sea-level default at a 2,600 m site overstates molecular scattering by about a quarter — and `NewBuilder` starts every builder at sea level.
- **`atmosphere.CleanMountainAOD550` / `ContinentalAOD550` / `UrbanAOD550`** are indicative optical depths per regime, so a caller with no measurement types a named constant rather than inventing a number. They are explicitly **not** from OPAC: OPAC supplies the optical *properties* the constructors carry, which are properties of an aerosol type; optical depth is how much is overhead tonight and cannot be a constant.
- **`cams.AOD550` fetches the real figure** from the Copernicus Data Space Ecosystem for a site and an hour, on the CAMS reader that already existed. CAMS rather than a satellite retrieval because this is a night model: a passive optical sensor infers aerosol from reflected sunlight, so retrievals exist only in daylight, while an assimilating forecast has a value at every hour. The grid orientation is verified against physical geography rather than assumed — measured, the Indo-Gangetic Plain reads 1.07 and the Antarctic plateau 0.043.
- **`CloudySkyglow` is validated against the Žilina runs of Kocifaj, Falchi & Kundracik (2025)**, in their own configuration: AOD 0.1, Ångström 1.3, SSA 0.90, asymmetry 0.65, cloud base 2 km, CF 0.9, the 500–600 nm band. Zenith radiance amplifies **122×** over the town against their "more than fifteenfold", horizontal illuminance **57.8×** against their "more than fourfold" irradiance, and the sky **screens beyond about 45 km** — both signs of the effect, monotonic in between. The crossover is further out than their 18.5 km observers and cannot be reconciled from the text: the cloud albedo and the town's upward emission split both move it and neither is stated, so the test asserts the signs and the ordering and records the crossover rather than requiring one. The 2007 contour figures are not digitised — the copy in hand is a text extraction with no image tooling — so that paper's prose claim about angular falloff is checked instead of numbers read off a plot.
- **`CloudySkyglow` covers the full range from clear to overcast**, completing Phase 5: Kocifaj, Falchi & Kundracik (2025) add an above-cloud scattering term `L_2` weighted by `(1-CF)[1-o(z,A)]`, and its transmissions compose into the one the below-cloud term already carries — so both are the same integrand over different limits, and at zero cover they sum to the clear-sky column exactly, which is now a test. **The acceptance criterion is met**: over a city an overcast deck amplifies the zenith **88×**, and 60 km outside it the same deck **screens at 0.80×**. Both signs emerge from geometry, which is the behaviour a universal cloud multiplier cannot produce and the reason this is radiative transfer rather than a factor. Two departures are recorded in the component's provenance rather than buried: the line-of-sight opacity is Beer-Lambert through a deck of stated optical depth where the paper ray-casts a stochastic 3D cloud field it does not specify closely enough to reproduce, and `L_infinity` is scaled by the cloud fraction where the printed Eq. 3 carries no such weight — without one a tenth-covered sky returns the reflection of a whole deck.
- **`skybrightness.CloudySkyglow` implements Kocifaj (2007) Eq. 27**, artificial skyglow with a reflecting cloud deck: a height-resolved radiative transfer whose two terms are light scattered by the air below the cloud and light returned by the cloud base. Measured over a city, an overcast deck makes the zenith **279×** brighter than the same sky clear — the amplification a model that multiplied a clear-sky answer by a transmission could never produce. It shares a `ComponentID` with `ArtificialSkyglow`, so `NewModel` refuses both at once: they compute the same contribution by different solutions and summing them would double-count artificial light. Fractional cloud cover is **refused** with `ErrPartialCloud` rather than approximated, because that needs the above-cloud term and line-of-sight opacity of Kocifaj, Falchi & Kundracik (2025).
- `skybrightness.GarstangEmission` is Garstang's (1986) upward-emission function, `B(Q,q,z₀) = 2Q(1−q)cos z₀ + 0.554q z₀⁴` — the shape the World Atlas, the SkyGlow Simulator and most of the light-pollution literature are built on. `EmissionShape` is now an interface, since the published forms are genuinely different functions rather than one function with different constants, and a struct holding both would have fields silently ignored depending on other fields.
- `atmosphere.ExponentialExtinction` and `ExponentialDepth` are the vertical profile Kocifaj (2007) Eq. 36 adopts, normalised so the integral of the extinction over the whole column is exactly the column depth — the two are used together and an atmosphere that attenuates at one rate and scatters at another is wrong everywhere at once. `atmosphere.VolumeScatteringFunction` is the local counterpart of `CombinedPhaseFunction`: the dimensional angular volume scattering coefficient a height integral needs, rather than a normalised phase function.
- **`skybrightness/dataset.Inputs` gathers everything a preset needs in one call**, and `dataset.Endpoints` reports what that will download so consent stays explicit at the call site. Assembling the reference data by hand meant five services across four packages — star map, dust map, airglow spectrum, passband, solar spectrum — each with its own client, cache and consent, and it was the same page of code every time. The subpackages stay exported for callers who want one dataset or a source this does not offer. Two things it deliberately will not decide: download consent, because a convenience that granted its own would fetch 145 MB because somebody typed a preset name; and the ground-emitter inventory, because satellite radiance alone cannot determine a source spectrum or an upward emission function and a default would be reporting somebody else's city.
- `examples/25_sky_brightness_compare` puts two comparisons side by side, each varying exactly one thing, because a table that changed the preset and the place at once could not say which caused the difference. Three natural presets at one site agree to **0.047 mag** at the zenith, and disagree in the way Masana et al. (2024) describe: `gambons-web` is the fainter at the zenith and the brighter at 10° altitude, so the sign of the gap flips between the columns. Then one preset over three atmospheres, where the ordering **runs backwards** — 21.226 on a clean mountain against 21.411 at hazy sea level — because no natural preset models artificial light, so more air and more aerosol can only extinguish starlight and zodiacal light faster than the diffuse term scatters them back. That is correct physics and a trap, and the example says so rather than leaving a reader to conclude a city is darker than Paranal. A third section decomposes one Observatory sky — the module's own model, kept out of the preset table because it changes the component set as well as the transfer and would vary two things at once. At Paranal under a near-full Moon it comes out **97.3 per cent moonlight** and **0.11 per cent artificial** from a town 120 km off, which is the right answer for a site chosen to be far from towns.
- `examples/18_sky_brightness` returns, rewritten against the V2 engine — the whole flow from consent to a mag/arcsec² number with a per-component breakdown. It measures 21.23 mag/arcsec² at the zenith at Cerro Paranal, and prints the run of altitudes that shows why sky brightness is not a function of airmass alone.
- **`internal/votable` reads the tabular payload of an IVOA VOTable.** TAP services must serve VOTable and are only encouraged to serve anything else, and Gaia@AIP's synchronous endpoint is outside that second group: it answers VOTable whatever `FORMAT` or `RESPONSEFORMAT` asks for, checked live against `csv` and `text/csv` alike. It reads `TABLEDATA` and nothing else — `BINARY`, `BINARY2` and FITS serialisations yield no rows rather than wrong ones — and surfaces `QUERY_STATUS=ERROR`, which is the failure a status code cannot catch: a rejected query still returns 200, so ignoring it reports an empty sky where the service actually refused the request. `OVERFLOW` is reported as `Truncated`, because a truncated result and a complete one are indistinguishable by inspection.
- `catalog/gaia.TestArchivesAgree` compares ESA and Gaia@AIP over the same cone. Measured over 340 sources at the north galactic pole: identical source sets, and positions agreeing to **0.0000 mas**. The field is sized against the query's `TOP N` cap on purpose — a result that reaches the limit is an arbitrary subset, so two services could differ in truncation rather than in data, and a much smaller field returned two sources, which agrees trivially however wrong an archive becomes.
- **A model built by `NewPreset` refuses to be evaluated under somebody else’s transfer.** `NewPreset` builds components, while kappa, higher scattering orders and the fidelity that decides whether the Eq. 11 integral runs at all live on the caller’s atmosphere and query — three things to remember, and forgetting any of them returned a plausible number rather than an error. The model now records its preset (`Model.Preset`) and `Estimate`/`SkyMap` reject a mismatch with `ErrPresetMismatch`, naming the setting and the value to fix it. `Model.WithoutPreset` is the deliberate way out, for measuring what one transfer choice is worth — which is the only legitimate reason to mismatch, and what this module’s own validation does. `Fast` is accepted wherever `Standard` is: what a preset is defined by is whether the integral runs, not which of the two cheaper labels a caller picked.
- **All four presets are now locked by golden tables, not two.** `NaturalSky` differs from the GAMBONS presets only in kappa, which is exactly what a shared lock would hide. `Observatory` is the one that needed it most: it is not reproducing a published model, so no external comparison can ever be written for it, and its numbers were asserted nowhere — the preset with the least external validation had the least internal validation too. Its moonlight and artificial-skyglow terms are locked alongside the five natural ones.
- `skybrightness.Preset` names published configurations of components and transfer: `GAMBONSWeb` reproduces the GAMBONS web service (the five natural components of Eq. 10, van Rhijn airglow at 87 km, κ = 0.5) and `NaturalSky` is the same physics at κ = 0.75 after Duriscoe (2013). `NewPreset` builds one; `Preset.DiffuseKappa` reports the transfer factor for `atmosphere.Builder.DiffuseScattering`. The GAMBONS validation tests now build their model through the preset, so what is checked against the published all-sky export is the configuration the library ships.
- `skybrightness.GAMBONSFull` is the third preset: GAMBONS as the paper computes it rather than as the web service does, with κ = 1 and the Eq. 11 scattering integral. `Preset.Fidelity` reports the level each preset must be evaluated at — `Reference` for the full model, `Standard` for the other two — because asking the full model at `Standard` yields a sky with no scattering treatment at all, which is a plausible number rather than an error. The two GAMBONS presets are locked and validated separately: the published all-sky export is a web-version run and validates only `GAMBONSWeb`, while Table 2 is a full-model composition and was never a target the web preset could be held to.
- `Model.Estimate` at `Reference` fidelity adds the Eq. 11 term per component, so a breakdown still attributes scattered light to whatever supplied it. Components that are themselves scattering integrals over a source outside the sky field (moonlight, artificial skyglow) keep their own treatment and the estimate records that through the new `PartialScattering` quality flag. A reference estimate costs about three orders of magnitude more than a standard one.
- `skybrightness.ScatteredIn` implements Masana et al. (2024) Eq. 11, the hemispheric scattering integral, over `atmosphere.SingleScatteredRadiance` and `atmosphere.CombinedPhaseFunction`; `Model.AboveAtmosphere` supplies its incoming field by dividing out the extended-source factor the natural components applied. Measured at the Table 2 geometry, the full model closes 37.6 per cent of the starlight-to-zodiacal gap the simplified transfer cannot touch at all.
- **`skybrightness/dataset/passband` returns, as a different package under the same name.** The removed one opened a versioned local bundle of curves; this one resolves a passband from the Spanish Virtual Observatory Filter Profile Service (`Fetch`/`Parse`, new `remote.SVOFilterProfile` endpoint), carrying the detector convention and the Vega zero point that a transcribed curve cannot supply.
- **Bright stars are multi-band.** `BrightStar.Mag` carries a magnitude per band and `AddBrightStars` takes the passband whose calibration it should use, so the stars Gaia saturates on reach B, V, R and I rather than V alone. B and I come from Hipparcos' own `B-V` and `V-I`; R comes from `V-R = (V-I) - (R-I)` with `R-I` from the Bright Star Catalogue (VizieR V/50) via the new opt-in `AddCousinsR`. Nothing is fitted — the alternative, a colour-colour relation predicting `V-R`, would put an estimate into a map that is otherwise measurements. A star the catalogue does not cover keeps no R entry rather than an invented one. Validated live: B, V and I for all 74 stars, R for 66, with every gap accounted for. The match propagates proper motion to J2000 and discriminates on V as well as position — α Centauri A moves 32 arcseconds between epochs, and its companion sits 0.02 arcseconds nearer the propagated position than it does.
- `dust.Open` reads the Schlegel, Finkbeiner & Davis (1998) 100 micron all-sky map locally (new `remote.SFDDustMap` endpoint, two 64 MB FITS hemispheres from doi:10.7910/DVN/EWCNL5) instead of asking IRSA one sightline per request. Measured, IRSA answers in 2.33 s per sightline — 13 hours and 20,000 requests to a shared service for a one-degree sky, against an engine that now does the same sky in 16 s. It has to be SFD specifically: the diffuse-galactic correlation is fitted against this map and the 0.8 MJy/sr the model subtracts is this map's own zero-point term. `dust.Fetch` stays, as the zero-setup path for a few directions and as the reference the local map is validated against; `dust.CachedDirections` exposes what the service has already answered, so that validation costs it nothing. Validated against IRSA over the 1,979 directions already cached: median ratio 1.00001, 5th to 95th percentile 0.956 to 1.056, the spread being interpolation across a 2.37 arcminute pixel.
- `starlight` gains the asynchronous query path (new `remote.GaiaAIPAsync` endpoint), which runs a whole-sky aggregation as one UWS job instead of 787 chunked queries — 1.8 billion sources in 27 minutes. Two things make it work and neither is obvious: `QUEUE=2h`, which selects the service queue that sets `statement_timeout` to 7,200,000 ms rather than cancelling a long query mid-flight, and `FORMAT=parquet`, which only the asynchronous endpoint honours. `BuildFromResult` assembles a map from a result already fetched, sniffing Parquet from CSV by the file's own magic.
- **Results are read as Parquet, not CSV.** Columnar, typed and compressed: a whole-sky four-band result is ~60 MB against 245 MB, and no number crosses the wire as a decimal string to be parsed back. Measured against the same job read both ways, the two agree to 1.6e-15 relative — the residual is the CSV's decimal round-trip of a double, and Parquet carries the archive's actual binary value.
- `magnitude.GaiaGToJohnsonR` and `magnitude.GaiaGToCousinsI` complete the Johnson-Cousins set alongside the existing V and B relations, from the Gaia DR3 photometric documentation Section 5.5.1 Table 5.9. G−I_C is a **quadratic** with σ = 0.03765, not a cubic — two readings of the rendered table disagreed on whether that number is a coefficient or the σ, and the raw table cell counts settle it (the B and R rows carry five coefficients plus σ, V four, I three). The Sun confirms it: the quadratic gives V−I = 0.727 against a published 0.71–0.72, where the cubic reading gives 0.747.
- `starlight.GaiaJohnsonCousins(name, passband)` builds a map band for B, V, R or I from the published colour polynomial (`JohnsonCousinsColorTerm`) and a passband's own calibration, with `starlight.VegaZeroFlux` deriving the zero point in W·m⁻²·nm⁻¹ from the service-published janskys and the band's pivot wavelength rather than transcribing it. **There is no U**: Gaia publishes no G-to-U relation, so four bands is what this catalogue can produce, and the constructor returns an error rather than an approximation.
- `atmosphere.Builder.DiffuseScattering` sets the effective-optical-depth factor κ, and `atmosphere.ExtendedSourceOpticalDepth` applies it with separate molecular and aerosol scale heights (Masana et al. 2021 Eq. 29).

### Changed — BREAKING (unreleased API)
- **`catalog/gaia.New` takes the archive to query and returns an error**: `New(endpoint remote.EndpointID) (*Provider, error)`. The zero value selects `gaia.DefaultEndpoint`, which is now **Gaia@AIP rather than ESA**. Both serve the same fixed DR3 tables, so neither can be more current; AIP answers a schema query in about three seconds against ESA's ten, and ESA has been unreachable for a whole working day while AIP answered throughout. A default is a decision made for callers who have not thought about it, so it should be the endpoint most likely to answer. `starlight.GaiaBuild.Endpoint` defaults the same way. ESA stays registered and selectable, and is what the new cross-archive validation checks the default against — it is no longer the path a caller takes, but the independent answer that path is measured by.
- `catalog/gaia` reads whichever serialisation the archive actually sent, sniffed from the payload rather than assumed from the request, the same way the star-map loader tells Parquet from CSV by the file's own magic. It has to: the request cannot determine the reply when the service ignores what it asked for. `targetFromRow` is now shared by both paths, so what a cell *means* has one definition rather than two to keep in step, and it looks every column up before indexing it — the previous form took index zero from a map for a missing name and read the wrong column as though it were the one asked for.

### Deprecated
- `remote.WorldAtlas` and `remote.LightPollution`. Neither is read by anything. The `skybrightness/atlas` package that consumed the World Atlas was removed in the V2 rewrite and nothing replaced it; the Atlas is a propagated model output rather than a measurement, so it can neither validate this module (§13) nor be served as its answer without presenting somebody else's model as this one's prediction, and its CC BY-NC licence makes it an awkward thing for a library to fetch on a caller's behalf. `LightPollution` returns VIIRS-derived raster values, and reading satellite radiance as sky brightness is on this module's prohibited list — an upward measurement standing in for a downward prediction. Both stay for the two minor releases the deprecation policy requires.

### Removed
- `remote.PassbandBundle`. It pointed at a release asset that was never published — its own doc said so — and `skybrightness/dataset/passband` has resolved curves from the Spanish Virtual Observatory since it was rewritten. Removed rather than deprecated because it never appeared in a tagged release, so nothing can be depending on it.

### Fixed
- **The preset golden tables depended on whichever IERS data happened to be on disk.** Zodiacal light is a function of solar elongation and moonlight of the Moon's position, so both resolve UT1 through Earth orientation parameters that load lazily from a cache a machine either has or has not. All four tables passed locally and all four failed in CI on every platform, by **2.5e-06 relative on zodiacal radiance** against a 1e-12 lock — six orders past it and nowhere near a rounding artefact. A `TestMain` now registers `time.ZeroModel` for the whole package, which also marks the choice authoritative so the lazy load cannot replace it, and the tables are regenerated under it. They are now a property of the code rather than of the machine.
- **`Angle.HMSString` and `DMSString` could emit a minus sign inside the seconds field** — `Deg(1)` rendered as `"00h04m0-0.0s"` on arm64. `m` truncates `rem`, so `rem - float64(m)` is non-negative in exact arithmetic, but a wider register for the subtraction than the value the truncation saw leaves about -1e-17; that is invisible until `FormatFloat` renders it `"-0.0"` after the leading-zero branch has already written a `0`. Both functions now clamp at zero, which also normalises the negative zero the existing `math.Abs` guard was aimed at.
- **The ARC and STG projections computed angular distance the worst possible way.** The parallel component is `cos(c)`, so `c = pi/2 - asin(cos c)` was the obvious route — and `asin` loses half its significant digits as its argument approaches 1, which is exactly where the reference point sits. Measured: a 1e-10 offset from the reference point round-tripped **1.48e-08 rad** out, about 3 milliarcsec. Both now take the angular distance from `Atan2` of the perpendicular and parallel components, which are the ones `phi` already uses, giving **2.22e-16** on the same case — a ten-millionfold improvement. Found because arm64 fuses the multiply-add and lands the dot product at 1-ε where amd64 gives exactly 1, so the defect was real everywhere and visible only in CI.
- **`airglow.Fetch` reached ESO SkyCalc on every single call**, the only dataset in the tier with no cache of its own — the star map, dust map and solar spectrum resolve through `remote.GetFile`, and `passband` caches its own profile. Measured, that was **3.7 s and three requests to Garching per `dataset.Open`**, so assembling four models over one night made twelve requests for four identical answers, and `examples/25_sky_brightness_compare` spent **33 s** doing it. `dataset.Open`'s own documentation claimed "five services on a cold cache and none on a warm one", which was simply untrue. It now caches the returned FITS keyed on a SHA-256 of the whole outgoing request — the answer depends on all thirty-five parameters, several of which have no `Spec` field, so hashing what is actually sent is the only key that cannot silently collide and it keeps working when a parameter is added. Repeat `dataset.Open` is **3.73 s → 0.72 s** and the example **33 s → 6.5 s**. Reusing the answer is sound because SkyCalc is a model rather than an observation, the same bargain `passband` already makes with SVO. Politeness as much as speed: SkyCalc is a shared research service.
- **`GarstangEmission` returned zero at exactly the horizon, which silently emptied `ArtificialSkyglow`.** Its guard rejected `sin <= 0` where `UpwardEmission` rejects `sin < 0`, and `ArtificialSkyglow` evaluates the emission function at *exactly* zero elevation by default — a ground source beyond a few kilometres sits at the observer's horizon, which is that component's own documented reasoning. Every Garstang-shaped emitter therefore contributed exactly nothing, at every distance and in every direction: not an error, just an artificial-skyglow term that was not there. The physics agrees with the arithmetic — at the horizon the reflected term vanishes with `cos z0` while the direct term does not, leaving `0.554*q*(pi/2)^4`, which for Q = q = 0.15 is **about twice the zenith value**. That near-horizontal lobe is the whole reason Garstang's function carries a `z0^4` term, so the guard deleted the largest part of the shape. Nothing caught it because every artificial test builds its city with `UpwardEmission`, and the one place `GarstangEmission` was exercised — `CloudySkyglow` — derives its elevation from geometry and so lands on exactly zero with probability zero. Found by writing `examples/25_sky_brightness_compare`. The `EmissionShape` contract said "Zero or below the horizon returns zero", which `UpwardEmission` already violated; it now states that only strictly-below returns zero, and says why the boundary is load-bearing.

### Changed — BREAKING (unreleased API)
- **`atmosphere.Builder.BoundaryLayer` is now `AerosolScaleHeight`**, and `Aerosol.BoundaryLayerHeight` is `Aerosol.ScaleHeight`. The old name was a trap: the models reading it — Kocifaj (2007) Eq. 36 and this package's transfer — use it as the scale height of an exponential profile, one to two kilometres, while a meteorologist's boundary-layer height over land at night is 100 to 500 m. A caller who looked up their site's nocturnal boundary layer was right by the name and wrong by the model, with nothing to say so.

### Changed
- **`CloudySkyglow` is 13× faster**, 9.35 ms per direction to 0.72 ms, by hoisting out of the spectral loop everything that does not vary across it: the airmass of each leg, both phase functions, and the exponential profile shapes, which factor into a wavelength-independent decay times a per-wavelength column depth. The two legs' transmissions also share one vertical depth, so they combine into a single exponential rather than two. The inner loop is now one exponential and a few multiplies per wavelength, where it had been repeating two square roots, a 1.5 power and four exponentials 671 times per height step for values that never changed. Verified exact: 32 sampled radiances moved by at most **1.9 ulps**, which is floating-point reassociation and not a change in the model.
- **`skybrightness`'s package doc no longer describes a package that does not exist.** It opened with "# Phase 0 — This is the spectral foundation only. It ships no `Component` implementations at all", which was the first thing a reader saw on pkg.go.dev and had been false for some time: seven components ship, four presets configure them, and the natural sky is validated against GAMBONS' published export at −0.03 mag. It now opens with the shortest path to a number and states plainly what is built and what is not.
- The preset golden fixture moves from 2026-03-20 to 2026-04-02 05:00 UTC, an instant with the Moon 73 degrees up and 3 degrees from full. At the old instant the Moon was below the horizon, so `Observatory`’s moonlight term evaluated to exactly zero at every altitude — a column of zeros that would have looked like coverage while guarding nothing about the largest capability difference between that preset and GAMBONS. All four tables were regenerated together, since the zodiacal light’s dependence on solar elongation moves the other three with the date too.
- The gap-closure figure for the Eq. 11 integral is stated as **37.6 per cent** in the three places that quote it, which previously disagreed with each other (34 in this file, 37 in the design doc and in `Observatory`’s own documentation). The test that measures it now logs one decimal rather than a whole number, so a half-per-cent drift can no longer move the quoted value without showing up as a change.
- The download-consent budget for the published star map is read from the endpoint registry in all six tests that grant it, rather than from a literal. Consent is checked against the registered `ApproxSize`, so a literal is a second copy of that number — and when the map went from one band to four the copies drifted, the tests passing only because the headroom happened to be enough.

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
- **The OPAC aerosol presets now carry a vertical profile**, from the paper's own Table 5 "Height profiles of all aerosol types": `N(h) = N(0)·exp(−h/Z)` with Z of **8 km** for continental and urban, **2 km** for desert, **1 km** for maritime. They previously set none, so `RuralAerosol(2635, 0.03)` built an atmosphere that looked complete, carried a zero scale height, and was refused by `ArtificialSkyglow` and `CloudySkyglow` when they finally read it — a failure three calls from its cause, on the recommended path. The spread between types is the point: the paper says Z = 8 km is "the value valid for air molecules", so continental aerosol is mixed as the air is, while sea salt is generated at the surface and falls out. A single value across all four would have been one paper's continental fit applied to the ocean and the Sahara alike.
- `dataset.LiveAerosol` no longer takes a scale height, since the preset supplies one; a caller reproducing a published run chains `AerosolScaleHeight`.

- **`examples/18_sky_brightness` misused the OPAC constructor.** `RuralAerosol(heightM, aod550)` takes the *site elevation* — it feeds the standard profile for surface conditions — and the example passed an aerosol scale height into it, then overrode the result. The atmosphere came out right by accident and the aerosol scale height was never set at all; the example only worked because `GAMBONSWeb` has no component that reads one.

- **`magnitude.Passband.PivotWavelength` ignored the detector convention.** A photon-counting response is an energy response times λ/hc, so the pivot differs by a factor of λ inside both integrals; it computed the photon-counting form for every band regardless of `Passband.Detector`. Wrong by up to 0.89 per cent in wavelength and twice that in an f_λ↔f_ν conversion — silent, systematic, and always making an energy-calibrated band look redder than it is. Checked against the Spanish Virtual Observatory's own `WavelengthPivot` for the five Bessell bands, all energy counters: honouring the detector reproduces every one to four decimals where the photon form misses by 0.33 to 0.89 per cent. Found while validating a four-band starlight map, whose V band agreed with the published V map to 0.91 per cent instead of the expected 0.08.

- **`fits.Read` never decoded image HDUs.** It appended a header-only HDU and seeked past the pixels, so a caller type-asserting to `*ImageHDU` got neither an image nor an error — the same defect `ReadBintable` had, where a table consumer silently got no rows. Tables were fixed in that round and images were missed. Found by the SFD dust map, which asks for the one thing `Read` would not give it.

- **`magnitude.GaiaGToJohnsonB` carried coefficients that appear in no published table.** It used a cubic, `B − G = −0.02907 + 0.6399·(BP−RP) − 0.09631·(BP−RP)² + 0.01023·(BP−RP)³`, sourced only as "Gaia DR3 photometric documentation" with no table number. The published relation (Gaia DR3 documentation Sect. 5.5.1, Table 5.9) is a **quartic** in the opposite direction: `G − B = 0.01448 − 0.6874·(BP−RP) − 0.3604·(BP−RP)² + 0.06718·(BP−RP)³ − 0.006061·(BP−RP)⁴`, σ = 0.0633. Measured against 4,000 stars with both Gaia and Tycho-2 photometry, the previous coefficients missed by −0.46 mag at BP−RP ≈ 0 growing to −2.0 mag by BP−RP = 3 — in **both** orientations, so the shape was wrong and not merely the sign — while the published quartic has a median residual of −0.010 mag. The function had no test at all; it now has one anchored on the Sun, on the direction, and on reproducing the Sun’s B−V through both transformations at once. **Table 5.10 restricts the relation to M giants beyond BP−RP = 1.75**, which is documented rather than clamped.
- The G-to-V and G-to-B transformations were cited as "Riello et al. (2021), Table 5.7". Both live in **Table 5.9** of the Gaia DR3 photometric documentation, Sect. 5.5.1, which does not cite Riello for them. Corrected across `magnitude`, `catalog/gaia`, `skybrightness/dataset/starlight` (including the provenance line written into every published map header) and `docs/skybrightness.md`.
- **The Gaia G to Johnson V colour transformation was applied inverted, making every converted magnitude about 0.48 mag too bright and the integrated-starlight map 1.3 times too bright at solar colour, 1.6 times at the sky’s median colour and 17 times at BP−RP = 3.** The Gaia DR3 photometric documentation, Section 5.5.1, Table 5.9, is tabulated as *G minus the target band*; the code read it as V − G. `magnitude.GaiaGToJohnsonV` and `catalog/gaia` returned `G + (G−V)` instead of `G − (G−V)`, and `starlight.GaiaJohnsonV` carried the negated polynomial, so the query applied the reciprocal of the intended factor. Confirmed against the Sun (G−V = −0.14 observed, −0.15 from the polynomial) and against 4,000 stars with both Gaia and Tycho-2 photometry, where the corrected form has a median residual of −0.002 mag and the previous one −0.479 mag. The error was invisible to every internal check — it leaves the map positive, smooth and correctly shaped — and the one test covering the sign asserted the inversion as its premise. It is caught now by the physical inequality instead: Gaia G spans 330–1050 nm against V’s ~500–600 nm, so a V map can never be brighter than the G map it came from. **Any map built with a previous revision must be rebuilt.**
- `starlight` clamps the recovered mean colour to the −0.5 < BP−RP < 5.0 interval Riello et al. fitted. Flux-weighting the mean lets one dominant star carry a pixel past any real stellar colour — it reaches 7.41 over the order-9 sky — where an unbounded cubic produced flux ratios above a hundred. Costs 0.0003 per cent of the whole-sky map across the 194 pixels that reach outside.
- **The Gaia colour transformation silently dropped every source without BP−RP** — 14.95 per cent of the catalogue all-sky and over half the sources in the densest pixels, because a null colour makes the polynomial null and SQL drops the row from the sum. The aggregation now returns the unconditional G flux, the coloured-source flux and the pixel mean colour, and scales the difference by the polynomial at that mean — the same treatment Masana et al. (2021) apply. The mean is weighted by **flux, not by count**: the numerous faint red sources dominate a plain `AVG(bp_rp)` while the bright, bluer ones dominate the light being scaled, which over-corrected the worst measured pixel by 19 per cent. `CASE` (rejected by ESA) and `FILTER` (rejected by Gaia@AIP) are both avoided via NULL propagation, so one query runs on either archive.
- **`starlight.Load` used 73 MB of heap to read a 6 MB map**, accumulating into a `map[int64][]float64` whose per-pixel bucket and single-element-slice overhead cost about ninety bytes each. It streams into flat slices now and scatters once — 25 MB peak including the parse temporaries, 6 MB retained, and 177 ms instead of 285 ms for an order-8 map.
- **`fits.Read` never decoded table extensions**, so a caller type-asserting an HDU to `*BintableHDU` never got one, and `ReadBintable` — which itself built an empty record batch and discarded the payload — was unreachable from the package's only entry point. `catalog/fits` and `skybrightness/dataset/solar` both shipped against that, getting no rows and no error. Binary tables are decoded for real now (big-endian, row-major, column widths checked against `NAXIS1`), and column names are trimmed of the padding FITS adds, which is why CALSPEC's `"WAVELENGTH "` read as a missing column.
- **`skybrightness.IntegratedStarlight` scaled its spectrum by the sum of the shape's samples**, so refining the spectral grid halved the starlight while every value stayed positive and plausible. It divides by the shape's passband average now, which is resolution-independent and reproduces the map value exactly.
- **The generated Gaia ADQL was rejected by the archive three ways** — `GROUP BY` repeating the expression instead of naming the select-list alias, `COALESCE` alongside the already-known `CASE`, and a per-band column lookup that did not match the archive's lowercased names. All three were invisible to the substring assertions covering the query and were found only by sending one.
- `ephemeris/jpl.Open` closed the LSK reader it had just handed to `lsk.NewReader`, which takes ownership of it, so `Provider.Close` would then close it a second time.
- `plan`'s live geocoding test pre-checked only Nominatim while `NewSiteEarthAddress` also calls Open-Elevation, so downtime on the second service failed the test instead of skipping it. Both hosts are pre-checked now, plus a request-timeout guard, matching the repository's policy that network-tagged tests never fail on external downtime.
- **`ephemeris/jpl/spk`'s SPK reader called `os.Open` on every single `ReadAt`** (thousands of calls per Chebyshev segment lookup) after the `remote/file` rebuild above swapped its backing store from a plain `*os.File` to a `*file.Bucket`/`NewRangeReader`, since gocloud's `fileblob` driver opens a fresh OS file handle on every ranged read — turning a millisecond-scale SPK evaluation into multi-minute Windows runs. Fixed by capturing the resolved local path once (via `blob.ReaderOptions.BeforeRead`'s reach-through) and reading through one persistent `*os.File` afterward, closed on `Close()`.
- `time/internal/iers`'s `GetFile` call passed an empty object name against a single-resource `IERSFinals2000A.URL`, silently failing every fetch — the endpoint's URL is now a directory-style prefix (see `### Added` above) and the call site names `finals2000A.all` explicitly.
- Several `network`/`validation`-tagged tests (`ephemeris/jpl/validation`'s Sun/Mercury/Moon Horizons OBSERVER-query cases, `catalog/vizier`'s cone-search live tests, `remote`'s concurrent-`GetFile` lock test) converted hard failures on live external-service errors (JPL Horizons' own confirmed 500s for topocentric Sun/Mercury/Moon queries, VizieR TAP backend 500/503/400 instability, a documented Windows `fileblob` rename race) to logged skips — these tests are opt-in-only and were never run in CI, so this only affects local runs, per this project's own "never fail CI for external downtime" policy.
- `ephemeris/jpl/spk.CacheAPI` now detects and rejects a Horizons-generated SPK whose file record claims a first summary record (`FWD != 0`) but whose summary area is actually all zero bytes — live-confirmed as a real Horizons server anomaly (decoded byte-for-byte from a raw API response) that previously got cached and silently reused forever, producing an ephemeris provider with no coverage for the requested body and no error. New `spk.ErrHorizonsEmptyKernel` sentinel; `ephemeris/jpl`'s two live small-body tests (Eros, Apophis) skip with a clear log line when they hit it, rather than failing on a confirmed external issue.
- `remote/file.LocalURL` built its `"file://"` URL by raw string concatenation instead of percent-encoding the path — a directory whose name contained `#` silently truncated the URL at the fragment separator (losing the rest of the path *and* `?create_dir=true` with no error, opening the wrong, shorter directory), and a stray `%` made the URL fail to parse outright. Now built via `url.URL{...}.String()`, which encodes correctly and makes `LocalURL`/`OSPath` genuine inverses of each other instead of only matching by coincidence for paths with no URL-reserved characters.

### Added
- **The integrated-starlight map is published**: `starmap-o8-BVRI-total.txt.gz`, 16.7 MB, reachable through `starlight.Open` and the `remote.GaiaStarMap` endpoint at release tag `starmap-v2`. HEALPix order 8 on GAMBONS’ own grid in Johnson-Cousins B, V, R and I — the four bands Gaia can reach, since no G-to-U relation is published and a U column would be a fit rather than a measurement. All 1,811,709,771 Gaia DR3 sources with no magnitude cut, colourless sources recovered rather than dropped, plus the 74 Hipparcos stars Gaia saturates on: 9.4 per cent of the flux in B falling to 4.7 in I, relatively more in the blue because the diffuse background is the redder of the two. B, V and I hold all 74 and R holds 66, the eight gaps each accounted for rather than filled. Fetched end to end through `starlight.Open` before release, and its V reproduces the V-only `starmap-v1` to 0.08 per cent — that residual being the passband service’s Vega calibration against the rounded literal the earlier file rested on, not a change in the sky.
- **`skybrightness.IntegratedStarlight`** completes the natural sky: a tabulated extra-atmospheric star map attenuated on its way down, the directly attenuated term of Masana et al. (2021) Eq. 8. The scattered term that would partly refill it is not modelled, so results below 30 degrees altitude carry `ExtrapolatedModel`. It takes the passband its map's values are averaged over, which is what makes the spectral rescaling exact rather than resolution-dependent.
- **`starlight.GaiaJohnsonV`** supplies the Gaia G to Johnson V band as one sourced value. The conversion needs three published numbers from three places — G's VEGAMAG zero point, the Riello et al. (2021) colour transformation, and V's own Vega zero point — and using G's zero point with V's flux density and no colour term yields a map that is neither, wrong by the colour of whatever mix of spectral types each pixel holds.
- **`starlight.Map.Band`** exposes one band of a map as a `skybrightness.StarMap`, carrying the frame with it so the component knows whether to convert a direction to galactic coordinates.
- **`solar.NewScatteredMoonlight`** fetches the CALSPEC reference and builds the moonlight component with it. The engine still takes the spectrum as an argument, since evaluation performs no I/O; the convenience lives in the dataset tier where I/O belongs.
- **`skybrightness/dataset/viirs`** turns NASA VIIRS annual composites into `GroundEmitter`s, restoring the VIIRS capability the rewrite deleted with `skybrightness/atlas` — now as a *source provider* rather than a sky-brightness lookup. A pixel radiance determines neither a spectrum nor an upward emission function, so both are required inputs and every emitter is flagged `AssumedSourceSpectrum | AssumedEmissionFunction`. Bins outside coverage or resolving to no-data are dropped rather than zeroed, since missing data is not measured darkness.
- **`skybrightness/dataset/raster`** carries the GeoTIFF decoder recovered from the deleted `atlas` package — classic TIFF, LZW and deflate, strips and tiles, float samples and the floating-point predictor — with its original 600-line test suite intact. It is source-agnostic and carries no units, because the products it serves are satellite radiances.
- **`viirs.Region` emits one source per azimuth sector, not per ring-and-sector cell.** Kocifaj & Bará (2019) Eq. 9 sums over *azimuthally separated* sources; the earlier ring×sector binning stacked several emitters at one azimuth and made the total scale with the bin count. `Rings` is replaced by `RadialSamples`, which refines the estimate within a sector without changing the emitter count. The absolute scale is still uncalibrated — inferring `L_S` from satellite radiance properly needs Elvidge et al. (2017) — but the N-scaling was this repository's bug, not a gap in the paper, and `docs/skybrightness.md` §17 retracts the earlier claim that Eq. 2 was missing a term.
- `atmosphere.MultipleScatteringFactor` implements Winkler (2022) §5.2's `f = 1 + 4.5·τ_R`, his revision of Noll et al. (2012)'s coefficient of 2.2. Applied in `ScatteredMoonlight`, it moves the validated full-Moon sky brightness from 18.92 to **18.62 mag/arcsec²** — brighter, which is the direction single scattering is known to err, and closer to the canonical ~18. The `SingleScatteringOnly` quality flag becomes `ApproximateMultipleScattering`.
- `atmosphere.GushchinAirmass` is the airmass formula Kocifaj & Bará (2019) Eq. 3 adopts, giving 35.7 at the horizon against Pickering's 38. `ArtificialSkyglow` now uses it rather than `atmosphere.Airmass`, because the two-index model's fit and its horizon limit are calibrated against that value.
- **`skybrightness/dataset/solar`** fetches the CALSPEC solar reference (`sun_reference_stis_002.fits`) through the new `remote.CALSPEC` endpoint, converting angstrom and erg s⁻¹ cm⁻² Å⁻¹ to nanometres and W m⁻² nm⁻¹. It fixes the absolute scale of every reflected-sunlight model — lunar irradiance today, zodiacal light next — and interpolation returns zero outside the tabulated range rather than extrapolating flux into a band the reference never covered.
- **`DiffuseGalacticLight`, `ZodiacalLight` and `Airglow` are now `Component` implementations**, not loose functions. They resolve their own geometry from the `Scene` — alt-az to galactic or ecliptic through a cached `coord.Context` — and `Model.Estimate` sums all five components together. The radiance kernels keep their names with a `Radiance` suffix (`DiffuseGalacticRadiance`, `ZodiacalRadiance`, `AirglowRadiance`) and remain usable standalone.
- **The assembled engine reproduces a real dark sky.** Over Paranal on a moonless night it gives **21.48 mag/arcsec² in V** against a real 21.5–22.0, with airglow at 40.5%, zodiacal at 35.6%, artificial at 21.8% and diffuse galactic at 2.1% — the ordering Leinert et al. and Masana et al. both give. `TestFullSkyComponentShares` asserts the composition, not just the total, because a term entering with the wrong scale leaves the total plausible while the mix is wrong. 287 µs per direction, 40 allocations.
- `skybrightness.Airglow` and `atmosphere.VanRhijn` implement the chemiluminescent emission of the upper atmosphere: Leinert et al. (1998) Eq. 13 applied to a caller-supplied zenith spectrum, as Masana et al. (2021) Eq. 19–20 does. Validated against Roach & Meinel (1955)'s published maximum of 5.7 for a 100 km layer. Airglow brightens toward the horizon by a factor of about six from pure geometry, which is why it cannot be modelled as a constant floor.
- The zenith spectrum is a required input, not a prediction. Airglow varies by up to 100% night to night, with season, with the solar cycle and with geomagnetic latitude; Leinert et al. and Masana et al. both treat it as a free parameter, and so does this. Results carry `ClimatologicalAirglow`, and past 40° from the zenith `ExtrapolatedModel` too — Leinert et al. state that extinction along the longer path changes the behaviour materially there, and this applies the geometry alone.
- `skybrightness.ZodiacalLight`, `ZodiacalBrightnessAt`, `ZodiacalColorCorrection` and `ZodiacalElongation` implement Leinert et al. (1998) Table 17 and Eq. 22 with Masana et al. (2021)'s heliocentric and seasonal factors. Validated against a number from outside the table: the ecliptic pole comes out at **23.26 mag/arcsec² in V**, against roughly a quarter of a 22.0 dark sky. The solar vicinity Table 17 leaves blank returns `ErrZodiacalGeometry` rather than an extrapolation into a region an order of magnitude brighter.
- `skybrightness.DiffuseGalacticLight` implements the optical/100 µm correlation — Kawara et al. (2017) Eq. 7 as Masana et al. (2021) Eq. 13–14 apply it — turning a Schlegel–Finkbeiner–Davis dust intensity into spectral radiance. DGL is 20–30% of the Milky Way's integrated light, so it cannot be folded into starlight. The empirical fit bounds nothing itself, so three clamps are applied and flagged rather than left to produce negative radiance.
- The published quadratic coefficient's power of ten is read as 10⁻⁵ against the printed 10⁵. The turnover `b/(2c)` then lands at 39–50 MJy sr⁻¹ across all six well-measured bands — the top of Kawara's own fitting range — where the printed value would put it at 10⁻⁹ and make DGL negative over the entire sky. `TestDGLTurnoverMatchesTheFittedRange` asserts it band by band, so the evidence is executable. See `docs/skybrightness.md` §17.
- **`skybrightness/dataset/crosssection`** reads MPI-Mainz UV/VIS Spectral Atlas tables into `atmosphere.CrossSection`, completing Phase 1's absorption path. The wavelength unit is a parameter rather than sniffed — the atlas mixes nanometres, angstrom and wavenumbers, and guessing wrong shifts every absorption feature by a factor of ten while still producing a plausible curve. Wavenumber files are re-sorted, since they arrive in descending wavelength order.
- No cross-section dataset is shipped or defaulted. The atlas holds dozens of measurements per species at different temperatures, and ozone's Chappuis-band cross section — the one that matters at optical wavelengths — is strongly temperature-dependent, so "the ozone cross section" is not a thing. The reference and temperature are the caller's choice, as with the solar spectrum, the airglow spectrum and the Gaia band transformations.
- **Corrected:** `dataset/viirs` claimed Elvidge et al. (2017) carried a published VIIRS-to-line-of-sight-radiance method that was not implemented. The paper was obtained and is an instrument and product description with no such conversion — Kocifaj & Bará's citation is to the data, not a recipe. It does settle the radiance unit as nW cm⁻² sr⁻¹.
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

[Unreleased]: https://github.com/TuSKan/astrogo/compare/v0.20.0...HEAD
[0.20.0]: https://github.com/TuSKan/astrogo/compare/v0.19.0...v0.20.0
[0.19.0]: https://github.com/TuSKan/astrogo/compare/v0.18.0...v0.19.0
[0.18.0]: https://github.com/TuSKan/astrogo/compare/v0.17.0...v0.18.0
[0.17.0]: https://github.com/TuSKan/astrogo/compare/v0.16.0...v0.17.0
[0.16.0]: https://github.com/TuSKan/astrogo/compare/v0.15.1...v0.16.0
[0.15.1]: https://github.com/TuSKan/astrogo/compare/v0.15.0...v0.15.1
[0.15.0]: https://github.com/TuSKan/astrogo/compare/v0.14.0...v0.15.0
[0.14.0]: https://github.com/TuSKan/astrogo/compare/v0.13.0...v0.14.0
[0.13.0]: https://github.com/TuSKan/astrogo/compare/v0.12.0...v0.13.0
[0.12.0]: https://github.com/TuSKan/astrogo/compare/v0.11.0...v0.12.0
[0.11.0]: https://github.com/TuSKan/astrogo/compare/v0.10.0...v0.11.0
[0.10.0]: https://github.com/TuSKan/astrogo/compare/v0.9.0...v0.10.0
[0.9.0]: https://github.com/TuSKan/astrogo/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/TuSKan/astrogo/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/TuSKan/astrogo/compare/v0.6.1...v0.7.0
[0.6.1]: https://github.com/TuSKan/astrogo/compare/v0.6.0...v0.6.1
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
