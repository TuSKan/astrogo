---
type: Fixed
pr: 583
---
**`fits/plan` reads the FITS standard's keywords.** `SiteFromFITS` takes the standard's `OBSGEO-X/Y/Z` and `OBSGEO-B/L/H`, and sexagesimal `SITELAT`/`SITELONG`, all of which it reported as missing. `TargetFromFITS` evaluates the header's own WCS at the frame centre, honouring `CTYPE` and `RADESYS`. It had returned `CRVAL` as RA/Dec, 0.57° off for a corner reference pixel and the Galactic Centre as RA 0, Dec 0. A frame it does not convert is the new `ErrUnsupportedFrame`.
