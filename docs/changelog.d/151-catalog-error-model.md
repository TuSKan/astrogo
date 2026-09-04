---
type: Changed — BREAKING
pr: 151
---
**`resolve.Provider` returns errors instead of a bool.** `Resolve` is now
`(Target, error)` and `Search` is `([]Target, error)`, so a transport failure,
a cancelled context or an unreachable service is no longer reported as
"target not found". `errors.Is(err, context.Canceled)` works through the
catalog layer for the first time. Adds `resolve.ErrUnsupported` for
cone-search-only providers, and `catalog/fink` and `catalog/fits` now take a
`context.Context`.
