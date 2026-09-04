---
type: Added
pr: 151
---
**Tests for the catalog error contract.** `resolve.Drain` is covered, and the
Resolver's semantics are pinned directly: a provider failure is never reported
as `ErrNotFound`, a cancelled context is caught before any provider is
consulted, `ErrUnsupported` is an answer rather than an incident, and one
broken provider neither denies an answer another gave nor suppresses partial
`Search` results.
