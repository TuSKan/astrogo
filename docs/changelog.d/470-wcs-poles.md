---
type: Fixed
pr: 470
---
**`fits.WCS` works at and near the celestial poles.** `WorldToPixel` failed
for most pixels in frames centered above 89°, a pixel at the pole deprojected
to NaN, and the pole itself could not be located (#469).
