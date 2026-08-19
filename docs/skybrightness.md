# Sky Brightness — Design

**Status:** Phase 0 (spectral foundation) and Phase 1 (atmospheric scattering) implemented. Phases 2–8 planned.

This document is the single source of truth for *why* the package is shaped the way it is,
and for the scientific provenance of everything it computes. Package doc comments explain
how to use a type; this explains the decisions behind it, so a maintainer does not have to
re-derive them from source.

The authoritative requirement is the implementation specification supplied by the project
owner. Where this document and that specification disagree, the specification wins.

---

## 1. Scope and the question this answers

The module predicts the spectral radiance of the night sky an instrument will actually see:

```
L_λ(λ, direction, observer, time, atmosphere)      W·m⁻²·sr⁻¹·nm⁻¹
```

for arbitrary terrestrial sites, arbitrary viewing directions, point or all-sky queries,
under clear and cloudy skies, with uncertainty and provenance attached to every result.

It exists to answer one operational question:

> Given this place, time, direction, atmosphere, cloud field, surrounding artificial-light
> environment and observing instrument, what spectral sky background will this instrument
> see, and how uncertain is that prediction?

Applications: observation scheduling, exposure-time estimation, instrument background
prediction, site characterisation, and light-pollution impact studies.

### What this is not

This is explicitly *not* the conventional architecture of

```
KS91 moonlight + constant dark-sky value + Bortle/VIIRS-derived zenith brightness
```

Every element of that is prohibited in production: a constant dark sky, KS91 as the
production Moon, Bortle class as physics, VIIRS radiance read as sky brightness,
geographic averaging in place of propagation, a universal cloud multiplier, and a single
extinction coefficient. Those approximations may exist only as separately named, documented
legacy or fast modes, and must never silently back the production model.

---

## 2. Scientific baseline

| Contribution | Model adopted | Primary reference | Status |
| :--- | :--- | :--- | :--- |
| Artificial, clear sky | Semi-analytic two-parameter all-sky | Kocifaj, Bará & Falchi (2022), MNRAS Lett. 513, L25; arXiv 2203.09322 | **In hand** |
| Artificial, cloudy sky | All-sky model spanning full cloud range | Kocifaj, Falchi & Kundracik (2025), PNAS 122(44) e2508001122 | **In hand** |
| Spectral/directional architecture | Per-pixel wavelength-dependent NSB | Roellinghoff, Spencer & Funk (2025), A&A; arXiv 2505.12895 | **In hand** |
| Lunar spectral reflectance | ROLO disk-equivalent albedo, model 311g | Kieffer & Stone (2005), AJ 129, 2887 | **In hand** |
| Lunar atmospheric scattering | Revised simplified scattering (RSS) | Winkler (2022), MNRAS 514, 208; doi:10.1093/mnras/stac1387 | **In hand** |
| Moonlight framework | Spectral moonlight model | Jones et al. (2013), A&A 560, A91 | To confirm |
| Natural sky | GAMBONS | Masana et al. (2021), MNRAS 501, 5443; arXiv 2101.01500 and arXiv 2408.17371 | Open access |
| Sky model / airglow | ESO SkyCalc | Noll et al. (2012), A&A 543, A92 | Open access |

Reference PDFs are **not** committed to the repository (copyright). Equations and
coefficients are transcribed here and into source comments with full citation, which is
normal scientific practice; the papers themselves are obtained by the reader.

### The rule that governs every component

A component whose primary source cannot be obtained confidently is **not implemented**. It
becomes an entry in §16 stating what is blocked and what would unblock it. No coefficient
is ever invented, tuned, or inferred from a secondary summary to make a phase look
finished. This is what separates this module from a plausible-looking one.

---

## 3. Package placement — nothing is segmented

Capability that belongs to an existing astrogo package is added *to that package*. The
module does not grow a private copy of atmospheric physics or photometry.

| Capability | Home | Rationale |
| :--- | :--- | :--- |
| Rayleigh and aerosol scattering, molecular absorption, transmission, vertical profiles, spherical airmass, cloud optical properties | `atmosphere` | Already owns `Atmosphere`, `Aerosol`, `CloudLayer`, `CloudPhase`, `VerticalProfile`, `Airmass`. A weather or seeing constraint needs the same physics. |
| Passbands, response curves, AB/Vega/ST systems, surface brightness | `magnitude` | Already owns photometric conversion (`GaiaGToJohnsonV`, `StarApparent`, `ExtinctionAtAltitude`). |
| Throughput, detector QE/PDE, collecting area, pixel solid angle, photon and electron rates | `optics` | Already owns `Telescope`, `Eyepiece`, `Sensor`. One `Sensor` definition then serves both optical arithmetic and background rates. |
| Spectral quantity types, the shared wavelength axis | `unit` | `SpectralRadiance`, `WavelengthNM`, `SpectralGrid` and friends. Must sit below both `magnitude` and `skybrightness`. |
| Physical constants | `constants` | `PhotonEnergyJ`, `ToPhoton`/`ToEnergy`, `ArcsecondSquaredToSteradian`, `SI2019`. |
| Geometry, Sun/Moon, time scales | `coord`, `ephemeris`, `time` | No astronomy is re-implemented here. |
| Dataset acquisition | `remote`, `remote/file` | Consent-gated bucket/key layer. |

**`skybrightness` therefore owns only radiance transport**: `Scene`, `Component`, `Model`,
`Query`, `Estimate`, uncertainty, quality, provenance, and all-sky operations. Files, not
subpackages. The single exception will be `skybrightness/dataset/...`, the only tier
permitted I/O.

A concrete payoff: Winkler (2022) and Kocifaj et al. (2022) independently adopt the **same**
Henyey–Greenstein aerosol phase function over a theoretical Rayleigh base. Under this rule
that becomes one implementation in `atmosphere` shared by the Moon and artificial
components, rather than two that can silently diverge.

---

## 4. Physical quantity model

The internal quantity is spectral radiance, W·m⁻²·sr⁻¹·nm⁻¹, and it stays spectral until
the moment a caller asks for something else.

`mag/arcsec²`, an SQM reading, luminance, a photon rate and a detector electron rate are
all *projections* of that one state. They are never the internal representation, because a
model can reproduce a correct V magnitude with an entirely wrong spectrum — and every
instrument projection downstream would then be wrong.

Radiance is linear and additive; magnitude is logarithmic and is not. Components sum in
radiance space and the conversion happens once, at the end. Summing magnitudes is a
correctness bug, and `TestComponentsSumLinearly` asserts the linear behaviour directly.

### The spectral grid

`unit.SpectralGrid` is a uniform axis: `StartNM`, `StepNM`, `N`. Uniform spacing makes
integration a fixed-weight sum, grid equality a three-field comparison, and resampling a
pure index computation. Datasets arriving on a non-uniform axis are resampled at their
provider boundary, where the interpolation choice is documented, rather than silently
inside a numeric kernel.

`DefaultOpticalGrid` is 330–1000 nm at 1 nm. The lower bound is where ozone Huggins-band
absorption makes ground-level night-sky radiance negligible and most detectors lose
response; the upper bound covers the near-infrared OH airglow bands that dominate a dark
site beyond 700 nm. 1 nm resolves airglow line structure well enough for broadband
projection while keeping a full-sky evaluation tractable.

Integration is trapezoidal. The integrands are products of measured response curves and
modelled spectra, both carrying sampling error far larger than the quadrature difference,
and Simpson would additionally require an odd sample count callers have no reason to
guarantee.

---

## 5. Public API

Two layers, as the specification requires: a high-level call for ordinary use and full
specification for expert use. Scientific choices are explicit rather than hidden behind
defaults.

```go
model, err := skybrightness.NewModel("v1", components...)

est, err := model.Estimate(ctx, skybrightness.Query{
    Scene:     scene,               // observer, time, atmosphere
    Direction: coord.NewAltAz(alt, az),
    Grid:      skybrightness.DefaultOpticalGrid(),
    Fidelity:  skybrightness.Standard,
})

v,    err := est.SurfaceBrightness(johnsonV, magnitude.Vega)
g,    err := est.SurfaceBrightness(sdssG, magnitude.AB)
rate, err := est.ElectronRate(camera)

spectrum   := est.SpectralRadiance()
components := est.ComponentIDs()
quality    := est.Quality
```

All of these originate from the same stored spectrum, which is what keeps them mutually
consistent.

---

## 6. Scene

`Scene` is the physical state an evaluation runs against: observer, time, atmosphere,
and (from Phase 3) an ephemeris provider.

It is explicit and caller-owned rather than hidden inside components, so two evaluations
differing in aerosol loading differ in a value the caller can see and record, not in a
component's private default. Every component reads the same `Scene`, which is what keeps
the Moon, the artificial sky and the natural sky consistent with one another.

A `Scene` carries no I/O. Atmospheric and cloud state are resolved by a provider layer and
handed in already populated. There is no default atmosphere: an unstated atmosphere is the
single largest source of silent error in a sky prediction, so it must be chosen explicitly
even when the choice is a climatology.

---

## 7. Three grids, kept separate

Confusing these is a category error the specification calls out explicitly.

| Grid | What it discretises |
| :--- | :--- |
| **Source grid** | Geographic distribution of artificial emitters. |
| **Atmospheric grid** | Vertical and horizontal structure the atmosphere and cloud models need. |
| **Sky grid** | Directions at the observer. |

A geographic grid is a numerical discretisation of emitters. It is **not** the
light-propagation algorithm. Averaging neighbouring cells is not atmospheric scattering.
The propagation kernel determines how light emitted by each source element reaches each
observer direction.

---

## 8. Fidelity levels

All levels share one API and one physics. They differ in approximation, not in model.

| Level | Permitted |
| :--- | :--- |
| `Fast` | Lookup tables, cached natural sky, spectral basis compression, surrogate kernels, reduced angular or spectral resolution. **Not** different undocumented physics. |
| `Standard` | The native semi-analytic model. Default. |
| `Reference` | Finest spectral grids, detailed atmospheric calculation, precomputed radiative transfer, minimal approximation. |

