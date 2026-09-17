---
type: Changed — BREAKING
pr: 358
---
**`atmosphere`'s heights and scale heights are now `unit.Length`**, following
`coord` in #355 and `plan` in #357. Retyped: `AtAltitude`, `HorizonDip`,
`StandardDefault`, `Builder.SurfaceAtAltitude`, `Builder.AerosolScaleHeight`,
`VanRhijn`, `MolecularScaleHeight`'s return, `ExtendedSourceOpticalDepth`'s
observer height, `ExponentialExtinction`/`ExponentialDepth`, the eight OPAC
aerosol preset constructors, and `CloudLayer.BaseAlt`/`TopAlt` and
`Aerosol.ScaleHeight`. In `skybrightness`: `AirglowRadiance`, `NewAirglow`,
`OpticalParameterT` and `dataset.AerosolPreset`.

`ContinentalScaleHeightM`, `DesertScaleHeightM`, `MaritimeScaleHeightM` and
`AirglowLayerHeightM` lose the `M` and become typed `unit.Length` constants,
since the unit is no longer part of the name.

`skybrightness.OpticalParameterT` declared its two scale heights as
`unit.OpticalDepth`. They are lengths, as its own arithmetic shows: it divides
an optical depth by one to get an extinction per unit length. The numbers were
right and the type was a lie; both scale heights and the separation are now
`unit.Length`, and no computed value changes.

`MolecularScaleHeightM` and `AerosolScaleHeightM` in `transfer.go` keep their
names and stay `float64`: they appear only inside
`ExtendedSourceOpticalDepth`'s own arithmetic and are named after the symbols
in Masana et al. (2021), rather than crossing the API the way the preset scale
heights do.
