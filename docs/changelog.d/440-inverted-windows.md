---
type: Fixed
pr: 440
---
**A `plan.Window` whose End is before its Start is empty**: it overlaps
nothing, and `Union`, `Intersect`, `Subtract` and `TotalDuration` leave it
out, where `Subtract` returned it whole from under a window covering it,
`TotalDuration` counted it as negative time, and `Union` returned it beside a
window it overlapped (#421).
