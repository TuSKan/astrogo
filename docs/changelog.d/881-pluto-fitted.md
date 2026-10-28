---
type: Fixed
pr: 881
---
**`ephemeris.Default()`'s Pluto is back to 0.14 AU from DE440 at worst over 1800-2050**, from the 1.96 AU
#872 made it by refitting Standish's fitted elements about the barycenter; `kepler.Provider.Register` now
keeps them about the Sun, and the Pluto validation suite measures `Default()` itself.
