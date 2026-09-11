---
type: Added
pr: 274
---
**`remote.HTTPError` is reachable again**, along with `RetryPolicy`, `Attempt`,
`DefaultRetryPolicy` and `ErrRetriable`. #195 removed the old `remote.HTTPError` because
it was a second, identically shaped type and `errors.As` against the wrong one failed
silently; these are aliases for the subpackage's own types, so there is one type with two
names and nothing to pick wrongly.
