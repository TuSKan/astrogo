---
type: Added
pr: 183
---
A guard keeps the six hand-written `go-version` pins in the CI workflows on the same
minor version as `go.mod`. They are deliberately decoupled from `go-version-file`
(#109), which is safe but invisible: the day `go.mod` moves past 1.25, every job would
silently start downloading a toolchain again while CI stayed green.
