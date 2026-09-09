---
type: Fixed
pr: 237
---
`ephemeris/jpl` no longer rejects every comet fetched by its Horizons SPK-ID as
a substituted body. Those IDs sit outside NAIF's numbered-asteroid block, where
`core.SmallBodyID` reports 0, and the substitution guard was comparing the
loaded bodies against that zero rather than against the comet (#237).
