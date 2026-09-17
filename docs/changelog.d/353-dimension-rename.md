---
type: Changed — BREAKING
pr: 353
---
**`unit`'s `Dimension` values are renamed with a `Dim` prefix** —
`unit.Length` → `unit.DimLength`, `unit.Velocity` → `unit.DimVelocity`, and the
same for the other fifteen. The short names are now the quantity types callers
write in signatures, which is where they will be typed hundreds of times; a
`Dimension` named `Length` was always ambiguous with an actual length, and
appears in about ten unit-table declarations. Exactly one call site outside
`unit` was affected (`constants/units.go`).
