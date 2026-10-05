---
type: Fixed
pr: 465
---
**CLAUDE.md's table of fuzz targets left out `ephemeris/satellite/sgp4`**, so
its two targets were outside the extended-fuzzing step. `internal/docsguard`
now holds the table to the code in both directions, and the package count
above it too.
