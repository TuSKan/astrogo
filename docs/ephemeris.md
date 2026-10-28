# Ephemeris: contracts, measurements and the next architecture

The design for astrogo's ephemeris layer: what a state *means*, what today's
code costs on the workloads that call it, and which architecture to move to.

**Status: design, no production change.** Everything measured here comes from
`internal/workload`, a test-only package added alongside this document: nine
workload benchmarks, the experiments behind §5, and a prototype of the two
internals every option shares (§6.1) that reproduces today's provider bit for
bit. #803 records the decision this document re-examines; the numbers below
confirm it and change one detail of it (§7.4).

**Since then: the first change has shipped** (#802, §6.1). The record cache
is in `spk.Reader`, the prototype it replaces is deleted, and the measured
production numbers are in §6.1 beside the prototype's. **And #803's first
half** (§9, phases 3 and 4): a provider `Locate`s a body about its natural
center, `State.Center` is an `ID`, and `ephemeris.At` composes an `Instant`
whose places follow SOFA's order, seen from the geocentre. The site as an
observer is the next change.

**Recommendation in one paragraph.** First, make SPK evaluation read nothing
it has read before — each segment's directory at open, its last decoded record
published atomically (#802). Prototyped, that takes a geometric sky from the
kernel on disk from **360 µs to 3.8 µs with zero allocations**, bit-identical,
with no API change, and every option below needs it. Then the breaking change
#803 describes, now with its identity question answered (§3.1): IDs that are
NAIF codes, states that name their center, one `Instant` per epoch composing
them — **1.58 µs** for the same sky — and SOFA's order for apparent places.

---

## 1. What exists

| provider | answers | from | notes |
| --- | --- | --- | --- |
| `jpl.Provider` | geocentric `State`, ICRF, AU and AU/day | SPK types 2, 3, 21 | each `State` sums the target's chain to the barycenter and the Earth's, and differences them |
| `eph.Default()` | the same | `kepler` over a SOFA base: `Epv00`, `Plan94`, `Moon98`; Pluto by two-body elements | analytical; no kernel |
| `kepler.Provider` | the same | two-body propagation, classical or universal variables | delegates every other body to its base |
| `satellite.Satellite` | GCRS, geocentric | SGP4, then TEME → GCRS | `id` is its NAIF code, −100000 − NORAD number (#809); it was 0 |

And the places built on them:

- `eph.ApparentState` — the light-time-retarded *geocentric* state, which carried first-order aberration because the Earth moves back with the target, plus solar deflection. Replaced by `Instant.Apparent` (#803).
- `eph.AstrometricState` — the same with the Earth's displacement over the light time put back, from SOFA's `Epv00`. Replaced by `Instant.Astrometric` (#803).
- `coord.Context.GeocentricToObserved` (`coord/context.go:277`) — subtracts the site, rotates to the horizon, applies diurnal aberration and refraction.

## 2. Audit findings

Filed one per issue; the measurements are §4–5's.

| | finding | effect | |
| --- | --- | --- | --- |
| 1 | A type 2/3 segment's final instant read past its last record | de440s put the Earth 6×10⁶¹ km out at its last covered instant, no error | #807, fixed in #808 |
| 2 | astrogo IDs 1–12 fall through as NAIF codes | `State(Pluto)` is served as NAIF 9 against `core.Pluto`'s documented contract; the barycenter is refused as "no coverage"; NAIF 1, 2, 3, 9 cannot be asked for | #809 |
| 3 | Kepler makes heliocentric orbits geocentric with `Epv00`'s Earth, whatever its base | 4.00 km rms, 11.71 km max against de440s over 1900–2100; 1.6″ at 0.01 AU | #810 |
| 4 | The chunk cache holds one mutex across every lookup and across the backend read on a miss | 16 readers do less in aggregate than one (5.4 µs/op against 3.9 µs/state) | #811 |
| 5 | The satellite TEME → GCRS matrix is recomputed per state | 99% of a satellite state (87 µs against 0.66 µs for SGP4); it rotates 0.016″ per hour | #812 |
| 6 | A segment's frame code is parsed and never checked | nothing astrogo fetches is affected (181 segments, all frame 1); a caller's ECLIPJ2000 kernel would be chained 23.4° off | #813 |
| 7 | Every SPK evaluation reads its directory and its record again, allocating both | 60% of an evaluation with no I/O at all; on disk a sky thrashes the 1 MiB chunk cache (1.2 MB allocated per sky) | #802 |
| 8 | `AstrometricState` takes the Earth's displacement from `Epv00` even with a kernel | 85% of its profile; 166 µs against 7 µs for the apparent place | #803 |
| 9 | Each geocentric `State` re-derives the Earth | a 9-body sky evaluates 30 segments where 13 are distinct | #803 |
| 10 | `ephemeris.earthBarycentric` describes `Epv00` as "accurate to about 100 m" | SOFA documents 4.6 km rms; harmless where it is used (a difference over the light time) | #810 |
| 11 | Event search is dominated by rebuilding `coord.Context`, not by the ephemeris | 67% of `DayEvents` | #790 |

## 3. Scientific contracts

What every state and place means, independent of the architecture that
produces it. A contract the code cannot meet is a defect, not a footnote.

### 3.1 Target identity

**An `ID` is a NAIF integer code, and nothing else** (#809). It was one
`uint32` holding an astrogo enumeration (1–12) and NAIF codes above it, and
the two overlapped exactly where NAIF puts the system barycenters (after #636
and the small-body overlap `SmallBodyBase` already removed). The constants are
names for codes:

```go
type ID int32 // a NAIF integer code

const (
	SolarSystemBarycenter ID = 0
	EarthMoonBarycenter   ID = 3
	MarsBarycenter        ID = 4 // 1–9 are the system barycenters
	JupiterBarycenter     ID = 5
	Sun                   ID = 10
	Mercury               ID = 199 // body centers are x99
	Venus                 ID = 299
	Earth                 ID = 399
	Moon                  ID = 301
	Mars                  ID = 499
	Jupiter               ID = 599
)
```

A body center and a system barycenter are different points and get
different names; which one a planner observes is the planner's choice, made
visibly (`plan.NewJupiter` keeps the barycenter it uses today, whose distance
from the body center is measured in `core.ID`'s doc, 0.05″ at worst). A
provider never substitutes one for the other. Small bodies keep astrogo's
`SmallBodyBase + n`, the Horizons convention, and an Earth satellite is
−100000 − its NORAD number, NAIF's code for a satellite with no DSN number.

### 3.2 Origin

Every state names the point it is measured from, as an `ID` (`State.Center`).
A geometric state is "target relative to center"; there is no implicit
geocentre.

### 3.3 Axes

ICRF axes, labeled. A geocentric place is in the GCRS, kinematically
non-rotating and aligned with the ICRF. An SPK segment is used only if its
frame is NAIF 1 ("J2000", which DE4xx and every kernel astrogo fetches mean as
the ICRF) — or a fixed inertial frame it knows how to rotate — and refused
otherwise (#813). TEME exists only inside the satellite package and in
`coord.Context.TEMEToGCRS`, which turns it to GCRS through the Context's own
precession-nutation rather than a fresh evaluation of it (#812).

### 3.4 Epoch and time scale

The ephemeris argument is TDB; an SPK's is TDB seconds past J2000, computed
straight from the input time and never through a summed Julian Date (#150). A
caller's `time.Time` may be in any scale; UTC goes through the leap-second
table the kernel's own LSK registered. Light time is solved in TDB. UT1 and
polar motion enter only at the observer (§3.6), through EOP.

### 3.5 Units

Kernel arithmetic is in km and km/s, the kernel's own units, and composition
happens there. A `State` at the API is in AU and AU/day, with the IAU 2012
au = 149 597 870.700 km and the 86 400 s TDB day.

### 3.6 Places

The four places, defined as SOFA defines them (`iauApco13`, `iauAtciq`,
`iauLd`, `iauAb`):

| place | position | velocity |
| --- | --- | --- |
| **geometric** | r_target(t) − r_center(t), both at TDB t | the derivative |
| **astrometric** | r_target(t − τ) − r_observer(t), τ solving \|·\| = cτ | v_target(t − τ) − v_observer(t) |
| **apparent** | the astrometric direction, deflected by the Sun (`iauLd`, true geometry) and then aberrated by the observer's barycentric velocity (`iauAb`); the astrometric range | the astrometric velocity: deflection and aberration bend a ray, not a motion |
| **observed** | the apparent topocentric direction through CIRS, Earth rotation, polar motion, the horizon and refraction | — |

The observer is the geocentre or a site, and a site's barycentric position is
the Earth's plus the site's GCRS position, so for a site τ is the light time
*from the site*. The astrometric velocity omits the (1 − dτ/dt) factor, whose
relative size is |ṙ|/c ≤ 10⁻⁴. The Shapiro delay is omitted: a few hundred
microseconds at worst, meters of motion, microarcseconds of direction.

One place astrogo produces departs from these definitions:

- The observed place subtracts the site *after* aberration has been applied
  to the geocentric direction. Measured in §5.3: at most 0.035″ for the Moon.
  The site as an observer of `Instant.Apparent` removes it.

The apparent place did too until #803, applying aberration to first order
through the retarded geocentric state: against Horizons' quantity 2, with
Uranus and Neptune in the comparison, its tail sat 32 mas above the median.
In SOFA's order it sits 2 mas above.

## 4. Measured baseline

`go test -tags=integration -run='^$' -bench=. -count=6 ./internal/workload/`
on an i9-11980HK, Windows 11, Go 1.27.2, at commit 8181ca5f; de440s
(1849–2150), and nep097 where named. Medians of six with benchstat's 95%
interval; counts are exact. The kernel is held three ways, so that reading is
separated from evaluating:

- **disk** — `jpl.NewProvider`, as a caller gets it: the cached file through `remote.Open`'s chunk cache;
- **mem** — the same chunk cache over a `mem://` copy;
- **bytes** — a `bytes.Reader`: no cache and no I/O.

The analytical paths (`default`, Kepler) show intervals of 20–87%: long
trigonometric loops on a laptop CPU whose clock moves under them. Read those
medians as an order of magnitude; the JPL paths are within 1–9%.

**B1, one body at one epoch.**

| | disk | mem | bytes | evaluations | allocs |
| --- | ---: | ---: | ---: | ---: | ---: |
| Mars | 1.87 µs | 1.97 µs | 1.67 µs | 3 | 15 |
| Moon | 2.65 µs | 2.58 µs | 2.31 µs | 4 | 20 |
| Mars, `eph.Default()` | 75 µs ±81% | | | | 0 |
| Moon, `eph.Default()` | 6.6 µs | | | | 0 |
| Pluto by Kepler, SOFA base | 70 µs ±56% | | | | 0 |
| Pluto by Kepler, de440s base | 75 µs ±87% | | | **0** | 0 |

The last row is #810: the kernel is never consulted.

**B2, the sky (Sun, Moon, eight planets less the Earth) at one epoch.**

| | disk | mem | bytes | `default` |
| --- | ---: | ---: | ---: | ---: |
| geometric | 360 µs | 207 µs | 15.9 µs | 498 µs |
| — evaluations, allocated | 30, 1.2 MB | 30, 1.2 MB | 30, 20 KB | |
| apparent | | | 86.7 µs, 48 states, 154 evaluations | 2.62 ms |

On disk the sky is 23× the same evaluations from memory. A sky touches 26
places in the file — the directory and the record in force of each of 13
segments — and the chunk cache holds 16 chunks of 64 KiB, so 18 of each sky's
60 reads miss and reload a chunk: the 1.2 MB allocated per sky (#802).

**B3, the Moon over 1000 epochs, per state.**

| | disk | mem | bytes |
| --- | ---: | ---: | ---: |
| 1-minute steps, in order | 2,640 ns | 2,505 ns | 2,358 ns |
| 1-minute steps, shuffled | 2,661 ns | 2,621 ns | 2,387 ns |
| 4-day steps, in order | 2,772 ns | 2,637 ns | 2,129 ns |
| 4-day steps, shuffled | 3,925 ns | 3,275 ns | 2,118 ns |

Twenty allocations per state throughout.

**B4, light time** (de440s from memory; `default` in brackets).

| | Moon | Mars | Neptune |
| --- | ---: | ---: | ---: |
| astrometric | 166 µs (194 µs) | 213 µs (421 µs) | 159 µs (312 µs) |
| — states, evaluations | 3, 12 | 4, 12 | 3, 9 |
| apparent | 7.3 µs (117 µs) | 7.7 µs (263 µs) | 8.9 µs (312 µs) |
| — states, evaluations | 4, 14 | 5, 15 | 6, 18 |

The astrometric place is 23× the apparent one on a kernel because 85% of it is
`Epv00` (finding 8).

**B5, a moon about its planet.**

| | through two geocentric states | native segments |
| --- | ---: | ---: |
| Moon about the Earth | 4.04 µs, 8 evaluations | 1.07 µs, 2 |
| Triton about Neptune (nep097) | 4.51 µs, 8 evaluations | 1.26 µs, 2 |

**B6, the observed sky from one site** (Barcelona, one `Context`): 79 µs from
memory, 378 µs from disk, 2.54 ms on `default`, with the same 48 states and
154 evaluations as B2's apparent sky — which, at 86.7 µs, is all of it.

**B7, event search.**

| | de440s, bytes | `default` | states | evaluations |
| --- | ---: | ---: | ---: | ---: |
| Mars oppositions over a year | 5.6 ms | 223 ms | 3,182 | 9,546 |
| a day of Moon events | 5.9 ms | 21 ms | 524 | 1,834 |
| a day of Mars events | 5.9 ms | 41 ms | 670 | 2,010 |

The day's events cost the same for either body on a kernel because they are
not ephemeris-bound: 67% of the profile is `coord.Context` being rebuilt
(#790).

**B8, a satellite** (ISS, 2008-09-20): SGP4 663 ns; `State` 87 µs; a 6-hour
pass at 1-minute steps 27.5 ms (#812).

**B9, sixteen goroutines on one provider**, the Moon at random epochs across a
decade, aggregate per state: disk 5.45 µs, mem 4.25 µs, bytes 0.52 µs (#811).

## 5. Experiments

Each asserts only what makes its measurement trustworthy — a control, or a
bound rounding predicts — and logs the measurement (`-v`).

### 5.1 One evaluation, taken apart

The Moon's segment (13 coefficients per axis), from memory:

| part | ns | allocs |
| --- | ---: | ---: |
| read the directory | 54 | 2 |
| read the record | 231 | 2 |
| Chebyshev, position and velocity | 202 | 0 |
| Chebyshev, position only | 118 | 0 |
| the whole `EvaluateSegment` | 410 | 4 |

Reads are 60% of an evaluation before any I/O. A type 21 evaluation (a
Horizons comet kernel) costs 995 ns and reads three times, its epoch table
included.

### 5.2 Native segments against the barycentric route

`TestNativeMoonAgreesWithTheBarycentricRoute`: the Moon about the Earth from
segments 301 and 399, against the difference of two geocentric states, 100
epochs over 2026–27. They differ by at most **3.0×10⁻⁸ km** (30 µm); Triton
about Neptune by **1.0×10⁻⁶ km** (1 mm). That is rounding at the barycentric
route's scale, 1 AU and 30 AU; the native route rounds at the moon's own
distance instead, and is 3.6–3.8× cheaper (B5).

### 5.3 Parallax before or after aberration

`TestObservedPlaceParallaxAndAberrationOrder`: today's observed place against
the SOFA-order one — the astrometric place from the site's barycentric
position, then `iauAtciq`'s deflection and aberration with the site's own
velocity — with refraction off in both, hourly through June 2026 from
Barcelona, above 5° and 20° from the Sun.

| | epochs | max | rms |
| --- | ---: | ---: | ---: |
| Mars, the control | 407 | 0.0040″ | 0.0030″ |
| the Moon | 270 | **0.035″** | 0.010″ |

That table was measured before #831, when the geocentric apparent place was
the geocentric state at the retarded epoch. That form carries aberration as
the Moon's 1 km/s geocentric velocity times the light time, so taking the site
off afterward misplaced it little.

#831 made the geocentric place SOFA-order: the astrometric place, aberrated as
a whole by the Earth's velocity. At the Moon's distance that is a 20.5″
displacement along the geocentric line of sight, and the parallax turns the
line of sight from the site by up to 0.95°. So the same route then misplaced
the Moon by up to **0.333″**: Barcelona, June 2026, 271 epochs, `Default()`
(#840). That is the 0.3″ first estimated here, for a reason other than the
one first given.

**What the route is now.** `ephemeris.Site(ctx)` is an observer at the
Context's site, built from `ObsVec` and `ObsVel`. Its apparent place is
SOFA's topocentric place:
- the light time is taken to the site;
- deflection is for the body where it is;
- aberration uses the Earth's and the site's velocity together.

`coord.Context.TopocentricToObserved` then rotates the place into the horizon
and refracts it, with no further parallax or aberration. `plan` observes its
own ephemeris targets this way.

`TestSiteApparentIsSOFAsTopocentricPlace` holds it to `iauAtciq`/`iauAtioq` on
the astrometric place from the site. The two differ by the deflection alone:
Atciq deflects as for a star at infinity, 4.07 mas·cot(E/2), while the Moon at
its distance is deflected under 0.01 mas.

| | max from Atciq |
| --- | ---: |
| the Moon, from the Site | 22.4 mas, at E = 20° |
| Mars, from the Site | 6.4 mas, at E = 31° |
| the Moon, geocentric route | 333 mas |

At every epoch, the test asserts that each Site place stays within the star
deflection at its elongation plus 0.1 mas. It also asserts that the
geocentric route exceeds that bound by more than 100 mas, so the bound can
fail.

`TopocentricToObserved` itself agrees with SOFA's `iauAtioq` to 0.0003 µas.

### 5.4 SOFA's Earth against the kernel's

`TestEpv00EarthAgainstTheKernel`: `Epv00`'s heliocentric Earth against
de440s's, 2000 epochs over 1900–2100: **4.00 km rms, 11.71 km max** (1942).
SOFA documents 3.7 and 11.2 km against DE405. This is the Earth every Kepler
body is placed against today (#810).

## 6. The six optimizations, measured

### 6.1 The prototype, and the record cache it became

`internal/workload/prototype_test.go` (since deleted, replaced by the
production cache below) built the two internals every option needs and
nothing else:

- **a record cache** — each type 2 segment's directory read once, its last
  record decoded and published through an `atomic.Pointer`, immutable once
  published, so readers share it without a lock;
- **an instant** — barycentric chains summed in the provider's order and
  memoized per epoch, so a body's chain is evaluated once whatever asks for
  it.

`TestPrototypeReproducesTheProvider` (since deleted) held it to
`jpl.Provider.State` (since renamed `Locate`, #803) **bit for bit** over a year of hourly epochs for the
whole sky, 78,840 states that cross thousands of records, and was canaried:
changing τ's division to a multiplication by the reciprocal failed it.

| workload | today | record cache | + instant | + position only |
| --- | ---: | ---: | ---: | ---: |
| geometric sky, disk | 360 µs | 3.81 µs | **1.58 µs** | 1.39 µs |
| geometric sky, bytes | 15.9 µs | 3.93 µs | **1.54 µs** | 1.19 µs |
| — allocations | 188 (disk) | 0 | 0 | 0 |
| Moon, 1-minute steps, disk | 2,640 ns/state | | **679 ns** | |
| Moon, 4-day steps, disk | 2,772 ns/state | | 1,629 ns | |
| 16 readers, one epoch, disk | | | 69 ns/op | |
| 16 readers, random epochs, disk | 5,448 ns/op | | 1,994 ns/op | |

**The record cache in production (#802).**

- **What it holds.** `spk.Reader` keeps each evaluated segment's directory and its last decoded record. Types 2 and 3 keep their four-word directory; type 21 keeps its epoch table.
- **How it's shared.** Records are published through an `atomic.Pointer` and keyed by the segment's addresses.
- **It's the reference arithmetic.** `TestRecordCacheReproducesDe440s` holds it to an uncached reader bit for bit over every de440s segment, a year of hourly epochs in order and shuffled.
- **Allocation contracts.** `TestRecordCacheHitAllocatesNothing` and `TestStateFromCachedRecordsAllocatesNothing` make zero allocations a contract. The segment lookup's escaping pointer, the one allocation the cache left, went in the same change.

The workload benchmarks, before and after on the same machine (benchstat medians of 6, every row p = 0.002):

| workload | before | after |
| --- | ---: | ---: |
| geometric sky, disk | 355 µs | **7.6 µs** |
| geometric sky, `mem://` | 151 µs | 7.5 µs |
| geometric sky, bytes | 16.4 µs | 6.8 µs |
| — allocations | 188 | **0** |
| the Moon at one epoch, disk | 2.57 µs, 20 allocations | 0.96 µs, 0 |
| the Moon at 1-minute steps, disk | 2,525 ns/state | 918 ns/state |
| apparent sky, bytes | 87.5 µs | 49.2 µs |
| observed sky from a site, disk | 388 µs | 54 µs |
| Mars oppositions over a year | 6.0 ms, ≈19,100 reads | 3.3 ms, 963 reads |
| a day of Moon events | 6.0 ms | 5.9 ms (not significant: `Context`-bound, #790) |
| 16 readers, random epochs, disk | 5.3 µs/op | 2.0 µs/op (misses still meet #811's lock) |

The three backends are now within noise of each other, as the prototype
predicted. The sky costs about twice the prototype's 3.8 µs because each
`State` still goes through the provider: its read lock, the segment lookup,
UTC → TDB, and the Earth's chain once per body, which is the instant's job.

### 6.2 Verdicts

1. **Native target–center segments.** 3.6–3.8× on a moon about its planet,
   rounding at the moon's distance rather than the barycenter's (§5.2). It falls out of states
   that name their center and an instant that finds the common ancestor; it
   is not a separate feature.
2. **Same-epoch reuse.** 30 evaluations become 13 on the sky; 3.81 → 1.58 µs
   with the record cache in place. Only an instant can own it: a memo hidden
   in the provider would need keying by a float epoch, invalidation and a
   lock — the B9 problem again.
3. **Segment and coefficient cache.** The largest single gain: 95× on the sky
   from disk, every allocation gone, the three backends made equal, and no API
   change. The first change (§9).
4. **Batch APIs.** Not worth an API. After 1 and 3 the sky costs 1.4–1.6 µs —
   thirteen Chebyshev sums and little else — and a root finder, the heaviest
   caller (B7), asks one epoch at a time and cannot batch. A track can be a
   loop over instants.
5. **EOP and observer context.** The largest remaining cost in event search,
   and not the ephemeris's: two thirds of `DayEvents` is `coord.Context`'s
   rebuild (#790). Satellites have the same shape (#812): the observer's
   precession–nutation is computed per sample when it moves 0.016″ an hour.
6. **Position-only evaluation.** 12–22% on the sky. Cheap — `EvalChebyshev`
   already takes the flag — and worth an instant-level option for samplers
   that need no velocity; not worth a second provider method.

## 7. Architecture options

Three, with the signatures each would ship.

### 7.1 A — keep `State`, cache beneath it

```go
// ephemeris/core — unchanged
type Provider interface {
	State(id ID, t time.Time) (State, error) // geocentric, ICRF, AU, AU/day
	Close() error
}
```

The record cache goes into `spk`; `AstrometricState` takes the Earth from
`p.State(SolarSystemBarycenter, t)` once #809 maps it; Kepler takes its Earth
from the base (#810). (Written against the interface as it was: #803 since
renamed `State` to `Locate` and replaced `AstrometricState` with
`Instant.Astrometric`.)

- **For:** no caller changes; most of 6.2.3's gain.
- **Against:** a geocentric-only interface cannot say "about Neptune", so a
  moon costs 8 evaluations instead of 2 and rounds at 30 AU (§5.2);
  same-epoch reuse has nowhere to live but a hidden, locked memo; the
  observer stays the geocentre, so §5.3 stays.

### 7.2 B — natural-center providers and one instant per epoch (#803)

```go
// ephemeris/core
type ID int32 // a NAIF code (§3.1)

type State struct {
	Pos, Vel vector.Vec3 // AU, AU/day, ICRF axes
	Center   ID          // what Pos is measured from
	Frame    Frame
}

// Provider answers a body about the center it computes it from: a JPL
// kernel about the segment's center, SOFA planets about the Sun, the Moon
// and satellites about the Earth, a Kepler orbit about its fitted center.
type Provider interface {
	Locate(id ID, t time.Time) (State, error)
	Close() error
}

// ephemeris
type Instant struct{ /* epoch, provider, memo of barycentric states */ }

func At(p Provider, t time.Time) *Instant

func (in *Instant) Geometric(target, center ID) (State, error)
func (in *Instant) Astrometric(target ID, obs Observer) (State, error)
func (in *Instant) Apparent(target ID, obs Observer) (State, error)

// Observer is the geocentre (the zero value) or a site, whose barycentric
// position and velocity come from a coord.Context at the same epoch.
type Observer struct{ ctx *coord.Context }

func Geocenter() Observer
func Site(ctx *coord.Context) Observer
```

`Locate` replaces `State` rather than redefining it: a forgotten caller that
still compiled would be 1 AU wrong. `Geometric` composes two `Locate` chains
to their lowest common ancestor, which is where native segments come from.
`Astrometric` and `Apparent` evaluate the observer once per instant and only
the target's chain per light-time iteration, then apply `iauLd` and `iauAb`
in SOFA's order.

- **For:** every contract in §3 is expressible; 6.2.1, 6.2.2 and 6.2.3 are
  all reached; the observer becomes a site, which removes §5.3; Kepler,
  SOFA, JPL and satellites compose because each says where it is measured
  from.
- **Against:** a breaking change through `ephemeris`, `plan`, `magnitude` and
  `coord`'s callers.

### 7.3 C — a query engine

```go
type Place uint8 // Geometric, Astrometric, Apparent

type Query struct {
	Targets []ID
	Center  ID
	Times   []time.Time
	Place   Place
}

func Evaluate(p Provider, q Query, out []State) error // len(out) = targets × times

type BatchProvider interface {
	LocateMany(ids []ID, ets []float64, out []State) error
}
```

Requests grouped by segment and record, evaluated in structure-of-arrays
loops.

- **For:** the most throughput for ephemeris generation and sky maps.
- **Against:** the callers that cost the most cannot use it (6.2.4); two
  paths, scalar and batch, to keep bit-identical; a second provider method.

### 7.4 Decision

**B, built on A's internals, without C.** A's caches are the first change
because every option needs them and they change no API. B is the contract —
it is the only option in which §3 can be written down as a type. C waits for
a workload that shows a need; nothing measured here does.

One detail of #803 changed twice. It proposed taking the centers at the
retarded epoch from the instant by Taylor extrapolation, with a stated error
per center. The review dropped that: with records cached an evaluation is
about 200 ns (§5.1), so the instant evaluated the target's whole chain at
t − τ instead, exact, with no error budget to state. Only the observer is
held at t, which is what the definition says.

That premise holds for a kernel and fails for `Default`, where the
evaluation at t − τ is SOFA's `Epv00`, 34 µs, and was half of every apparent
place (#791). So the extrapolation is back, for one body and under a bound.
The target is evaluated at t − τ about B, the first body its chain shares
with the Earth's, and B is carried back from its state at t:

- **The Earth**, for the Moon: under the Sun's pull, while τ ≤ 71.5 s.
- **The Sun**, for the planets: linearly, while τ ≤ 2.60 h.

Those are the light times over which what each model leaves out moves a place
by at most 1 µas, a·τ/2c. That is the Moon's pull on the Earth, 4.07e-5 m/s²,
and every planet's on the Sun at its perihelion, 3.11e-7 m/s². Beyond them B
is evaluated, as before, so Neptune and Uranus for part of its year keep two
evaluations.

On `Default` through 2026, every carried place is within 1 µas of the exact
one, plus the exact path's own rounding at the target's distance. The worst
are the Moon at 4.96 µas, inside the 6.51 µas that rounding allows there, and
Uranus at 0.69 µas. Every place out to Saturn takes one `Epv00` series
instead of two. On a JPL kernel, B is the Earth-Moon or the Solar System
barycenter, so nothing is carried for a planet.

## 8. Correctness gates

Performance is reported and never gates (CLAUDE.md); these gate.

1. **Bit identity for refactors that claim to change only work.** The record
   cache and the instant's geometric path reproduce the old provider to the
   last bit over a year of hourly epochs for the sky, as
   `TestRecordCacheReproducesDe440s` and `TestRecordCacheIsBitIdentical` do
   for the record cache, canaried. Comparing two
   *different* arithmetic paths bit for bit is not allowed (it fails under
   arm64's FMA); those get bounds.
2. **Independent references.**
   - JPL Horizons vectors (`EPHEM_TYPE=VECTORS`, explicit `CENTER`) for
     geometric states, Sun through Neptune, the Moon, Triton with nep097,
     a numbered asteroid — tolerance from the measured de440s-against-Horizons
     residual with the kernel named, never a round number.
   - Horizons quantities 1 and 2 for astrometric and apparent RA/Dec, the
     validation suite extended to Uranus and Neptune (it stops at Saturn,
     which hid #803's 9.8 mas scatter), the known +49 mas RA term (#260)
     removed before the bound.
   - Horizons topocentric azimuth and elevation with refraction off, against
     the observed place.
   - SOFA's own test vectors for `Ab`, `Ld` and `Apco13`, read from the gofa
     module cache, not recalled.
3. **Invariants with no reference.** Native and barycentric routes agree to
   rounding (§5.2); each segment's last instant continues its record (#807);
   the light-time residual |r(t − τ) − r_obs(t)| − cτ is under a microsecond
   of light; scale invariance (`ephemeris/scaleinvariance_test.go`);
   concurrent readers return what a sequential reader does, under `-race`.
4. **Allocation contracts.** A cached record evaluation and
   `Instant.Geometric` allocate nothing; `testing.AllocsPerRun`, untagged,
   in every CI job.
5. **Identity.** Every named `ID` resolves to the NAIF code it names, and an
   unknown one is refused, never reinterpreted (#809).

## 9. Phases

1. **This change.** The document and `internal/workload`; no production code.
2. **SPK evaluation without reads (#802) — done.**
   - **What it does.** A segment's directory and its type 21 epoch table are read on first use; its last record is decoded and published atomically; a hit allocates nothing.
   - **Gates.** 1, 3 and 4.
   - **Measured** (§6.1): the sky from disk 355 → 7.6 µs, a minute track 2.5 → 0.92 µs per state, all three backends equal.
   - **No API change.**
3. **Identity, center and frame (#809, #813, the first half of #803) — done.**
   `ID` as a NAIF code, `State.Center` as an `ID`, `Locate`, segments checked
   for their frame. Breaking.
4. **`Instant` and SOFA-order places (the rest of #803's first PR).** Done
   for the geocentre, with #791 and #792's repeated Earth and #810, by
   composition. The site as an observer is next, and with it §5.3.
   Gates 2 and 3.
5. **Kepler (#803's second PR, docs/kepler.md).**
6. **In parallel, independent of 3–5:** the observer context (#790), the
   satellite matrix (#812), the chunk cache's lock (#811).

## 10. Reproducing

```bash
go test -tags=integration -run='^$' -bench=. -count=6 ./internal/workload/ | tee new.txt
benchstat old.txt new.txt
go test -tags=integration -count=1 -v -run 'Test' ./internal/workload/
```

The suite downloads nothing beyond what jpl's own tests do: de440s and its
leap-second file, and the IERS bulletin. nep097 and the Horizons comet kernel
are used only if a previous consented run cached them; otherwise those cases
skip and say why.
