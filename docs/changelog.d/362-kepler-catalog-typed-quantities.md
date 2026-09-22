---
type: Changed — BREAKING
pr: 362
---
**The orbital semi-major axis and three `catalog/resolve.Target` fields are now
typed**, closing the part of #130 that spans `ephemeris/kepler` and `catalog`
together.

`kepler.NewElements` takes a `unit.Length` semi-major axis and
`Elements.SemiMajorAxis` returns one; `ephemeris.NewElements` follows, since it
re-exports it. The element set was already half-typed — `Inclination`,
`AscendingNode`, `ArgPeriapsis` and `MeanAnomaly` are `angle.Angle` — and the
semi-major axis was the one member still carrying its unit in a doc comment.

`resolve.Target.SemiMajorAxis` and `.Diameter` become `unit.Length`, and
`.RadialVelocity` becomes `unit.Velocity`. The providers name the unit where
they decode it: SBDB's `phys_par` diameter in kilometers, SIMBAD's
`rvz_radvel` in km/s, MPCORB's semi-major axis in astronomical units.

`Elements.WithPeriod`/`Period` stay `float64` days: a duration, and `unit` has
no type for one.
