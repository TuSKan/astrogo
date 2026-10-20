---
type: Added
pr: 814
---
**`docs/ephemeris.md` and `docs/kepler.md`: the ephemeris and two-body designs,
with every claim measured.** `internal/workload` benchmarks nine ephemeris
workloads and prototypes the record cache and per-epoch instant bit for bit;
`internal/keplerlab` holds Kepler solvers to a 256-bit oracle. No production
code changes.
