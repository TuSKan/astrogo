---
type: Changed — BREAKING
pr: 1246
---
**An ICRS star carries the epoch it is given at.** `coord.NewICRSWithKinematics` takes that epoch as its last argument, and `coord.ICRS.Epoch` reports it. Every transform moves the star from there, including `coord.ICRSToFK4` and `coord.ICRSToFK5`. A Gaia DR3 row passed at J2016.0 now lands where the star is, not 16 years of proper motion off. `coord.PropagateEpoch` takes the target epoch alone and starts from the star's own. `coord.ICRS` is a pure value. Each of `coord.ICRS.SetRA`, `coord.ICRS.SetDec`, `coord.ICRS.SetDist`, `coord.ICRS.SetProperMotion`, `coord.ICRS.SetParallax`, `coord.ICRS.SetRV`, `coord.ICRS.FromUnitVector` and `resolve.ConeRequest.Epoch` was removed. `coord.ICRSFromUnitVector` builds a direction from a vector, and a cone center carries its own epoch.
