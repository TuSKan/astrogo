---
type: Added
pr: 1231
---
**`plan.UnscheduledBlock` says why a block was not scheduled:** `Cause` is `plan.CauseNoTime`, `plan.CauseConstraint` or `plan.CauseCadence`, `Constraint` names the constraint that rejected the most placements, `Observed` counts the observations a cadence got before it ran out, and `Reason` says it in words with the constraint's own reason. Every unscheduled block used to carry one catch-all reason.
