---
type: Changed — BREAKING
pr: 721
---
**The last unit-named float64 fields carry their units.** `optics.Instrument`'s `CollectingArea`, `PixelSolidAngle` and `DarkCurrent` (were `…M2`, `…SR`, `…EPerSec`), `skybrightness.SkyPoint.SolidAngle`, `atmosphere.CrossSection.Sigma` (was `SigmaCM2`), `plan.BasicTransitionModel.SlewRate` and `satellite.Satellite.MeanMotion` are typed, and `Satellite.OrbitalPeriod` returns a `unit.Duration`. A bare number for `SlewRate` or `MeanMotion` still compiles, as radians per second: write `unit.DegreesPerSec(2)`.
