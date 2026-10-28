---
type: Fixed
pr: 902
---
**A tabulated wavelength must be positive and finite, not only increasing.** The new `unit.ValidateWavelengths` is the one check
behind `Resample`, `magnitude.Passband`, `optics.Throughput`, `atmosphere.CrossSection` and VIIRS source spectra. Before, a NaN,
infinite or negative wavelength validated: a passband at −600 to −500 nm had a 546.6 nm pivot.
