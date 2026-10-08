---
type: Fixed
pr: 614
---
**Gaia DR3's reference epoch J2016.0 was written as JD 2457388.5**, half a day early, in `catalog/gaia` and `catalog/vizier`. SIMBAD's J2000 was built on the UTC scale. Both now match their definitions; the effect on any propagated position is under 0.015″.
