---
type: Fixed
pr: 1081
---
**A NaN or infinite number in a catalog answer is read as an absent one.** The SIMBAD, VizieR, Gaia, MAST, FINK,
OpenNGC and MPCORB parsers kept them, and a target with a NaN position or proper motion was silently "not
observable" and never rose. `plan.FromCatalog` now refuses a non-finite coordinate with `plan.ErrNoCoordinates`.
