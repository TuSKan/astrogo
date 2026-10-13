---
type: Changed — BREAKING
pr: 715
---
**`plan.CrescentParams` and `ScoreConfig` carry their units.** `ArcV`, `ArcL`, `DAZ`, `MAlt` and `W` are `angle.Angle` (they were degrees, and arcminutes for `W`), and `LT` and `Age` are `unit.Duration` (minutes and hours). `ScoreConfig.MoonFullPenaltyDeg` is now `MoonFullPenalty`, an `angle.Angle`. A number in a `CrescentParams` literal still compiles, as radians or seconds: write `angle.Deg(3)`.
