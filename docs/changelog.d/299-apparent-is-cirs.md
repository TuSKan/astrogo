---
type: Fixed
pr: 299
---
**Documentation: the CIRS place says so.** The type now called `coord.CIRS`
had a doc comment saying only "the true geocentric position of an object",
which reads as the equinox-based apparent place it is not. A caller comparing
its `RA()` against an almanac's apparent right ascension was 20 arcminutes out
with nothing in the type saying why. The comment now names the system and
points at `TETE` (#126). It was renamed from `Apparent` in the same release
(#298), which is the other half of the same fix.
