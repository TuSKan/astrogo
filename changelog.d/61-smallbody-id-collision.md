---
type: Fixed
pr: 61
---
**Ceres, Pallas, Juno and Vesta could not be loaded at all.** A small body was identified by its bare number, so asteroid 4 Vesta and Mars were both `core.ID(4)` — as were Ceres and Mercury, Pallas and Venus, and every asteroid numbered up to 12. A provider holding a planetary kernel resolved the planet and dropped the asteroid as a duplicate, and `ErrNoSmallBodyKernel` then blamed the designation, which was never the cause. Small bodies now keep NAIF's own `20000000+` identifier via `core.SmallBodyID`.
