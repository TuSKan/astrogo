---
type: Changed
pr: 477
---
**`plan.SatellitePasses` does half the precession-nutation work**, with
identical results: each satellite state evaluates the series once instead of
twice, and culminations are sampled through the pass search's Context cache
rather than a full Context per sample (#476).
