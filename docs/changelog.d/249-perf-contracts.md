---
type: Added
pr: 249
---
Allocation contracts for `coord`, `time` and `atmosphere`'s hot paths, as
ordinary tests that fail a build — `allocs/op` is deterministic where `ns/op`
on a shared runner is not. The Benchmarks CI job now reports a `benchstat`
delta in its job summary instead of uploading numbers nothing reads (#249).
