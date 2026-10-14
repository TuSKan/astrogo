---
type: Fixed
pr: 759
---
**`coord.Offset` reports a NaN or infinite distance as `ErrNotFinite`.** It wrapped `ErrNilGeodetic`, the sentinel for a nil start, so a caller branching on that read a bad distance as a missing location.
