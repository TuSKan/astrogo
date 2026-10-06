---
type: Fixed
pr: 543
---
**`plan.TransitEstimate` and `MaxAltitudeInWindow` missed a peak on the window's edge.**
The window's end was never sampled unless it was a whole number of 10-minute steps, so a target still rising at the end was reported up to 10 min early and 1.45° low.
`RankObservable` and `VisibleTonight` inherit the fix.
