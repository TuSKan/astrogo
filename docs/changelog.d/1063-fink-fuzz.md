---
type: Fixed
pr: 1063
---
**`fink.Provider.Resolve` returned a target with no ID for an answer that names no body, and a malformed SSOFT table
panicked it inside Arrow's parquet decoder.** The first is now refused and the second returned as an error wrapping
`fink.ErrInvalidParquet`, and both answers have a fuzz target.
