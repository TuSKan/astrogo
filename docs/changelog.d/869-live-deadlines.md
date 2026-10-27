---
type: Fixed
pr: 869
---
**Ten live-service tests sent requests with no deadline**, so a hung service
killed the whole package at `go test`'s timeout. Each now has its endpoint's
own, and `internal/docsguard` refuses an unbounded live request.
