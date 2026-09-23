---
type: Fixed
pr: 392
---
**`kepler.NewElements` reports a comet's open orbit as `ErrUnsupportedOrbit`.**
Given a = q/(1−e), which is negative or infinite for e >= 1, it used to return
`ErrInvalidElements`, indistinguishable from bad data (#374).
