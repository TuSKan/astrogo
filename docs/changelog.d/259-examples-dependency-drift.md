---
type: Added
pr: 259
---
`docsguard` now catches a dependency bumped in the root module but not mirrored
into `examples/`. The examples module carries the library's dependencies as
indirect requirements, and a stale copy makes `go build ./...` inside it refuse
outright — previously visible only after a push, from CI (#259).
