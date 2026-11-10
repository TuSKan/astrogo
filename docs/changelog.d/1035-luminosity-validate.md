---
type: Fixed
pr: 1035
---
**`luminosity.Parse` refuses a curve holding a NaN, an infinity or a negative wavelength**, so `luminosity.Open` fetches a cached copy holding one again. It used to return that curve, keep the copy, and leave every later use failing until the cache was deleted by hand.
