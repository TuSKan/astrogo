---
type: Fixed
pr: 1056
---
**`constellation.Lookup` answered Ursa Minor, with no error, for a NaN or infinite position.** It now refuses one
with `coord.ErrNotFinite`.
