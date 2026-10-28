---
type: Fixed
pr: 899
---
**`plan.SatellitePasses` finds a pass that clears `minElevation` for less than its 30 s step**. Every sampled maximum below the
threshold is refined to its true peak. Before, such a pass was found or missed depending on when the window started: 19 of 30 starts
missed one on the ISS test fixture.
