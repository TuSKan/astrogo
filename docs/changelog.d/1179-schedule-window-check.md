---
type: Fixed
pr: 1179
---
`plan.Scheduler.BuildSchedule` and the strategies' `Schedule` methods refuse a window that ends before it starts, or at a time that is not a number, with `plan.ErrReversedInterval` and `plan.ErrNonFiniteInterval`; they returned an empty schedule with every block unscheduled and a nil error.
