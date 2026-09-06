---
type: Fixed
pr: 190
---
**`catalog.ErrNotFound` was a different error value from `resolve.ErrNotFound`, with
identical text.** `catalog.Provider` is an alias for `resolve.Provider`, whose contract is
written in terms of the latter — so a caller following it and testing
`errors.Is(err, resolve.ErrNotFound)` got false for an object that simply does not exist,
and fell into their "the service is down" branch. Both are now the same value (#141).
