# Kepler: the numerical contract and the solver

The design for astrogo's two-body propagation: what `kepler.SolveKepler` and
`Elements.StateAt` should promise, measured against an exact reference, and the
algorithm that keeps the promise.

**Status: design, no production change.** The measurements come from
`internal/keplerlab`, a test-only package added with this document: a
multiprecision oracle, seven candidate solvers, and the contract test the
proposal passes. The orbit-centre question (heliocentric or barycentric fits)
is #803's second half and is not re-argued here.

**Recommendation in one paragraph.** Replace the solver with Markley's (1995)
starter and fifth-order correction evaluated on a *stable* residual, solve on
an exactly reduced mean anomaly, and build the state from cancellation-free
forms. Measured against the exact root, that is within **1.52 ulps of E
everywhere** — eccentricity up to the last float64 below 1, mean anomaly down
to 10⁻²⁰ — where today's solver is off by up to 2.75×10¹² ulps in position near
perihelion of near-parabolic orbits; and it is **about twice as fast**. The
"accuracy limit" the recent literature attributes to every derivative-based
method is a property of the naive residual, not of the problem, which §2.2
shows is never ill-conditioned in relative terms.

---

## 1. What exists

`ephemeris/kepler` propagates two ways:

| path | used for | how |
| --- | --- | --- |
| classical | `NewElements` (0 ≤ e < 1) | `SolveKepler` for E, then x = a(cos E − e), y = a√(1−e²)·sin E, Ė = n/(1 − e·cos E) (`perifocalClassical`) |
| universal | `FromPerihelion` (any e ≥ 0) | universal anomaly χ by bracketed Newton, Stumpff functions, Lagrange coefficients (`perifocalUniversal`) |

Both end in `orient`: three rotations of the position and three of the
velocity, each computing its own sine and cosine, then the rotation by the
J2000 ecliptic's obliquity.

`SolveKepler` is Newton's method on the direct residual E − e·sin E − M from
M + e·sin M (±π above e = 0.9), with M wrapped to (−π, π] since #781 and the
answer returned in [0, 2π). Its only caller outside tests is
`perifocalClassical`; `StateAt`'s callers are `kepler.Provider`,
`catalog/sbdb` and tests.

### Findings

1. **The residual and the slope cancel near perihelion as e → 1.** E − e·sin E
   is evaluated with an absolute rounding of about ε·E while its value near
   the root is M, and 1 − e·cos E with an absolute rounding of about ε while
   its value is (1 − e) + e·(1 − cos E). Both are the textbook form and both
   fail exactly where an orbit is closest to parabolic (§3).
2. **The state is built from cancelling forms.** cos E − e, 1 − e·cos E and
   √(1 − e²) each lose the digits that 1 − e has.
3. **The answer in [0, 2π) puts E near 2π just before perihelion,** where its
   sine carries an absolute error of an ulp of 2π, 8.9×10⁻¹⁶, against a value
   that may be far smaller.
4. **E(−0) = +0, and E(−M) ≠ −E(M) at 533 of the sweep's points.** The
   equation is odd in M; its solution should be odd bit for bit.
5. **Nothing about the orbit is computed once.** The mean motion, √(1 − e²)
   and the six orientation sines and cosines are recomputed on every call.
