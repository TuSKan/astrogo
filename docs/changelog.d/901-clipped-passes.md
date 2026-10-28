---
type: Fixed
pr: 901
---
**`plan.SatellitePasses` reports a pass the search window cuts off**, at the window's edge and marked by the new `SatellitePass`
fields `RiseCutOff` and `SetCutOff`. Before, a pass in progress when the window opened or closed was dropped, so asking during an
overhead pass returned none.
