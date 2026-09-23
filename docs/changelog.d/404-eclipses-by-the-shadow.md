---
type: Fixed
pr: 404
---
**`plan.LunarEclipses` and `plan.SolarEclipses` decide an eclipse by the
shadow, not a fixed 1.58° latitude.** Over six centuries of NASA's Five
Millennium Canons they reported 173 eclipses that do not happen and missed one
that does; they now agree on every eclipse, with greatest eclipse 0.16 minutes
from the canon's on average instead of 0.8. `Gamma` is measured against the
real limit, and a provider error mid-search is returned (#401).
