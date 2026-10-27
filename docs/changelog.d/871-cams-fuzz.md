---
type: Added
pr: 871
---
**`atmosphere/dataset/cams` is fuzzed**, now that scigolib/hdf5 v0.14.2 bounds
the allocations a malformed file used to end the process with, and `ReadPlane`
bounds a plane by its declared bytes, not only its values. A sweep of corrupted
files guards that no variant ends the process or allocates more than its file
could hold.
