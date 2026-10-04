---
type: Fixed
pr: 459
---
**`plan.SatellitePasses` returns propagation failures** rather than dropping
the pass, leaving out its culmination, or filling a pass event with zeros,
each of which it did with a nil error (#454).
