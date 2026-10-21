---
type: Changed
pr: 832
---
**`kepler.Elements.StateAt` takes 45% less time.** The constructors and the `With`
methods compute an orbit's epoch-independent quantities once, its orientation
as the perifocal axes P and Q, instead of on every call.
