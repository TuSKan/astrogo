# Equinox & Solstice Almanac

**Predicting Earth's Seasons from First Principles with `astrogo`**

---

## Overview

This showcase computes all equinoxes, solstices, Earth apsides, moon phases, and eclipses
for 2024–2033 using JPL DE442 ephemerides and sub-second Chandrupatla root refinement.

Every number is derived from the gravitational physics encoded in NASA's planetary
ephemerides — no lookup tables, no analytical approximations, no curve fits.

> **Observer:** São Paulo, Brazil (23°33'02"S, 46°37'60"W, 760m).
> All times are **BRT** (UTC−3). Equinoxes, solstices, apsides, and eclipses are
> geocentric events — they occur at the same instant worldwide, displayed here in local time.

**Run it yourself:**

```sh
go run ./examples/17_equinox_prediction/
```

---

## Equinoxes & Solstices (2024–2033)

The Sun's ecliptic longitude crosses 0° (vernal equinox), 90° (summer solstice),
180° (autumnal equinox), and 270° (winter solstice). AstroGo finds these crossings by:

1. Sampling the Sun's ecliptic longitude daily
2. Detecting when a target longitude is crossed
3. Refining to sub-second precision via Chandrupatla's method

```go
events, _ := plan.Seasons(2026, prov)
for _, e := range events {
    fmt.Printf("%-20s %s\n", e.Season, e.Time.In(brtz).Format("Jan 02 15:04:05"))
}
```

| Year | Vernal Equinox | Summer Solstice | Autumnal Equinox | Winter Solstice |
|------|----------------|-----------------|------------------|-----------------|
| 2024 | Mar 20 00:04:37 | Jun 20 17:49:39 | Sep 22 09:42:45 | Dec 21 06:20:17 |
| 2025 | Mar 20 06:01:44 | Jun 20 23:43:14 | Sep 22 15:20:47 | Dec 21 12:04:57 |
| 2026 | Mar 20 11:48:26 | Jun 21 05:27:55 | Sep 22 21:08:55 | Dec 21 17:53:57 |
| 2027 | Mar 20 17:29:19 | Jun 21 11:16:12 | Sep 23 03:07:05 | Dec 21 23:47:30 |
| 2028 | Mar 19 23:23:15 | Jun 20 17:08:34 | Sep 22 08:51:48 | Dec 21 05:26:06 |
| 2029 | Mar 20 05:08:43 | Jun 20 22:55:32 | Sep 22 14:45:31 | Dec 21 11:20:45 |
| 2030 | Mar 20 10:58:56 | Jun 21 04:38:35 | Sep 22 20:33:37 | Dec 21 17:15:37 |
| 2031 | Mar 20 16:47:15 | Jun 21 10:23:26 | Sep 23 02:20:50 | Dec 21 23:00:24 |
| 2032 | Mar 19 22:26:43 | Jun 20 16:13:16 | Sep 22 08:14:38 | Dec 21 04:59:04 |
| 2033 | Mar 20 04:25:24 | Jun 20 22:03:30 | Sep 22 13:53:16 | Dec 21 10:46:49 |

These times match the U.S. Naval Observatory's published values to within **1 minute**
(validated by 41 integration tests in `plan/usno_test.go`).

---

## Season Durations and Kepler's Second Law

The four seasons are **not equal** in length. This asymmetry is a direct consequence
of Earth's orbital eccentricity (e ≈ 0.0167) and Kepler's second law: the Earth
sweeps equal areas in equal times, so it moves faster near perihelion (January) and
slower near aphelion (July).

| Season (N. Hemisphere) | Duration | Days |
|------------------------|----------|------|
| Spring (Equinox → Solstice) | 92d 17h | 92.74 |
| **Summer** (Solstice → Equinox) | **93d 15h** | **93.65** |
| Autumn (Equinox → Solstice) | 89d 20h | 89.86 |
| **Winter** (Solstice → Equinox) | **88d 23h** | **88.98** |
| **Tropical year** | | **365.24** |

Northern summer is **4.7 days longer** than northern winter — a measurable effect
of the Earth being near aphelion during July.

---

## Earth's Apsides

```go
apsides, _ := plan.Apsides(2026, prov)
```

| Event | Date (BRT) | Distance |
|-------|-----------|----------|
| Perihelion | Jan 03 14:15:38 | 0.983302 AU |
| Aphelion | Jul 06 14:30:40 | 1.016644 AU |

**Orbital eccentricity:** e = 0.016671

The 3.3% distance difference produces a **7% flux difference** — perihelion
receives ~1,412 W/m² vs aphelion ~1,318 W/m². This is overwhelmed by axial tilt
for seasonal temperatures, but it measurably affects season durations.

---

## Eclipses of 2026

2026 features four eclipses — two total lunar and two solar:

| Type | Date (BRT) | |β| | γ | Visible from São Paulo? |
|------|-----------|------|-------|--------------------------|
| 🌕 Solar (Total/Annular) | Feb 17 09:12 | 0.919° | 0.581 | ❌ No — path crosses Antarctica/S. Atlantic |
| 🌑 Lunar (Total) | Mar 03 08:34 | 0.358° | 0.227 | ✅ Yes — visible at moonset (partial) |
| 🌕 Solar (Total/Annular) | Aug 12 14:46 | 0.887° | 0.562 | ❌ No — path crosses Europe/N. Africa |
| 🌑 Lunar (Total) | Aug 28 01:13 | 0.463° | 0.293 | ✅ Yes — fully visible overnight |

Both lunar eclipses have very low |β| (ecliptic latitude), indicating deep, central
passages through Earth's shadow. The γ values (0.23 and 0.29) confirm these are
near-central total eclipses with long totality durations.

> **Note:** Eclipse times are the moment of **greatest eclipse** (geocentric). Solar eclipse
> visibility depends on the narrow shadow path; lunar eclipses are visible from the
> entire night hemisphere.

---

## Topocentric Moon (v0.1.3)

The v0.1.3 release added topocentric corrections for all moving bodies. The Moon
benefits most — its diurnal parallax is ~1° (the Moon is only ~60 Earth radii away).

**Observer:** São Paulo, Brazil (23°33'02"S, 46°37'60"W, 760m elevation)

At the moment of the 2026 Vernal Equinox (Mar 20 11:48:26 BRT):

| Property | Value |
|----------|-------|
| RA | 01h 10m 41.2s |
| Dec | +11° 39' 49" |
| Altitude | +47° 21' 51" |
| Distance | 0.0024 AU |
| Elongation | 21.4° |
| Illumination | 3.3% |
| Moonrise | 07:30:47 BRT |
| Moonset | 19:19:12 BRT |

The RA/Dec are **topocentric** — corrected for the observer's position on Earth's
surface. This is critical for the Moon: the geocentric and topocentric positions
can differ by up to 1° in declination.

---

## Implementation Notes

- **Ecliptic longitude:** computed via SOFA's IAU 2006 precession + IAU 2000A nutation
  (`Eqec06`), with the 20.496" aberration constant subtracted for the Sun's apparent position
- **Root finding:** Chandrupatla's method with guaranteed convergence and sub-second precision
- **Eclipse detection:** ecliptic latitude filtering at syzygy (Danjon limit ≈1.58° penumbral)
- **Topocentric correction:** observer ICRS vector subtracted from geocentric body vector
  (`ctx.ObsVec()`)

---

## References

- Meeus, J. (1998). *Astronomical Algorithms*, 2nd ed.
- Standish, E.M. (1998). JPL Planetary Ephemerides DE405/DE406.
- U.S. Naval Observatory. *Astronomical Applications Department*.
- Chandrupatla, T.R. (1997). *A New Hybrid Quadratic/Bisection Algorithm for Finding the Zero of a Nonlinear Function Without Using Derivatives*.
