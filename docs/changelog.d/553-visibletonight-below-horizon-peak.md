---
type: Fixed
pr: 553
---
**`plan.VisibleTonight` no longer fails the whole night over a target that peaks just below 0°.**
From an elevated site its windows reach the dipped horizon, below the astronomical one, where airmass is undefined; such a peak now takes the horizon's airmass, a lower bound on its extinction.
