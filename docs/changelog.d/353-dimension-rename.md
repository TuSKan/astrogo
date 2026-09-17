---
type: Changed — BREAKING
pr: 353
---
**The `Dimension` type and its seventeen values move from `unit` into a new
`unit/dim` package**, so they are now `dim.Dimension`, `dim.Length`,
`dim.Velocity` and so on. `unit.Unit` keeps its `Dimension` field, retyped to
`dim.Dimension`.

The names collided: the dimension of a length held the name a caller wants for
the *quantity* a signature carries, which is what `unit.Length` and
`unit.Velocity` now are. Prefixing the dimensions instead would have read as
`DimVolume.Div(DimMass).Div(DimTime.PowInt(2))` at the places that actually use
them — `constants/units.go` composes six units that way — and the dimensionless
one stuttered. A package is how Go namespaces, so those lines now say
`dim.Volume.Div(dim.Mass).Div(dim.Time.PowInt(2))`.

Dimensions are also the more primitive idea: a unit is a scale on a dimension,
so `unit` imports `dim` and not the reverse. Fifteen call sites outside `unit`
were affected, all in `constants`.
