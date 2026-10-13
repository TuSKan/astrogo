---
type: Changed — BREAKING
pr: 717
---
**`skybrightness` and its datasets carry their units.** `ZodiacalGeometry.SunDistanceAU` is now `SunDistance`, a `unit.Length`. The raster `SampleBilinear`, `Grid.LonLat` and `viirs.Raster.RadianceAt` take and return `angle.Angle` instead of float64 degrees, and the airglow wavelengths are `unit.WavelengthNM`. A number passed for a longitude or latitude still compiles, as radians: write `angle.Deg(-70.4)`.
