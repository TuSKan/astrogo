---
type: Changed
pr: 201
---
**`plan.SatellitePasses` is about twice as fast** — 201 ms to 96 ms over a
six-hour window at 30 s sampling. It rebuilt a full `coord.Context` per sample,
which was 61% of each one; it now derives them with `Context.AtTime` from a base
rebuilt hourly, costing ≲0.1″ against SGP4's own kilometre-scale error.
