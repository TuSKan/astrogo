---
type: Added
pr: 1016
---
**`remote.WithRead` runs a provider's parser on the copy `GetFile` returns, and heals the cache with it.** A cached copy it refuses is removed and fetched again in the same call. A fresh download it refuses, or `WithValidate` refuses, is not downloaded again while the source serves that version: `GetFile` returns the new `remote.ErrRejectedSource` instead. The record is the client's and is kept in memory, never in the cache.
