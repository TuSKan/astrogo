---
type: Fixed
pr: 429
---
**A search interval that ends before it starts is `plan.ErrReversedInterval`**
from every interval search — `EventSolver.Find` and the helpers on it,
`MoonPhases`, the eclipse searches, `SatellitePasses`, `TransitEstimate`,
`Episode` and `Find` — where it used to panic inside a sampler (#418).