6. **The Earth a heliocentric orbit is placed against is SOFA's `Epv00`,
   whatever the base provider is** — 4.0 km rms, 11.7 km at worst against
   de440s (#810).

## 2. The contract

### 2.1 What the solver promises

- **Domain.** 0 ≤ e < 1, up to `math.Nextafter(1, 0)`; any finite M with
  |M| < 2⁵². At |M| ≥ 2⁵² a float64 no longer resolves the phase to a radian,
  and the solver refuses rather than returning a number that means nothing.
- **Reduction.** M is reduced by 2π *exactly*: the result is the float64
  nearest M − 2πk for the integer k that brings it into [−π, π], taken with
  three-part 2π and fused multiply-adds, so a phase a hair past a whole
  revolution keeps its relative precision. The product k·2π must be rounded
  by an explicit `float64(…)` conversion before its exact error is
  subtracted: Go may fuse a product into a later addition across statements,
  arm64 does, and the lab's first macOS CI run failed at every phase past a
  revolution until the conversion forbade it.
- **Range and symmetry.** E is in [−π, π]; E(−M) = −E(M) bit for bit, and
  E(±0) = ±0.
- **Accuracy.** |E − E*| ≤ 2 ulps of E*, where E* is the exact root for the
  exactly reduced M — relative, everywhere in the domain.

### 2.2 Why relative accuracy is the right promise

The relative condition number of E with respect to M is

    κ = (M / E) · (dE/dM) = (E − e·sin E) / (E · (1 − e·cos E)).

For E in (0, π/2), κ ≤ 1 because tan E ≥ E; for E in [π/2, π], because
E·cos E ≤ 0 ≤ sin E. **So κ ≤ 1 everywhere**: a relative perturbation of M
never produces a larger relative perturbation of E, near perihelion of a
near-parabolic orbit included. Kepler's equation is well conditioned; only its
usual *evaluation* is not.

That evaluation is the residual. Computed as E − e·sin E − M it carries an
absolute rounding of about ε·E, so the root it determines is uncertain by
ε·E/(1 − e·cos E) — exactly the "limiting accuracy" of Tommasini & Olivieri
(2021; 2022, eq. 21), which they hold applies to any method using
derivatives. Computed instead as (1 − e)·E + e·(E − sin E) − M, with E − sin E
from its series, every term has the sign of M and the rounding is about ε·M,
so the uncertainty is ε·M/(1 − e·cos E) = ε·κ·E ≤ ε·E. The limit belongs to
the formula, and Markley (1995) had already removed it.

### 2.3 What the state promises

The state cannot promise a fixed number of ulps, because its sensitivity to E
grows as a/r = 1/(1 − e·cos E) — a hundred near perihelion at e = 0.99 — and E
itself is a float64. It promises the solver's contract carried through that
sensitivity, plus two ulps for its own arithmetic:

    |Δr| ≤ |∂r/∂E| · 2 ulp(E) + 2 ulp(|r|)
    |Δv| ≤ |∂v/∂E| · 2 ulp(E) + 2 ulp(max(|v|, n·a))

The velocity's floor is n·a, the speed on the circle of radius a, because near
aphelion of a near-parabolic orbit the speed is orders of magnitude below it
and set by π − E, which carries E's own rounding whatever solved for it.

### 2.4 Phase

M = M₀ + n·Δt is formed with one rounding (a fused multiply-add) and reduced
exactly. What remains is the inputs' own: n's last bit times Δt, and Δt's
last bit times n. The solver does not pretend to know more than the phase it
is given.

## 3. The candidates

Each implemented from its primary source in `internal/keplerlab`, and none
copied: the methods are published mathematics, and no published code is used.

| | source | licence of the source | idea |
| --- | --- | --- | --- |
| production | `kepler.SolveKepler` | — | Newton on the direct residual |
| Markley, published | Markley 1995, *Kepler Equation Solver*, NASA GSFC (NTRS 19950021346), eqs. 2–29 | US Government work, public use permitted | a cubic built on a Padé approximation to sin E, solved with Stumpff's rationalization, then one fifth-order correction |
| Markley, stable | the same with his own eqs. 30–33 | as above | the slope as (1−e) + 2e·sin²(E/2) and the residual without cancellation — here from an exact-coefficient series for E − sin E instead of his Padé fit, so no seventeen-digit coefficient is transcribed from a scan |
| Mikkola, published | Mikkola 1987, *Celest. Mech.* 40, 329, eqs. 6–9, 24 | © Reidel; the method only | a cubic in s = sin(E/3), Cardano, an s⁵ correction, then Burkhart & Danby's dx₄ — the same recurrence as Markley's δ₅ |
| Mikkola, stable | the same on the stable residual | as above | |
| ENRKE | Tommasini & Olivieri 2022, *A&A* 658, A196, §2.1 | © ESO; the method only | a rational seed, one Odell–Gooding δ₁₃₁ step, Newton with their stopping rule, and bisection in their "critical region" e > 0.99, M < 0.0045 |
| Newton, stable | proposed alternative | — | Markley's starter, then bracketed Newton on the stable residual |

### 3.1 The oracle

`math/big` at 256 bits. π is computed by Machin's formula and by the
Gauss–Legendre AGM, and the two must agree to 1,356 bits; sin and cos by
Taylor series after an exact reduction; the root by Newton held inside the
bracket [M, min(M + e, π)] the equation proves. The oracle's own tests check
sin² + cos² = 1, sin(π/6) = ½ and a residual at the working precision from
e = 0 to `Nextafter(1, 0)` and M up to `MaxFloat64`.

Every error below is against the **exact root of the equation the candidate
was given** — not against another float64 solver, and not against an M
re-derived from a float64 E, which would build the candidate's own rounding
into the reference.

### 3.2 Accuracy

Thirteen eccentricities from 0 to `Nextafter(1, 0)`; M log-spaced from 10⁻²⁰
to π in both signs, the float64s either side of whole revolutions, and
phases up to 10¹². Worst error in ulps of E:

| | e ≤ 0.5 | 0.5 < e ≤ 0.99 | e > 0.99, \|M\| ≥ 0.0045 | critical: e > 0.99, \|M\| < 0.0045 | \|M\| > π |
| --- | ---: | ---: | ---: | ---: | ---: |
| production | 6.6×10¹⁵ | 6.6×10¹⁵ | 24 | 6.6×10¹⁵ | 4.5×10¹⁵ |
| Markley, published | 0.75 | 74 | 13 | 3.5×10¹² | 4.5×10¹⁵ |
| Mikkola, published | 435 | 1,140 | 15 | 3.5×10¹² | 4.5×10¹⁵ |
| ENRKE | 0.90 | 70 | 15 | 3.6×10¹² | 4.5×10¹⁵ |
| **Markley, stable** | **0.75** | **1.52** | **0.81** | **0.93** | **1.16** |
| Mikkola, stable | 435 | 1,140 | 3.2 | 1.2 | 1.7 |
| Newton, stable | 0.72 | 1.16 | 1.19 | 0.93 | 1.16 |

How to read the large numbers:

- **6.6×10¹⁵ ulps is an E lost entirely.** Production answers in [0, 2π), so
  for M just below zero its E is a float64 near 2π, where the spacing is
  8.9×10⁻¹⁶: an E of −4.6×10⁻¹⁶ cannot be represented there, and as an angle
  the answer is wrong by more than its whole size.
- **4.5×10¹⁵ is the float64 2π.** Reducing by it is 2.4×10⁻¹⁶ short per
  revolution, so a phase a hair past a revolution loses its sign; at
  M = 10¹² the absolute error is 3.7×10⁻⁴ rad.
- **3.5×10¹² in the critical region is the naive residual's limit,** ε·E/(1 −
  e·cos E). It reaches 3×10⁻¹⁰ rad absolute at e = 1 − 10⁻¹³. ENRKE's bisection
  evaluates the same residual, so it is subject to the same limit: its sign
  is only trustworthy outside an interval of that width. That contradicts the
  3×10⁻¹⁵ rad its paper reports; this is a reimplementation from the paper's
  description, and the difference is measured, not explained.
