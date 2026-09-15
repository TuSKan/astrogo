---
type: Fixed
pr: 299
---
**Documentation: `coord.Apparent` is CIRS.** Its doc comment said only "the
true geocentric position of an object", which reads as the equinox-based
apparent place it is not. A caller comparing `Apparent.RA()` against an
almanac's apparent right ascension was 20 arcminutes out with nothing in the
type saying why. The comment now names the system and points at `TETE` (#126).
