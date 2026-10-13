---
type: Changed — BREAKING
pr: 713
---
**The asteroid spin-axis pole is an `angle.Angle`.** `resolve.Target.SpinRA` and `SpinDec` were bare float64 degrees, and `plan.WithSpin` now takes `(ra, dec angle.Angle, oblateness float64)`. `plan.SpinAxis`, which no exported API returned, no longer exists. A literal passed to these still compiles, as radians: write `angle.Deg(45)`.
