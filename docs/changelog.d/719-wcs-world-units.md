---
type: Changed
pr: 719
---
`fits.WCS.PixelToWorld` and `WorldToPixel` keep a bare float64 per axis, now documented as decided rather than pending: celestial axes in degrees, every other axis in the unit its `CUNITi` declares, which `CUnit` reports.
