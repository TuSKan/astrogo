---
type: Fixed
pr: 447
---
**`EventSolver` refines a crossing from the two samples that found it**, rather
than evaluating the ends again: a sample within 1e-7° of the threshold could
read differently the second time, and the whole search failed with
`ErrBracketingViolated` instead of returning its events (#425).
