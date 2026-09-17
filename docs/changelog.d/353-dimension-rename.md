---
type: Changed — BREAKING
pr: 353
---
**`unit`'s `Dimension` values are renamed with a `Dim` prefix** —
`unit.Length` → `unit.DimLength`, `unit.Velocity` → `unit.DimVelocity`, and the
same for the other fourteen. `unit.Dimensionless` keeps its name: the prefix
exists to separate a dimension from a quantity type of the same name, and a
dimensionless quantity is a `float64`, so there is nothing for it to be
confused with and `DimDimensionless` would stutter for no reason. The short names are now the quantity types callers
write in signatures, which is where they will be typed hundreds of times; a
`Dimension` named `Length` was always ambiguous with an actual length, and
appears in about ten unit-table declarations. Exactly one call site outside
`unit` was affected (`constants/units.go`).
