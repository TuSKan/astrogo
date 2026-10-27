---
type: Changed
pr: 872
---
**`kepler.Provider.Register` fits a far orbit about the barycenter** when that leaves less perturbation
at its epoch than the Sun, and `Locate` reports which: against Horizons, a Centaur or a trans-Neptunian
object is now 0.03 to 30 km off at 30 days, not 815 to 950.
