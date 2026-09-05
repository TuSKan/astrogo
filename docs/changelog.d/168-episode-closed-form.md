---
type: Fixed
pr: 168
---
**`plan.Episode` searched up to 366 days to discover a target never rises.**
For a fixed target that answer is two arcsines — declination does not change,
so upper and lower culmination bound the whole window — and `IsNeverUp` /
`IsCircumpolar` already computed it while having no caller in the library.
Measured: 2.84 s → 67 ns for the never-rises case, and the `plan` package
18.6 s → 12.7 s. A one-degree margin keeps anything refraction or parallax
could decide on the search path, and moving bodies always search (#110).
