---
type: Fixed
pr: 997
---
**`plan`'s interval searches refuse a NaN or infinite window with the new `ErrNonFiniteInterval`.** Such a time compares false both ways, so it passed the reversed-interval check. Eighteen searches then panicked sizing their sample buffer, `RankObservable` crashed the process from a goroutine, and `Oppositions` never returned. `VisibleIntervals` and `ObservableWindows`, which checked no window at all, now return `ErrReversedInterval` for a reversed one instead of an empty answer.
