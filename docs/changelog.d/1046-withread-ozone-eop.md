---
type: Fixed
pr: 1046
---
**`crosssection.Ozone` and the EOP loader replace a cached copy their parser refuses**, as the other dataset providers have since #1016. A cached ozone file that went bad used to fail every later call for good, and a damaged cached IERS bulletin stayed in use until IERS published a new one.
