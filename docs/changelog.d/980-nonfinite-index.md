---
type: Fixed
pr: 980
---
**Three float-to-index conversions refuse a non-finite input instead of turning it into an answer.** `coord.CompassDirection` called a NaN or infinite azimuth "N" and now returns "". `coord.SmallCircle` returned NaN points for a non-finite radius and now refuses it with `ErrNotFinite`. The CAMS aerosol reader mapped a NaN site to a grid cell, silently on arm64, and now refuses it with `ErrAOD`.
