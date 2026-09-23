---
type: Fixed
pr: 411
---
**`fits` reads the D exponent the FITS standard allows** (`1.5D-05`), and a
keyword that is present but does not parse is an error, never its default.
Before, `ExtractWCS` silently read such a header as reference point (0, 0),
unit scale and no distortion, and `ReadImage` dropped a `BZERO = 3.2768D+04`,
leaving 16-bit unsigned pixels 32768 low. Adds `fits.ErrWCSMalformedKeyword`
(#409).
