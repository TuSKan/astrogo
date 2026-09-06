---
type: Fixed
pr: 199
---
**`fits.ReadHeader` allocated about five times the file it was reading.** Its
failsafe bounded blocks read, not cards retained, so 10,000 blocks of distinct
keywords was 360,000 retained cards — measured at 144.1 MB from a 28.8 MB input.
A second bound on retained cards holds it at 8.5 MB and, more to the point, flat
as the input grows.
