---
type: Fixed
pr: 78
---
**`catalog/sbdb` returned orbital elements rounded to three significant
figures.** SBDB rounds unless asked not to, so Eros resolved with a = 1.46
rather than 1.458243716; two-body propagation from those elements was 690,000
km out at their own epoch of osculation, where they are exact by construction.
