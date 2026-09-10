---
type: Added
pr: 271
---
**`fits.WCS.CUnit` exposes the axis units the header declares**, which astrogo
did not read at all. `PixelToWorld` returns CRVAL plus a linear offset for any
non-celestial axis, so its value was correct in a unit no part of the API could
name — metres or Angstrom for a spectral axis, seconds or days for a time one.
Both transforms now also document what they return: degrees for celestial axes,
each other axis in its own `CUNITi` (#178).
