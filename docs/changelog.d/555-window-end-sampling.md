---
type: Fixed
pr: 555
---
**`plan.VisibleIntervals`, `Find` and `ObservableWindows` now sample the end of their range.**
They stopped at the last whole step before it, so a target setting after that step was reported up until the end, and one rising after it had no final window at all.
