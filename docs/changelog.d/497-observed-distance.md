---
type: Fixed
pr: 497
---
**`Satellite.GetDetails` reported every satellite at 0 km**: it read the
distance from `coord.Context.GeocentricToObserved`, which set none. The
transform now carries the topocentric distance, and the details keep the range
they computed (#495).
