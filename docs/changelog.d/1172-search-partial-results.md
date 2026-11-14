---
type: Changed
pr: 1172
---
`catalog.Resolver.Search` returns the matches it found together with an error wrapping the new `catalog.ErrIncomplete` when some catalogs could not answer, naming each and its reason; it used to return them with a nil error. `plan.ErrIncomplete` is the same value, defined once as `resolve.ErrIncomplete`.
