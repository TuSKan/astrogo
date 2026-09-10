---
type: Fixed
pr: 273
---
**A body id above `MaxInt32` was converted rather than refused.** `core.ID` is
unsigned and a NAIF id is signed 32-bit, so the top half of the range wrapped to
a *negative* id — which is meaningful, since that is how NAIF numbers
spacecraft. `core.ID(0xFFFFFFFF)` was looked up as −1 and answered for a body
the caller never named. `jpl.Provider.State` now returns `ErrBodyIDOutOfRange`,
and `plan`'s designation parser bounds to 31 bits (#273).
