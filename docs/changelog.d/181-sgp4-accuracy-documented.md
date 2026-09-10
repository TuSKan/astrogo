---
type: Changed
pr: 181
---
**`docs/VALIDATION.md` and the `satellite` package doc now state that SGP4 is wrong
for low-perigee, deep-space and decaying orbits**, measured for the first time by the
suite above. Ordinary orbits are unaffected; `Satellite.Verified` says which side a
given element set falls on.
