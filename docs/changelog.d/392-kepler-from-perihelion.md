---
type: Added
pr: 392
---
**Parabolic and hyperbolic orbits in `ephemeris/kepler`.**
`kepler.FromPerihelion` (and `eph.ElementsFromPerihelion`) builds elements
from perihelion time, distance and eccentricity, the MPC's comet form, for
any e >= 0, and propagates them by universal variables. It agrees with
Horizons' own conversion of 1I, 2I, 2P and C/2023 A3 to 1e-11 AU.
`Elements.PerihelionDistance` reports q for either form (#374).
