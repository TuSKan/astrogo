---
type: Fixed
pr: 676
---
**`coord.ICRSToEcliptic` and `EclipticToICRS` read the caller's time scale as TT.** A UTC instant was taken 69 seconds early, about 3e-8 degrees; both now convert to TT. The Galactic and ecliptic transforms are also anchored to their defining constants, the Hipparcos frame angles to 1e-9 degrees and the IAU 2006 obliquity to 0.05″, where the anchors had been held to 0.01 degrees.
