---
type: Fixed
pr: 564
---
**A satellite in Earth's shadow no longer gets a magnitude.** `Satellite.ApparentMagnitudeCtx` returns the new `plan.ErrSatelliteEclipsed` instead; it gave the ISS −3.9 on a pass spent entirely in the shadow. `LimitingMagnitudeConstraint` rejects an eclipsed satellite rather than falling back to its standard magnitude. Shadow entries and exits agree with Skyfield within 0.35 ms.
