---
type: Changed — BREAKING
pr: 355
---
**`coord`'s distances and speeds are now `unit.Length` and `unit.Velocity`
instead of `float64`.** Every signature that carried a length or a speed changed:
`ICRS`/`AltAz`/`Galactic`/`Ecliptic`'s `Dist`/`SetDist`, `Astrometric.RV`/`SetRV`,
`NewICRSWithKinematics`, `NewFK4WithProperMotion`, `NewFK5WithProperMotion` and
`FK4.RV`/`FK5.RV`, `Geodetic.Height`/`NewGeodetic`/`MustGeodetic`, `Ellipsoid.A`,
`NewObserversLocation`/`SetHeight`, `GroundDistance`, `Offset`,
`ParallaxDistance`, `SpaceSpeed`, `LSRCorrection`, `LSRApex`, the
`GalactocentricFrame` parameters and `FromICRS`/`ToICRS`, `Galactocentric`'s
`X`/`Y`/`Z`/`Distance`/`Radius` and its two constructors, and `Context`'s five
radial-velocity methods. `ephemeris/satellite.Satellite.Altitude` follows, since
it returns a `Geodetic` height.

`ICRS.Dist` is the reason: it held astronomical units on an ephemeris path and
kilometers on a satellite one, and nothing in the signature said which. Callers
read the unit they want — `d.Km()`, `d.AU()`, `d.Pc()` — and write the unit they
mean — `unit.KmPerSec(-7.6)`. Named `float64`s, so this costs nothing: measured
at 1.88 ns and zero allocations against the same for a bare `float64`.

`NewEarthLocation` deliberately keeps plain `float64` degrees and meters, since
its whole purpose is to accept numbers copied off a GPS or a map service.

Part of #130; `plan`, `ephemeris`, `atmosphere`, `magnitude` and `optics` still
take bare `float64` and convert at the `coord` boundary.
