---
type: Removed
pr: 544
---
**`remote/file.StagingSuffixes` (since removed) is unexported**; `IsStagingName` is how a caller recognizes a staging object.
The guard against exported package-level maps now covers slices and arrays too, which any importer could also edit in place, and this was one it found.
