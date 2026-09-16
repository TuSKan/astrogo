---
type: Deprecated
pr: 320
---
`satellite.Satellite.Verified` — it now returns true for every element set,
because the regime it warned about no longer exists: the low-perigee divergences
it flagged were a defect in the old dependency, and those cases agree to
nanometres now. Callers wanting the model's own branch predicates should use
`Satellite.Propagator` and `sgp4.Propagator`'s `SimplifiedDrag`, `DeepSpace` and
`PerigeeAltitude`, which describe the orbit rather than astrogo's coverage. [#310]
