# Equinox & Solstice Almanac

**Predicting Earth's Seasons from First Principles with `astrogo`**

---

## Overview

This showcase computes all equinoxes, solstices, Earth apsides, moon phases, and eclipses
for 2024–2033 using JPL DE442 ephemerides and sub-second Chandrupatla root refinement.

Every number is derived from the gravitational physics encoded in NASA's planetary
ephemerides — no lookup tables, no analytical approximations, no curve fits.

> **Observer:** Quinta Calixto, Brazil (22°31'43"S, 46°28'23"W, 835 m).
> All times are **BRT** (UTC−3). Equinoxes, solstices, apsides, and eclipses are
> geocentric events — they occur at the same instant worldwide, displayed here in local time.

**Run it yourself:**

```sh
go -C examples run ./17_equinox_prediction/
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
| 2024 | Mar 20 00:06:24 | Jun 20 17:50:59 | Sep 22 09:43:39 | Dec 21 06:20:34 |
| 2025 | Mar 20 06:01:28 | Jun 20 23:42:15 | Sep 22 15:19:20 | Dec 21 12:03:05 |
| 2026 | Mar 20 11:45:57 | Jun 21 05:24:30 | Sep 22 21:05:13 | Dec 21 17:50:14 |
| 2027 | Mar 20 17:24:41 | Jun 21 11:10:50 | Sep 23 03:01:43 | Dec 21 23:42:09 |
| 2028 | Mar 19 23:17:08 | Jun 20 17:02:00 | Sep 22 08:45:18 | Dec 21 05:19:39 |
| 2029 | Mar 20 05:01:58 | Jun 20 22:48:17 | Sep 22 14:38:30 | Dec 21 11:14:06 |
| 2030 | Mar 20 10:52:05 | Jun 21 04:31:18 | Sep 22 20:26:53 | Dec 21 17:09:37 |
| 2031 | Mar 20 16:40:58 | Jun 21 10:17:08 | Sep 23 02:15:18 | Dec 21 22:55:33 |
| 2032 | Mar 19 22:21:53 | Jun 20 16:08:46 | Sep 22 08:10:53 | Dec 21 04:55:56 |
| 2033 | Mar 20 04:22:43 | Jun 20 22:01:08 | Sep 22 13:51:40 | Dec 21 10:46:00 |

These times match the U.S. Naval Observatory's published values to within **1 minute**,
which is USNO's own rounding (`TestUSNO_Seasons` in `plan/usno_test.go`, 20 events across
2020–2035), and Skyfield's to under a second. Until #414 nutation in longitude was left
out, and this table ran up to 7 minutes late: its 2027 vernal equinox read 17:29:19.

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

2026 features four eclipses — two lunar and two solar:

| Type | Date (BRT) | |β| | γ | Visible from Quinta Calixto? |
|------|-----------|------|-------|--------------------------|
| 🌕 Solar (Annular) | Feb 17 09:11 | 0.919° | 0.626 | ❌ No — path crosses Antarctica/S. Atlantic |
| 🌑 Lunar (Total) | Mar 03 08:33 | 0.358° | 0.240 | ✅ Yes — visible at moonset (partial) |
| 🌕 Solar (Total) | Aug 12 14:45 | 0.888° | 0.585 | ❌ No — path crosses Europe/N. Africa |
| 🌑 Lunar (Partial) | Aug 28 01:12 | 0.462° | 0.317 | ✅ Yes — fully visible overnight |

Both lunar eclipses have low |β| (ecliptic latitude), and their γ values (0.24 and
0.32, the Moon's closest approach to the shadow axis as a fraction of the grazing
distance) put both deep in Earth's shadow. March 3 is total; August 28 is a deep
partial, umbral magnitude 0.93 in NASA's canon.

> **Note:** Eclipse times are the moment of **greatest eclipse** (geocentric). Solar eclipse
> visibility depends on the narrow shadow path; lunar eclipses are visible from the
> entire night hemisphere.

---

## Topocentric Moon (v0.1.3)

The v0.1.3 release added topocentric corrections for all moving bodies. The Moon
benefits most — its diurnal parallax is ~1° (the Moon is only ~60 Earth radii away).

**Observer:** Quinta Calixto, Brazil (22°31'43"S, 46°28'23"W, 835 m elevation)

At the moment of the 2026 Vernal Equinox (Mar 20 11:45:57 BRT):

| Property | Value |
|----------|-------|
| RA | 01h 10m 38.1s |
| Dec | +11° 38' 14" |
| Altitude | +47° 56' 54" |
| Distance | 0.0024 AU |
| Elongation | 21.4° |
| Illumination | 3.3% |
| Moonrise | 07:36 BRT |
| Moonset | 19:12 BRT |

The RA/Dec are **topocentric** — corrected for the observer's position on Earth's
surface. This is critical for the Moon: the geocentric and topocentric positions
can differ by up to 1° in declination.

---

## Implementation Notes

- **Ecliptic longitude:** the Sun's apparent place from the ephemeris (light time and
  aberration), referred to the mean ecliptic and equinox of date by SOFA's `Eqec06` (IAU 2006
  precession), plus the IAU 2000A nutation in longitude from `Nut06a` for the true equinox —
  which `Eqec06` does not apply (#414)
- **Root finding:** Chandrupatla's method with guaranteed convergence and sub-second precision
- **Eclipse detection:** at each syzygy, greatest eclipse against that month's shadow,
  sized as NASA's Five Millennium Canons size it (Danjon's rule for Earth's shadow, the
  WGS 84 spheroid for the Moon's), which agrees with the canons on every eclipse in six
  centuries (#401)
- **Topocentric correction:** observer ICRS vector subtracted from geocentric body vector
  (`ctx.ObsVec()`)

---

## References

- Meeus, J. (1998). *Astronomical Algorithms*, 2nd ed.
- Standish, E.M. (1998). JPL Planetary Ephemerides DE405/DE406.
- U.S. Naval Observatory. *Astronomical Applications Department*.
- Chandrupatla, T.R. (1997). *A New Hybrid Quadratic/Bisection Algorithm for Finding the Zero of a Nonlinear Function Without Using Derivatives*.
