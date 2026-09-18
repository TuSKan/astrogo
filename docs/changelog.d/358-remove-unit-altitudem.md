---
type: Removed
pr: 358
---
**`unit.AltitudeM` no longer exists.** Use `unit.Length`, which stores meters,
so every value that type held is already correct: a conversion becomes
`unit.Meters(2635)` and a declaration changes type name only.

It named one quantity in one unit, which is the pattern `unit.Length` exists
to replace — a caller holding one could not ask for it in kilometers, and a
length crossing into `coord` or `plan` needed a cast at every boundary.
`atmosphere` was the only package using it.
