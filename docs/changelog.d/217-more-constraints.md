---
type: Added
pr: 217
---
Four `plan` constraints: `SunSep` (the companion to `MoonSep` for the other
bright source), `GalacticLatitude` (distance from the plane, unsigned so it
serves both the surveys that avoid it and those that want it), `TimeWindow` (a
coordination window or a deadline, the constraint with nothing to do with the
sky), and `Horizon`, which is the first consumer of `WithHorizonProfile` — the
per-azimuth terrain limit `Altitude` cannot express (#129).
