---
type: Added
pr: 673
---
**Sidereal time is held to USNO's.** `TestUSNO_SiderealTime` compares `Time.GAST` and `Site.LocalSiderealTime` with USNO's sidereal-time service (NOVAS) at six epochs from 1990 to 2049, east and west of Greenwich, to 0.2 ms; every residual is within USNO's own 0.05 ms rounding. Before, nothing held sidereal time tighter than 0.5°, and the test of that name checked only that the result lay in [0°, 360°).
