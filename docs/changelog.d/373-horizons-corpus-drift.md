---
type: Changed
pr: 373
---
**The Horizons reference corpus is refreshed**, after establishing what moved and
why: Horizons changed something below its own emission resolution, and 94 of 300
values sat near enough to a rounding boundary to tip. Every change is one unit in
the last digit Horizons prints — 1e-06 deg for azimuth and elevation, 1e-14 AU at
Jupiter and 1e-13 at Saturn for range — while RA/Dec and the geocentric vectors
did not move at all. Two new tests keep the question answerable: one asks
Horizons the same query twice to tell a recomputation from a re-rounding, and one
checks the manifest's record of the query against what the code would ask today,
since a moved observatory reports as changed values rather than a changed query.
The generator's diff summary now groups by field instead of reporting a single
maximum.
