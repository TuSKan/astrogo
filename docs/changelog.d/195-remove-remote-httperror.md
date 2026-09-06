---
type: Removed
pr: 195
---
**`remote.HTTPError` was removed** — use `remote/api.HTTPError`, which is what every HTTP
path in the module returns. The `remote` one was a leftover from before the transport
split, referenced by nothing, and two identically named, identically shaped errors is a
trap rather than dead weight: `errors.As` against the wrong one fails silently on a
message that reads the same.