Every surrogate must be generated from, and its error measured against, the model it
stands in for. A regression fitted to observations and called radiative transfer is not a
surrogate; it is a different model wearing the name.

The fidelity used is recorded in `Reproducibility`.

---

## 9. Component contract

```go
type Component interface {
    ID() ComponentID
    AddRadiance(ctx, dst SpectralRadiance, grid unit.SpectralGrid, dir coord.AltAz, scene *Scene) error
    Provenance() Provenance
}
```

Components accumulate into a caller-owned buffer rather than returning a new spectrum: a
full-sky evaluation runs this across ~10⁴ directions, and allocating per component per
direction would dominate cost.

A component must add **observer-level** radiance — light arriving after atmospheric
propagation — not a top-of-atmosphere emission. Summing unpropagated source terms is
physically wrong. The interface cannot enforce this, so it is stated in the contract and
each component's provenance records how it propagates.

The physically distinct contributions are separated because each has its own literature,
data, validity domain and uncertainty: starlight, diffuse Galactic light, extragalactic
background, zodiacal light, airglow continuum, airglow lines, moonlight, twilight,
artificial.

Radiance is validated **per component**, not only on the total: a negative term cancelled
by a positive one would otherwise pass unnoticed, and the point of the check is to name
which component is wrong.

---

## 10. Uncertainty, quality, provenance, reproducibility

**Uncertainty** is a first-class output, kept per component rather than collapsed into one
scalar. Which term dominates is itself the useful answer: a caller can act on "airglow
dominates" by taking a measurement, but cannot act on a single opaque percentage.
Uncertainties are relative because the dominant terms — airglow variability, aerosol
loading, assumed source spectra — are multiplicative. Independence is not assumed;
`UncertaintyBudget.Correlated` combines linearly when terms share a systematic.

**Quality** flags record how the prediction was constrained: measured versus
climatological atmosphere, aerosol, cloud, airglow; measured versus assumed source spectrum
and emission function; precomputed RT; extrapolation beyond a validity domain. A caller
must be able to distinguish a prediction constrained by current observatory measurements
from one resting on climatology. Phase 0 sets `NoComponents`.

**Provenance** per component: model, version, primary and secondary references, the
equations implemented, datasets with versions, validity domain, known approximations,
expected accuracy. Naming a paper is not enough — a reader needs to know which equations
this code actually reproduces.

**Reproducibility** carries model version, fidelity, grid, atmospheric provider and
timestamp, dataset versions and per-component provenance, so a scientific user can explain
why two calculations differ.

---

## 11. Component design notes

Each section below is written before its component is implemented, and carries the
**equation → Go function → validation test** map the specification requires.

### 11.1 Artificial clear sky — Kocifaj, Bará & Falchi (2022)

**Goal.** All-sky spectral radiance from one ground source, summed over surrounding
sources.

**Scientific model.** A semi-analytic two-parameter model whose parameters `t` and `g`
encode the atmospheric state, the source–observer distance, and the source's angular and
spectral emission pattern. It generalises Kocifaj & Bará (2019) to include scattering
orders up to the fifth, provided *ab initio* rather than fitted a posteriori to site
imagery.

**Equations.**

| Paper | Content | Go function | Validation test |
| :--- | :--- | :--- | :--- |
| Eq. 1 | Total scattering phase function `P(g,Θ) = [P_a(g_a,Θ)·ϖ_a·k_a + P_R(Θ)·k_R] / (k_a + k_R)` | `atmosphere.CombinedPhaseFunction` | `TestCombinedPhaseFunctionWeighting`, `TestRayleighPhaseFunctionNormalisation` |
| Eq. 2 | All-sky radiance `L(z,A)` from `L_S`, `P(g,Θ)`, `(1−g)²/(1+g)`, `M(z)`, `M_S`, `t` | `AllSkyRadiance` | `TestKocifaj2022Eq2HorizonLimit`, `…FallsWithDistance`, `…BrightensTowardTheHorizon`, `…SingularityIsSmooth`, `…NearHorizonTurnover` |
| Eq. 3 | `t = (τ_a/H_a + τ_R/H_R)·D / M_S` for an exponential atmosphere | `OpticalParameterT` | `TestKocifaj2022Eq3ExponentialAtmosphere` |
| Eq. 4 | `g = c₀ + c₁·g_a + c₂·g_a²` | `AsymmetryParameter` | `TestKocifaj2022Eq4CleanAtmosphereAsymptote`, `…Monotonic` |
| Eq. 5 | `c₀ = 0.33 + 0.15τ_a`, `c₁ = 0.9τ_a^0.51`, `c₂ = 1.3τ_a^1.85` | `AsymmetryParameter` | `TestKocifaj2022Eq5Coefficients`, `…LeavesPhysicalRange` |

**Symbols.** `z` observational zenith angle; `A` azimuth; `Θ` scattering angle; `L_S` the
radiance leaving the source in the observer's azimuth (azimuthal symmetry assumed), as it
**reaches** the observer — see the Eq. 2 discussion below, since this is the single easiest
thing to get wrong about the model;
`M(z)`, `M_S` optical air mass toward the sky element and toward the source; `τ_a`, `τ_R`
vertical aerosol and molecular optical thickness; `H_a`, `H_R` the corresponding scale
heights; `D` source–observer separation; `ϖ_a` aerosol single-scattering albedo; `g_a`
aerosol asymmetry parameter.

**Behaviour worth checking.** At the horizon Eq. 2 reduces to `L_S·P(g,Θ)·(1−g)²/(1+g)`,
which is a cheap analytic check — and, as the next section explains, a badly insufficient
one on its own. As `τ_a → 0`, `g → 0.33`, excluding isotropic scattering even in a clean
atmosphere, consistent with `c₀`'s constant term.

**Validation.** Figure 1 gives `g` versus `g_a` for three aerosol optical depths; that is
the Level-2 reproduction target. The paper's corroboration used the MSOS multiple-scattering
code at 450 and 550 nm, scale heights 1.5 and 2.2 km, `g_a` from 0 to 0.9, to fifth
scattering order, with `ϖ_a ≈ 0.95`.

**Known limitation, to be recorded in the implementation.** The `g` fit is derived at
550 nm (and 450 nm), stated as representative over roughly a ±20–30 nm band. The artificial
component is therefore *not* spectrally resolved to the same degree as the rest of the
model, and its error budget must say so rather than implying full spectral fidelity.

**Implementation status.** Eq. 1 (`atmosphere.CombinedPhaseFunction`, shared with the
lunar model), Eq. 2 (`AllSkyRadiance`), Eq. 3 (`OpticalParameterT`) and Eq. 4/5
(`AsymmetryParameter`) are all implemented and tested.

**Eq. 2, and why it was withdrawn once.**

	L(z,A) = L_S · P(g,θ) · (1−g)²/(1+g) · M(z)/(M_S·t) · (e^{[M_S−M(z)]t} − 1)/(M_S − M(z))

A first implementation was withdrawn after tests found radiance *growing* with distance —
a city at 80 km coming out brighter than the same city at 10 km. The typeset equation
later confirmed the transcription had been correct all along, including the paper's stated
horizon limit. **The test was wrong, not the equation**, and the mistake is worth recording
because it is easy to repeat.

Eq. 2 contains no distance term. Distance enters twice, and both are the caller's
responsibility:

- through `t`, which Eq. 3 makes proportional to the source–observer separation `D`;
- through `L_S`, which is the source radiance **as it reaches the observer**, and must
  therefore already carry the transmission `e^{−M_S·t}` over that separation — `M_S·t` is
  exactly that optical depth, which is the physical meaning Eq. 3 attaches to the product.

The withdrawn test varied `t` with distance while holding `L_S` fixed, which is not a more
distant city; it is the same city with more air in front of it and no dimming applied. The
kernel alone does grow with `M_S·t`, so that produced a two-order-of-magnitude inversion.
Apply the transmission that belongs with it and the fall-off is monotonic, which
`TestKocifaj2022Eq2FallsWithDistance` now asserts — while a companion assertion pins the
bare kernel's growth, so a future change cannot quietly make the doc comment's warning
false.

The horizon limit could not have caught this: at `M_S = M(z)` the exponential is 1 under
either sign convention. Analytic limits check transcription; only physical trends check
understanding, and the trend has to be the one the model actually claims.

**Directional structure, and where it inverts.** The brightest direction is set by one
number, `M_S·t`. Below about 2 — the ordinary case — radiance rises monotonically from the
zenith to the source's horizon, several times over. Above it the maximum moves inward: at
`M_S·t = 6` it sits at airmass 3 and the zenith is *brighter* than the horizon. That is a
very distant or very hazy source, under a quarter of a per cent transmission, and the model
inverting its directional structure there is documented by
`TestKocifaj2022Eq2NearHorizonTurnover` rather than asserted to be correct.

**Numerics.** The removable singularity at `M_S = M(z)` is evaluated as
`t·expm1(u·t)/(u·t)` with `u = M_S − M(z)`, exact at `u = 0` and accurate approaching it,
where a bare `(e^{u·t}−1)/u` loses all precision. Without that the sky map would carry a
notch or a spike at the source azimuth.

**The component, and the two choices the paper leaves open.** `ArtificialSkyglow` joins
emitters, geometry and the kernel. Eq. 2 needs `L_S` and `M_S`, and arXiv:2203.09322 says
how to get neither from a real installation, so both are decided here and stated rather
than buried:

1. **`M_S` is the horizon airmass.** The paper's own limit — Eq. 2 reducing to
   `L_S·P·(1−g)²/(1+g)` at the horizon — holds exactly when `M(z)` reaches `M_S`. A ground
   source beyond a few kilometres sits at the observer's horizon, so taking `M_S` at zero
   elevation is what makes the model consistent with its own stated limit.
