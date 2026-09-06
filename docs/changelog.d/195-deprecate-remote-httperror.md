---
type: Deprecated
pr: 195
---
`remote.HTTPError` — use `remote/api.HTTPError`, which is what every HTTP path in the
module actually returns. The `remote` one is a leftover from before the transport split
and is referenced by nothing; two identically named, identically shaped errors is a trap
rather than dead weight.
