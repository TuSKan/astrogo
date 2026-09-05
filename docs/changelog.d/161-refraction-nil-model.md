---
type: Fixed
pr: 161
---
**Following the documented `RefractionModel` API panicked.** Every constructor
leaves `Refraction.Model` nil, so `env.Model.RefractFromTrue(...)` was a nil
dereference. `Refraction` now answers for itself via `RefractFromTrue`,
`RefractFromApparent` and `EffectiveModel`, which resolve nil to the new
`RefractionSOFA` when a pressure is set and to `RefractionNone` otherwise —
moving the "nil means SOFA" convention out of `coord` and into the package that
owns the type. `coord.Reducer.Disperse` consequently stops reporting zero
dispersion for an environment `Reduce` had just refracted through (#118).
