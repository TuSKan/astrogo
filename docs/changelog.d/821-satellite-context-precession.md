---
type: Added
pr: 821
---
**`satellite.Satellite.StateCtx` and `coord.Context.TEMEToGCRS`** place a
satellite with the precession-nutation a Context already holds. `plan.LookAngle`
uses them, so a six-hour pass search takes 2.6 ms where it took 78 ms, and a
look angle no longer depends on how far the Context has moved from its epoch.
