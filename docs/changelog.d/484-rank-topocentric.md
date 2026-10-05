---
type: Fixed
pr: 484
---
**`Planner.RankObservable` scored the Moon and satellites as stars at
infinity**: its adapter dropped `GeocentricVec`, so the Moon ranked up to 0.78°
high and the ISS at 64° while below the horizon (#483).
