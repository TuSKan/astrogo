---
type: Fixed
pr: 81
---
**Radial velocities composed by adding a correction instead of multiplying
redshifts.** `coord.Context.BarycentricRadialVelocity` is the correct
conversion and `ObservedRadialVelocity` is now its exact inverse; the dropped
`rv*corr/c` term reached 4.66 m/s at a target velocity of 46.6 km/s and 30 m/s
for a halo star.
