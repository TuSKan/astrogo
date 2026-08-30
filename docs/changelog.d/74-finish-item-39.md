---
type: Added
pr: 74
---
**Planetary-satellite propagation is complete.** `kepler.PlutoElements` lets the default base answer Pluto, so a Charon orbit can be placed at all — before, it failed at the parent rather than the satellite. `Elements.WithPeriod` and `kepler.SecularPrecession` apply the published anomalistic period and apsidal precession as a pair, recovering the sidereal period (Io: 1.769137 d, from the table's own columns) and cutting the six-month error against Horizons from 125,000 km to 74,000. Applied singly, either is an order of magnitude worse than neither; the tabulated *node* rate is measured and deliberately not applied.
