---
type: Fixed
pr: 1221
---
**A nil site or target is an error in `plan`, not a panic**: `IsObservable`, `RankObservables`, every built-in constraint and the interval searches return the new `plan.ErrNilSite` or `plan.ErrNilTarget`, and `SatellitePasses` refuses a nil observer.
