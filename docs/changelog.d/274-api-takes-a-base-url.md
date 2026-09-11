---
type: Changed
pr: 274
---
**An API client no longer resolves endpoints; `remote` does, per request.** The client
underneath takes the base URL its request goes to, and `remote.APIClient` resolves `id`
through `remote.URL(id)` immediately before each call. That removed the import cycle that
kept `remote` from fronting its own API client, and makes `SetOffline`, `Disable` and
`SetURL` apply to a client that already exists rather than to the next one built.
