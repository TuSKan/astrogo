---
type: Changed — BREAKING
pr: 320
---
Two exported symbols no longer exist, and neither appeared in a tagged release,
so **no released version is affected**. `Satellite.Verified` is gone because
there is no longer a regime for it to flag — use `Satellite.Propagator` with
`sgp4.Propagator`'s `SimplifiedDrag`, `DeepSpace` and `PerigeeAltitude`, which
describe the orbit rather than astrogo's coverage. `sgp4.ErrDeepSpace` was
scaffolding while SDP4 was being written; deep space propagates now, so nothing
raises it and the branch handling it can go. Recorded because a caller pinned to
a `main` commit between those changes gets a compile error, which is the one
audience a release-to-release note would miss. [#310]
