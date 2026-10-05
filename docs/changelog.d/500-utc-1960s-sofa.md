---
type: Changed
pr: 500
---
**UTC from 1960 to 1971 is read as SOFA reads it**: TT through SOFA's TAI−UTC
rather than ΔT, and the days UTC jumped by a fraction of a second stretched as
iauDtf2d stretches them. `TT()` and `TAI().TT()` now agree there (#479).
Also fixes a UTC label an ulp below a leap-second midnight converting to TAI a
second early (#499).