2. **Light leaves the source horizontally**, so the emission function is evaluated at zero
   elevation above the source's horizon — the same geometry, and the part of a luminaire's
   output that reaches a distant sky at all, which is why `UpwardEmission` carries
   `HorizontalFraction` separately from its cosine lobe. `WithEscapeElevation` overrides it.

The component applies the transmission `e^{−M_S·t}` to each emitter's output before calling
`AllSkyRadiance`, which is that function's documented contract and the step whose absence
withdrew the kernel once.

| Claim | Validation test |
| :--- | :--- |
| A city twice as far contributes *less* | `TestArtificialSkyglowFallsWithDistance` |
| Sources add in linear radiance space | `TestArtificialSkyglowSumsLinearly` |
| Shielding reduces skyglow | `TestArtificialSkyglowRespondsToShielding` |
| The hoisted phase weighting equals Eq. 12 | `TestArtificialSkyglowPhaseWeighting` |
| Emitter quality flags reach the caller | `TestArtificialSkyglowPropagatesEmitterFlags` |

**The azimuthal profile is not monotonic, and that is the physics.** Skyglow is brightest
toward the city, darkest about **90° away in azimuth**, and rises again toward the
anti-city direction. The rise is the back-scattering lobe of the Rayleigh phase function,
which goes as `1.06 + cos²Θ` and is therefore equally strong at 20° and 160°; the
Henyey–Greenstein aerosol term is what tilts the profile forward on top of it. A scalar
"sky quality" number cannot represent any of this, which is the argument for a directional
model in one sentence.

**Cost.** 54 µs per direction per source on the 671-point default grid, zero allocations —
about 12× the moonlight component, because that one evaluates 32 ROLO bands and resamples
while this one evaluates every grid point. The band-independent molecular phase function is
hoisted out of the per-band loop (13%); the rest is Phase 8 work.

**Where it is weakest.** The asymmetry parameter is evaluated per band from Eq. 4/5, which
was fitted at 450 and 550 nm for a band 20–30 nm wide — so this component is *less*
spectrally resolved than the grid it writes onto, and the validity domain says so. Values
of `g` that leave (−1, 1) are clamped to ±0.95 and flagged `ExtrapolatedModel` rather than
failing the whole sky for a hazy atmosphere.

### 11.2 Artificial cloudy sky — Kocifaj, Falchi & Kundracik (2025)

**Goal.** Artificial sky radiance over the hemisphere for cloud fractions from clear to
overcast, with cloud type and geometry.

**Scientific model.** Radiance decomposes as `L = L₁ + L₂ + L∞`, with the scattering angle
evaluated at height and the observational zenith angle carried through. Clouds participate
in propagation rather than multiplying a clear-sky answer.

