---
type: Fixed
pr: 440
---
**A `plan.Window` whose End is before its Start is empty**: it overlaps
nothing, and `Union`, `Intersect`, `Subtract` and `TotalDuration` leave it
out, where they subtracted from it as if whole, counted it as negative time,
and returned it beside a window it overlapped (#421).