- **Mikkola's 435–1,140 ulps are his starter's s = z − α/z cancelling at tiny
  M** for moderate e, which the stable residual cannot repair; Markley's
  starter avoids it with Stumpff's rationalization (his eq. 13).

Symmetry, bit for bit over the sweep:

| | E(±0) ≠ ±0 | E(−M) ≠ −E(M) |
| --- | ---: | ---: |
| production | 11 of 26 | 533 points |
| ENRKE | 12 of 26 | 0 |
| Mikkola, published | 18 of 26 | 0 |
| every stable candidate | 0 | 0 |

### 3.3 Speed

Nanoseconds per solve over 4096 fixed (M, e) pairs: "typical" spans the
planets to a Halley-type comet (e from 0.0167 to 0.967), "high-e" 0.99 to
0.999999, "critical" e ≥ 0.995 with M < 0.0045. i9-11980HK, Go 1.27.2,
`-count=6`, medians with benchstat's interval:

| | typical | high-e | critical |
| --- | ---: | ---: | ---: |
| production | 207 ±12% | 356 ±30% | 492 ±23% |
| Markley, published | 144 ±36% | 158 ±18% | 134 ±68% |
| Mikkola, published | 141 ±39% | 185 ±56% | 120 ±83% |
| ENRKE | 129 ±24% | 170 ±23% | 2,457 ±53% |
| **Markley, stable** | **135 ±17%** | **131 ±33%** | **221 ±40%** |
| Mikkola, stable | 130 ±34% | 226 ±34% | 121 ±46% |
| Newton, stable | 227 ±6% | 239 ±5% | 403 ±16% |

