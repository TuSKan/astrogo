---
type: Fixed
pr: 148
---
**Corrected the stated cost of the UTC-for-UT1 fallback.** `Time.GAST()` and
`satellite.subSatellitePoint` both described it as "a few hundred ms of error
at worst"; it is bounded by the leap-second system at 0.9 s — about 13.5
arcsec, or 420 m of sub-satellite ground position — and that bound disappears
when leap seconds end in 2035.
