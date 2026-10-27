---
type: Added
pr: 871
---
**`atmosphere/dataset/cams` is fuzzed**, now that scigolib/hdf5 v0.14.2 bounds
the allocations a malformed file used to end the process with, and a sweep of
corrupted files guards that no variant does.
