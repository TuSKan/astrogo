---
type: Fixed
pr: 1126
---
**A celestial WCS projection astrogo does not implement is refused**: `fits.ExtractWCS` evaluated ZEA, CAR, MOL and
every code but TAN, SIN, ARC, STG and AIT as a linear plane, so `fitsplan.TargetFromFITS` returned a position 7″ off
on a 1″-per-pixel frame; it now returns `fits.ErrWCSUnsupported`. A malformed `NAXISn` is refused too, and `NAXIS = 0`
gives CRVAL instead of an error.
