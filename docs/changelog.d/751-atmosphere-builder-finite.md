---
type: Fixed
pr: 751
---
**`atmosphere.Builder` refuses NaN and infinite inputs.** Only the ozone column was checked properly: a NaN AOD built an atmosphere whose extinction was NaN with no error, and NaN pressure, temperature, single-scattering albedo, asymmetry and cloud optical depth were all accepted. The aerosol reference wavelength and Ångström exponent are now checked too, and `Airmass` and `OzoneAirmass` return `ErrAltitude` for a non-finite elevation.
