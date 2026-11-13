---
type: Fixed
pr: 1144
---
A satellite resolved through `catalog.Resolver` keeps its TLE epoch. The merge dropped it, so SGP4 propagated the element set from JD 0 without an error, and its two lines could come from different candidates.
