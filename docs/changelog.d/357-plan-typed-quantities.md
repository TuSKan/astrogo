---
type: Changed — BREAKING
pr: 357
---
**`plan`'s distances and speeds are now `unit.Length` and `unit.Velocity`**,
following `coord` in #355. Two optional-capability interfaces changed, so a
target type outside this repo that implements either needs its signature
updated: `MeasuredRadialVelocity() (unit.Velocity, bool)` and
`PhysicalRadius() (unit.Length, bool)`.

Also retyped: `WithRadialVelocity`, `WithDSORadialVelocity`, `WithDiameter`,
`RadialVelocity`, `BodyEquatorialRadius`, `TargetDetails.Distance`,
`ApsisEvent.Distance`, `PassEvent.Range` and `MeteorShower.VelocityKmS` — the
last renamed to `Velocity`, since the unit is no longer part of the name.
`Site.Height` is new and returns a `unit.Length`; `Site.HeightMeters` is
deprecated in its favour.

`TargetDetails` is the one worth reading about. Its `Distance float64` meant
parsecs for a star, au for a planet and kilometers for a satellite, and the
neighbouring `DistanceUnit string` was the only record of which — so a caller
reading `Distance` without reading `DistanceUnit` alongside it got a number
three different scales could produce. `Distance` now carries its own unit and
`DistanceUnit` only decides which one `String` prints.

`NewSiteEarthLocation` deliberately keeps plain `float64` degrees and meters,
for the reason `coord.NewEarthLocation` does.
