---
type: Fixed
pr: 764
---
**The Gaia and VizieR cone searches send their center and radius exactly, and refuse an invalid radius.** They were printed to six decimals of a degree, so a radius under 1.8 mas went out as 0 and cones within 3.6 mas of one another shared a cache entry; a NaN, infinite or negative radius is now `resolve.ErrInvalidRadius` before anything is sent.
