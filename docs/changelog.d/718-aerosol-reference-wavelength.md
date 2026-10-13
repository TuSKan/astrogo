---
type: Changed — BREAKING
pr: 718
---
**`atmosphere.Builder.Aerosol` takes its reference wavelength as a `unit.WavelengthNM`**, the type the aerosol state already stores it as. It counts nanometers, so a caller's `550` still means 550 nm and only a `float64` variable needs converting.
