---
type: Changed — BREAKING
pr: 359
---
**`optics` and `magnitude` now carry lengths as `unit.Length`**, finishing the
mechanical half of #130 after `coord` (#355), `plan` (#357) and `atmosphere`
(#358).

`optics` loses its `MM` suffixes: `NewTelescope`, `NewEyepiece` and
`WithFieldStop` take `unit.Length`; `Telescope.ApertureMM`/`FocalLengthMM`
become `Aperture`/`FocalLength`, `Eyepiece.FocalLengthMM`/`FieldStopMM` become
`FocalLength`/`FieldStop`, `Telescope.ExitPupil` returns a `unit.Length`, and
`Sensor`'s `WidthMM`/`HeightMM`/`PixelMicrons` become `Width`/`Height`/
`PixelPitch`.

`magnitude.SatelliteApparent` takes an `observerRange unit.Length` in place of
`rangeKm`, and `ROLOIrradiance` takes typed sun and moon distances.
`ROLOStandardDistanceKM` becomes `ROLOStandardDistance`, a typed constant.

**`Telescope.PixelScale` moves by 9.4e-7 of itself.** It computed
`206265·pixelPitch(µm)/focalLength(mm)`, where the constant is a rounded
radian-to-arcsecond conversion (206264.806…) and the 1000 reconciled microns
against millimeters. Between two `unit.Length` values the small-angle relation
is the ratio itself and `angle.Angle` converts exactly, so both factors are
gone. For a 3.76 µm pixel at 1000 mm that is 0.7755557″ against the previous
0.7755564″ — below any plate solve's precision, and the only computed value in
this change that moves at all.
