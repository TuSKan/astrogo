---
type: Fixed
pr: 186
---
**A TLE with a day-of-year past the end of the year panicked inside the SGP4 backend**,
reachable from `satellite.NewFromTLE` with an element set that is 69 columns,
checksum-valid and numeric in every field. `days2mdhms` guards a twelve-element month
array with `i < 22`. `ValidateTLE` now range-checks the epoch day (#139).