The intervals are wide because this is a laptop whose clock moves under long
trigonometric loops, and an earlier run of the same benchmarks put every
median 10–30% elsewhere. Two conclusions held in both runs: stable Markley is
1.5–2.6× faster than production in every set, and ENRKE's bisection makes its
critical region about ten times its own typical cost. The four non-iterative
methods are within each other's noise.

### 3.4 Decision

**Markley, stable.** It is the only candidate that is both inside the contract
everywhere and the fastest of those that are: one square root, one cube root,
one `Sincos`, and no iteration — so no stopping rule to tune and no loop to
bound. Newton, stable, is as accurate and is kept as the cross-check in the
contract test; it is slower because it iterates.

## 4. The state

The proposed construction, with every quantity that cancels near perihelion as
e → 1 written as a sum of same-signed terms:

    1 − cos E   = sin²E / (1 + cos E)        while cos E > 0
    x           = a·((1 − e) − (1 − cos E))
    y           = a·√((1 − e)(1 + e))·sin E
    r           = a·((1 − e) + e·(1 − cos E))
    Ė           = n·a / r

Worst error over the sweep, through the public `Elements.StateAt` for
production, with M exactly the given one (elements evaluated at their own
epoch):

| | e ≤ 0.5 | 0.5 < e ≤ 0.99 | e > 0.99, \|M\| ≥ 0.0045 | critical |
| --- | ---: | ---: | ---: | ---: |
| production, position (ulps of \|r\|) | 5.3 | 67 | 270 | 5.2×10¹² |
| production, velocity (ulps of max(\|v\|, n·a)) | 3.6 | 53 | 132 | 3.2×10¹² |
| proposed, position | 1.4 | 4.7 | 2.6 | 2.9 |
| proposed, velocity | 1.6 | 3.7 | 2.1 | 2.7 |

The proposed 4.7 at e = 0.99 is the contract working as §2.3 says: E's 1.5
ulps carried through a sensitivity of about 1.8, plus the construction's own.
`TestProposedContract` asserts it per point against the exact ∂state/∂E.

In kilometres, from an exact solution of the same two-body problem — the
arithmetic's error, not the model's:

| orbit (elements as `NewElements` takes them) | M | production | proposed |
| --- | ---: | ---: | ---: |
| main belt, q 2.55 AU, e 0.08 | any | 2×10⁻⁸ km | 2×10⁻⁸ km |
| Halley-type, q 0.59 AU, e 0.967 | 10⁻⁹ | 3.6×10⁻⁸ km | 3.1×10⁻⁹ km |
| long-period, q 0.5 AU, e 0.9999 | 10⁻¹ | 1.0 m | 4.4 cm |
| near-parabolic, q 1 AU, e 0.999999 | 10⁻⁹ | 11 m | 5 µm |
| near-parabolic, q 1 AU, e 0.999999 | 10⁻¹ | 882 m | 4.0 m |

The main belt shows no difference because nothing there cancels. The
near-parabolic comet loses most of a kilometre to arithmetic today. Such a
comet should come through `FromPerihelion` and the universal path, which has
no 1 − e to lose — but `NewElements` accepts it, and the contract is for
everything the constructor accepts.

## 5. A prepared orbit, and no batch API

Everything in an `Elements` that does not depend on the epoch can be computed
once: the mean motion, the semi-minor axis, and the orientation as the two unit
vectors P and Q along which the perifocal x and y lie in the ICRF. Prototyped
(`preparedOrbit`), a state is then the phase by one fused multiply-add, the
solve, the construction, and r = x·P + y·Q:

| | per state |
| --- | ---: |
| `Elements.StateAt` today | 1.10 µs ±34% |
| prepared orbit | 0.50 µs ±20% |

