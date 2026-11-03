---
type: Changed — BREAKING
pr: 947
---
**`GroundEmitter` has a new method, `SourceUncertainty`, and `ArtificialSkyglow` now reports its model's own error.** That error is the 25% by which Kocifaj, Barà & Falchi (2022) find the model departs from an exact multiple-scattering solution. The component combines it with each emitter's stated uncertainty, weighted by that emitter's share of the radiance. `UniformEmitter.RelativeUncertainty` and `viirs.Region.RelativeUncertainty` supply the emitter's figure. A source outside the paper's validated 1.3–81.3 km, or aerosol optical depth above 0.57, is flagged `ExtrapolatedModel`.
