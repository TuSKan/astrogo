# The Great Planet Parade of 2025

**Computing the Seven-Planet Evening Alignment with `astrogo`**

---

*"When you have eliminated the impossible, whatever remains, however improbable, must be the truth."*
— Arthur Conan Doyle

---

## The Event

On the evening of **February 28, 2025**, all seven planets of the solar system — Mercury,
Venus, Mars, Jupiter, Saturn, Uranus, and Neptune — were simultaneously above the horizon
in the evening sky. This "Planet Parade" was widely covered by media outlets worldwide and
photographed by thousands of amateur astronomers.

But what does "all seven planets aligned" actually mean? Were they really aligned? How close
were they? Could you really see them? And when exactly was the optimal viewing window?

These are questions that can be answered with computation, not opinion. Let's use `astrogo`
to reconstruct the entire evening — to sub-minute precision — and verify every claim made
in the press.

---

## Table of Contents

- [Part I: What Is an "Alignment"?](#part-i-what-is-an-alignment)
- [Part II: The Evening of February 28](#part-ii-the-evening-of-february-28)
- [Part III: Visibility Analysis](#part-iii-visibility-analysis)
- [Part IV: The Conjunction Geometry](#part-iv-the-conjunction-geometry)
- [Part V: How Rare Is This?](#part-v-how-rare-is-this)
- [Appendix: Runnable Code](#appendix-runnable-code)

---

## Part I: What Is an "Alignment"?

The word "alignment" in popular astronomy is misleading. The planets don't line up like
pearls on a string. They orbit the Sun in slightly different planes (inclinations from
0.77° for Jupiter to 7.0° for Mercury), so they can never truly align in three dimensions.

What the media calls a "planet parade" is an **ecliptic clustering**: all planets happen
to be on the same side of the Sun as seen from Earth, so they appear in the same half of
the sky. Because all planetary orbits are roughly co-planar (within ~7° of the ecliptic),
they always appear near the ecliptic — a great circle on the sky. A "parade" means they
are spread along this circle in the same general direction.

`astrogo` can compute the **ecliptic longitude** of each planet at any moment. When all
longitudes fall within a span of less than 180°, they're all in the same half of the sky.
When the span is less than 90°, the clustering is tight.

```go
prov, _ := eph.NewProvider(eph.Planets, "de442")
defer prov.Close()

t := time.Date(2025, time.February, 28, 19, 0, 0, 0, time.LocationUTC)
ctx := coord.NewContext(t, loc, atmosphere.AtAltitude(760))

planets := []plan.Target{
    plan.NewMercury(prov), plan.NewVenus(prov), plan.NewMars(prov),
    plan.NewJupiter(prov), plan.NewSaturn(prov),
    plan.NewUranus(prov), plan.NewNeptune(prov),
}

for _, p := range planets {
    icrs, _ := p.Position(t)
    altaz, _ := ctx.ICRSToAltAz(icrs)
    ecl := coord.ICRSToEcliptic(icrs, t)
    fmt.Printf("%-8s  λ=%6.1f°  Alt=%+5.1f°  Az=%5.1f°\n",
        p.Name(), ecl.Lon().Degrees(), altaz.Alt().Degrees(), altaz.Az().Degrees())
}
```

### The Result

On February 28, 2025 at civil dusk (18:58 BRT / 21:58 UTC), from São Paulo (23.55°S, 46.63°W, 760m):

| Planet | Ecliptic λ | Altitude | Azimuth | Airmass |
|---|---|---|---|---|
| **Mercury** | 356.2° | +4.7° | 270.5° (W) | 10.9 |
| **Venus** | 10.8° | +9.6° | 285.8° (WNW) | 5.8 |
| **Neptune** | 358.9° | +7.6° | 271.5° (W) | 7.2 |
| **Saturn** | 350.7° | +2.5° | 265.1° (W) | 17.3 |
| **Mars** | 107.2° | +34.1° | 29.6° (NNE) | 1.8 |
| **Jupiter** | 72.3° | +43.3° | 345.7° (NNW) | 1.5 |
| **Uranus** | 53.6° | +38.4° | 321.8° (NW) | 1.6 |

The ecliptic longitude span: **116°** — all seven are in the same half of the sky. ✓

But the key insight is the **altitude** column. Saturn at +2.5° and Mercury at +4.7° are
barely above the horizon in bright twilight, while Mars and Jupiter are high overhead.

---

## Part II: The Evening of February 28

To fully reconstruct the observing window, we need the exact sunset time, twilight
boundaries, and the setting times of each planet. This is where `astrogo`'s USNO-validated
rise/set engine (≤0.6 min accuracy) becomes critical.

### Observing Window

From São Paulo on February 28, 2025 (computed with JPL DE442):

| Event | Time (BRT) | Note |
|---|---|---|
| **Sunset** | **18:39:30** | Sun below −0.833° (USNO convention) |
| Civil dusk | 18:58:48 | Sun below −6° — Mercury becomes findable |
| Nautical dusk | 19:25:36 | Sun below −12° — faint planets possible |
| Astronomical dusk | 19:52:44 | Sun below −18° — full darkness |

```go
prov, _ := eph.NewProvider(eph.Planets, "de442")
defer prov.Close()

loc, _ := coord.NewEarthLocation(-23.5505, -46.6333, 760)
site, _ := plan.NewSite("São Paulo", loc, plan.WithTimeZone(brtz))

day := time.Date(2025, 2, 28, 0, 0, 0, 0, brtz)
next := day.Add(24 * time.Hour)

_, sunset, _ := plan.SunriseSunset(day, next, site, prov)
_, civilDusk, _ := plan.CivilDawnDusk(day, next, site, prov)
_, nautDusk, _ := plan.NauticalDawnDusk(day, next, site, prov)
_, astroDusk, _ := plan.AstronomicalDawnDusk(day, next, site, prov)
```

### The Altitude Timeline

This table shows how each planet's altitude evolved throughout the evening, sampled
every 5 minutes from sunset. Exact set times (from 1-minute resolution) are annotated
where they fall between grid points.

| Time (BRT) | Mercury | Venus | Mars | Jupiter | Saturn | Uranus | Neptune |
|---|---|---|---|---|---|---|---|
| **18:39** (sunset) | +9° | +14° | +32° | +44° | +7° | +41° | +12° |
| 18:44 | +8° | +13° | +32° | +44° | +6° | +40° | +11° |
| 18:49 | +7° | +12° | +33° | +44° | +5° | +40° | +10° |
| 18:54 | +6° | +10° | +34° | +44° | +3° | +39° | +9° |
| **18:59** (civil dusk) | +5° | +9° | +34° | +43° | +2° | +38° | +7° |
| 19:04 | +3° | +8° | +35° | +43° | +1° | +38° | +6° |
| 19:09 | +2° | +7° | +35° | +43° | **0°** | +37° | +5° |
| *19:10* | | | | | **set** ↓ | | |
| 19:14 | +1° | +6° | +36° | +42° | — | +36° | +4° |
| *19:19* | **set** ↓ | +5° | +36° | +42° | — | +35° | +3° |
| 19:24 | — | +4° | +37° | +41° | — | +35° | +2° |
| **19:26** (nautical dusk) | — | +3° | +37° | +41° | — | +34° | +1° |
| 19:29 | — | +3° | +37° | +41° | — | +34° | +1° |
| *19:32* | — | +2° | +37° | +41° | — | +33° | **set** ↓ |
| 19:34 | — | +2° | +38° | +41° | — | +33° | — |
| 19:39 | — | +1° | +38° | +40° | — | +32° | — |
| *19:42* | — | **set** ↓ | +38° | +40° | — | +32° | — |
| 19:44 | — | — | +38° | +40° | — | +31° | — |
| 19:49 | — | — | +39° | +39° | — | +30° | — |
| **19:53** (astro dusk) | — | — | +39° | +39° | — | +30° | — |

The exact set times (1-minute resolution, SOFA refraction model):

| Planet | Sets (BRT) | Sky Condition | Window |
|---|---|---|---|
| **Saturn** | **19:10** | Civil twilight — bright sky | 31 min after sunset |
| **Mercury** | **19:19** | Late civil twilight | 40 min after sunset |
| **Neptune** | **19:32** | Nautical twilight | 53 min after sunset |
| **Venus** | **19:42** | Late nautical twilight | 63 min after sunset |

The timeline reveals the fundamental problem with the "seven-planet alignment" headline:

- **Saturn** sets at 19:10 BRT — 11 minutes after sunset, still in bright civil twilight
- **Mercury** sets at 19:19 BRT — gone 7 minutes before nautical dusk
- **Neptune** sets at 19:32 BRT — needs a telescope AND dark sky, but sky is still nautical twilight
- **Venus** sets at 19:42 BRT — gone 11 minutes before astronomical dusk at 19:53

The simultaneous visibility window — all seven above the horizon in a dark enough sky —
was approximately **zero minutes** for naked-eye observers.

---

## Part III: Visibility Analysis

### What Was Actually Visible?

Let's be honest about what a casual observer from São Paulo would have seen:

| Planet | Naked Eye? | Alt at civil dusk | Notes |
|---|---|---|---|
| **Jupiter** | ✅ Brilliant | +43° | Unmissable, high in the NNW |
| **Uranus** | ❌ Telescope | +38° | Too faint naked-eye, needs dark sky + optics |
| **Mars** | ✅ Bright | +34° | Reddish, steady in the NNE |
| **Venus** | ✅ Brilliant | +10° | Brightest object in the sky, low WNW |
| **Neptune** | ❌ Telescope | +8° | Always requires optics |
| **Mercury** | ⚠️ Difficult | +5° | Bright but deep in twilight glow, low |
| **Saturn** | ⚠️ Difficult | +2° | Faint and barely above horizon |

The honest headline should have been: **"Three planets brilliant, two difficult, two need a telescope."**

### The Mercury Problem

Mercury is the hardest planet to see. It orbits closest to the Sun, so it never strays
far from the horizon at sunset. On February 28, Mercury was only +9° above the horizon at
sunset and dropping fast. By civil dusk it was at +5°. The observing window for Mercury
was approximately **20 minutes** between first visibility and setting.

---

## Part IV: The Conjunction Geometry

While the "parade" was the headline event, the real astronomical story was the
**near-conjunctions** happening within the parade. `astrogo`'s conjunction solver
can identify the closest pairings.

### Near-Conjunctions (February 15 – March 15, 2025)

```go
appulses, _ := plan.Appulses(searchStart, searchEnd,
    plan.NewMercury(prov), plan.NewSaturn(prov))
```

| Pair | Closest Approach | Separation | Date |
|---|---|---|---|
| **Mercury – Saturn** | Feb 25, 09:44 UTC | **1.44°** | Within binocular FOV |
| Mars – Jupiter | Mar 3, 17:16 UTC | 35.06° | Same general region |

The Mercury–Saturn pairing at just 1.44° was the tightest conjunction of the parade.
Both were low in the west, so catching them together required a clear western horizon and
impeccable timing — the pair was only 7° up at sunset and sinking fast.

### Venus and Neptune: Hidden in Plain Sight

Venus (ecliptic λ = 10.8°) and Neptune (λ = 358.9°) were separated by only ~12° in
ecliptic longitude. They were close enough that pointing binoculars at brilliant Venus
would have Neptune in or near the same field of view — one of the rare moments when
Neptune is easy to locate.

---

## Part V: How Rare Is This?

The press called it "once in a lifetime." Was it?

A seven-planet clustering where all ecliptic longitudes span less than 180° (same half
of the sky) occurs roughly every **5–8 years**. A tighter clustering (< 120°) is rarer,
happening every **15–25 years**. The truly spectacular "all seven within 90°" events
are **once per century** phenomena.

The February 2025 event (116° span) was solidly in the "interesting" category — visible
to billions, photogenic, educational — but not the tightest possible configuration.

For comparison, on **May 5, 2000**, Mercury, Venus, Mars, Jupiter, and Saturn were all
within 26° of ecliptic longitude — but below the horizon during conjunction (behind the
Sun). The planets don't care about our viewing convenience.

---

## Appendix: Runnable Code

All computations in this document are reproduced by the example program:

**Program:** [`examples/16_planet_parade/`](../examples/16_planet_parade/)

```sh
go run ./examples/16_planet_parade/
```

### What the program computes:

1. **Solar events** — Sunset, civil/nautical/astronomical dusk via `plan.SunriseSunset()`
   and `plan.CivilDawnDusk()` (validated to ≤0.6 min vs USNO)
2. **Planetary positions** — Altitude, azimuth, ecliptic longitude for all 7 planets at
   15-minute intervals using `plan.NewMercury(prov)` etc. with JPL DE442
3. **Conjunctions** — Closest pairings via `plan.Appulses()` with Chandrupatla root-finding
4. **Altitude timeline** — Per-planet tracking from sunset through darkness
5. **Ecliptic clustering** — Minimum arc span analysis

### Dependencies

- `astrogo` — all computation
- **JPL DE442** (auto-downloaded, ~100 MB) — covers 1550–2650 AD

### Verification

Every number in this document can be cross-checked against:
- **JPL Horizons** — [https://ssd.jpl.nasa.gov/horizons/](https://ssd.jpl.nasa.gov/horizons/)
- **Stellarium** — free open-source planetarium
- **USNO API** — [https://aa.usno.navy.mil/data/api](https://aa.usno.navy.mil/data/api)
- **Your own photographs** from the evening of February 28, 2025

---

## Technical Notes

### Rise/Set Precision

All rise/set times use `astrogo`'s USNO-validated pipeline:
- Standard atmospheric refraction: 34' (0.5667°)
- Solar semi-diameter: 16' (0.2667°)
- Total threshold at sea level: −50' (−0.8333°)
- Accuracy: **≤0.6 min** vs USNO across 41 edge-case tests

### Refraction at Low Altitude

Mercury at 5° altitude is significantly affected by atmospheric refraction (~10
arcminutes). `astrogo` uses SOFA's Refa/Refb coefficients (from `Apco13`) for all
altitude queries, ensuring physically correct refraction even at the horizon.

### Apparent Magnitude

`astrogo` does not compute apparent magnitudes. The visibility assessments in this
document are qualitative, based on each planet's typical brightness range and altitude.
For precise apparent magnitudes (which depend on phase angle, heliocentric distance,
geocentric distance, and surface albedo), consult JPL Horizons or the Astronomical
Almanac.

---

*All computations performed with `astrogo` using JPL DE442 ephemerides and SOFA-based
coordinate transformations (IAU 2006/2000A). Rise/set times validated against
the U.S. Naval Observatory API to ≤0.6 min accuracy.*