An earlier run gave 0.70 and 0.39 µs: the same factor of about two, in a
different place on this machine's clock.

It agrees with `StateAt` to 4.6×10⁻¹⁵ relative over two revolutions, a bound
canaried by substituting the IAU 2006 obliquity for the elements' IAU 1976 one
— the 0.042″ #391 once cost.

**No new type.** `Elements` is already an immutable value validated at
construction; it carries the prepared quantities in unexported fields set by
`NewElements` and `FromPerihelion`, and `StateAt` stays the one entry point.
An orbit with secular apsidal precession rotates P and Q about the orbit
normal by Δω = ω̇·Δt: one `Sincos` on the calls that need it.

Shipped in phase 3, for both forms — the perihelion form prepares its mass
parameter, its period and its speed at perihelion the same way. Interleaved,
six runs each, on `BenchmarkStateAt` (a main-belt orbit, 400 phases):

| | per state |
| --- | ---: |
| `StateAt` after #827 | 773 ns ±20% |
| `StateAt` prepared | 426 ns ±29% |

−45% (p = 0.002), no allocations either way. The four `With` methods prepare
again, and a set built as a bare literal is prepared by `StateAt` itself, to
the same bits as the constructor's. The gate is
`TestPreparedOrientationIsTheRotation`: P and Q against the three rotations
they replace, on the ecliptic and a Laplace plane, with the apsis fixed and
turning, over a decade, to eight ulps plus the turning angle's own rounding.
Canaried by a `WithLaplacePlane` that does not prepare, it fails at 3×10¹³
times its bound.

**No batch method.** Once prepared, a state is the solve and a few
multiply-adds; a batch would save a function call per element and add a second
entry point to keep identical with the first. The same conclusion as the
ephemeris layer's (docs/ephemeris.md §6.2).

## 6. Correctness gates

1. **The oracle contract.** `TestProposedContract` moves into
   `ephemeris/kepler`'s own tests with the oracle: E within 2 ulps of the exact
   root, symmetry bit for bit, and the state within §2.3's bound, over the
   whole sweep (thinned under `-short`). It was canaried twice: the naive
   residual in place of the stable one fails it at 7,172 points, and
   x = a(cos E − e) in place of the stable form at 2,314.
2. **What already guards the package stays:** the Horizons comparisons
   (`validation_test.go`, `ephemeris/jpl/validation/kepler_*`), the
   near-perihelion convergence of #781, and the classical-against-universal
   agreement in `perihelion_test.go`.
3. **No bit identity with today.** The new solver is a different arithmetic
   path; the gate against today's answers is a bound, never equality.

## 7. Phases

1. **This change.** The document and `internal/keplerlab`; no production code.
2. **The solver and the state.** `SolveKepler` becomes stable Markley on an
   exact reduction, odd in M, answering in [−π, π] and refusing |M| ≥ 2⁵²;
   `perifocalClassical` takes the stable construction; gate 1 comes with it.
   The answer's range changes, so the fragment is `Changed — BREAKING`, though
   every caller in the module uses E only as an angle. **Done in #827**, as
   `TestSolveKeplerContract`; the "production" rows above are what it
   replaced, and `internal/keplerlab` keeps that solver so §3.2 reproduces.
3. **Prepared `Elements`.** The derived quantities at construction, the
   orientation as P and Q, the phase by one fused multiply-add. The phase
   came with #827; the rest is **done in #832**, measured in §5.
4. **The Earth from the base (#810),** then the per-orbit centre (#803).
5. **The universal path's own audit.** It is the stable form already and is
   not measured here: the oracle gains the universal anomaly and the Stumpff
   functions, and C(z) = (1 − cos √z)/z takes its cancellation-free form
   2·sin²(√z/2)/z.

## 8. Reproducing

```bash
go test -count=1 -v -run 'Test' ./internal/keplerlab/
go test -tags=validation -run='^$' -bench=. -count=6 ./internal/keplerlab/ | tee kepler.txt
```

The tests are untagged and run in every CI job on all three platforms. The
benchmarks are behind the `validation` tag, so CI's Benchmarks job — every
untagged benchmark, six times, on each push to main — does not repeat a
comparison this document has already made.
