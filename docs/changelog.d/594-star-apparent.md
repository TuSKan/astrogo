---
type: Changed — BREAKING
pr: 594
---
**`magnitude.StarApparent` takes the extinction coefficient as a required argument**: `StarApparent(catMag, airmass, k)`. Its hidden default of 0.20 mag/airmass is gone; pass `air.Extinction(λ)` for the band the magnitude is in.
