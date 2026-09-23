---
type: Fixed
pr: 408
---
**`magnitude.PlanetApparent` computes Pluto**, which it listed as supported
and refused: the Explanatory Supplement's historical law for Pluto and Charon
together, V(1,0) = −1.01 and 0.041 mag/°, documented as an approximation that
can be tenths of a magnitude off, not a prediction. `plan.VisibleTonight`,
which dropped that error and so never listed Pluto at any limit, now reports a
body or planetary moon it cannot evaluate through `ErrIncomplete` (#407).
