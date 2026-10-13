---
type: Fixed
pr: 736
---
**`fits/plan`'s `TargetFromFITS` reads the deprecated `EPOCH` keyword as the equinox when `EQUINOX` is absent.** A header giving `EPOCH = 1950.0` was read as ICRS, 0.6° from where it points, while the same header with `EQUINOX = 1950.0` was refused as FK4. Both are now refused alike, and `EQUINOX` wins when both are present.
