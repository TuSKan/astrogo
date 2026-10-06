---
type: Changed
pr: 544
---
**The guard against exported package-level maps now covers slices and arrays too**, which any importer could also edit in place.
The two it found, in `internal/changelog` and `remote/file`, were read only inside their own packages and are unexported.
