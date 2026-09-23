---
type: Fixed
pr: 406
---
**`constants.IAU2015.SunEquatorialRadius` is IAU 2015 B3's 695,700 km, not
696,000 km**, so `plan.AngularDiameter` agrees with JPL Horizons for the Sun
instead of running 0.85″ large. `MeanEarthRadius` keeps its 6,371 km but no
longer cites B3, which defines no mean radius, or claims to be exact (#403).
