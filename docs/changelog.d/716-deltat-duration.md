---
type: Changed — BREAKING
pr: 716
---
**`time.DeltaT` and `time.DeltaTUncertainty` return a `unit.Duration`, and `time.LeapSecond.DeltaAT` is one**, where all three were float64 seconds. Read a number with `.Seconds()`. A literal such as `DeltaAT: 37` keeps its meaning, since the second is `Duration`'s base unit.
