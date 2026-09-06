---
type: Added
pr: 187
---
Tests for four exported symbols that had no reference anywhere in the module — not a
caller, not a test, not an example: `plan.EventAnyPhase` (a documented four-phase
wildcard nothing had ever passed), `plan.NewEarth`, `plan.WithStep` and
`time.FileEOPLoader`, the recommended no-dependencies EOP path (#106).
