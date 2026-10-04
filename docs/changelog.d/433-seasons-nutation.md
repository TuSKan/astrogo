---
type: Fixed
pr: 433
---
**`plan.Seasons` includes nutation in longitude**, which SOFA's `Eqec06` does
not apply, and takes the Sun's apparent place from the ephemeris: equinoxes and
solstices that were up to 8.5 minutes off now agree with Skyfield to under a
second and with USNO to its rounding (#414).
