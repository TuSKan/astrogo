---
type: Fixed
pr: 262
---
**`Context.GeocentricToObserved` omitted diurnal aberration**, so the vector
reduction route and the stellar route disagreed by up to 0.32″ for the same
target — 0.3150″ at the equator, 0.1966″ at Greenwich — while applying
refraction, the other half of what "observed" means. `Reducer.Reduce`,
`ReduceBatch` and every `plan` caller shared the defect (#261).
