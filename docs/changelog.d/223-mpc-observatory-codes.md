---
type: Added
pr: 223
---
`plan.NewMPCSite(ctx, code)` and `plan.MPCObservatories(ctx)` resolve the IAU
Minor Planet Center's ~2,700 observatory codes to a `*Site`, recovering each
position from the published parallax constants. `MPCObservatory.ResolutionM`
reports how finely a row was published — from ±3 m to ±3.2 km — because the
register mixes both and a recovered height does not say which it is. New
`remote.MPCObsCodes` endpoint, download-gated like every other bulk fetch
(#125).
