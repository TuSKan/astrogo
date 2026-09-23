# SGP4 from scratch

A plan for replacing astrogo's SGP4 dependency with an implementation written
directly from Vallado's published algorithm, as a public package at
`ephemeris/satellite/sgp4`.

This document is the design. It is meant to be argued with before any of it is
built.

---

## 1. Why, and the evidence

astrogo propagates satellites through
[`github.com/joshuaferrara/go-satellite`](https://github.com/joshuaferrara/go-satellite)
— untagged, last touched in 2022, BSD-2. The usual reasons to leave such a
dependency (no releases, no maintainer, an awkward API) are real here but weak
on their own. The strong reason is that it is **wrong**, and astrogo already
measured how wrong without knowing why.

### 1.1 What the Vallado suite has been saying

`ephemeris/satellite/sgp4_vallado_validation_test.go` compares astrogo against
the reference states published with Vallado et al. (2006), *Revisiting
Spacetrack Report #3* (AIAA 2006-6753). Twenty-three of the thirty readable
cases agree to a median of 4 cm. Seven do not:

| satellite | Vallado's own label | miss |
| :--- | :--- | ---: |
| 28350 | near-Earth, perigee 127 km, "s4 mod" | 3438.51 km |
| 22312 | SL-6 R/B(2), last set before it decayed | 1828.92 km |
| 16925 | "the s4 > 20 modification" | 1328.37 km |
| 11801 | the original STR#3 deep-space case | 781.71 km |
| 28623 | H-2 R/B — deep space **and** perigee 136 km | 485.94 km |
| 28872 | perigee −51 km, "lost in 50 minutes" | 7.78 km |
| 23333 | WIND — "the Kepler solver fails past ~200 min" | 0.2175 km |

The test's own doc comment recorded the signature: *exact at tsince = 0, growing
quadratically with time — a wrong secular drag term, not a wrong initial state
and not a time-handling error in this package.* That was right, and it names the
defect precisely enough to find it.

### 1.2 The defect

`sgp4.go:53` in the dependency:

```go
qzms24temp := (128.0 - sfour) / radiusearthkm
```

Vallado, and every faithful port:

```c
qzms24temp = (120.0 - sfour) / radiusearthkm;
qzms24     = qzms24temp * qzms24temp * qzms24temp * qzms24temp;
```

**128 where the algorithm says 120.** The constant is the upper edge of the
atmospheric density band in the s⁴ drag model, and the branch it sits in runs
only when perigee is below 156 km:

```
sfour = ss; qzms24 = qzms2t
if perige < 156 { sfour = perige - 78; if perige < 98 { sfour = 20 }
                  qzms24 = ((120 - sfour)/Re)^4 ... }
```

`qzms24` feeds `coef = qzms24 · tsi⁴`, which feeds `cc1`, the secular drag
coefficient — so the error is exactly zero at epoch and grows as t², which is
the signature the test comment described. For a perigee just under 156 km,
sfour = 78, and the drag term comes out `(50/42)⁴ = 2.0` times too large: the
satellite is dragged down twice as fast as SGP4 says.

### 1.3 Proof, measured

The dependency was copied, the single character changed, and the same suite
re-run against it:

| satellite | today | with `120.0` | ratio |
| :--- | ---: | ---: | ---: |
| 28350 | 3438.51 km | **0.000105 km** | 32,700,000× |
| 22312 | 1828.92 km | **0.0179 km** | 102,000× |
| 16925 | 1328.37 km | **0.000262 km** | 5,070,000× |
| 11801 | 781.71 km | **0.0000025 km** | 310,000,000× |
| 28623 | 485.94 km | **0.0000944 km** | 5,150,000× |
| 28872 | 7.78 km | **0.000239 km** | 32,600× |
| 23333 | 0.2175 km | 0.2175 km | unchanged |

Six of the seven divergences are one digit. The seventh, 23333, does not move —
and should not: its perigee is 187 km, above the 156 km branch, so the constant
never applies to it, and Vallado annotates that case himself as the point where
the Spacetrack Report #3 Kepler iteration stops converging. The one case the fix
does *not* explain is the one the algorithm's own author says is a property of
the algorithm.

That is as clean a confirmation as this kind of finding gets.

### 1.4 What follows from it

The rewrite stops being a preference and becomes the obvious move, for a reason
that is not "the repo is stale":

- **astrogo has been shipping kilometer-scale errors** on decaying LEO objects
  and low-perigee deep-space orbits — exactly the objects a re-entry watcher or
  a debris-conjunction screen cares about. `Satellite.Verified` was built to warn
  callers off that regime; after this it should have nothing to warn about.
- **A one-character patch cannot be shipped.** Fixing it means a fork, which is
  a maintained repository with a release cadence, which is the thing nobody
  wants. Writing the algorithm is less work than owning a fork.
- **It refutes "a port is a port".** The dependency passes its own test suite,
  because its six cases (5, 4632, 6251, 88888, 24208, 23599) all have perigees
  above 156 km and never enter the branch. Correctness here is not a property
  you inherit by copying; it is a property you measure. That argues for owning
  the code *and* for the validation strategy in §7.

This finding is recorded separately as an astrogo issue, and is worth reporting
upstream regardless of whether that repository is alive.

---

## 2. What the rewrite must achieve

Concrete, falsifiable, and set before anything is written.

| # | criterion |
| :-- | :--- |
| **A1** | All **33** cases of the Vallado suite reproduced, including the three astrogo currently refuses (§6.5), with max position residual **≤ 1e-6 km** and max velocity residual **≤ 1e-9 km/s**, *except* 23333, which carries a documented, asserted exclusion for the Kepler iteration. |
| **A2** | The `divergent` map in the validation test is **empty**. `Satellite.Verified` either returns true for every element set in the suite or is deleted. |
| **A3** | `propagateECI`'s sub-second correction — 30 lines of comment and a 5.94 km bug in its history — **does not exist**, because the propagator takes a float time argument. |
| **A4** | `go-satellite` is absent from `go.mod`. |
| **A5** | Propagation is **allocation-free** and at least as fast per call as the incumbent. |
| **A6** | A `*Propagator` is **immutable after construction**, so concurrent propagation of one satellite at many times is safe and lock-free, asserted under `-race`. |
| **A7** | No input — including adversarial fuzz — can `panic`, `log.Fatal`, or `os.Exit`. The incumbent can do all three. |

A1's 1e-6 km is not arbitrary: `tcppver.out` prints positions to `%17.8f`, so
1e-8 km is the reference's own resolution. A faithful implementation given the
same float time argument should land within two orders of that, and the
incumbent's 4 cm median is three orders worse than it needs to be purely because
of the whole-second API.

---

## 3. Provenance and licensing

The algorithm is published, the implementations are not all equally usable, and
the distinction matters enough to write down.

- **Source of truth**: Vallado, Crawford, Hujsak & Kelso (2006), *Revisiting
  Spacetrack Report #3*, AIAA 2006-6753, and the C++ that accompanies it
  (`SGP4.cpp`/`SGP4.h`), released through CelesTrak/CSSI for unrestricted use.
  Hoots & Roehrich (1980), Spacetrack Report #3, is the original.
- **Reading reference**: `python-sgp4` (Brandon Rhodes, MIT) is a line-faithful
  transliteration that preserves Vallado's own comments, including every
  `sgp4fix` annotation explaining a deviation from STR#3. It is used here as a
  *cross-check on understanding*, the way one reads two translations of a text.
- **What is not used**: no code, structure, naming, or comment text is taken
  from any BSD-2 Go port. The new package is written from the algorithm as
  published.
- **Recorded where it matters**: `doc.go` carries the citation; every
  `sgp4fix`-derived deviation carries the reason in the Go comment, because
  those are the parts a future reader will otherwise "clean up" back into a bug.
- **Test data**: `testdata/vallado/` already holds `SGP4-VER.TLE` and
  `tcppver.out` with SHA-256s and provenance (see its README). Unchanged.

---

## 4. Package layout

```
ephemeris/satellite/sgp4/
  doc.go          package doc: what SGP4 is, what it is not, accuracy, citation
  elements.go     Elements — the mean element set, units, self-validation
  tle.go          TLE text <-> Elements; checksums as a separate concern
  gravity.go      Gravity (WGS72, WGS84, WGS72Old) and the constant sets
  errors.go       the sentinel errors, and which ones still return a state
  propagator.go   New, Option, At, AtTime, and the accessors
  initialize.go   initl + the sgp4init setup path
  nearearth.go    the near-Earth secular/periodic evaluation
  deepspace.go    dscom, dsinit, dspace, dpper — the SDP4 additions
  gmst.go         SGP4's own 1982 GMST, unexported, with the note in §6.7
```

Nine files, one concern each, in the shape `internal/gofaext` and
`skybrightness` already use. Estimated 2,000–2,400 lines of Go excluding tests;
the algorithm is about 1,600 lines of dense arithmetic in any language.

**Why public and not `internal/`.** An internal package would force every caller
who wants raw TEME states, or who has elements from an OMM rather than a TLE, to
go through `satellite.Satellite` — which is an *ephemeris provider* that returns
GCRS astronomical units, and is the wrong shape for that. SGP4 is a named,
standard, independently-useful algorithm; wrapping it in a private box that only
astrogo's own provider can open is the kind of API nobody wants to use.

---

## 5. Public API

```go
package sgp4

// Elements is the mean element set SGP4 propagates. It is not an orbit:
// the values only mean anything to SGP4 itself, fitted through SGP4 by
// whoever produced them.
type Elements struct {
    Name           string
    NORAD          int
    COSPAR         string
    Classification byte
    ElementSet     int
    RevAtEpoch     int

    Epoch time.Time // astrogo time, UTC

    Inclination angle.Angle
    RAAN        angle.Angle
    ArgPerigee  angle.Angle
    MeanAnomaly angle.Angle

    Eccentricity   float64 // dimensionless
    MeanMotion     float64 // revolutions per day, Kozai
    MeanMotionDot  float64 // rev/day^2, already halved as the TLE stores it
    MeanMotionDDot float64 // rev/day^3, already sixthed as the TLE stores it
    BStar          float64 // inverse Earth radii
}

func ParseTLE(line1, line2 string) (Elements, error)
func VerifyTLEChecksums(line1, line2 string) error
func Checksum(line string) int

type Gravity int
const (
    WGS72 Gravity = iota // default — the model TLEs are fitted with
    WGS84
    WGS72Old
)

type Mode int
const (
    ModeImproved Mode = iota // Vallado's 'i'
    ModeAFSPC                // Vallado's 'a'
)

type Option func(*config)
func WithGravity(Gravity) Option
func WithMode(Mode) Option

type Propagator struct{ /* unexported, immutable after New */ }

func New(el Elements, opts ...Option) (*Propagator, error)

// At returns TEME position (km) and velocity (km/s) at tsince minutes from
// the element epoch. Negative tsince propagates backwards.
func (p *Propagator) At(tsince float64) (pos, vel vector.Vec3, err error)

// AtTime is At for an absolute instant. t is converted to UTC.
func (p *Propagator) AtTime(t time.Time) (pos, vel vector.Vec3, err error)

func (p *Propagator) Elements() Elements
func (p *Propagator) DeepSpace() bool     // SGP4's own 225-minute branch
func (p *Propagator) SimplifiedDrag() bool // SGP4's own isimp branch
func (p *Propagator) PerigeeAltitude() float64 // km, the value SGP4 branches on
func (p *Propagator) ApogeeAltitude() float64  // km
```

That is the whole surface: four types, three constructors, two propagation
methods, five accessors.

---

## 6. Design decisions

Each of these is a place where the modern-Go answer and the transliterated
answer differ, with the reason for choosing.

### 6.1 `tsince` is a `float64` in minutes, and that is the primary method

The incumbent exposes `Propagate(sat, y, m, d, h, min, sec int)` — whole seconds
only — and truncates the element epoch the same way when it builds `jdsatepoch`.
astrogo compensates by stepping linearly along the velocity vector, and the
correction has to be `frac(t) − frac(epoch)` rather than `frac(t)`, a subtlety
that cost 5.94 km on Vallado's satellite 5 before the reference suite caught it.

SGP4's own argument is minutes-since-epoch as a real number. Taking it directly:

- deletes the workaround, its 30-line explanation, and its entire failure mode;
- makes the validation test compare against Vallado's own `tsince` column with
  **no conversion at all**, removing a whole class of error from the measurement
  rather than merely bounding it;
- gives sub-microsecond time resolution instead of one second.

`AtTime` is the convenience wrapper and uses `time.Time.JDParts` so the
difference is formed in two-part arithmetic rather than losing the fraction.

### 6.2 Errors are returned, and some come with a usable state

Vallado's code sets `satrec.error` to a small integer. The incumbent surfaces
that as a struct field, and elsewhere calls `log.Fatal` and `os.Exit` on bad
input — which is why `satellite.ValidateTLE` exists as a shield, and why
`FuzzValidateTLE` found a panic in `days2mdhms` within two seconds.

The new package returns errors, with named sentinels for each of Vallado's
conditions:

| sentinel | Vallado | state returned? |
| :--- | :--- | :--- |
| `ErrMeanMotion` | error 2 | no |
| `ErrEccentricity` | error 1 | no |
| `ErrPerturbedEccentricity` | error 3 | no |
| `ErrSemiLatusRectum` | error 4 | no |
| `ErrSubOrbital` | error 5 | no (at `New`) |
| `ErrDecayed` | error 6 | **yes** |
| `ErrKeplerNotConverged` | — (new) | **yes** |

The rule, stated once and tested: **`ErrDecayed` and `ErrKeplerNotConverged`
come with a populated state; every other error leaves the vectors zero.** Both
describe a result that exists and should not be trusted, which is a different
thing from a computation that could not be performed.

`ErrKeplerNotConverged` has no counterpart in the reference, which silently
stops after ten iterations and uses whatever it has. That silence *is* the 23333
divergence. Reporting it changes no number and turns a mystery into a message.

### 6.3 WGS-72 is the default, and WGS-84 is available but discouraged

A TLE's mean elements are the output of fitting observations *through SGP4 with
WGS-72 constants*. Handing them to a propagator configured for WGS-84 asks a
different model to interpret numbers this one produced. Measured on the same
suite, that single choice was worth 93× the error (p50 0.0346 → 0.0000 km);
astrogo shipped it for the whole life of the package and it is fixed separately.

`WGS84` stays available because a caller may have elements fitted that way, and
`WGS72Old` because Vallado keeps it for reproducing historical output. Both
carry doc comments saying when they are the wrong answer.

### 6.4 The gravity table is frozen in the package, and cross-checked against `constants`

The obvious objection to `gravity.go` is that astrogo already has a `constants`
package, and an Earth radius sitting in a propagator looks like a constant that
escaped it. Two things are true at once, and they pull in opposite directions.

**`constants` is genuinely missing WGS 72**, and says so itself —
`constants/wgs84.go`'s doc comment reads *"if a second ellipsoid standard is ever
needed (GRS80, WGS72, ...), add it as its own set at that point."* That point has
arrived, and a WGS 72 set is added there (a, 1/f, GM, ω) alongside the geocentric
gravitational constant that `WGS84Set` names in a comment but does not carry.
That is its own change, independent of this one, and lands separately.

**But SGP4 must not read from it.** SGP4's table is not a description of the
Earth; it is part of the model's definition. Vallado's `wgs84` entry uses
μ = 398600.5 km³/s², while the modern standard — and `constants.DE440` — give
398600.4355. Neither is wrong; they are different vintages, and SGP4 means the
older one, because the mean elements it propagates were fitted through code
holding that number. A propagator that read a live constant would silently change
every satellite position the day `constants` tracked a new realization. That is
precisely the failure mode astrogo just measured on the WGS-72/WGS-84 mix-up
(§6.3), and reading from `constants` would rebuild it with a longer fuse.

So `gravity.go` holds its own frozen table, and a **test asserts the
relationship** rather than leaving it to a comment:

- The package's WGS 72 radius and μ **agree** with the WGS 72 set in
  `constants`, converted — so a typo in either is caught by the other.
- The package's WGS 84 μ **deliberately disagrees** with the WGS 84 set in
  `constants` by 0.0582 km³/s², and the test asserts that gap with the reason
  attached, so that a future reader who notices the mismatch finds an answer
  instead of filing a bug.

This also closes the duplication the question came from: `ephemeris/satellite/regime.go`
currently carries a third private copy of these constants, and §6.8 deletes it
outright rather than re-plumbing it.

### 6.5 Checksums are a separate call from parsing

`ParseTLE` reads the fields. `VerifyTLEChecksums` checks the transport-integrity
digit. They are different questions — one is "are these elements", the other is
"did this text arrive intact" — and separating them has a concrete payoff:
Vallado's cases **33333, 33334 and 33335** exist to exercise SGP4's error
returns and carry check digits he never maintained, so astrogo currently refuses
them and tests 30 of 33 cases. With the two concerns separated, those three can
finally be propagated and their error returns asserted against what Vallado
built them to produce.

`satellite.NewFromTLE`, the front door, calls both. That is the layer where
"this came off a network feed" is true.

### 6.6 The deep-space resonance integrator is made pure

`dspace` is a fixed-step integrator that Vallado deliberately made stateful —
`atime`, `xli`, `xni` persist between calls so a forward march does not redo its
steps ("sgp4fix take out atime = 0.0 and fix for faster operation"). That is why
the incumbent's `satrec` is mutated by propagation, and why concurrent use of
one satellite is unsound.

The step grid is anchored at t = 0 with a fixed 720-minute step, so restarting
from zero every call produces *bit-identical* results to resuming — the carried
state is a memo, not a path dependency. The new implementation therefore keeps
the integrator state in locals, making `At` a pure function of `(p, tsince)`.

The cost is `|tsince|/720` steps for resonant deep-space orbits: two steps for a
day, 730 for a year, microseconds either way. It buys A6 — a `*Propagator` that
any number of goroutines can propagate at once with no lock. If a measured
workload ever needs the memo back, it returns as an explicit cursor type, not as
hidden mutation. Not before it is measured.

### 6.7 SGP4 keeps its own GMST, and the comment says why

`dscom`/`dsinit` need Greenwich sidereal time, and Vallado's `gstime` is the
1982 IAU expression evaluated on UTC-as-UT1. astrogo has `time.Time.GAST` — IAU
2006/2000A, with real EOP — and using it here would be a *worse* answer, because
the lunisolar terms were fitted against the 1982 expression and the model is
self-consistent, not accurate. The package therefore carries a private `gmst82`
with a comment saying exactly that, so nobody "improves" it later.

The same boundary in the other direction: TEME→GCRS is `satellite`'s job and
uses the good rotation. SGP4 produces TEME and stops.

### 6.8 What moves out of `ephemeris/satellite`

The rewrite deletes as much as it adds:

| today | after |
| :--- | :--- |
| `propagateECI`'s sub-second correction (~45 lines with its comment) | gone — §6.1 |
| `parseTLENumerics` (~80 lines reproducing the backend's string surgery exactly so it can predict what `strconv` will be handed) | gone — the parser is ours and returns errors |
| `checkEpochDay` (~40 lines predicting a `lmonth[12]` panic) | gone — no panic to predict |
| `ValidateTLE`'s `log.Fatal` shield | gone |
| `regime.go`'s 25-line re-derivation of `initl`'s perigee, with its own copy of the gravity constants | gone — `Propagator.PerigeeAltitude()` |
| `Satellite.Verified` | reassessed once A2 is measured; kept only if something still needs flagging |

`satellite.ValidateTLE` is public, so it stays as a documented forwarder to
`ParseTLE` + `VerifyTLEChecksums` rather than being removed. Its behavior does
not change.

### 6.9 Not in this work

- **OMM / JSON / XML element sources.** `Elements` being a struct rather than
  two strings makes this a small later addition; adding formats before anyone
  asks is API surface with no caller.
- **SGP8/SDP8, and any "improved" variant.** SGP4 is what TLEs are fitted with.
- **Covariance, drag-model tuning, or fitting elements from observations.** A
  propagator, not an orbit-determination system.
- **Replacing the Kepler iteration with a better solver.** It would stop
  reproducing the reference, which is the one thing this package must do. The
  limitation is reported (§6.2), not fixed.

---

## 7. Validation

The dependency's own suite passed while it was 3,438 km wrong. That is the
standard to design against.

**1. Vallado, at native `tsince` (`validation` tag).** All 33 cases, compared
against `tcppver.out` with no unit or time conversion between the reference's
argument and ours. Contract per A1. The `divergent` map must be empty; the
exclusion for 23333 is asserted by name with Vallado's own explanation attached,
and from both sides, so that if it ever stops diverging somebody is told.

**2. Differential against the incumbent (`validation` tag, temporary).** Both
implementations over every element set in the suite plus a corpus of real
CelesTrak sets, at one-minute steps across ±1440 minutes. Expected: agreement to
the whole-second workaround's residual everywhere *except* the seven cases of
§1.1, where the new implementation must be the one closer to Vallado. This is
the test that proves the migration changed nothing it should not have; it is
deleted in the PR that drops the dependency, and its findings are recorded in
`docs/VALIDATION.md` rather than left in a deleted file.

**3. Vallado's error cases.** 33333/33334/33335 propagated and their error
returns asserted against what each was constructed to trigger (§6.5).

**4. Fuzz.** `FuzzParseTLE` over arbitrary bytes; `FuzzPropagate` over element
values including the degenerate ones — zero eccentricity, zero inclination,
retrograde, e→1, n→0, huge `|tsince|`. Property: never panics, and either
returns an error or returns finite vectors. Never both zero and nil.

**5. Properties, not just fixtures.**
- Continuity: `|r(t+δ) − r(t)| ≈ |v|·δ` for small δ, across the near-Earth /
  deep-space boundary and across the `isimp` boundary.
- Reversibility: `At(+t)` then `At(−t)` from the same propagator agree, which is
  what A6/§6.6 claim and is meaningless if the integrator carries state.
- Determinism under concurrency: N goroutines × M times, compared against the
  serial result, under `-race`.
- No silent NaN: asserted, since a NaN position is how the incumbent signals
  failure today.

**6. Benchmarks.** `BenchmarkPropagateNearEarth`, `BenchmarkPropagateDeepSpace`,
`BenchmarkNew`, each with `ReportAllocs`, plus an allocation assertion via
`testing.AllocsPerRun` so A5 cannot silently regress.

**7. The existing suites keep running.** `satellite`'s own tests — ground track,
look angles, passes, `regime`, TLE numerics — are the integration check, and
none of their expected values may change except where §6.7 says so.

---

## 8. Delivery

Six pull requests, each green and independently reviewable. No stacking: each
branches from `main` once its predecessor merges, so CI actually runs.

| PR | contents | how it is judged |
| :-- | :--- | :--- |
| **1** | This document. The astrogo issue recording §1.2 with its measurement. | Is the plan right? |
| **2** | `sgp4` package skeleton: `doc.go`, `Elements`, `ParseTLE`, checksums, `Gravity`, `errors.go`. No propagation. | Parses every element set in the suite and every fuzz corpus entry without panicking; round-trips against the existing `parseTLENumerics` on all 33 cases. |
| **3** | `initialize.go` + `nearearth.go`. Near-Earth SGP4 only; deep-space element sets return a "not yet implemented" error. | The ~13 near-Earth cases of the suite meet A1. |
| **4** | `deepspace.go`. | All 33 cases meet A1. A2 measured and reported. |
| **5** | Differential test vs the incumbent, fuzz targets, property tests, benchmarks. | A5, A6, A7. |
| **6** | Switch `ephemeris/satellite` to `sgp4`; the deletions in §6.8; drop `go-satellite` from `go.mod`; update `docs/VALIDATION.md`, `README.md` and `CHANGELOG`. | A3, A4. No behavior change outside the seven cases. |

PR 3's split is not cosmetic. The near-Earth path is where the drag branch of
§1.2 lives, so it is where the headline claim is either confirmed or refuted —
and it is confirmed or refuted against 13 reference cases before a single line
of deep-space code exists.

---

## 9. Risks

**The remaining divergence is not the Kepler solver.** §1.3 shows 23333
unchanged by the drag fix and Vallado names the cause, but "named by the author"
is not the same as "measured here". PR 4 measures it: if a from-scratch
implementation reproduces 23333 exactly, the exclusion is wrong and comes out.

**`opsmode`.** Vallado's `'a'`/`'i'` modes differ in the node handling and in
which sidereal expression is used, and `tcppver.out` is generated under one of
them. Both are implemented; which one the reference file uses is determined by
measurement in PR 3 and written down, rather than assumed from the default.

**Numerical divergence from re-association.** Vallado's arithmetic is written in
a specific order, and "simplifying" `a*b + a*c` to `a*(b+c)` moves the last bits.
A1's 1e-6 km has room for that; anything that does not meet A1 is investigated
rather than accommodated by loosening it.

**Scope.** This replaces a propagator. It does not touch TEME→GCRS, ground
tracks, look angles, passes or magnitudes, and PR 6 is explicitly a
no-behavior-change PR outside §6.8 and the seven cases.
