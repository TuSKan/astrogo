---
type: Fixed
pr: 1001
---
**`Model.Estimate` and `CloudySkyglow` refuse a NaN or infinite direction with the new `ErrDirection`.** `CloudySkyglow` returned NaN in every bin with no error for one. Through `Estimate` that surfaced as the component failing, and other components' errors for the same input named an airmass or an optical depth, never the direction. A model whose components ignore the direction answered it outright.
