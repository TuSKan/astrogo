---
type: Changed — BREAKING
pr: 928
---
**`skybrightness.Component.AddRadiance` returns `(Contribution, error)`**, carrying the flags and, where the component's sources give one, the relative uncertainty of the radiance it added; `Model.Estimate` fills the uncertainty budget from it. Extragalactic light, zodiacal light, diffuse galactic light and moonlight now report sourced figures; airglow, starlight and artificial light take the caller's through the new `WithUncertainty`, and until then `TotalUncertainty` names them as unknown.
