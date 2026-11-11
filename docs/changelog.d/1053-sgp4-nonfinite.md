---
type: Fixed
pr: 1053
---
**`sgp4.Propagator.At` never returned for a NaN or infinite time on a resonant deep-space orbit, and non-finite
inputs came back as a NaN state labeled `ErrKeplerNotConverged`.** Such times, states, elements and TLE text are
now refused, the first two with the new `sgp4.ErrNotFinite`.
