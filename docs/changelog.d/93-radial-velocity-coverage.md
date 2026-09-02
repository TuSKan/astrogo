---
type: Fixed
pr: 93
---
**`coord.Context.TopocentricRadialVelocity` shipped with no offline test.** Its
only cover was a `network`-tagged comparison against JPL Horizons, invisible to
an ordinary coverage run — it was at 0%, and it is at 100% now, checked against
the geometry rather than a service.
