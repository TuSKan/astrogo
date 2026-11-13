---
type: Fixed
pr: 1122
---
**A slew time too long for a `time.Duration` is refused**: `plan.BasicTransitionModel` converted it unguarded, so a
slew rate below 2e-8 deg/s gave a setup time of -292 years on amd64, which scheduled the next block centuries early,
and +292 years on arm64. A NaN rate dropped the slew silently. `Overhead` now returns `plan.ErrSlewTime` for all three.
