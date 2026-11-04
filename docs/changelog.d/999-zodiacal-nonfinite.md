---
type: Fixed
pr: 999
---
**`ZodiacalLight` refuses a NaN or infinite direction instead of answering it as one beside the Sun.** Such a direction gave zero radiance, `ExtrapolatedModel` and no error, the answer reserved for the solar vicinity the table leaves blank, so an estimate quietly lost its zodiacal term. `ZodiacalBrightnessAt` also returned a NaN brightness with no error for some non-finite geometries. Both now give `ErrZodiacalGeometry`, and a direction that really is beside the Sun is answered as before.
