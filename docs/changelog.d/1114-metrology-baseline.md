---
type: Fixed
pr: 1114
---
**The validation suites are compared with a recorded baseline**: `internal/metrology`'s baseline check existed, but no
baseline was recorded and nothing called it, so a suite could get several times worse inside its contract unnoticed.
`docs/validation-baseline.json` now holds the last accepted run's 31 suites, and every scheduled Validation run fails
for a percentile that has more than doubled.
