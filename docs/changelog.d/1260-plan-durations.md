---
type: Changed — BREAKING
pr: 1260
---
**`plan`'s durations are `unit.Duration`**, as `Time.Add` and `Time.Sub` already were: windows, blocks, cadences, setup and slew times, strategy steps and satellite passes. A NaN or infinite one is the new `plan.ErrNonFiniteDuration`, where a NaN step used to make `EventSolver.Find` panic, and a slow slew is a long time rather than `ErrSlewTime`.
