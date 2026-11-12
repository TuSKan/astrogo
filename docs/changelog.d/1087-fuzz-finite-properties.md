---
type: Changed
pr: 1087
---
**The `mast` and `fink` fuzz targets hold resolved values to being finite**: a MAST target claiming a position must
have a finite one with its declination in range, and every value a FINK target claims must be finite, as every catalog
parser has read a non-finite number since #1081.
