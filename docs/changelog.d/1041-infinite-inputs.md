---
type: Fixed
pr: 1041
---
**Eight functions refuse an infinite input they used to answer with NaN, ±Inf or zero and no error:** `atmosphere`'s `CrossSection.OpticalDepth`, `MultipleScatteringFactor` and `ExtendedSourceOpticalDepth` (which also took a NaN airmass and `kappa`), `magnitude.ROLOIrradiance`, `skybrightness.ZodiacalRadiance` and `NewScatteredMoonlight`, the VIIRS source spectrum, and `solar.Parse`, which now also refuses wavelengths out of order rather than reading zero sunlight.
