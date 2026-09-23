---
type: Fixed
pr: 388
---
**Saturn is no longer too faint when the south face of its rings is lit.**
`magnitude.PlanetApparent` gave the ring inclination a sign, so the ring terms
dimmed Saturn instead of brightening it: 1.9 mag too faint at the 2002
opposition, 0.49 mag at 2026's. It now agrees with JPL Horizons to 0.001 mag
on both faces (#375).
