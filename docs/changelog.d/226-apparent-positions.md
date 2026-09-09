---
type: Fixed
pr: 226
---
**The scheduler placed planets, asteroids and comets geometrically, not
apparently.** Neither light time nor annual aberration reached the alt/az it
computes: measured at Paranal across 2026, Mars was out by up to 38.7″, Venus
44.8″ and the Sun 20.5″. `MovingBody.GeocentricVec` now returns the apparent
place, which is what its consumer expects; `Position` stays geometric because
its consumer applies aberration itself (#117).
