---
type: Fixed
pr: 905
---
**A refraction with wavelength 0 refracts at the visual default, 0.55 µm, under SOFA's model**, through the new
`atmosphere.Refraction.EffectiveWavelength`. SOFA used to clamp a zero to 0.1 µm, twice the refraction from 10° to the zenith. Every
`Builder`-made atmosphere carried that zero, and with it the Sun and Moon that `skybrightness` places.
