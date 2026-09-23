---
type: Fixed
pr: 408
---
**`magnitude.PlanetApparent` computes Pluto**, which it listed as supported
and refused, from the Explanatory Supplement's V(1,0) = −1.01 and 0.041
mag/°; `plan.VisibleTonight`, which dropped that error and so never listed
Pluto at any limit, now reports a body it cannot evaluate through
`ErrIncomplete` (#407).