**Validation targets (from the paper's own results, Žilina, Slovakia, ~80,000 population).**

| Quantity | Reported effect |
| :--- | :--- |
| Zenith artificial radiance over the city | amplified more than ×15 |
| Ground-level irradiance over the city | amplified more than ×4 |
| Zenith luminance, photopic, low cloud | up to ×27 |
| Horizontal illuminance, photopic | up to ×17 |
| Outside the city | screening — radiance *reduced* |

The sign reversal between "over the city" and "outside the city" is the qualitative
behaviour a universal cloud multiplier cannot reproduce, and is the acceptance criterion
for this component: amplification and screening must both emerge from geometry alone.
Regimes to test: pristine clear, urban clear, thin cloud, broken cloud, cloud over the
observer, cloud over the source city, cloud between city and observer, overcast.

### 11.2b Zodiacal light and diffuse galactic light

**Zodiacal light** — Leinert et al. (1998) Table 17 for the 500 nm spatial distribution,
Eq. 22 for the colour correction, with the heliocentric `R^-2.3` and the high-latitude
seasonal factor applied as Masana et al. (2021) Eq. 18 does.

| Piece | Go function | Validation test |
| :--- | :--- | :--- |
| Table 17 map | `ZodiacalBrightnessAt` | `TestZodiacalPoleMatchesKnownBrightness`, `…TableCorners` |
| Eq. 22 colour | `ZodiacalColourCorrection` | `TestZodiacalColourCorrectionSign` |
| Full component | `ZodiacalLight` | `…HeliocentricScaling`, `…SeasonalTerm` |

**The external anchor:** the ecliptic pole comes out at **23.26 mag/arcsec² in V**. A dark
site's total V sky brightness is around 22.0 and zodiacal light is roughly a quarter of it
at high ecliptic latitude, which puts the component near 23.5 — so landing at 23.3
exercises the table's `10⁻⁸` prefix, the per-micron to per-nanometre conversion and the
separately quoted pole value at once. A factor of ten anywhere shows up as 2.5 magnitudes.

**The solar vicinity is refused, not extrapolated.** Table 17 is blank within roughly 15° of
the Sun, where the brightness climbs by another order of magnitude. `ErrZodiacalGeometry`
is returned there; a night-sky model has no business reporting a number a few degrees from
the Sun.

**A numerical trap worth recording.** `angle.Angle` holds radians, so a direction given in
degrees round-trips imprecisely — `angle.Deg(15).Degrees()` is 14.999999999999998. Without
snapping, a direction sitting exactly on a grid line gives the neighbouring cell a weight of
4×10⁻¹⁶. Harmless in an ordinary interpolation; fatal at the edge of the blank region, where
that neighbour is missing and a vanishing weight still vetoes the lookup. It cost the
brightest cell in the table. `bracketAxis` now snaps within `gridSnapTolerance`, and the
corner weights are computed *before* the missing-data check so a zero-weight blank never
vetoes anything. This is the third time this session that a degree-to-radian round trip has
bitten on an exact grid line — see also the HEALPix 45° face boundary.

**Diffuse galactic light** — see §17 for the Kawara et al. (2017) coefficient reading;
`DiffuseGalacticLight` implements Eq. 7 over the SFD 100 µm map.

### 11.3 Moon — Kieffer & Stone (2005) reflectance, Winkler (2022) scattering

**Goal.** Spectral moonlight scattered into the line of sight.

**Chain.** Solar spectral irradiance → Sun–Moon geometry → ROLO reflectance → lunar
spectral irradiance at Earth → Moon–target geometry → Rayleigh and aerosol scattering →
atmospheric attenuation → multiple-scattering correction → observer radiance.

**ROLO disk-equivalent albedo, model 311g.** Kieffer & Stone (2005) Eq. 10:

```
ln A_k = Σ(i=0..3) a_ik·g^i + Σ(j=1..3) b_jk·Φ^(2j−1)
         + c₁·θ + c₂·φ + c₃·Φθ + c₄·Φφ
         + d₁k·e^(−g/p₁) + d₂k·e^(−g/p₂) + d₃k·cos((g−p₃)/p₄)
```

`g` absolute phase angle; `θ`, `φ` selenographic latitude and longitude of the observer;
`Φ` selenographic longitude of the Sun. `a` multiplies powers of `g`; `b` the odd powers of
`Φ`; `c` the four libration terms; `d` two exponentials and a cosine scaled by `p₁–p₄`.

- 32 bands, 350–2383.6 nm, 10 wavelength-dependent coefficients each (Table 4). These are
  **triple-sourced and verified**: an HTML transcription, the rendered table, and the
  journal's ASCII export agree row for row, including `549.1 nm d₃ = −0.00000`, the
  `2126.3 nm` outlier band (`a₁ = −2.55069`, `a₂ = 2.10026`) and the `2383.6 nm` sign flips
  on `b₂`/`b₃`. The published precision is 5 decimals.
- 8 wavelength-independent coefficients (Table 5, asterisked): `c₁ = 0.0003`,
  `c₂ = −0.0013`, `c₃ = 0.0010`, `c₄ = 0.0006` (deg⁻¹ and deg⁻¹·rad⁻¹); `p₁ = 4.06`,
  `p₂ = 12.88`, `p₃ = −30.59`, `p₄ = 16.75` (deg).
- Lunar solid angle `Ω_M = 6.4177×10⁻⁵ sr`. Phase-angle validity 1.55°–97°. Fit residual
  0.0096 in ln(reflectance), residual σ 0.015.
- Uncertainty budget (Table 6), for §10: atmospheric correction 0.743 % statistics-based /
  0.7 % practical; radiance calibration 0.216 % / 3.1 %; lunar disk centring 0.075 % /
  0.4 %; sum-to-irradiance 0.00432 % / 0.2 %; bias 0.0368 %; dark current 0.0846 %;
  flat-fielding 2.23×10⁻⁴ %.

**Implementation status.** Eq. 10 is implemented as `magnitude.ROLOReflectance`, with the
full Table 4 and Table 5 coefficients. It lives in `magnitude`, not here: the package
already owns solar-system photometry (`AsteroidHG`, the Mallama planet models,
`SatelliteMagnitude`), and lunar disk-equivalent reflectance is that same kind of thing.
`magnitude.ROLOIrradiance` converts reflectance to irradiance at the observer.

| Equation | Go function | Validation test |
| :--- | :--- | :--- |
| Eq. 10 | `magnitude.ROLOReflectance` | `TestROLOReflectanceEquation10` (term-by-term hand evaluation) |
| — | — | `TestROLOReflectanceMatchesKnownGeometricAlbedo` (external reference) |
| — | — | `TestROLOReflectanceUsesBothAngleUnits`, `…OppositionSurge`, `…WaxingWaningAsymmetry` |
| E = A·E_sun·Ω_M/π | `magnitude.ROLOIrradiance` | `TestROLOIrradiance` |

**The unit trap in Eq. 10.** `g` appears in two units in the same equation: radians in the
`a` polynomial, because those coefficients are published as rad⁻ⁱ, and degrees in the `d`
exponentials and cosine, because `p₁–p₄` are published in degrees. `Φ` is in radians and
`θ`, `φ` in degrees. An implementation that picks one unit throughout produces smooth,
plausible, wrong numbers, so the API takes `angle.Angle` rather than `float64` and a test
asserts the all-radians reading is far from the correct one.

**Independent validation.** The reflectance the model returns near full Moon at 553.8 nm is
0.134, against an independently known lunar V-band geometric albedo of about 0.12 — and at
zero phase those two quantities are the same thing by definition. The 2383.6 nm / 553.8 nm
ratio comes out at 2.4, reproducing the Moon's red slope. Neither number comes from
Kieffer & Stone, so this is a genuine external check on the coefficient table, the unit
conventions and the term structure at once.

**Precision caveat, a real error-budget line.** Table 5 prints `c₁–c₄` at 1–2 significant
figures against listed Effects on `ln A` of 0.005–0.028. The rounding is a material
fraction of the libration terms' own contribution. Full precision exists in the paper's
display equation and in the journal's ASCII export of that table; until captured, the
implementation records this limitation rather than implying ROLO's full accuracy.

A second caveat on the same eight constants: Table 5 is headed "Example Lunar Irradiance
Model Coefficients" and its *wavelength-dependent* values match no Table 4 row, so it shows
a different fit. Only its asterisked entries are used, on the strength of its NOTE that
those are constant for all wavelengths — which is also why Table 4, tabulating only
per-band terms, omits them. The reading is sound but it is a reading.

**The selenographic angles are an open input.** `Φ`, `θ` and `φ` need the Moon's
orientation — IAU rotation elements or a JPL binary PCK — which this module does not yet
have, so `ROLOGeometry` takes them as explicit inputs rather than deriving them. `θ` and
`φ` are the four smallest terms in the model (≤ 0.03 in `ln A` by Table 5's own accounting,
pinned by `TestROLOLibrationIsASmallCorrection`), so a caller can pass zero and record the
omission. `Φ` is not small — its `b` terms carry effects of 0.137–0.157 — and it is what
makes the model asymmetric between waxing and waning Moon. Because the Moon's prime
meridian points at Earth to within libration, `Φ` is close to the signed phase angle, but
this module deliberately ships no helper asserting that: an approximation with no primary
source behind it does not get to look like geometry. See §16.

**The component.** `skybrightness.ScatteredMoonlight` joins the two halves:
`magnitude.ROLOReflectance` for what the Moon sends, `atmosphere.SingleScatteredRadiance`
for what the air does with it, and `atmosphere.CombinedPhaseFunction` for the angular
redistribution. It caches the direction-independent geometry per scene behind a read-write
lock and pools its scratch buffers, so a full-sky evaluation resolves the Moon's position
once and allocates nothing per direction (4.6 µs, 0 allocs).

| Step | Go function | Validation test |
| :--- | :--- | :--- |
| Reflectance → irradiance → scattering → radiance | `ScatteredMoonlight.AddRadiance` | `TestScatteredMoonlightFullMoonSkyBrightness` |
| Directional falloff away from the Moon | — | `TestScatteredMoonlightDarkensAwayFromTheMoon` |
| Per-scene geometry cache | — | `TestScatteredMoonlightCacheRespectsScene`, `…Concurrent` |

**What it does not do, stated plainly.** Multiple scattering is applied, but as an
empirical factor rather than a transfer solution: `atmosphere.MultipleScatteringFactor`
gives `f = L/L1 = 1 + 4.5·τ_R` from Winkler (2022) §5.2, which revises the
`1 + 2.2·τ_R` of Noll et al. (2012) after Staude (1975). Winkler states the coefficient
as approximately 4.5, on the grounds that it better matches both his measured values and
the most likely single-scattering albedos — it is a suggested revision, not a formally
fitted parameter with a quoted uncertainty, and it comes from one site under low aerosol
loading. It is a function of the molecular depth alone, because Winkler notes a larger
share of the Mie optical depth may be absorption than usually assumed. Every call
therefore returns `ApproximateMultipleScattering`. The selenographic libration angles are
not supplied (bounded at 0.03 in ln A) and the solar spectrum is a required caller
input because this package ships none and the choice moves the absolute scale.

**Why `Component.AddRadiance` returns a `Flag`.** The interface was changed for this
component, while there were still zero implementations and the change was free. Flags
cannot be fixed per component: the same model is an interpolation in one geometry and an
extrapolation in another, and a caller deciding whether to trust a number needs to know
which case it got. `Model.Estimate` ORs them into the `Estimate`'s `Quality`.

**Atmospheric scattering — Winkler (2022).** The revised simplified scattering model,
derived from SAAO Sutherland multifilter photometry across a wide range of sky directions,
lunar phases and lunar positions. Rayleigh phase function per Bucholtz (1995); Mie via
**Henyey–Greenstein**, explicitly replacing the exponential angular relationship used by
earlier work including KS91. Eq. 13 gives the Rayleigh optical depth
`τ_R = P·(1.229×10¹⁰)·λ^−4.05`. Deviations attributable to multiple scattering and airglow
are quantified in the paper and become error-budget entries here.

**KS91.** Retained only as `LegacyKS91` for regression and comparison — the comparison
Winkler's own paper motivates. It is never the production path and never a silent fallback.

### 11.5 Atmosphere — Rayleigh, aerosol and molecular absorption

**Implemented (Phase 1), in `atmosphere`.**

**Rayleigh choice, now settled.** Winkler (2022) is adopted over Bodhaine et al. (1999).
The reason is consistency rather than preference: the lunar scattering model derives its
own published results with these expressions, and the artificial model (Kocifaj et al.
2022) uses the same Henyey–Greenstein aerosol phase function, so one shared implementation
keeps the two components from silently disagreeing about the atmosphere they propagate
through. A future switch to Bodhaine must be made for both at once, with its own
validation.

| Paper | Content | Go function | Validation test |
| :--- | :--- | :--- | :--- |
| Winkler Eq. 13 | `τ_R = (P/P₀)·1.229×10¹⁰·λ^−4.05`, P₀ = 1013.5 hPa, λ in nm (after Dutton et al. 1994) | `atmosphere.RayleighOpticalDepth` | `TestRayleighOpticalDepthSeaLevel550` (≈0.098 at 550 nm, an independently known value), `…ScalesWithPressure`, `…SpectralSlope` |
| Winkler Eq. 9 | `p_R(Θ) = 3(1−ρ)/(16π(1+2ρ))·[(1+3ρ)/(1−ρ) + cos²Θ]` (after Bucholtz 1995) | `atmosphere.RayleighPhaseFunction` | `TestRayleighPhaseFunctionNormalisation` (integrates to 1 over the sphere), `…Symmetry` |
| — | `ρ = 0.0148`, giving `(1+3ρ)/(1−ρ) = 1.06` | `atmosphere.RayleighDepolarisation` | `TestRayleighDepolarisationMatchesKS91Coefficient` |
| Winkler Eq. 10 | `p_M(Θ) = (1−g²)/(4π(1+g²−2g·cosΘ)^{3/2})` (Henyey & Greenstein 1941) | `atmosphere.HenyeyGreensteinPhaseFunction` | `TestHenyeyGreensteinNormalisation`, `…Limits` |
| Winkler Eq. 12 | `p(Θ) = (τ_R/τ_s)·p_R + (τ_M/τ_s)·p_M` | `atmosphere.CombinedPhaseFunction` | `TestCombinedPhaseFunction` (normalisation plus both pure limits) |
| Ångström | `τ_a(λ) = τ_a(λ₀)·(λ/λ₀)^−α` | `atmosphere.AerosolOpticalDepth` | `TestAerosolOpticalDepthAngstrom` |

The `ρ = 0.0148` value is worth noting: it is the depolarisation for which Bucholtz's
theoretical phase function reproduces the `1.06 + cos²Θ` coefficient Krisciunas & Schaefer
(1991) had fitted empirically, and it lies inside Bucholtz's tabulated 0.01384–0.01557. The
resulting peak-to-side ratio is 1.9434, not the ideal-dipole 2 — a detail the test asserts
exactly rather than approximately.

**Airmass.** `atmosphere.Airmass` already implements Pickering (2002), which stays
well-behaved to the horizon where a plane-parallel `sec z` diverges. It is reused rather
than replaced. Minimum altitude per fidelity is set when the components that need it land.

**Molecular absorption.** `atmosphere.CrossSection` applies a tabulated cross section over
a column via Beer–Lambert, with the Dobson Unit derived from the SI-exact Boltzmann
constant and the STP definition rather than hardcoded (`TestDobsonUnitDerivation`
reproduces 2.687×10¹⁶ molecules·cm⁻²).

It ships **no tabulated cross-section data**. O₃ (Chappuis), O₂ (A and B bands) and H₂O
are the species that matter over 330–1000 nm, but their cross sections are datasets with
their own provenance — Serdyuchenko et al. (2014) for ozone, HITRAN for O₂ and H₂O — and
inventing numbers for them is exactly what the design forbids. See §16.

One caveat recorded in the code: Beer–Lambert with a band-averaged cross section is valid
for the ozone Chappuis continuum and **wrong** for the narrow O₂ A band, where a 1 nm grid
cannot resolve individual lines and a band-averaged transmittance is not the average of the
monochromatic transmittances. A provider supplying O₂ must supply an already
band-averaged effective cross section for the target grid and say so.

### 11.4 Natural sky — integrated starlight

**Goal.** The summed light of resolved and unresolved stars, as seen from the ground.

**Scientific model.** Masana et al. (2021) Eq. 8 splits the observed radiance into a
directly attenuated term and a scattered term. `IntegratedStarlight` implements the
first:

    L_obs(lambda) = L_0(lambda) * T(lambda, z)

`L_0` is an extra-atmospheric map, `T` the line-of-sight transmission at that wavelength
and airmass, built from `atmosphere.RayleighOpticalDepth` and the scene's aerosol.

**Primary references.** Masana, E., Carrasco, J.M., Bará, S. & Ribas, S.J. (2021), MNRAS
501, 5443 (GAMBONS); Riello, M. et al. (2021), A&A 649, A3 and the Gaia DR3 photometric
documentation Table 5.7 for the G to V transformation; Górski, K.M. et al. (2005), ApJ
622, 759 for the HEALPix indexing the map is built on.

**Inputs.** A `StarMap` — passband-averaged extra-atmospheric spectral radiance by
direction, in its own frame — a spectral shape, the spectral grid, and the passband the
map's values are averaged over.

**Why the passband is required.** The map holds one number per direction. Spreading it
across wavelengths needs an assumed spectrum, and scaling that spectrum needs a
definition of what "reproduces the map value" means. Dividing the shape by the sum of its
samples is the obvious choice and is wrong: it makes the answer depend on the grid
spacing, so refining the grid halves the starlight while every value stays positive and
plausible. Dividing by the shape's average over the passband is exact and
resolution-independent, and it is what `TestIntegratedStarlightIsIndependentOfGridResolution`
holds in place.

**Why the spectral shape is the caller's.** Integrated starlight is the summed light of
stars of every spectral type, and no single blackbody is right. This is one of the two
inputs the module refuses to guess; the other is the airglow zenith spectrum.

**Why the frame travels with the map.** A galactic map read as equatorial rotates the
Milky Way across the sky and still returns plausible numbers everywhere. `StarMap.Galactic`
carries the answer so the component converts rather than assumes, and
`TestIntegratedStarlightHonoursTheMapFrame` asserts the two frames are sampled at
different coordinates.

**The map: server-side Gaia aggregation.** A Gaia `source_id` carries its HEALPix index in
the high bits, so `source_id / 2^(59-2k)` is the level-k nested pixel and the whole
aggregation becomes a `GROUP BY` the archive performs. At order 8 — GAMBONS' grid,
786,432 pixels — that is 787 chunked TAP queries. **This is heavy use of a shared
service and is not the recommended path.** `starlight.Fetch` asks about the directions a
caller intends to observe, which is a single query for a night's target list, and caches
what it gets; `BuildFromGaia` exists for the case where a whole sky is genuinely wanted.

**What a whole-sky build actually costs, measured.** The first successful run took 38
minutes. Timing per query is not stable: the same forty-pixel query took 3 seconds when
the archive was quiet, 18 seconds later the same afternoon, then 182 seconds, and then
the query endpoint stopped answering altogether — returning no bytes at all, including
for a deliberately malformed query that should have failed at its parser in milliseconds,
while ordinary GETs to the same host kept returning 200 throughout. TCP and TLS completed
normally. That is a service defending itself, not a service that is broken, and three
whole-sky builds plus some probing in one afternoon is a plausible cause.

Queries are therefore spaced and serialised (`api.WithMinInterval`, two seconds here),
which roughly doubles a whole-sky build. The cost of getting this wrong is paid by every
other user of a shared research instrument rather than by us.

The bulk release deserves more credit than an earlier draft of this section gave it. ESA
publishes `gaia_source` as at least 2,911 files totalling at least 649 GB compressed —
measured from the CDN listing, which was still truncated where the count stopped. The
files are named for the HEALPix level-8 range they hold
(`GaiaSource_000000-003111.csv.gz`), which is exactly this aggregation's grid, so a bulk
route parallelises and resumes cleanly. Against it: `gaia_source` carries about 150
columns where this needs three, and CSV.gz cannot be pruned server-side, so it moves
roughly 700 GB to use two per cent of it. That trade is about *our* bandwidth. The bulk
files exist precisely so that heavy users do not ask the query service 787 times, and for
a whole-sky map that argument now looks stronger than the bandwidth one. TAP remains
clearly right for `Fetch`-scale use: tens of pixels, once per observing session.

**One published map, and no magnitude cut.** GAMBONS applies none — it integrates *"the
contributions of all the stars"* and reaches the bright end with Hipparcos rather than
trimming it. This package briefly carried a `FainterThan` cut, reasoned from "a resolved
star is not background". That is defensible for an instrument and is not what the reference
does, and a map built with it could not be compared to any published figure — which is how
the map became unvalidatable. The cut is removed entirely rather than left as an option
nobody should reach for.

The published asset is `starmap-o8-V-total.txt.gz`, matching GAMBONS' definition and its
grid, and `Open` takes no specification because there is one map to fetch. `total` rather
than `all`: Gaia sees nothing brighter than G = 5, so `all` would promise what the data
does not hold, and the composition sits in the filename for the same reason the order and
band do — each changes the numbers, and two files differing by twenty per cent must never
share a name.

**Why the band is a constructor.** Converting Gaia G flux to Johnson V spectral flux
density needs three published numbers from three sources: G's VEGAMAG zero point
(25.6874), the G to V colour transformation, and Johnson V's own Vega zero point
(3.63e-11 W m⁻² nm⁻¹). Using G's zero point with V's flux density and no colour term is
the obvious mistake and produces a map that is neither a G map nor a V map, wrong by the
colour of whatever mix of spectral types each pixel holds — plausible everywhere, worst
along the Galactic plane where the ensemble is reddest. One constructor removes the
opportunity.

**Sources without a colour.** The transformation is applied per star inside the aggregate,
because transforming a sum is not the same as summing transformations when the
transformation depends on colour. A source with no BP-RP makes the polynomial null and SQL
drops it; the archive rejects both `CASE` and `COALESCE`, so no default can be substituted
in the query. Each pixel therefore reports two counts — total sources and sources with a
colour — so the exclusion is visible rather than assumed small.

**The map, measured.** A full order-8 build over all 786,432 pixels aggregated
1,811,709,771 sources — Gaia DR3's entire catalogue, which is what proves the chunks
tile the sky exactly rather than overlapping or leaving gaps. It took 38 minutes over
787 queries and left no pixel empty. Binned by galactic latitude:

| \|b\| | mag arcsec⁻² |
| :--- | :--- |
| 0–15° | 21.01 |
| 15–30° | 22.06 |
| 30–45° | 22.29 |
| 45–60° | 22.34 |
| 60–75° | 23.19 |
| 75–90° | 23.48 |

The polar value lands on the ~23.5 that Masana et al. and Leinert et al. give for
high-latitude integrated starlight, with nothing tuned to put it there, and the sky spans
6.2 magnitudes between its 1st and 99th percentile pixels. That contrast is also the
sharpest available test of the frame: mislabelling an ICRS map as galactic does not dim
the plane, it moves it, so the plane and the cap would come out alike while every value
stayed positive and every direction stayed covered.

**The map is Gaia-only, and Gaia does not see the brightest stars.** Masana et al. (2021)
state it directly: *"Gaia cannot observe very bright objects (with G < 5 mag). For those
stars, the Hipparcos catalogue"* is used instead, and *"in terms of flux, the Hipparcos
stars account for around 20 per cent of the total integrated star light."*

So `NoMagnitudeCut` is a misleading name for what it produces: a map already missing the
brightest fifth of the light, cut not by us but by the instrument. Two consequences, both
of which invalidate earlier claims in this document.

First, comparing that map against a published *total* integrated-starlight figure is not a
validation. The order-8 build gives 23.48 mag arcsec⁻² at the cap against a quoted ~23.5,
and that agreement is arithmetically impossible to be meaningful when a fifth of the flux
is absent — it is luck, and it was reported here as confirmation.

Second, it makes the internal comparison the discriminating one. Cutting at G > 6 moved
the cap from 23.48 to 23.68, a 17 per cent flux reduction, against the 19 per cent
measured directly over a thousand pixels and the ~20 per cent Masana et al. attribute to
Hipparcos. Three routes to the same fraction, and that consistency is a real check where
the absolute comparison was not.

Closing this needs Hipparcos for G < 5, which is what GAMBONS does and what would make an
absolute comparison mean something. Until then, no published map should claim to be total
integrated starlight.

**Known limitations.**

- The scattered term of Eq. 8 is not modelled. It returns to the line of sight some of
  what extinction removed, so attenuation alone **overstates** the dimming toward the
  horizon. Masana et al. put the difference between their full and simplified scattering
  at under 0.1 mag arcsec⁻², and every result below 30° altitude carries
  `ExtrapolatedModel`.
- The colour transformation is fitted for −0.5 < BP−RP < 5.0 and extrapolates outside it.
- A direction the map does not cover returns nothing and flags it, rather than reading as
  a dark sightline.

**Equation → function → test.**

| Equation | Go | Test |
| :--- | :--- | :--- |
| Masana Eq. 8, direct term | `IntegratedStarlight.AddRadiance` | `TestIntegratedStarlightDimsTowardTheHorizon`, `TestIntegratedStarlightReddens` |
| Shape normalisation | `NewIntegratedStarlight` | `TestIntegratedStarlightReproducesTheMapValue`, `TestIntegratedStarlightIsIndependentOfGridResolution` |
| `source_id / 2^(59−2k)` | `GaiaBuild.ADQL` | `TestGaiaADQLDivisor`, `TestGaiaQueryIsAcceptedByTheArchive` |
| Riello Table 5.7, negated | `GaiaJohnsonV` | `TestGaiaJohnsonV`, `TestGaiaJohnsonVBrightensRedStars` |

**Still outstanding for this section.** The extragalactic background light is not
implemented and is not folded into anything else; it is a separate component with its own
spectrum and uncertainty when its reference is settled. Airglow currently takes a
caller-supplied zenith spectrum without separating continuum from lines, and without the
climatology / solar-adjusted / calibrated modes the architecture calls for.

---

## 12. Dataset and provider architecture

**`skybrightness/dataset/raster`** decodes georeferenced single-band GeoTIFFs — classic
TIFF, LZW and deflate, strips and tiles, 32/64-bit float samples, the floating-point
predictor — either wholly into a `Grid` or through a window for a composite too large to
hold. It is source-agnostic and **carries no units**, precisely because the products it
serves are satellite radiances and reading one as a sky brightness is the error this module
exists to prevent.

**`skybrightness/dataset/viirs`** turns NASA VIIRS annual nighttime-lights composites into
`GroundEmitter`s. Its contract is the §14 one: VIIRS is a *source input*, never a sky
brightness. A pixel radiance cannot determine a spectrum — high-pressure sodium, 3000 K LED
and metal halide give the same DNB reading and scatter completely differently under a
λ⁻⁴ law — nor an upward emission function, so both are required inputs and every emitter is
flagged `AssumedSourceSpectrum | AssumedEmissionFunction`. Bins outside coverage or
resolving to no-data are **dropped, not zeroed**: missing data is not measured darkness.



Large scientific datasets never become generated Go source. They are fetched by an explicit
provider layer through `remote`, which is consent-gated, and handed to the numeric layer
already resolved.

Anticipated products: Gaia-derived sky tiles, DGL maps, zodiacal lookup data, atmospheric
cross-sections, molecular spectra, lunar coefficients, VIIRS rasters, DEM data, cloud
fields, RT lookup tables.

The numeric API is decoupled from storage and assumes nothing about a local POSIX
filesystem — `remote/file` addresses everything as a bucket plus key.

**No hidden network dependency.** `Model.Estimate` is deterministic for a given scene and
dataset version and performs no acquisition. This is enforced behaviourally:
`TestEstimateWorksOffline` runs an evaluation under `remote.SetOffline(true)` and requires
byte-identical output. A structural direct-import check complements it
(`TestCoreDoesNotImportIOPackages`); a *transitive* ban would be wrong rather than
stricter, because the specification requires reusing `coord`, `ephemeris` and `time`, which
legitimately reach `remote` for Earth-orientation data and JPL kernels, both consent-gated.

---

## 13. Validation strategy

| Level | Content |
| :--- | :--- |
| 1 — equation | Each equation checked against values computed independently from the paper. |
| 2 — published figures and tables | Reproduce reference plots or tables, e.g. Kocifaj 2022 Fig. 1, the Kocifaj 2025 amplification factors. |
| 3 — cross-model | Compare against ESO SkyCalc, GAMBONS output, published Kocifaj calculations, Illumina-v2. |

**The GAMBONS web tool is a Level-3 target, not a data source.** Its "Calculate Radiance"
export returns azimuth, altitude and mag arcsec⁻² per pixel — the sky *after* GAMBONS' own
atmospheric propagation — so it cannot feed this module, which applies its own. It is
directly usable the other way round: run it for a site, date and atmosphere, run astrogo's
natural-sky components with matching parameters, and compare all-sky. Its MultiQuery mode
returns a time series in one file, which makes a night-long comparison a single export.

Two conditions must match for that comparison to mean anything. GAMBONS excludes the Moon
and the Sun, so astrogo must be queried with only its natural-sky components registered —
`ScatteredMoonlight` off. And GAMBONS' airglow is an ESO SkyCalc spectrum computed for
Paranal (2640 m, 350–1050 nm, 100 sfu by default) scaled by a percentage; the same spectrum
has to be handed to astrogo, because airglow is a free parameter in both models rather than
a prediction. Comparing two different airglow assumptions would measure nothing.

GAMBONS' band set fixes what can be compared without a passband of our own: **UBVRI** and
**Gaia G** in the Vega system; **SQM**, **TESS-W**, **RGB** and **Sloan ugriz** in AB.
UBVRI and RGB use filters normalised to unit peak transmission; Gaia G and Sloan do not —
which changes the zero point, not just the shape.
| 4 — observational | Infrastructure for calibrated all-sky cameras, SQM networks, photometry, spectrometers, extinction measurements, AERONET. |

Validation is **spectral and directional**, not V-band-at-zenith. A model can obtain the
right V magnitude with the wrong spectrum, and the instrument layer depends on the
spectrum. Directional cases: zenith, 30°, 15°, near horizon, toward and away from a light
dome, toward the zodiacal light, Galactic plane, high Galactic latitude, several Moon
separations.

Reference sites spanning real conditions: Paranal, Armazones, Mauna Kea, La Palma, an urban
European site, a remote dark site, a high-aerosol site, a humid site, a high-altitude dry
site, both hemispheres. The model is not tuned to one observatory and then claimed global.

**Phase 0 proves none of this.** It proves unit correctness, integration correctness,
linear additivity, determinism, projection consistency and allocation behaviour. It ships
no physics, so it makes no accuracy claim whatsoever.

---

## 14. Legacy classification

The previous package (3,911 LOC) had approximately the right architecture and almost none
of the physics.

| Disposition | Items |
| :--- | :--- |
| Superseded, re-derived | Spectral grid and integration, provenance, quality, uncertainty shapes. |
| Moved to their proper packages | Instrument → `optics`; passband and derived outputs → `magnitude`; transmission → `atmosphere`. |
| Legacy-only, explicitly named | KS91 → `LegacyKS91`; constant airglow → named climatology fallback. |
| Deleted | `bortle.go`, `natural.TophatJohnson`, `natural.NewFastEngine`, `mode.go` (replaced by `Fidelity`), `scratch.go`, `atmos.RayleighOnly`, `limitingmag.go`. |

`plan.LimitingMagnitudeConstraint`, `plan.ScoreObservableSky` and examples 18 and 21 were
removed with it; they return once Phases 2–3 make a defensible limiting magnitude possible.
`plan.MeteorShower.ObservedRate` now takes a limiting magnitude directly, which decoupled
meteor-rate arithmetic from sky brightness entirely.

---

## 15. Phase roadmap

**What the module can and cannot do today.** All five components are implemented and the
engine runs them together. Assembled over Paranal on a moonless night — a reference airglow
spectrum, a stand-in uniform dust map and one modest city 30 km away — it produces:

| Component | Radiance at 554 nm | Share | mag/arcsec² |
| :--- | ---: | ---: | ---: |
| Airglow | 1.52×10⁻⁹ | 40.5% | 22.52 |
| Zodiacal | 1.34×10⁻⁹ | 35.6% | 22.66 |
| Artificial | 8.18×10⁻¹⁰ | 21.8% | 23.19 |
| Diffuse galactic | 7.79×10⁻¹¹ | 2.1% | 25.74 |
| Moonlight | — | — | absent, new Moon |
| **Total** | | | **21.48** |

Paranal's real moonless V sky is 21.5 to 22.0 mag arcsec⁻², and Leinert et al. and Masana et
al. both put airglow first among the natural terms with zodiacal light second. Both the
total and the ordering come out right, which is a stronger check than any single component's
because a term entering with the wrong scale would leave the total plausible while the
composition was wrong. `TestFullSkyComponentShares` asserts it.

One term is still absent: **integrated starlight**, which needs a map. `dataset/starlight`
holds the map type, its loader and a Gaia TAP builder; what it does not yet hold is data.
Until it does, the Milky Way is missing from the sky this engine draws, and a sightline
along the galactic plane is underestimated accordingly.

Performance: 287 µs per direction with all five components and per-scene caches warm, 40
allocations.

| Phase | Content | State |
| :--- | :--- | :--- |
| 0 | Architecture and spectral foundation | **Done** |
| 1 | Atmospheric foundation, into `atmosphere` | **Done.** Scattering, transfer, scale heights and van Rhijn; absorption reads through `dataset/crosssection`. No cross-section dataset is shipped, and deliberately: ozone's Chappuis-band cross section is strongly temperature-dependent, so a reference and a temperature are the caller's scientific choice |
| 2 | Natural moonless sky (GAMBONS, Gaia DR3) | **Largely built.** DGL, zodiacal light and airglow are implemented and validated; `dataset/starlight` holds the map type, its loader and a Gaia TAP builder. What remains is reference data, not code: an ISL map (requested, or buildable), Kawara's `c` decade (resolved), and band transformations |
| 3 | Modern Moon (Jones 2013, ROLO, Winkler 2022) | **`ScatteredMoonlight` shipped** — ROLO reflectance + single-scattering transfer with Winkler's multiple-scattering factor, validated at 18.9 mag/arcsec². The solar spectrum is now supplied by `dataset/solar` from CALSPEC. Remaining: Winkler's own model is empirical at one site, so the multiple-scattering factor is a correction rather than a transfer solution |
| 4 | Artificial clear sky (Kocifaj 2022, VIIRS as source) | **`ArtificialSkyglow` + `dataset/viirs` shipped.** Absolute scale is uncalibrated, but not for the reason an earlier revision of this row gave: Eq. 2 is not missing an area term, and §17 records why that reading was wrong. The real blocker is narrower and is a literature gap rather than a transcription one — Kocifaj & Bará say `L_i` can be inferred from satellite radiance data and cite Elvidge et al. (2017), which is an instrument and product description carrying no DNB-pixel-to-line-of-sight-radiance conversion. There appears to be no published recipe, so none is implemented and `dataset/viirs` uses a stated substitute; directional structure is meaningful, absolute scale is not. Fig. 1 is checked against the properties the paper states about it (`TestKocifaj2022Fig1Curves`); a digitised comparison remains |
| 5 | Clouds (Kocifaj 2025) | **Planned — but not from nothing.** `atmosphere` already carries the scene vocabulary (`CloudLayer`, `CloudPhase`, `CloudMorphology`) and `skybrightness` already has the `UnknownCloud` quality flag, so a scene can describe a cloud field it cannot yet evaluate. What is missing is the radiative model (`L = L1 + L2 + Linf`), the cloud optical properties in `atmosphere`, and a `Component`. The paper is in hand and open access |
| 6 | External high-fidelity RT (Illumina-v2, precomputed) | Planned |
| 7 | Operational observatory calibration | Planned |
| 8 | Performance: LUTs, surrogates, caching, loop optimisation | **Partly done, and deliberately so.** Caching was built in rather than deferred: `coord.Context` is reused per epoch, every natural component holds a `frameCache`, and `ScatteredMoonlight` caches its per-scene geometry — these are correctness-shaped decisions (a `Context` per direction would dominate a full-sky evaluation) and were never Phase 8 work. The §43 benchmark set exists as the Phase 0 baseline: single direction, 100 directions, full sky, spectral resolution, instrument projection. What remains is LUTs, surrogates and loop optimisation, none of which should start before there is a number saying they are needed |

Delivery order departs from the numbering: Phase 1, then 4, then 3, because their literature
was in hand. Phase 2 is last because it is not one model but five — integrated starlight,
diffuse galactic light, extragalactic background, zodiacal light and airglow — each with its
own literature and its own dataset:

| Phase 2 ingredient | What it needs |
| :--- | :--- |
| Integrated starlight | GAMBONS' equations, plus precomputed all-sky Gaia DR3 tiles. `catalog/gaia` does cone searches; an all-sky integral needs a different, prepared product. |
| Diffuse galactic light | Its own scattering model over a dust map. |
| Extragalactic background | **Decided, values not yet verified.** Koushan et al. (2021), MNRAS 503, 2033 (GAMA/DEVILS): integrated galaxy light over 0.3-2.2 um, which spans the optical grid, superseding Driver et al. (2016b) in that range by a stated 5-15 per cent. Table 3 gives per-band EBL in nW m^-2 sr^-1 for u, g, r, i, Z, Y, J, H, Ks with pivot wavelengths from 0.3577 to 2.1549 um — six of the nine fall inside 330-1000 nm, so the component interpolates between roughly six published points rather than a dense spectrum, and that coarseness belongs in its error budget. **The numbers themselves reached this document through a fetched summary of the publisher's page, not through the paper's own text, and must be confirmed against the preprint before any of them enters the code.** A summariser's table is not a primary source, and the rule about never fabricating a coefficient does not weaken just because a plausible table was easy to obtain. |
| Extragalactic background, uncertainty model | **Decided.** Integrated galaxy counts are a floor by construction — galaxies that were not detected were not counted — so the value is reported as a lower limit rather than a central estimate, with the very-high-energy gamma-ray absorption constraint carried as one-sided headroom for anything undetected. Koushan et al. state the VHE data are consistent with their IGL across u to Ks "without the need to include any significant additional source of diffuse light", which is the bound this uses. This differs deliberately from the other components' symmetric uncertainties, because the measurement is asymmetric and pretending otherwise would misreport it. |
| Zodiacal light | A solar-elongation-dependent model, e.g. Leinert et al. (1998). |
| Airglow | **Decided, not yet wired.** ESO SkyCalc, Noll et al. (2012), A&A 543, A92 — the Cerro Paranal sky model, already cited here — which carries the pseudo-continuum, the emission lines and the solar-activity scaling in one place rather than requiring a line atlas and a continuum to be reconciled onto one grid. Its interface and terms of use need checking before anything is wired. |
| Airglow, solar activity | **Decided.** F10.7 enters as a scene input. Airglow tracks the solar radio flux and it is the single largest source of its variability, so a caller with a real date gets real variability and the module never invents a solar cycle it cannot know. |
| Airglow, why its lines are representable | Worth recording because it looks like the case this module rejected for O₂ and H₂O and is not. Those were refused because absorption band-averaged onto a nanometre grid is systematically wrong: `exp(−τ)` is convex, so averaging the cross section overestimates absorption. Airglow is **emission**, and emission adds linearly, so band-averaging the OH Meinel bands and the O I lines onto the optical grid is exact. The narrow-line objection does not transfer between the two. |
| Airglow, where the fetch lives | SkyCalc is a *service*, and evaluation performs no I/O. The spectrum is therefore resolved under `skybrightness/dataset/` and handed in through the `Scene`, the same way `dataset/solar` supplies the CALSPEC spectrum the Moon needs. A per-scene SkyCalc call during `Estimate` would break the property `TestEstimateWorksOffline` exists to hold. |
| Airglow, the SkyCalc interface | **Checked against ESO's CLI documentation.** Parameters: `msolflux` is the monthly averaged 10.7 cm solar radio flux in sfu, default 130.0 — which is the F10.7 the scene will carry, under the service's own name. `incl_airglow` toggles the upper-atmosphere term. `wmin`/`wmax` accept 300 to 30000 nm and `wdelta` defaults to 0.1 nm, so the 330-1000 nm grid sits inside the range at finer sampling than it needs. The response is a **binary FITS table returning `FLUX_AEL` (upper-atmosphere emission lines) and `FLUX_ARC` (airglow residual continuum) as separate columns** — the continuum-and-line separation this design called for arrives from the service rather than having to be constructed. Worth noting that a binary FITS response is only readable here because `fits.Read` was taught to decode BINTABLE extensions earlier in this work; before that it returned headers and skipped every payload, so a SkyCalc response would have parsed to nothing at all. |
| Airglow, what is still unknown | Two things the CLI documentation does not state and which must be settled before wiring: **the HTTP endpoint the CLI posts to**, which is not on that page and has to come from the client's source rather than be guessed, and **the terms of use** — no rate limit, acknowledgement or citation requirement is stated there. Given that this project has already been throttled once today by a shared research service for asking too often without identifying itself, the second is not a formality. |
| Hipparcos bright stars, why not Gaia Sky | The ZAH/ARI Gaia Sky repository mirrors van Leeuwen (2007) — the right reduction, 8 MB, versioned with md5 and sha256 — and is tempting because it needs no TAP service. It is not used, and the reason is arithmetic rather than caution. The star data is `catalog/hipparcos.bin`, whose per-star layout no documentation page specifies; the format documentation covers the JSON scene descriptors and STIL loading of VOTable, FITS and CSV, which is the *input* side of their pipeline. Nothing states the photometric **system** of the magnitudes it carries, and a magnitude whose band is unstated cannot be converted to radiance at all, because a zero point is band-specific — the same reason `GaiaBand.FluxToRadiance` is required here rather than defaulted. The documentation also says the absolute magnitude drives a "pseudo-size which is used for rendering purposes only", and the dataset holds 177,955 objects against Hipparcos' 118,218 with no explanation of the difference. Reading a product built for rendering as a physical quantity is the mistake this module already prohibits for VIIRS. The primary source is used instead: VizieR I/311/hip2, van Leeuwen (2007), documented columns and stated units. |
| Airglow, the SkyCalc protocol | **Read from the client's source, version 1.4.** Host `https://etimecalret-002.eso.org`; `POST /observing/etc/api/skycalc` with the parameters as a JSON body, which returns `{status, tmpdir}` rather than data. The spectrum is then fetched from `/observing/etc/tmp/{tmpdir}/skytable.fits`, and finally `GET /observing/etc/api/rmtmp?d={tmpdir}` releases it. The almanac is a second endpoint, `/observing/etc/api/skycalc_almanac`. |
| Airglow, the obligation nobody documents | **The third call is not optional.** Each request makes ESO allocate a temporary directory on their server, and it is the client that deletes it. A client which fetches its FITS and stops leaves that directory behind on every call. Nothing on the help page says so — it is visible only in the client's source — and it matters more here than for a person running the tool by hand, because a library calls it once per user rather than once per afternoon. Whatever wraps this must delete the directory even when the fetch fails. |
| Airglow, a unit trap | The client notes its own break: output wavelengths are nanometres in version 1.4 and were **micrometres** in 1.3. A reader that assumes either silently is out by a thousand, which is the class of error this module treats as unacceptable elsewhere. The unit has to be asserted, not inherited. |

Nothing here is blocked on a decision — it is blocked on obtaining five sources and their
data, and §2 forbids standing in for any of them with a fitted constant. The arXiv listing
for GAMBONS (2101.01500) is reachable but its PDF does not yield equations to automated
extraction; a supplied copy would unblock the starlight term, as it did for Kocifaj and
Kieffer & Stone.

### Phase 0 baseline

Framework cost with five trivial components on the 671-sample default grid, before any
physics. Recorded so later phases can distinguish a machinery regression from the expected
cost of real models.

| Workload | ns/op | allocs/op |
| :--- | ---: | ---: |
| Single direction | 21,570 | 20 |
| 100 directions | 1,302,310 | 2,008 |
| Full sky, 18 rings | 9,945,475 | 16,570 |
| Single band | 1,675 | 17 |
| Full spectrum | 10,405 | 18 |
| Surface-brightness projection | 3,870 | 2 |
| Electron-rate projection | 5,030 | 3 |

Nothing is optimised yet; that is Phase 8. The point is numbers before opinions.

---

## 16. Unresolved dependencies

| Item | Blocks | What would unblock it |
| :--- | :--- | :--- |
| ROLO `c₁–c₄` at full precision | Nothing outright; caps Phase 3 accuracy | The display equation following "the eight constant 311g coefficients are", or the ASCII export of Table 5. |
| Lunar orientation (IAU rotation elements or a JPL binary PCK) | The selenographic longitude of the Sun `Φ` and the libration angles `θ`, `φ`, which `magnitude.ROLOReflectance` therefore takes as inputs. Archinal et al. (2018) WGCCRE report, or PCK support in `ephemeris/jpl`, would close it. |
| ~~Solar spectral irradiance at the 32 ROLO bands~~ | **Resolved.** `skybrightness/dataset/solar` fetches the CALSPEC reference (`sun_reference_stis_002.fits`) and `solar.NewScatteredMoonlight` samples it onto the ROLO bands. `magnitude.ROLOIrradiance` still takes the spectrum as a parameter, since ROLO's absolute scale depends on the choice and the engine performs no I/O. |
| O₃ absorption cross section | Molecular absorption in the visible | **Decided, not yet wired.** Serdyuchenko et al. (2014), AMT 7, 609 and 625: 213–1100 nm at 0.02–0.06 nm in the UV–visible, eleven temperatures from 193 to 293 K in 10 K steps, better than 3 per cent absolute over most of the range. It spans the 330–1000 nm grid with no extrapolation at either end, and it is on MPI-Mainz, whose format `dataset/crosssection` already parses. **223 K** is the temperature to ship: ozone peaks near 22 km where the stratosphere is roughly 220–230 K, and 223 K is the nearest *measured* point to the ~226 K effective temperature used in total-column retrieval — interpolating a table to 226 K would be inventing data to gain nothing. The choice rests on the Chappuis band being far less temperature-sensitive than the UV Huggins band, which should be confirmed against Part 2 once the data is in hand; if it does not hold, the cross section has to be weighted per scene against `atmosphere`'s vertical profile instead. |
| O₂ and H₂O absorption | Nothing in the visible continuum; narrow bands only | **Not a data gap — a capability gap.** These are line absorbers: O₂ at 688 and 762 nm, water vapour at 720, 820 and 940 nm. Beer–Lambert with a cross section band-averaged onto a 1 nm grid is systematically wrong and always in the same direction, because `exp(−τ)` is convex — averaging the cross section first overestimates absorption. Representing them needs a band model or a correlated-k treatment, which is a different capability from `atmosphere.CrossSection` rather than a dataset for it. HITRAN supplies the line lists whenever that capability exists. |
| Jones et al. (2013) confirmation | Phase 3 framing | Confirm the A&A open-access text. |
| Illumina-v2 product format | Phase 6 | A sample precomputed product and its dimension conventions. |

No component is blocked outright. Every model named in the specification has its primary
source in hand.

---

## 17. Open scientific questions

**Integrated starlight can be built from Gaia without the bulk download — tested.**

The ESA Gaia archive's TAP service aggregates server-side, and `source_id` encodes the
HEALPix index directly: `source_id / 2^43` is the level-8 (nside 256) nested pixel, which is
exactly GAMBONS' grid. A single ADQL query returns summed flux per pixel:

	SELECT source_id/8796093022208 AS hpx8, COUNT(*), SUM(phot_g_mean_flux)
	FROM gaiadr3.gaia_source WHERE source_id BETWEEN ... GROUP BY hpx8

Verified against the live service: 1,000 pixels in one synchronous query, no truncation,
sub-second. Each chunk is a `source_id` range, so it uses the primary-key index rather than
scanning the table. **786,432 pixels is 787 such queries — roughly half an hour, against
at least 649 GB and 2,911 files for the bulk route.** astrogo already has the TAP client.

A spot check confirms the numbers are real: pixel 100000 holds 567 sources summing to
4.94×10⁶ e⁻/s, which is G = 8.95 integrated, or **23.5 mag/arcsec²** over the pixel's
1.5979×10⁻⁵ sr — a textbook integrated-starlight surface brightness at high galactic
latitude.

The colour-dependent per-star transformation also evaluates inline, so the band conversion
happens inside the aggregate rather than after it — which matters, because transforming a
summed flux is not the same as summing transformed fluxes when the transformation depends
on colour:

	SUM(phot_g_mean_flux * POWER(10, -0.4*(a + b*bp_rp + c*bp_rp*bp_rp)))

**Gaia DR3 is the right source.** DR3 shares EDR3's source list, astrometry and G/BP/RP
photometry — DR3 only *added* astrophysical parameters, variability and spectra. So the
`gdr3` tables carry exactly the photometry GAMBONS' current version uses.

**What this does not yet solve**, and none of it is a data-availability problem:

| Gap | Note |
| :--- | :--- |
| Photometric transformations | The coefficients above were a placeholder for the capability test and the sign convention in it was wrong. Real ones must come from GAMBONS' recomputed EDR3 transformations or the Gaia documentation, verified. |
| Bright stars | Gaia omits the brightest; Hipparcos supplies them, and they carry disproportionate weight. |
| Missing colours | Over 300 million faint sources have no BP−RP. GAMBONS assigns the local mean colour. |
| Faint-star completion | Below G = 20, via the Besançon model. Under 3% except on the galactic plane. |
| Independent validation | GAMBONS shipped a DR2 bug that underestimated ISL for months. A from-scratch pipeline needs checking against their web export (§13). |

This is still a data-preparation job producing a map, not a runtime operation — the
architecture is unchanged, since `dataset/starlight` already consumes such a map. What it
changes is that the map can now be produced without waiting on anyone.

**Kawara et al. (2017) DGL coefficients — RESOLVED, including a misreading of my own.**

I claimed the published units were dimensionally impossible. They are not, and the reason
is in the adjacent row: the slope is tabulated as **`νb_i`**, not `b_i`, with
`νb_i = [3000/λ(µm)]·b_i`. So Eq. 7 is written in `I_ν` (MJy sr⁻¹) throughout, which makes
`b` dimensionless and `c` in `(MJy sr⁻¹)⁻¹` — exactly as printed. I had read the tabulated
slope as `b` itself.

That leaves only the power of ten, and the physics settles it. The quadratic turns over at
`I₁₀₀ = b/(2c)`; Kawara fit samples bounded at 5, 10, 15, 20, 30 and 50 MJy sr⁻¹, and
Masana et al. restrict the relation to `I₁₀₀ < 50`. Reading the tabulated `c` as ×10⁻⁵:

| λ (µm) | 0.319 | 0.369 | 0.418 | 0.472 | 0.550 | 0.648 |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| turnover (MJy sr⁻¹) | 46.3 | 47.5 | 39.5 | 41.7 | 41.9 | 50.4 |

Every well-measured band turns over at the top of the fit's own range — where a quadratic
fitted to saturating data should. The printed `10⁵` would put the turnover at 10⁻⁹ MJy sr⁻¹
and make DGL negative over the whole sky, so the printed exponent sign is a typo.

Implemented as `DiffuseGalacticLight` and `DGLCoefficientsAt`, with
`TestDGLTurnoverMatchesTheFittedRange` asserting the turnover for every band — the evidence
for the reading is executable, not a comment.

**Remaining limit:** Kawara stops at 648 nm. The default grid runs to 1000 nm, so the red
end holds the endpoint coefficients rather than extrapolating a still-rising slope, and
reports `ExtrapolatedModel`. Masana et al. interpolate across the same gap without saying
so.

**Kocifaj 2022 Eq. 2's "missing source-area term" — RESOLVED, and the earlier entry here
was wrong.**

An earlier revision of this document claimed Eq. 2 carried no source-area term and that a
raster-derived prediction's absolute scale was therefore meaningless. That was a
misreading. Kocifaj & Bará (2019) Eq. 9 — the model Eq. 2 generalises — defines the
quantity exactly:

> `L_i(π/2, A_0i)` is the line-of-sight radiance, measured at the detector, of the i-th
> city or town located on the horizon

`L_S` is what a photometer pointed at the horizon in the source's direction reads. It is
not a property of the source's surface and needs no area factor. No term is missing.

What *was* wrong was this repository's own code: `dataset/viirs` binned the raster into
rings **and** sectors, stacking several emitters at one azimuth. Eq. 9 sums over
**azimuthally separated** sources — one per direction. That double-counting, not the
paper, produced the N-scaling. Fixed: `Region` now takes `Sectors` (the emitter count) and
`RadialSamples` (which refines the estimate within a sector without adding emitters), and
`TestEmittersOnePerAzimuth` pins it.

One genuine gap remains, and it is narrower than it looked. Kocifaj & Bará say `L_i` "can be
inferred from satellite radiance data", citing **Elvidge et al. (2017)** — but that paper
was obtained and is an instrument and product description. It carries no conversion from a
DNB pixel to a line-of-sight radiance, so the citation is to the data rather than to a
recipe, and there appears to be no published method to implement. It does settle the unit:
average radiance in nW cm⁻² sr⁻¹. `dataset/viirs` instead sums upward radiances along each azimuth and
places the result at the radiance-weighted mean distance — which preserves Eq. 9's
structure and the relative weighting between azimuths, but is not the published inference.
Absolute scale uncalibrated; directional structure meaningful.

**Kocifaj 2022 Eq. 5 exponents — RESOLVED.** The typeset equation confirms the exponents
sit on `τ_a`, not on the coefficients:

	c₀ = 0.33 + 0.15·τ_a,  c₁ = 0.9·τ_a^0.51,  c₂ = 1.3·τ_a^1.85

Implemented as `AsymmetryParameter`. One property of the published fit is worth recording:
it is not bounded to the physical range, and around `τ_a = 0.5` with `g_a = 0.9` it
evaluates above 1, where a Henyey–Greenstein phase function is undefined. That is a limit
of the parameterisation, so the value is returned together with `ErrAsymmetryOutOfRange`
rather than clamped.

The superseded analysis follows, kept because it explains why the ambiguity was real.

**Historical — why the text layer was ambiguous.**

`c₀ = 0.33 + 0.15·τ_a` is settled: it is corroborated independently by the paper's own
statement that `g → 0.33` as `τ_a → 0`, which the constant term reproduces exactly.

`c₁` and `c₂` are not. Raw PDF text extraction yields the token sequences `0.9`, `0.51`,
`a` and `1.3`, `1.85`, `a`. In `c₀` the τ glyph drops out of extraction entirely, leaving a
bare `a` for `τ_a`, so those sequences are consistent with **either** reading:

| Reading | `c₁` | `c₂` |
| :--- | :--- | :--- |
| A — exponent on `τ_a` | `0.9·τ_a^0.51` | `1.3·τ_a^1.85` |
| B — exponent on the coefficient | `0.9^0.51·τ_a` = `0.949·τ_a` | `1.3^1.85·τ_a` = `1.62·τ_a` |

Physical plausibility does not discriminate: both give `g = 0.33` at `τ_a = 0`, and both
give `g > 1` at `τ_a = 0.5, g_a = 0.9`, which is invalid for a Henyey–Greenstein
asymmetry parameter — so the formula's validity domain is bounded under either reading and
cannot be used to rule one out. Reading A has the shape of an empirical fit with distinct
exponents; Reading B would more naturally have been written as `0.95` and `1.62`.

Resolving it needs the typeset Eq. 5, or Fig. 1's plotted `g` values to test a reproduction
against. Until then `g` stays an explicit input.

**Airglow calibration state.** The `Calibrated` mode implies a fitting procedure against
recent local measurements. Which observable is fitted, over what window, and how the
resulting uncertainty is reported are open and must be settled before Phase 2.
