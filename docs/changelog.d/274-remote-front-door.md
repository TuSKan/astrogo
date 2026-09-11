---
type: Changed — BREAKING
pr: 274
---
**`remote/file` and `remote/api` are internal to `remote`.** Everything they do is
reachable from `remote` itself — `Bucket` (an alias for `blob.Bucket`), `OpenBucket`,
`Save`, `ReaderAt`/`NewReaderAt`/`WithChunkSize`/`WithCachedChunks`, `APIClient`/
`NewAPIClient` with the `With*` options, `HTTPError`, `RetryPolicy`/`Attempt`/
`DefaultRetryPolicy`, `DefaultAPITimeout` — and importing either package from outside
`remote/` now fails `TestSubpackagesAreNotImportedDirectly`. Going around the front door
skipped the endpoint registry, offline mode and the consent gate for that one call site,
silently.
