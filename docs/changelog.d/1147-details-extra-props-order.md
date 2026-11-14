---
type: Fixed
pr: 1147
---
`plan.TargetDetails.String` prints a target's extra properties in key order, rather than in Go's map order, which changed from run to run.
