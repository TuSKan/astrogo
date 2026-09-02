---
type: Added
pr: 90
---
**Radial velocity now works for every target kind.** `plan.RadialVelocity`
returns it for a solar-system body, computed from the ephemeris and agreeing
with JPL Horizons to under 6 m/s, and `*DeepSkyObject` carries the catalog
value SIMBAD publishes for galaxies. Adds
`coord.Context.TopocentricRadialVelocity` and `constants.WGS84.AngularVelocity`.
