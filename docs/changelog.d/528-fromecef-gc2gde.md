---
type: Fixed
pr: 528
---
**`coord.FromECEF` lost height accuracy with altitude** — 1.5 mm at the ISS, 31
cm at geostationary orbit — because Bowring's single step is exact only near
the ground. It now uses SOFA's `iauGc2gde`, which holds 2e-8 m at
geostationary orbit, and is twice as fast. `coord.ErrInvalidEllipsoid` reports an
ellipsoid SOFA refuses instead of returning NaNs.
