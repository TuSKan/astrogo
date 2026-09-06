---
type: Fixed
pr: 181
---
**Satellite positions carried up to a second of orbital motion of error — 5.94 km
for Vallado's reference case, worst at the element epoch where it should be exact.**
The SGP4 backend truncates the element epoch to a whole second as well as the query,
so the sub-second correction must be `frac(t) - frac(epoch)`, not `frac(t)`.
